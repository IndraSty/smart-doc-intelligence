package domain

import (
	"context"
	"time"
)

// JobStatus mirrors the processing pipeline states stored in Redis.
type JobStatus string

const (
	JobStatusQueued     JobStatus = "queued"
	JobStatusProcessing JobStatus = "processing"
	JobStatusCompleted  JobStatus = "completed"
	JobStatusFailed     JobStatus = "failed"
)

// ProcessingJob tracks the async processing state of a document.
// The primary store is Redis for speed; PostgreSQL is the source of truth.
type ProcessingJob struct {
	ID          string     `json:"id"`
	DocumentID  string     `json:"document_id"`
	Status      JobStatus  `json:"status"`
	Attempts    int        `json:"attempts"`
	LastError   *string    `json:"last_error,omitempty"`
	QueuedAt    time.Time  `json:"queued_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// QueueMessage is the payload published to RabbitMQ.
// Kept intentionally small — the worker fetches full document
// details from the database using DocumentID.
type QueueMessage struct {
	JobID      string `json:"job_id"`
	DocumentID string `json:"document_id"`
	UserID     string `json:"user_id"`
	Attempt    int    `json:"attempt"`
}

// JobRepository defines all operations for job status tracking.
// Redis is the primary store; PostgreSQL is the fallback.
// Implemented in internal/repository/redis/job_repo.go
type JobRepository interface {
	// SetStatus writes the job status to Redis with a 24-hour TTL.
	SetStatus(ctx context.Context, job *ProcessingJob) error

	// GetStatus reads job status from Redis.
	// Returns ErrNotFound if the key has expired or never existed.
	GetStatus(ctx context.Context, jobID string) (*ProcessingJob, error)

	// GetStatusByDocumentID reads job status keyed by document ID.
	GetStatusByDocumentID(ctx context.Context, documentID string) (*ProcessingJob, error)

	// Delete removes a job status entry from Redis.
	Delete(ctx context.Context, jobID string) error
}

// ProcessingUsecase defines the business logic for triggering
// and tracking document processing jobs.
// Implemented in internal/usecase/processing_usecase.go
type ProcessingUsecase interface {
	// EnqueueJob creates a job record and publishes it to RabbitMQ.
	EnqueueJob(ctx context.Context, documentID, userID string) (*ProcessingJob, error)

	// GetJobStatus returns the current job status, checking Redis first.
	GetJobStatus(ctx context.Context, jobID, userID string) (*ProcessingJob, error)

	// MarkProcessing updates job status to processing and records start time.
	MarkProcessing(ctx context.Context, jobID string) error

	// MarkCompleted updates job status to completed with completion time.
	MarkCompleted(ctx context.Context, jobID string) error

	// MarkFailed updates job status to failed and records the error message.
	MarkFailed(ctx context.Context, jobID, errMsg string) error
}
