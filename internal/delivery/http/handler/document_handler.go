package handler

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/IndraSty/smart-doc-intelligence/internal/delivery/http/middleware"
	"github.com/IndraSty/smart-doc-intelligence/internal/domain"
	"github.com/IndraSty/smart-doc-intelligence/pkg/logger"
)

// DocumentHandler handles all document-related HTTP endpoints.
type DocumentHandler struct {
	docUsecase domain.DocumentUsecase
	log        *logger.Logger
}

// NewDocumentHandler creates a new DocumentHandler.
func NewDocumentHandler(docUsecase domain.DocumentUsecase, log *logger.Logger) *DocumentHandler {
	return &DocumentHandler{
		docUsecase: docUsecase,
		log:        log,
	}
}

// uploadResponse is returned immediately after a document is uploaded.
type uploadResponse struct {
	DocumentID string `json:"document_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	JobID      string `json:"job_id"      example:"7f3a1b2c-4d5e-6f7a-8b9c-0d1e2f3a4b5c"`
	Status     string `json:"status"      example:"queued"`
	Message    string `json:"message"     example:"Document uploaded and queued for processing"`
}

// listResponse wraps a paginated list of documents.
type listResponse struct {
	Documents  []*domain.Document `json:"documents"`
	TotalCount int                `json:"total_count" example:"42"`
	Limit      int                `json:"limit"       example:"20"`
	Offset     int                `json:"offset"      example:"0"`
}

// downloadURLResponse holds the presigned download URL.
type downloadURLResponse struct {
	URL       string `json:"url"        example:"https://xyz.supabase.co/storage/v1/object/sign/..."`
	ExpiresIn string `json:"expires_in" example:"15 minutes"`
}

// statusResponse wraps the processing job status.
type statusResponse struct {
	Job *domain.ProcessingJob `json:"job"`
}

// Upload godoc
// @Summary      Upload a document
// @Description  Uploads a PDF, PNG, JPG, or TXT file (max 10MB) for async AI processing.
// @Description  Returns a job_id immediately — use GET /documents/{id}/status to poll processing state.
// @Description  Processing pipeline: classify → extract fields → summarize → embed → store.
// @Tags         documents
// @Accept       multipart/form-data
// @Produce      json
// @Param        file        formData file   true  "Document file — PDF, PNG, JPG, JPEG, or TXT. Max 10MB."
// @Param        webhook_url formData string false "HTTPS URL to call when processing completes"
// @Success      202  {object} uploadResponse    "Accepted — processing started asynchronously"
// @Failure      400  {object} errorResponse     "Missing file or invalid webhook URL"
// @Failure      413  {object} errorResponse     "File exceeds 10MB limit"
// @Failure      415  {object} errorResponse     "Unsupported file type"
// @Failure      503  {object} errorResponse     "Storage service unavailable"
// @Security     BearerAuth
// @Security     ApiKeyAuth
// @Router       /api/v1/documents [post]
func (h *DocumentHandler) Upload(c echo.Context) error {
	userID := middleware.GetUserID(c)

	fileHeader, err := c.FormFile("file")
	if err != nil {
		if err == http.ErrMissingFile {
			return echo.NewHTTPError(http.StatusBadRequest, "file field is required")
		}
		return echo.NewHTTPError(http.StatusBadRequest, "failed to read file from request")
	}

	file, err := fileHeader.Open()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to open uploaded file")
	}
	defer func() { _ = file.Close() }()

	fileData, err := io.ReadAll(file)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to read file data")
	}

	var webhookURL *string
	if wh := c.FormValue("webhook_url"); wh != "" {
		webhookURL = &wh
	}

	input := domain.UploadInput{
		UserID:     userID,
		Filename:   fileHeader.Filename,
		FileSize:   fileHeader.Size,
		FileData:   fileData,
		WebhookURL: webhookURL,
	}

	doc, jobID, err := h.docUsecase.Upload(c.Request().Context(), input)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrFileTooLarge):
			return echo.NewHTTPError(http.StatusRequestEntityTooLarge, "file exceeds 10MB limit")
		case errors.Is(err, domain.ErrInvalidFileType):
			return echo.NewHTTPError(http.StatusUnsupportedMediaType, "file type not supported")
		case errors.Is(err, domain.ErrWebhookInvalid):
			return echo.NewHTTPError(http.StatusBadRequest, "webhook_url must be a valid HTTPS URL")
		case errors.Is(err, domain.ErrStorageFailed):
			return echo.NewHTTPError(http.StatusServiceUnavailable, "storage service unavailable")
		default:
			h.log.Error().Err(err).
				Str("user_id", userID).
				Str("filename", fileHeader.Filename).
				Msg("Upload failed")
			return echo.NewHTTPError(http.StatusInternalServerError, "upload failed")
		}
	}

	// Record upload metrics
	middleware.RecordUpload(doc.FileType, doc.FileSize)

	h.log.Info().
		Str("user_id", userID).
		Str("document_id", doc.ID).
		Str("job_id", jobID).
		Msg("Document uploaded successfully")

	return c.JSON(http.StatusAccepted, uploadResponse{
		DocumentID: doc.ID,
		JobID:      jobID,
		Status:     string(doc.Status),
		Message:    "Document uploaded and queued for processing",
	})
}

// List godoc
// @Summary      List documents
// @Description  Returns a paginated list of documents belonging to the authenticated user.
// @Description  Use status and type filters to narrow results.
// @Tags         documents
// @Produce      json
// @Param        status  query string false "Filter by status"  Enums(uploaded,queued,processing,completed,failed)
// @Param        type    query string false "Filter by type"    Enums(invoice,contract,identity,financial,receipt,other)
// @Param        limit   query int    false "Page size"         default(20) minimum(1) maximum(100)
// @Param        offset  query int    false "Page offset"       default(0)  minimum(0)
// @Success      200  {object} listResponse
// @Failure      500  {object} errorResponse
// @Security     BearerAuth
// @Security     ApiKeyAuth
// @Router       /api/v1/documents [get]
func (h *DocumentHandler) List(c echo.Context) error {
	userID := middleware.GetUserID(c)

	filter := domain.ListDocumentsFilter{
		Limit:  parseIntQuery(c, "limit", 20),
		Offset: parseIntQuery(c, "offset", 0),
	}

	if s := c.QueryParam("status"); s != "" {
		status := domain.DocumentStatus(s)
		filter.Status = &status
	}

	if t := c.QueryParam("type"); t != "" {
		docType := domain.DocumentType(t)
		filter.DocumentType = &docType
	}

	docs, total, err := h.docUsecase.List(c.Request().Context(), userID, filter)
	if err != nil {
		h.log.Error().Err(err).Str("user_id", userID).Msg("List documents failed")
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to list documents")
	}

	if docs == nil {
		docs = []*domain.Document{}
	}

	return c.JSON(http.StatusOK, listResponse{
		Documents:  docs,
		TotalCount: total,
		Limit:      filter.Limit,
		Offset:     filter.Offset,
	})
}

// GetByID godoc
// @Summary      Get document details
// @Description  Returns a document with its AI classification, extracted fields, and summary.
// @Description  Extraction results are only available when status is 'completed'.
// @Tags         documents
// @Produce      json
// @Param        id   path string true "Document UUID" example("550e8400-e29b-41d4-a716-446655440000")
// @Success      200  {object} domain.Document
// @Failure      403  {object} errorResponse "Document belongs to another user"
// @Failure      404  {object} errorResponse "Document not found"
// @Security     BearerAuth
// @Security     ApiKeyAuth
// @Router       /api/v1/documents/{id} [get]
func (h *DocumentHandler) GetByID(c echo.Context) error {
	userID := middleware.GetUserID(c)
	docID := c.Param("id")

	doc, err := h.docUsecase.GetByID(c.Request().Context(), docID, userID)
	if err != nil {
		return handleDomainError(err)
	}

	return c.JSON(http.StatusOK, doc)
}

// GetDownloadURL godoc
// @Summary      Get presigned download URL
// @Description  Generates a time-limited presigned URL (15 minutes) for downloading the original file.
// @Description  Files are served directly from Supabase Storage — not proxied through this server.
// @Tags         documents
// @Produce      json
// @Param        id   path string true "Document UUID" example("550e8400-e29b-41d4-a716-446655440000")
// @Success      200  {object} downloadURLResponse
// @Failure      403  {object} errorResponse "Access denied"
// @Failure      404  {object} errorResponse "Document not found"
// @Security     BearerAuth
// @Security     ApiKeyAuth
// @Router       /api/v1/documents/{id}/download [get]
func (h *DocumentHandler) GetDownloadURL(c echo.Context) error {
	userID := middleware.GetUserID(c)
	docID := c.Param("id")

	signedURL, err := h.docUsecase.GetDownloadURL(c.Request().Context(), docID, userID)
	if err != nil {
		return handleDomainError(err)
	}

	return c.JSON(http.StatusOK, downloadURLResponse{
		URL:       signedURL,
		ExpiresIn: "15 minutes",
	})
}

// Delete godoc
// @Summary      Delete a document
// @Description  Permanently deletes the document record and its file from Supabase Storage.
// @Description  This action is irreversible.
// @Tags         documents
// @Produce      json
// @Param        id   path string true "Document UUID" example("550e8400-e29b-41d4-a716-446655440000")
// @Success      204  "No Content — deleted successfully"
// @Failure      403  {object} errorResponse "Access denied"
// @Failure      404  {object} errorResponse "Document not found"
// @Security     BearerAuth
// @Security     ApiKeyAuth
// @Router       /api/v1/documents/{id} [delete]
func (h *DocumentHandler) Delete(c echo.Context) error {
	userID := middleware.GetUserID(c)
	docID := c.Param("id")

	if err := h.docUsecase.Delete(c.Request().Context(), docID, userID); err != nil {
		return handleDomainError(err)
	}

	return c.NoContent(http.StatusNoContent)
}

// GetStatus godoc
// @Summary      Get processing status
// @Description  Returns the current async processing status of a document.
// @Description  Poll this endpoint after upload until status is 'completed' or 'failed'.
// @Description  Status flow: uploaded → queued → processing → completed | failed
// @Tags         documents
// @Produce      json
// @Param        id   path string true "Document UUID" example("550e8400-e29b-41d4-a716-446655440000")
// @Success      200  {object} statusResponse
// @Failure      403  {object} errorResponse "Access denied"
// @Failure      404  {object} errorResponse "Document not found"
// @Security     BearerAuth
// @Security     ApiKeyAuth
// @Router       /api/v1/documents/{id}/status [get]
func (h *DocumentHandler) GetStatus(c echo.Context) error {
	userID := middleware.GetUserID(c)
	docID := c.Param("id")

	job, err := h.docUsecase.GetStatus(c.Request().Context(), docID, userID)
	if err != nil {
		return handleDomainError(err)
	}

	return c.JSON(http.StatusOK, statusResponse{Job: job})
}

// parseIntQuery reads an integer query param with a default fallback.
func parseIntQuery(c echo.Context, key string, defaultVal int) int {
	val := c.QueryParam(key)
	if val == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(val)
	if err != nil || n < 0 {
		return defaultVal
	}
	return n
}

// handleDomainError maps domain errors to HTTP error responses.
func handleDomainError(err error) error {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return echo.NewHTTPError(http.StatusNotFound, "resource not found")
	case errors.Is(err, domain.ErrForbidden):
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	case errors.Is(err, domain.ErrUnauthorized):
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	case errors.Is(err, domain.ErrConflict):
		return echo.NewHTTPError(http.StatusConflict, "resource already exists")
	case errors.Is(err, domain.ErrInvalidInput):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	case errors.Is(err, domain.ErrStorageFailed):
		return echo.NewHTTPError(http.StatusServiceUnavailable, "storage service unavailable")
	default:
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
}
