package handler

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/IndraSty/smart-doc-intelligence/internal/delivery/http/middleware"
	"github.com/IndraSty/smart-doc-intelligence/internal/domain"
	"github.com/IndraSty/smart-doc-intelligence/pkg/logger"
)

// SearchHandler handles all search-related HTTP endpoints.
type SearchHandler struct {
	searchUsecase domain.SearchUsecase
	log           *logger.Logger
}

// NewSearchHandler creates a new SearchHandler.
func NewSearchHandler(searchUsecase domain.SearchUsecase, log *logger.Logger) *SearchHandler {
	return &SearchHandler{
		searchUsecase: searchUsecase,
		log:           log,
	}
}

// Search godoc
// @Summary      Search documents
// @Description  Search your documents using three strategies:
// @Description  - **semantic**: vector similarity search using Gemini embeddings. Best for conceptual queries.
// @Description  - **fulltext**: PostgreSQL tsvector keyword search. Best for exact terms.
// @Description  - **hybrid**: combines both with Reciprocal Rank Fusion (RRF). Best overall results.
// @Tags         search
// @Produce      json
// @Param        q      query string true  "Search query"                          example("total invoice amount")
// @Param        type   query string false "Search strategy"                       Enums(semantic,fulltext,hybrid) default(hybrid)
// @Param        limit  query int    false "Maximum number of results (max 20)"    default(20) minimum(1) maximum(20)
// @Success      200  {object} domain.SearchOutput
// @Failure      400  {object} errorResponse "Missing query parameter"
// @Failure      500  {object} errorResponse "Search failed"
// @Security     BearerAuth
// @Security     ApiKeyAuth
// @Router       /api/v1/search [get]
func (h *SearchHandler) Search(c echo.Context) error {
	userID := middleware.GetUserID(c)
	start := time.Now()

	query := c.QueryParam("q")
	if query == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "query parameter 'q' is required")
	}

	searchType := c.QueryParam("type")
	if searchType == "" {
		searchType = "hybrid"
	}

	validTypes := map[string]bool{
		"semantic": true,
		"fulltext": true,
		"hybrid":   true,
	}
	if !validTypes[searchType] {
		return echo.NewHTTPError(http.StatusBadRequest,
			"type must be one of: semantic, fulltext, hybrid")
	}

	limit := parseIntQuery(c, "limit", 20)
	if limit > 20 {
		limit = 20
	}

	input := domain.SearchInput{
		UserID:     userID,
		Query:      query,
		SearchType: searchType,
		Limit:      limit,
	}

	output, err := h.searchUsecase.Search(c.Request().Context(), input)
	if err != nil {
		middleware.RecordSearchRequest(searchType, 0, time.Since(start), false)
		h.log.Error().Err(err).
			Str("user_id", userID).
			Str("query", query).
			Str("type", searchType).
			Msg("Search failed")
		return echo.NewHTTPError(http.StatusInternalServerError, "search failed")
	}

	// Record search metrics
	middleware.RecordSearchRequest(searchType, output.TotalCount, time.Since(start), true)

	return c.JSON(http.StatusOK, output)
}
