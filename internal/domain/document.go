package domain

import (
	"context"
	"time"
)

// DocumentStatus represents the lifecycle state of a document
// as it moves through the processing pipeline.
type DocumentStatus string

const (
	StatusUploaded   DocumentStatus = "uploaded"
	StatusQueued     DocumentStatus = "queued"
	StatusProcessing DocumentStatus = "processing"
	StatusCompleted  DocumentStatus = "completed"
	StatusFailed     DocumentStatus = "failed"
)

// DocumentType is the AI-classified category of a document.
type DocumentType string

const (
	TypeInvoice   DocumentType = "invoice"
	TypeContract  DocumentType = "contract"
	TypeIdentity  DocumentType = "identity"
	TypeFinancial DocumentType = "financial"
	TypeReceipt   DocumentType = "receipt"
	TypeOther     DocumentType = "other"
)

// AllowedFileTypes maps MIME-like extension strings to their magic byte signatures.
// File type validation must use magic bytes, never just the extension.
var AllowedMagicBytes = map[string][]byte{
	"pdf":  {0x25, 0x50, 0x44, 0x46}, // %PDF
	"png":  {0x89, 0x50, 0x4E, 0x47}, // .PNG
	"jpg":  {0xFF, 0xD8, 0xFF},       // JFIF/EXIF
	"jpeg": {0xFF, 0xD8, 0xFF},       // JFIF/EXIF
}

// Document is the central entity representing an uploaded file
// and its AI processing results.
type Document struct {
	ID                       string         `json:"id"`
	UserID                   string         `json:"user_id"`
	Filename                 string         `json:"filename"` // original name for display
	StoragePath              string         `json:"-"`        // UUID path, never exposed
	FileType                 string         `json:"file_type"`
	FileSize                 int64          `json:"file_size"`
	Status                   DocumentStatus `json:"status"`
	DocumentType             *DocumentType  `json:"document_type,omitempty"`
	ClassificationConfidence *float64       `json:"classification_confidence,omitempty"`
	Summary                  *string        `json:"summary,omitempty"`
	ErrorMessage             *string        `json:"error_message,omitempty"`
	WebhookURL               *string        `json:"-"` // never expose webhook URL in responses
	CreatedAt                time.Time      `json:"created_at"`
	UpdatedAt                time.Time      `json:"updated_at"`
}

// UploadInput holds the data for a new document upload request.
type UploadInput struct {
	UserID     string
	Filename   string
	FileType   string
	FileSize   int64
	FileData   []byte // raw file bytes for magic byte validation and storage
	WebhookURL *string
}

// ListDocumentsFilter defines optional filters for listing a user's documents.
type ListDocumentsFilter struct {
	Status       *DocumentStatus
	DocumentType *DocumentType
	Limit        int
	Offset       int
}

// DocumentRepository defines all database operations for the documents table.
// Implemented in internal/repository/postgres/document_repo.go
type DocumentRepository interface {
	// Create inserts a new document record and returns it.
	Create(ctx context.Context, doc *Document) (*Document, error)

	// FindByID returns a document by UUID.
	// Returns ErrNotFound if it does not exist.
	FindByID(ctx context.Context, id string) (*Document, error)

	// FindByIDAndUserID returns a document only if it belongs to the given user.
	// This enforces multi-tenant isolation at the repository level.
	// Returns ErrNotFound if not found, ErrForbidden if user mismatch.
	FindByIDAndUserID(ctx context.Context, id, userID string) (*Document, error)

	// ListByUserID returns all documents for a user with optional filters.
	ListByUserID(ctx context.Context, userID string, filter ListDocumentsFilter) ([]*Document, int, error)

	// UpdateStatus updates the status field and updated_at timestamp.
	UpdateStatus(ctx context.Context, id string, status DocumentStatus) error

	// UpdateAfterProcessing updates all AI result fields after processing completes.
	UpdateAfterProcessing(ctx context.Context, id string, docType DocumentType, confidence float64, summary string) error

	// UpdateError sets the error message and marks the document as failed.
	UpdateError(ctx context.Context, id string, errMsg string) error

	// Delete removes a document record. The caller is responsible for
	// also deleting the file from storage.
	Delete(ctx context.Context, id, userID string) error
}

// DocumentUsecase defines all business logic for document management.
// Implemented in internal/usecase/document_usecase.go
type DocumentUsecase interface {
	// Upload validates, stores, and enqueues a document for processing.
	// Returns the created document and job ID immediately without waiting for AI.
	Upload(ctx context.Context, input UploadInput) (*Document, string, error)

	// GetByID returns a document with its extraction results if available.
	GetByID(ctx context.Context, id, userID string) (*Document, error)

	// List returns paginated documents for a user with optional filters.
	List(ctx context.Context, userID string, filter ListDocumentsFilter) ([]*Document, int, error)

	// GetDownloadURL generates a presigned URL valid for 15 minutes.
	GetDownloadURL(ctx context.Context, id, userID string) (string, error)

	// Delete removes the document record and its file from storage.
	Delete(ctx context.Context, id, userID string) error

	// GetStatus returns the current processing status from Redis (fast)
	// with fallback to PostgreSQL.
	GetStatus(ctx context.Context, id, userID string) (*ProcessingJob, error)
}
