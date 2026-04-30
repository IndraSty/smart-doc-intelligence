package domain

import (
	"context"
	"time"
)

// Field represents a single extracted key-value pair from a document.
// The Value is interface{} to support strings, numbers, arrays, and nested objects.
type Field struct {
	Key        string      `json:"key"`
	Value      interface{} `json:"value"`
	Confidence float64     `json:"confidence"` // 0.0 to 1.0
}

// ExtractionResult holds all AI-generated output for a document.
type ExtractionResult struct {
	ID            string    `json:"id"`
	DocumentID    string    `json:"document_id"`
	Fields        []Field   `json:"fields"`
	RawAIResponse string    `json:"-"` // stored for debugging, not exposed in API
	Embedding     []float32 `json:"-"` // vector(768), not exposed in API
	ProcessedAt   time.Time `json:"processed_at"`
}

// AIResult holds everything returned by the AI layer after processing.
// This is the internal transfer object between the AI layer and usecase layer.
type AIResult struct {
	DocumentType DocumentType
	Confidence   float64
	Summary      string
	Fields       []Field
	RawResponse  string
	Embedding    []float32
}

// ExtractionRepository defines all database operations for the extractions table.
// Implemented in internal/repository/postgres/extraction_repo.go
type ExtractionRepository interface {
	// Create inserts a new extraction result including the embedding vector.
	Create(ctx context.Context, result *ExtractionResult) (*ExtractionResult, error)

	// FindByDocumentID returns the extraction result for a given document.
	// Returns ErrNotFound if processing is not yet complete.
	FindByDocumentID(ctx context.Context, documentID string) (*ExtractionResult, error)

	// SearchSemantic performs cosine similarity search using pgvector.
	// Returns document IDs ranked by similarity score.
	SearchSemantic(ctx context.Context, userID string, embedding []float32, limit int) ([]SearchResult, error)

	// SearchFullText performs PostgreSQL tsvector full-text search.
	// Returns document IDs ranked by text relevance.
	SearchFullText(ctx context.Context, userID string, query string, limit int) ([]SearchResult, error)
}

// SearchResult holds a document ID and its relevance score from a search query.
type SearchResult struct {
	DocumentID string  `json:"document_id"`
	Score      float64 `json:"score"`
	Rank       int     `json:"rank"`
}

// SearchInput holds parameters for a search request.
type SearchInput struct {
	UserID     string
	Query      string
	SearchType string // "semantic", "fulltext", "hybrid"
	Limit      int
}

// SearchOutput holds the final ranked search results with document details.
type SearchOutput struct {
	Results    []SearchResultDetail `json:"results"`
	TotalCount int                  `json:"total_count"`
	SearchType string               `json:"search_type"`
}

// SearchResultDetail combines the search score with document metadata
// so the API can return enriched results in a single response.
type SearchResultDetail struct {
	Document *Document `json:"document"`
	Score    float64   `json:"score"`
	Rank     int       `json:"rank"`
}

// SearchUsecase defines all search business logic.
// Implemented in internal/usecase/search_usecase.go
type SearchUsecase interface {
	// Search routes to the correct search strategy based on SearchInput.SearchType.
	Search(ctx context.Context, input SearchInput) (*SearchOutput, error)
}
