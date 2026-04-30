package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/IndraSty/smart-doc-intelligence/internal/domain"
	"github.com/IndraSty/smart-doc-intelligence/pkg/logger"
)

const (
	// jobKeyPrefix is used for job-id-based lookups
	jobKeyPrefix = "job:"

	// docJobKeyPrefix is used for document-id-based lookups
	docJobKeyPrefix = "docjob:"

	// jobTTL is how long job status is retained in Redis after completion
	jobTTL = 24 * time.Hour
)

type jobRepository struct {
	client *redis.Client
	log    *logger.Logger
}

// NewJobRepository creates a new Redis-backed JobRepository.
func NewJobRepository(client *redis.Client, log *logger.Logger) domain.JobRepository {
	return &jobRepository{client: client, log: log}
}

// NewRedisClient creates and validates a Redis client connection.
func NewRedisClient(redisURL string, log *logger.Logger) (*redis.Client, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Redis URL: %w", err)
	}

	client := redis.NewClient(opts)

	// Validate connection immediately
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	log.Info().Msg("Redis connection established")
	return client, nil
}

func (r *jobRepository) SetStatus(ctx context.Context, job *domain.ProcessingJob) error {
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("jobRepository.SetStatus marshal: %w", err)
	}

	// Store by job ID for direct lookups
	jobKey := jobKeyPrefix + job.ID
	if err := r.client.Set(ctx, jobKey, data, jobTTL).Err(); err != nil {
		return fmt.Errorf("jobRepository.SetStatus set job key: %w", err)
	}

	// Store by document ID for lookups from the document perspective
	docKey := docJobKeyPrefix + job.DocumentID
	if err := r.client.Set(ctx, docKey, data, jobTTL).Err(); err != nil {
		return fmt.Errorf("jobRepository.SetStatus set doc key: %w", err)
	}

	r.log.Debug().
		Str("job_id", job.ID).
		Str("document_id", job.DocumentID).
		Str("status", string(job.Status)).
		Msg("Job status updated in Redis")

	return nil
}

func (r *jobRepository) GetStatus(ctx context.Context, jobID string) (*domain.ProcessingJob, error) {
	key := jobKeyPrefix + jobID

	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, &domain.NotFoundError{Resource: "job", ID: jobID}
		}
		return nil, fmt.Errorf("jobRepository.GetStatus: %w", err)
	}

	return unmarshalJob(data)
}

func (r *jobRepository) GetStatusByDocumentID(ctx context.Context, documentID string) (*domain.ProcessingJob, error) {
	key := docJobKeyPrefix + documentID

	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, &domain.NotFoundError{Resource: "job", ID: documentID}
		}
		return nil, fmt.Errorf("jobRepository.GetStatusByDocumentID: %w", err)
	}

	return unmarshalJob(data)
}

func (r *jobRepository) Delete(ctx context.Context, jobID string) error {
	// We need the job data to also delete the doc key
	job, err := r.GetStatus(ctx, jobID)
	if err != nil {
		// If already gone, that's fine
		return nil
	}

	pipe := r.client.Pipeline()
	pipe.Del(ctx, jobKeyPrefix+jobID)
	pipe.Del(ctx, docJobKeyPrefix+job.DocumentID)

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("jobRepository.Delete: %w", err)
	}

	return nil
}

// HealthCheck pings Redis to verify the connection is alive.
// Used by the /health endpoint.
func HealthCheck(ctx context.Context, client *redis.Client) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis ping failed: %w", err)
	}

	return nil
}

func unmarshalJob(data []byte) (*domain.ProcessingJob, error) {
	job := &domain.ProcessingJob{}
	if err := json.Unmarshal(data, job); err != nil {
		return nil, fmt.Errorf("unmarshalJob: %w", err)
	}
	return job, nil
}
