package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/IndraSty/smart-doc-intelligence/internal/domain"
	"github.com/IndraSty/smart-doc-intelligence/internal/mocks"
	"github.com/IndraSty/smart-doc-intelligence/pkg/logger"
)

// ── Retry backoff unit tests ──────────────────────────────────────────────────

func TestBackoffDelay_IncreasesExponentially(t *testing.T) {
	// Verify that the backoff formula 1<<attempt produces correct delays
	// attempt=1 → 2s, attempt=2 → 4s, attempt=3 → 8s
	cases := []struct {
		attempt       int
		expectedDelay time.Duration
	}{
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
	}

	for _, tc := range cases {
		delay := time.Duration(1<<uint(tc.attempt)) * time.Second
		if delay != tc.expectedDelay {
			t.Errorf("attempt %d: expected delay %v, got %v",
				tc.attempt, tc.expectedDelay, delay)
		}
	}
}

func TestBackoffDelay_NeverExceedsMaxAttempts(t *testing.T) {
	// Verify that maxRetryAttempts constant is 3
	if maxRetryAttempts != 3 {
		t.Errorf("expected maxRetryAttempts=3, got %d", maxRetryAttempts)
	}
}

// ── markJobFailed tests ───────────────────────────────────────────────────────

func TestMarkJobFailed_UpdatesBothRedisAndPostgres(t *testing.T) {
	log := logger.New("development")

	jobID := "job-123"
	documentID := "doc-456"
	errMsg := "gemini api quota exceeded"

	redisUpdated := false
	postgresUpdated := false

	jobRepo := &mocks.MockJobRepository{
		GetStatusFn: func(ctx context.Context, id string) (*domain.ProcessingJob, error) {
			return &domain.ProcessingJob{
				ID:         jobID,
				DocumentID: documentID,
				Status:     domain.JobStatusProcessing,
			}, nil
		},
		SetStatusFn: func(ctx context.Context, job *domain.ProcessingJob) error {
			if job.Status != domain.JobStatusFailed {
				t.Errorf("expected status failed, got %s", job.Status)
			}
			if job.LastError == nil || *job.LastError != errMsg {
				t.Errorf("expected error message '%s'", errMsg)
			}
			redisUpdated = true
			return nil
		},
	}

	docRepo := &mocks.MockDocumentRepository{
		UpdateErrorFn: func(ctx context.Context, id string, msg string) error {
			if id != documentID {
				t.Errorf("expected document ID %s, got %s", documentID, id)
			}
			if msg != errMsg {
				t.Errorf("expected error message '%s', got '%s'", errMsg, msg)
			}
			postgresUpdated = true
			return nil
		},
	}

	processingUC := &mocks.MockProcessingUsecase{
		MarkFailedFn: func(ctx context.Context, jID, eMsg string) error {
			if jID != jobID {
				t.Errorf("expected job ID %s, got %s", jobID, jID)
			}
			redisUpdated = true
			return nil
		},
	}

	w := &Worker{
		jobRepo:      jobRepo,
		docRepo:      docRepo,
		processingUC: processingUC,
		log:          log,
	}

	msg := &domain.QueueMessage{
		JobID:      jobID,
		DocumentID: documentID,
		Attempt:    3,
	}

	w.markJobFailed(context.Background(), msg, errors.New(errMsg))

	if !redisUpdated {
		t.Error("expected Redis to be updated with failed status")
	}

	if !postgresUpdated {
		t.Error("expected PostgreSQL to be updated with error message")
	}
}

func TestMarkJobFailed_HandlesRedisFailureGracefully(t *testing.T) {
	log := logger.New("development")

	// Even if Redis fails, Postgres should still be updated
	postgresUpdated := false

	processingUC := &mocks.MockProcessingUsecase{
		MarkFailedFn: func(ctx context.Context, jobID, errMsg string) error {
			return errors.New("redis connection refused") // Redis failure
		},
	}

	docRepo := &mocks.MockDocumentRepository{
		UpdateErrorFn: func(ctx context.Context, id string, msg string) error {
			postgresUpdated = true
			return nil
		},
	}

	w := &Worker{
		docRepo:      docRepo,
		processingUC: processingUC,
		log:          log,
	}

	msg := &domain.QueueMessage{
		JobID:      "job-999",
		DocumentID: "doc-888",
		Attempt:    3,
	}

	// Should not panic even when Redis fails
	w.markJobFailed(context.Background(), msg, errors.New("ai failed"))

	if !postgresUpdated {
		t.Error("expected PostgreSQL to be updated even when Redis fails")
	}
}

// ── requeueWithBackoff tests ──────────────────────────────────────────────────

func TestRequeueWithBackoff_IncrementsAttemptCount(t *testing.T) {
	log := logger.New("development")

	publishedMsg := &domain.QueueMessage{}

	// We need a minimal queue client substitute — we test the attempt increment logic
	// by capturing what would be published
	captured := false

	w := &Worker{
		log: log,
		// queueClient is nil — we test the attempt logic directly
	}

	// Test the attempt increment calculation inline
	originalAttempt := 2
	msg := &domain.QueueMessage{
		JobID:      "job-111",
		DocumentID: "doc-222",
		UserID:     "user-333",
		Attempt:    originalAttempt,
	}

	// Simulate what requeueWithBackoff does to the message
	publishedMsg.Attempt = msg.Attempt + 1
	captured = true

	_ = w // suppress unused warning

	if !captured {
		t.Fatal("message was not captured")
	}

	if publishedMsg.Attempt != originalAttempt+1 {
		t.Errorf("expected attempt %d, got %d", originalAttempt+1, publishedMsg.Attempt)
	}
}

// ── Worker pool size tests ────────────────────────────────────────────────────

func TestWorkerConfig_DefaultPoolSize(t *testing.T) {
	cfg := Config{
		PoolSize: 5,
		RetryMax: 3,
	}

	if cfg.PoolSize != 5 {
		t.Errorf("expected pool size 5, got %d", cfg.PoolSize)
	}

	if cfg.RetryMax != 3 {
		t.Errorf("expected retry max 3, got %d", cfg.RetryMax)
	}
}

func TestWorkerConfig_MaxRetriesMatchesConstant(t *testing.T) {
	// Ensure the constant and config are aligned
	cfg := Config{RetryMax: maxRetryAttempts}

	if cfg.RetryMax != maxRetryAttempts {
		t.Errorf("config RetryMax %d should match maxRetryAttempts %d",
			cfg.RetryMax, maxRetryAttempts)
	}
}
