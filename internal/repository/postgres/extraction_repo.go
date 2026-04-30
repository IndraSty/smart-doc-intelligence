package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/IndraSty/smart-doc-intelligence/internal/domain"
	"github.com/IndraSty/smart-doc-intelligence/pkg/logger"
)

type extractionRepository struct {
	pool *pgxpool.Pool
	log  *logger.Logger
}

// NewExtractionRepository creates a new PostgreSQL-backed ExtractionRepository.
func NewExtractionRepository(pool *pgxpool.Pool, log *logger.Logger) domain.ExtractionRepository {
	return &extractionRepository{pool: pool, log: log}
}

func (r *extractionRepository) Create(ctx context.Context, result *domain.ExtractionResult) (*domain.ExtractionResult, error) {
	// Serialize fields to JSON for the JSONB column
	fieldsJSON, err := json.Marshal(result.Fields)
	if err != nil {
		return nil, fmt.Errorf("extractionRepository.Create marshal fields: %w", err)
	}

	// Convert []float32 embedding to pgvector string format: '[0.1,0.2,...]'
	embeddingStr := float32SliceToVector(result.Embedding)

	query := `
		INSERT INTO extractions (document_id, fields, raw_ai_response, embedding)
		VALUES ($1, $2, $3, $4)
		RETURNING id, document_id, fields, raw_ai_response, processed_at
	`

	created := &domain.ExtractionResult{}
	var fieldsRaw []byte

	err = r.pool.QueryRow(ctx, query,
		result.DocumentID,
		fieldsJSON,
		result.RawAIResponse,
		embeddingStr,
	).Scan(
		&created.ID,
		&created.DocumentID,
		&fieldsRaw,
		&created.RawAIResponse,
		&created.ProcessedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("extractionRepository.Create: %w", err)
	}

	// Deserialize fields from JSONB
	if err := json.Unmarshal(fieldsRaw, &created.Fields); err != nil {
		return nil, fmt.Errorf("extractionRepository.Create unmarshal fields: %w", err)
	}

	return created, nil
}

func (r *extractionRepository) FindByDocumentID(ctx context.Context, documentID string) (*domain.ExtractionResult, error) {
	query := `
		SELECT id, document_id, fields, raw_ai_response, processed_at
		FROM extractions
		WHERE document_id = $1
		LIMIT 1
	`

	result := &domain.ExtractionResult{}
	var fieldsRaw []byte

	err := r.pool.QueryRow(ctx, query, documentID).Scan(
		&result.ID,
		&result.DocumentID,
		&fieldsRaw,
		&result.RawAIResponse,
		&result.ProcessedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &domain.NotFoundError{Resource: "extraction", ID: documentID}
		}
		return nil, fmt.Errorf("extractionRepository.FindByDocumentID: %w", err)
	}

	if err := json.Unmarshal(fieldsRaw, &result.Fields); err != nil {
		return nil, fmt.Errorf("extractionRepository.FindByDocumentID unmarshal: %w", err)
	}

	return result, nil
}

func (r *extractionRepository) SearchSemantic(
	ctx context.Context,
	userID string,
	embedding []float32,
	limit int,
) ([]domain.SearchResult, error) {
	// Cosine similarity search using pgvector <=> operator
	// Join with documents to enforce multi-tenant isolation
	query := `
		SELECT
			e.document_id,
			1 - (e.embedding <=> $1::vector) AS score
		FROM extractions e
		INNER JOIN documents d ON d.id = e.document_id
		WHERE d.user_id = $2
			AND d.status = 'completed'
			AND e.embedding IS NOT NULL
		ORDER BY e.embedding <=> $1::vector
		LIMIT $3
	`

	embeddingStr := float32SliceToVector(embedding)

	rows, err := r.pool.Query(ctx, query, embeddingStr, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("extractionRepository.SearchSemantic: %w", err)
	}
	defer rows.Close()

	return scanSearchResults(rows)
}

func (r *extractionRepository) SearchFullText(
	ctx context.Context,
	userID string,
	query string,
	limit int,
) ([]domain.SearchResult, error) {
	// Full-text search using PostgreSQL tsvector and plainto_tsquery
	// plainto_tsquery handles natural language queries without special syntax
	sqlQuery := `
		SELECT
			e.document_id,
			ts_rank(e.search_vector, plainto_tsquery('english', $1)) AS score
		FROM extractions e
		INNER JOIN documents d ON d.id = e.document_id
		WHERE d.user_id = $2
			AND d.status = 'completed'
			AND e.search_vector @@ plainto_tsquery('english', $1)
		ORDER BY score DESC
		LIMIT $3
	`

	rows, err := r.pool.Query(ctx, sqlQuery, query, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("extractionRepository.SearchFullText: %w", err)
	}
	defer rows.Close()

	return scanSearchResults(rows)
}

// scanSearchResults reads rows into []SearchResult and assigns rank by position.
func scanSearchResults(rows pgx.Rows) ([]domain.SearchResult, error) {
	var results []domain.SearchResult
	rank := 1

	for rows.Next() {
		var r domain.SearchResult
		if err := rows.Scan(&r.DocumentID, &r.Score); err != nil {
			return nil, fmt.Errorf("scanSearchResults scan: %w", err)
		}
		r.Rank = rank
		rank++
		results = append(results, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scanSearchResults rows: %w", err)
	}

	return results, nil
}

// float32SliceToVector converts a []float32 to the pgvector string format.
// Example: [0.1, 0.2, 0.3] → "[0.1,0.2,0.3]"
func float32SliceToVector(v []float32) string {
	if len(v) == 0 {
		return "[]"
	}

	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = fmt.Sprintf("%f", f)
	}

	return "[" + strings.Join(parts, ",") + "]"
}
