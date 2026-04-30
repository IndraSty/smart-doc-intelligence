package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/IndraSty/smart-doc-intelligence/internal/domain"
	"github.com/IndraSty/smart-doc-intelligence/pkg/logger"
)

type documentRepository struct {
	pool *pgxpool.Pool
	log  *logger.Logger
}

// NewDocumentRepository creates a new PostgreSQL-backed DocumentRepository.
func NewDocumentRepository(pool *pgxpool.Pool, log *logger.Logger) domain.DocumentRepository {
	return &documentRepository{pool: pool, log: log}
}

func (r *documentRepository) Create(ctx context.Context, doc *domain.Document) (*domain.Document, error) {
	query := `
		INSERT INTO documents (
			user_id, filename, storage_path, file_type,
			file_size, status, webhook_url
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING
			id, user_id, filename, storage_path, file_type,
			file_size, status, document_type, classification_confidence,
			summary, error_message, webhook_url, created_at, updated_at
	`

	created := &domain.Document{}
	err := r.pool.QueryRow(ctx, query,
		doc.UserID,
		doc.Filename,
		doc.StoragePath,
		doc.FileType,
		doc.FileSize,
		doc.Status,
		doc.WebhookURL,
	).Scan(
		&created.ID,
		&created.UserID,
		&created.Filename,
		&created.StoragePath,
		&created.FileType,
		&created.FileSize,
		&created.Status,
		&created.DocumentType,
		&created.ClassificationConfidence,
		&created.Summary,
		&created.ErrorMessage,
		&created.WebhookURL,
		&created.CreatedAt,
		&created.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("documentRepository.Create: %w", err)
	}

	return created, nil
}

func (r *documentRepository) FindByID(ctx context.Context, id string) (*domain.Document, error) {
	query := `
		SELECT
			id, user_id, filename, storage_path, file_type,
			file_size, status, document_type, classification_confidence,
			summary, error_message, webhook_url, created_at, updated_at
		FROM documents
		WHERE id = $1
		LIMIT 1
	`

	doc := &domain.Document{}
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&doc.ID,
		&doc.UserID,
		&doc.Filename,
		&doc.StoragePath,
		&doc.FileType,
		&doc.FileSize,
		&doc.Status,
		&doc.DocumentType,
		&doc.ClassificationConfidence,
		&doc.Summary,
		&doc.ErrorMessage,
		&doc.WebhookURL,
		&doc.CreatedAt,
		&doc.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &domain.NotFoundError{Resource: "document", ID: id}
		}
		return nil, fmt.Errorf("documentRepository.FindByID: %w", err)
	}

	return doc, nil
}

func (r *documentRepository) FindByIDAndUserID(ctx context.Context, id, userID string) (*domain.Document, error) {
	// First check if the document exists at all
	doc, err := r.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Then enforce multi-tenant isolation
	if doc.UserID != userID {
		return nil, &domain.ForbiddenError{
			UserID:     userID,
			ResourceID: id,
		}
	}

	return doc, nil
}

func (r *documentRepository) ListByUserID(
	ctx context.Context,
	userID string,
	filter domain.ListDocumentsFilter,
) ([]*domain.Document, int, error) {
	// Build dynamic WHERE clause based on optional filters
	conditions := []string{"user_id = $1"}
	args := []interface{}{userID}
	argIdx := 2

	if filter.Status != nil {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, string(*filter.Status))
		argIdx++
	}

	if filter.DocumentType != nil {
		conditions = append(conditions, fmt.Sprintf("document_type = $%d", argIdx))
		args = append(args, string(*filter.DocumentType))
		argIdx++
	}

	where := strings.Join(conditions, " AND ")

	// Get total count for pagination
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM documents WHERE %s", where)
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("documentRepository.ListByUserID count: %w", err)
	}

	// Apply pagination
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20 // default page size
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	args = append(args, limit, offset)
	listQuery := fmt.Sprintf(`
		SELECT
			id, user_id, filename, storage_path, file_type,
			file_size, status, document_type, classification_confidence,
			summary, error_message, webhook_url, created_at, updated_at
		FROM documents
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, where, argIdx, argIdx+1)

	rows, err := r.pool.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("documentRepository.ListByUserID query: %w", err)
	}
	defer rows.Close()

	var docs []*domain.Document
	for rows.Next() {
		doc := &domain.Document{}
		if err := rows.Scan(
			&doc.ID,
			&doc.UserID,
			&doc.Filename,
			&doc.StoragePath,
			&doc.FileType,
			&doc.FileSize,
			&doc.Status,
			&doc.DocumentType,
			&doc.ClassificationConfidence,
			&doc.Summary,
			&doc.ErrorMessage,
			&doc.WebhookURL,
			&doc.CreatedAt,
			&doc.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("documentRepository.ListByUserID scan: %w", err)
		}
		docs = append(docs, doc)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("documentRepository.ListByUserID rows: %w", err)
	}

	return docs, total, nil
}

func (r *documentRepository) UpdateStatus(ctx context.Context, id string, status domain.DocumentStatus) error {
	query := `
		UPDATE documents
		SET status = $1, updated_at = NOW()
		WHERE id = $2
	`

	result, err := r.pool.Exec(ctx, query, string(status), id)
	if err != nil {
		return fmt.Errorf("documentRepository.UpdateStatus: %w", err)
	}

	if result.RowsAffected() == 0 {
		return &domain.NotFoundError{Resource: "document", ID: id}
	}

	return nil
}

func (r *documentRepository) UpdateAfterProcessing(
	ctx context.Context,
	id string,
	docType domain.DocumentType,
	confidence float64,
	summary string,
) error {
	query := `
		UPDATE documents
		SET
			status = $1,
			document_type = $2,
			classification_confidence = $3,
			summary = $4,
			updated_at = NOW()
		WHERE id = $5
	`

	result, err := r.pool.Exec(ctx, query,
		string(domain.StatusCompleted),
		string(docType),
		confidence,
		summary,
		id,
	)
	if err != nil {
		return fmt.Errorf("documentRepository.UpdateAfterProcessing: %w", err)
	}

	if result.RowsAffected() == 0 {
		return &domain.NotFoundError{Resource: "document", ID: id}
	}

	return nil
}

func (r *documentRepository) UpdateError(ctx context.Context, id string, errMsg string) error {
	query := `
		UPDATE documents
		SET
			status = $1,
			error_message = $2,
			updated_at = NOW()
		WHERE id = $3
	`

	result, err := r.pool.Exec(ctx, query,
		string(domain.StatusFailed),
		errMsg,
		id,
	)
	if err != nil {
		return fmt.Errorf("documentRepository.UpdateError: %w", err)
	}

	if result.RowsAffected() == 0 {
		return &domain.NotFoundError{Resource: "document", ID: id}
	}

	return nil
}

func (r *documentRepository) Delete(ctx context.Context, id, userID string) error {
	// Enforce multi-tenant isolation — only delete if the document belongs to the user
	query := `
		DELETE FROM documents
		WHERE id = $1 AND user_id = $2
	`

	result, err := r.pool.Exec(ctx, query, id, userID)
	if err != nil {
		return fmt.Errorf("documentRepository.Delete: %w", err)
	}

	if result.RowsAffected() == 0 {
		return &domain.NotFoundError{Resource: "document", ID: id}
	}

	return nil
}
