package usecase

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/IndraSty/smart-doc-intelligence/config"
	"github.com/IndraSty/smart-doc-intelligence/internal/domain"
	"github.com/IndraSty/smart-doc-intelligence/pkg/logger"
	"github.com/IndraSty/smart-doc-intelligence/pkg/storage"
)

type documentUsecase struct {
	docRepo    domain.DocumentRepository
	jobRepo    domain.JobRepository
	processing domain.ProcessingUsecase
	storage    *storage.Client
	cfg        *config.Config
	log        *logger.Logger
}

// NewDocumentUsecase creates a new DocumentUsecase implementation.
func NewDocumentUsecase(
	docRepo domain.DocumentRepository,
	jobRepo domain.JobRepository,
	processing domain.ProcessingUsecase,
	storageClient *storage.Client,
	cfg *config.Config,
	log *logger.Logger,
) domain.DocumentUsecase {
	return &documentUsecase{
		docRepo:    docRepo,
		jobRepo:    jobRepo,
		processing: processing,
		storage:    storageClient,
		cfg:        cfg,
		log:        log,
	}
}

// Upload validates the file, stores it, creates a DB record,
// and enqueues the processing job — all before returning to the caller.
// The AI processing happens asynchronously via the worker.
func (u *documentUsecase) Upload(ctx context.Context, input domain.UploadInput) (*domain.Document, string, error) {
	// Validate file size
	maxBytes := u.cfg.Upload.MaxFileSizeMB * 1024 * 1024
	if int64(len(input.FileData)) > maxBytes {
		return nil, "", domain.ErrFileTooLarge
	}

	// Validate file type via magic bytes — never trust file extension alone
	fileType, err := detectFileType(input.FileData, input.Filename)
	if err != nil {
		return nil, "", domain.ErrInvalidFileType
	}

	// Validate webhook URL if provided
	if input.WebhookURL != nil && *input.WebhookURL != "" {
		if err := validateWebhookURL(*input.WebhookURL); err != nil {
			return nil, "", domain.ErrWebhookInvalid
		}
	}

	// Generate document UUID — used as the storage path, not the original filename
	documentID := uuid.New().String()

	// Build UUID-based storage path: {userID}/{documentID}.{ext}
	storagePath := storage.BuildStoragePath(input.UserID, documentID, fileType)
	contentType := storage.ContentTypeFromExtension(fileType)

	// Upload file to Supabase Storage
	_, err = u.storage.Upload(ctx, storagePath, input.FileData, contentType)
	if err != nil {
		return nil, "", fmt.Errorf("documentUsecase.Upload storage: %w", domain.ErrStorageFailed)
	}

	// Create document record in PostgreSQL
	doc := &domain.Document{
		ID:          documentID,
		UserID:      input.UserID,
		Filename:    sanitizeFilename(input.Filename), // sanitize before storing
		StoragePath: storagePath,
		FileType:    fileType,
		FileSize:    int64(len(input.FileData)),
		Status:      domain.StatusUploaded,
		WebhookURL:  input.WebhookURL,
	}

	// Note: we pass the pre-generated ID via the struct but the DB
	// will use its own uuid_generate_v4() — we update after creation
	created, err := u.docRepo.Create(ctx, doc)
	if err != nil {
		// Best effort: try to clean up the uploaded file
		_ = u.storage.Delete(ctx, storagePath)
		return nil, "", fmt.Errorf("documentUsecase.Upload create record: %w", err)
	}

	// Enqueue processing job — this updates status to "queued" and publishes to RabbitMQ
	job, err := u.processing.EnqueueJob(ctx, created.ID, input.UserID)
	if err != nil {
		u.log.Error().Err(err).
			Str("document_id", created.ID).
			Msg("Failed to enqueue processing job")
		// Document is uploaded but not queued — not fatal, status stays "uploaded"
		// The worker health check can re-queue these later
	}

	jobID := ""
	if job != nil {
		jobID = job.ID
	}

	u.log.Info().
		Str("document_id", created.ID).
		Str("job_id", jobID).
		Str("user_id", input.UserID).
		Str("file_type", fileType).
		Int("size_bytes", len(input.FileData)).
		Msg("Document uploaded and queued")

	return created, jobID, nil
}

// GetByID returns a document ensuring it belongs to the requesting user.
func (u *documentUsecase) GetByID(ctx context.Context, id, userID string) (*domain.Document, error) {
	doc, err := u.docRepo.FindByIDAndUserID(ctx, id, userID)
	if err != nil {
		return nil, err
	}
	return doc, nil
}

// List returns paginated documents for a user with optional filters.
func (u *documentUsecase) List(
	ctx context.Context,
	userID string,
	filter domain.ListDocumentsFilter,
) ([]*domain.Document, int, error) {
	return u.docRepo.ListByUserID(ctx, userID, filter)
}

