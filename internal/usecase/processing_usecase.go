package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/IndraSty/smart-doc-intelligence/internal/domain"
	"github.com/IndraSty/smart-doc-intelligence/pkg/logger"
	"github.com/IndraSty/smart-doc-intelligence/pkg/queue"
)

type processingUsecase struct {
	jobRepo domain.JobRepository
	docRepo domain.DocumentRepository
	queue   *queue.Client
	log     *logger.Logger
}

// NewProcessingUsecase creates a new ProcessingUsecase implementation.
func NewProcessingUsecase(
	jobRepo domain.JobRepository,
	docRepo domain.DocumentRepository,
	queueClient *queue.Client,
	log *logger.Logger,
) domain.ProcessingUsecase {
	return &processingUsecase{
		jobRepo: jobRepo,
		docRepo: docRepo,
		queue:   queueClient,
		log:     log,
	}
}

// EnqueueJob creates a processing job and publishes it to RabbitMQ.
// Updates the document status to "queued".
func (u *processingUsecase) EnqueueJob(ctx context.Context, documentID, userID string) (*domain.ProcessingJob, error) {
	jobID := uuid.New().String()
	now := time.Now()

	job := &domain.ProcessingJob{
		ID:         jobID,
		DocumentID: documentID,
		Status:     domain.JobStatusQueued,
		Attempts:   0,
		QueuedAt:   now,
	}

	// Persist job status to Redis immediately
	if err := u.jobRepo.SetStatus(ctx, job); err != nil {
		return nil, fmt.Errorf("processingUsecase.EnqueueJob set status: %w", err)
	}

	// Update document status to "queued" in PostgreSQL
	if err := u.docRepo.UpdateStatus(ctx, documentID, domain.StatusQueued); err != nil {
		u.log.Error().Err(err).
			Str("document_id", documentID).
			Msg("Failed to update document status to queued")
		// Non-fatal — Redis has the correct status
	}

	// Build queue message — intentionally minimal
	msg := &domain.QueueMessage{
		JobID:      jobID,
		DocumentID: documentID,
		UserID:     userID,
		Attempt:    1,
	}

	// Publish to RabbitMQ
	if err := u.queue.Publish(ctx, msg); err != nil {
		// Roll back job status if queue publish fails
		job.Status = domain.JobStatusFailed
		errMsg := err.Error()
		job.LastError = &errMsg
		_ = u.jobRepo.SetStatus(ctx, job)
		return nil, fmt.Errorf("processingUsecase.EnqueueJob publish: %w", domain.ErrQueueFailed)
	}

	u.log.Info().
		Str("job_id", jobID).
		Str("document_id", documentID).
		Msg("Job enqueued successfully")

	return job, nil
}

// GetJobStatus returns the current status of a processing job.
// Checks Redis first, falls back to reconstructing from document status.
func (u *processingUsecase) GetJobStatus(ctx context.Context, jobID, userID string) (*domain.ProcessingJob, error) {
	job, err := u.jobRepo.GetStatus(ctx, jobID)
	if err != nil {
		return nil, err
	}

	return job, nil
}

// MarkProcessing transitions a job to the "processing" state.
// Called by the worker when it picks up a job from the queue.
func (u *processingUsecase) MarkProcessing(ctx context.Context, jobID string) error {
	job, err := u.jobRepo.GetStatus(ctx, jobID)
	if err != nil {
		return fmt.Errorf("processingUsecase.MarkProcessing get job: %w", err)
	}

	now := time.Now()
	job.Status = domain.JobStatusProcessing
	job.StartedAt = &now
	job.Attempts++

	if err := u.jobRepo.SetStatus(ctx, job); err != nil {
		return fmt.Errorf("processingUsecase.MarkProcessing set status: %w", err)
	}

	// Sync to PostgreSQL
	if err := u.docRepo.UpdateStatus(ctx, job.DocumentID, domain.StatusProcessing); err != nil {
		u.log.Error().Err(err).
			Str("job_id", jobID).
			Msg("Failed to sync processing status to PostgreSQL")
	}

	return nil
}

// MarkCompleted transitions a job to the "completed" state.
// Called by the worker after successful AI processing.
func (u *processingUsecase) MarkCompleted(ctx context.Context, jobID string) error {
	job, err := u.jobRepo.GetStatus(ctx, jobID)
	if err != nil {
		return fmt.Errorf("processingUsecase.MarkCompleted get job: %w", err)
	}

	now := time.Now()
	job.Status = domain.JobStatusCompleted
	job.CompletedAt = &now

	if err := u.jobRepo.SetStatus(ctx, job); err != nil {
		return fmt.Errorf("processingUsecase.MarkCompleted set status: %w", err)
	}

	u.log.Info().
		Str("job_id", jobID).
		Str("document_id", job.DocumentID).
		Msg("Job marked as completed")

	return nil
}

// MarkFailed transitions a job to the "failed" state with an error message.
// Called by the worker after all retry attempts are exhausted.
func (u *processingUsecase) MarkFailed(ctx context.Context, jobID, errMsg string) error {
	job, err := u.jobRepo.GetStatus(ctx, jobID)
	if err != nil {
		return fmt.Errorf("processingUsecase.MarkFailed get job: %w", err)
	}

	now := time.Now()
	job.Status = domain.JobStatusFailed
	job.CompletedAt = &now
	job.LastError = &errMsg

	if err := u.jobRepo.SetStatus(ctx, job); err != nil {
		return fmt.Errorf("processingUsecase.MarkFailed set status: %w", err)
	}

	// Sync failure to PostgreSQL
	if err := u.docRepo.UpdateError(ctx, job.DocumentID, errMsg); err != nil {
		u.log.Error().Err(err).
			Str("job_id", jobID).
			Msg("Failed to sync failure status to PostgreSQL")
	}

	u.log.Error().
		Str("job_id", jobID).
		Str("document_id", job.DocumentID).
		Str("error", errMsg).
		Msg("Job marked as failed")

	return nil
}