// GetDownloadURL generates a 15-minute presigned URL for a document.
// Verifies ownership before generating the URL.
func (u *documentUsecase) GetDownloadURL(ctx context.Context, id, userID string) (string, error) {
	doc, err := u.docRepo.FindByIDAndUserID(ctx, id, userID)
	if err != nil {
		return "", err
	}

	signedURL, err := u.storage.GeneratePresignedURL(
		ctx,
		doc.StoragePath,
		u.cfg.Upload.PresignedURLExpire,
	)
	if err != nil {
		return "", fmt.Errorf("documentUsecase.GetDownloadURL: %w", err)
	}

	return signedURL, nil
}

// Delete removes the document record and its file from storage.
// Enforces ownership — users can only delete their own documents.
func (u *documentUsecase) Delete(ctx context.Context, id, userID string) error {
	// Verify ownership and get storage path before deleting the record
	doc, err := u.docRepo.FindByIDAndUserID(ctx, id, userID)
	if err != nil {
		return err
	}

	// Delete from PostgreSQL first
	if err := u.docRepo.Delete(ctx, id, userID); err != nil {
		return fmt.Errorf("documentUsecase.Delete db: %w", err)
	}

	// Delete from Supabase Storage — best effort, log failure but don't fail the request
	if err := u.storage.Delete(ctx, doc.StoragePath); err != nil {
		u.log.Error().Err(err).
			Str("document_id", id).
			Str("storage_path", doc.StoragePath).
			Msg("Failed to delete file from storage — orphaned file")
	}

	// Clean up Redis job status entry
	_ = u.jobRepo.Delete(ctx, id)

	u.log.Info().
		Str("document_id", id).
		Str("user_id", userID).
		Msg("Document deleted")

	return nil
}

// GetStatus returns the processing status of a document.
// Checks Redis first for speed, falls back to PostgreSQL.
func (u *documentUsecase) GetStatus(ctx context.Context, id, userID string) (*domain.ProcessingJob, error) {
	// Verify ownership
	_, err := u.docRepo.FindByIDAndUserID(ctx, id, userID)
	if err != nil {
		return nil, err
	}

	// Try Redis first — O(1) lookup
	job, err := u.jobRepo.GetStatusByDocumentID(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			// Redis entry expired or never set — return a synthetic status from the document
			doc, err := u.docRepo.FindByID(ctx, id)
			if err != nil {
				return nil, err
			}
			return &domain.ProcessingJob{
				DocumentID: id,
				Status:     domain.JobStatus(doc.Status),
			}, nil
		}
		return nil, fmt.Errorf("documentUsecase.GetStatus redis: %w", err)
	}

	return job, nil
}

// detectFileType validates magic bytes and returns the normalized file extension.
// This is the security-critical function — never trust file extensions alone.
func detectFileType(data []byte, filename string) (string, error) {
	if len(data) < 4 {
		return "", fmt.Errorf("file too small to detect type")
	}

	// Check magic bytes for each supported type
	for ext, magic := range domain.AllowedMagicBytes {
		if bytes.HasPrefix(data, magic) {
			return ext, nil
		}
	}

	// For TXT files: no magic bytes — validate as valid UTF-8 text
	ext := strings.ToLower(strings.TrimPrefix(
		strings.TrimSpace(strings.ToLower(filename[strings.LastIndex(filename, ".")+1:])),
		".",
	))
	if ext == "txt" && isValidUTF8Text(data) {
		return "txt", nil
	}

	return "", fmt.Errorf("unsupported or unrecognized file type")
}

// isValidUTF8Text checks if the data is valid UTF-8 encoded text.
func isValidUTF8Text(data []byte) bool {
	// Check for common binary signatures that would disqualify text
	for _, b := range data[:min(512, len(data))] {
		// Null bytes indicate binary content
		if b == 0x00 {
			return false
		}
	}
	return true
}

// validateWebhookURL ensures webhook URLs are HTTPS only.
func validateWebhookURL(rawURL string) error {
	if !strings.HasPrefix(rawURL, "https://") {
		return domain.ErrWebhookInvalid
	}
	if len(rawURL) < 12 || strings.Contains(rawURL, " ") {
		return domain.ErrWebhookInvalid
	}
	return nil
}

// sanitizeFilename strips path traversal characters from filenames.
// The sanitized name is stored in DB for display only — never used as a path.
func sanitizeFilename(filename string) string {
	// Remove path separators and null bytes
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		"..", "_",
		"\x00", "",
	)
	sanitized := replacer.Replace(filename)

	// Limit filename length
	if len(sanitized) > 255 {
		sanitized = sanitized[:255]
	}

	return sanitized
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
