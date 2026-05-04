package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	internali "github.com/IndraSty/smart-doc-intelligence/internal/ai"
	"github.com/IndraSty/smart-doc-intelligence/internal/domain"
	"github.com/IndraSty/smart-doc-intelligence/pkg/embedding"
	"github.com/IndraSty/smart-doc-intelligence/pkg/logger"
	"github.com/IndraSty/smart-doc-intelligence/pkg/metrics"
	"github.com/IndraSty/smart-doc-intelligence/pkg/queue"
	"github.com/IndraSty/smart-doc-intelligence/pkg/storage"
)

const (
	// maxRetryAttempts is the maximum number of times a job is retried
	// before being marked as permanently failed.
	maxRetryAttempts = 3

	// webhookTimeout is the maximum time to wait for a webhook response.
	webhookTimeout = 10 * time.Second

	// workerConsumerPrefix is prepended to the worker ID for RabbitMQ consumer tags.
	workerConsumerPrefix = "worker-"
)

// Worker holds all dependencies needed to process a document job.
type Worker struct {
	aiProvider     internali.AIProvider
	docRepo        domain.DocumentRepository
	extractionRepo domain.ExtractionRepository
	jobRepo        domain.JobRepository
	processingUC   domain.ProcessingUsecase
	storageClient  *storage.Client
	embedder       *embedding.Generator
	queueClient    *queue.Client
	httpClient     *http.Client
	log            *logger.Logger
	poolSize       int
}

// Config holds worker pool configuration.
type Config struct {
	PoolSize int
	RetryMax int
}

// NewWorker creates a new Worker with all required dependencies.
func NewWorker(
	aiProvider internali.AIProvider,
	docRepo domain.DocumentRepository,
	extractionRepo domain.ExtractionRepository,
	jobRepo domain.JobRepository,
	processingUC domain.ProcessingUsecase,
	storageClient *storage.Client,
	embedder *embedding.Generator,
	queueClient *queue.Client,
	cfg Config,
	log *logger.Logger,
) *Worker {
	return &Worker{
		aiProvider:     aiProvider,
		docRepo:        docRepo,
		extractionRepo: extractionRepo,
		jobRepo:        jobRepo,
		processingUC:   processingUC,
		storageClient:  storageClient,
		embedder:       embedder,
		queueClient:    queueClient,
		httpClient: &http.Client{
			Timeout: webhookTimeout,
		},
		log:      log,
		poolSize: cfg.PoolSize,
	}
}

// Start launches the worker pool and begins consuming jobs from RabbitMQ.
// It blocks until the context is cancelled (graceful shutdown).
// Each goroutine in the pool processes one message at a time.
func (w *Worker) Start(ctx context.Context) error {
	w.log.Info().
		Int("pool_size", w.poolSize).
		Msg("Starting worker pool")

	// Launch one consumer per worker goroutine
	// Each consumer has its own channel registered with RabbitMQ
	errCh := make(chan error, w.poolSize)

	for i := 0; i < w.poolSize; i++ {
		workerID := fmt.Sprintf("%s%d", workerConsumerPrefix, i)

		deliveries, err := w.queueClient.Consume(workerID)
		if err != nil {
			return fmt.Errorf("worker.Start consume worker %s: %w", workerID, err)
		}

		go w.runWorkerLoop(ctx, workerID, deliveries, errCh)
	}

	w.log.Info().Msg("All workers started — waiting for jobs")

	// Block until context is cancelled
	<-ctx.Done()

	w.log.Info().Msg("Worker pool shutting down — draining in-flight jobs")

	// Give in-flight jobs up to 30 seconds to complete
	time.Sleep(30 * time.Second)

	w.log.Info().Msg("Worker pool stopped")
	return nil
}

// runWorkerLoop is the main loop for a single worker goroutine.
// It processes deliveries one at a time and acks or nacks each message.
func (w *Worker) runWorkerLoop(
	ctx context.Context,
	workerID string,
	deliveries <-chan amqp.Delivery,
	errCh chan<- error,
) {
	log := w.log.With().Str("worker_id", workerID).Logger()

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Worker shutting down")
			return

		case delivery, ok := <-deliveries:
			if !ok {
				log.Warn().Msg("Delivery channel closed")
				return
			}

			msg, err := queue.ParseMessage(delivery)
			if err != nil {
				log.Error().Err(err).
					Str("body", string(delivery.Body)).
					Msg("Failed to parse message — nacking without requeue")
				_ = delivery.Nack(false, false)
				continue
			}

			// Track active jobs
			metrics.WorkerActiveJobs.Inc()
			jobStart := time.Now()

			log.Info().
				Str("job_id", msg.JobID).
				Str("document_id", msg.DocumentID).
				Int("attempt", msg.Attempt).
				Msg("Processing job")

			err = w.processJob(ctx, msg)

			// Record job duration regardless of outcome
			metrics.WorkerJobDuration.Observe(time.Since(jobStart).Seconds())
			metrics.WorkerActiveJobs.Dec()

			if err != nil {
				log.Error().Err(err).
					Str("job_id", msg.JobID).
					Msg("Job processing failed")

				if msg.Attempt < maxRetryAttempts {
					metrics.WorkerJobsProcessed.WithLabelValues(workerID, "retried").Inc()
					w.requeueWithBackoff(ctx, msg, err)
					_ = delivery.Ack(false)
				} else {
					metrics.WorkerJobsProcessed.WithLabelValues(workerID, "failed").Inc()
					w.markJobFailed(ctx, msg, err)
					_ = delivery.Ack(false)
				}
				continue
			}

			metrics.WorkerJobsProcessed.WithLabelValues(workerID, "success").Inc()
			_ = delivery.Ack(false)

			log.Info().
				Str("job_id", msg.JobID).
				Str("document_id", msg.DocumentID).
				Msg("Job completed successfully")
		}
	}
}

// processJob runs the full AI processing pipeline for a single document:
// 1. Fetch document metadata from PostgreSQL
// 2. Download the file from Supabase Storage
// 3. Send to Gemini for classification, extraction, and summarization
// 4. Generate embedding vector
// 5. Save extraction result to PostgreSQL
// 6. Update document status to completed
// 7. Fire webhook callback if configured
func (w *Worker) processJob(ctx context.Context, msg *domain.QueueMessage) error {
	log := w.log.WithDocumentID(msg.DocumentID)
	processingStart := time.Now()

	if err := w.processingUC.MarkProcessing(ctx, msg.JobID); err != nil {
		log.Warn().Err(err).Msg("Failed to mark job as processing — continuing anyway")
	}

	doc, err := w.docRepo.FindByID(ctx, msg.DocumentID)
	if err != nil {
		return fmt.Errorf("processJob fetch document: %w", err)
	}

	fileData, err := w.downloadFile(ctx, doc.StoragePath)
	if err != nil {
		return fmt.Errorf("processJob download file: %w", err)
	}

	log.Info().
		Int("file_size", len(fileData)).
		Str("file_type", doc.FileType).
		Msg("File downloaded from storage")

	mimeType := mimeTypeFromExt(doc.FileType)
	aiInput := internali.ProcessInput{
		DocumentID: doc.ID,
		FileData:   fileData,
		FileType:   doc.FileType,
		MIMEType:   mimeType,
		Filename:   doc.Filename,
	}

	aiResult, err := w.aiProvider.Process(ctx, aiInput)
	if err != nil {
		// Record failed AI processing
		metrics.DocumentsProcessed.WithLabelValues("unknown", "failed").Inc()
		return fmt.Errorf("processJob ai processing: %w", err)
	}

	log.Info().
		Str("document_type", string(aiResult.DocumentType)).
		Float64("confidence", aiResult.Confidence).
		Int("field_count", len(aiResult.Fields)).
		Msg("AI processing completed")

	// Record processing duration per document type
	metrics.DocumentProcessingDuration.
		WithLabelValues(string(aiResult.DocumentType)).
		Observe(time.Since(processingStart).Seconds())

	fieldsMap := fieldsToMap(aiResult.Fields)
	embeddingText := embedding.BuildDocumentText(aiResult.Summary, fieldsMap)

	vector, err := w.embedder.GenerateForDocument(ctx, embeddingText)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to generate embedding — document will not be searchable semantically")
		vector = nil
	}

	extraction := &domain.ExtractionResult{
		DocumentID:    doc.ID,
		Fields:        aiResult.Fields,
		RawAIResponse: aiResult.RawResponse,
		Embedding:     vector,
	}

	if _, err := w.extractionRepo.Create(ctx, extraction); err != nil {
		return fmt.Errorf("processJob save extraction: %w", err)
	}

	if err := w.docRepo.UpdateAfterProcessing(
		ctx,
		doc.ID,
		aiResult.DocumentType,
		aiResult.Confidence,
		aiResult.Summary,
	); err != nil {
		return fmt.Errorf("processJob update document: %w", err)
	}

	if err := w.processingUC.MarkCompleted(ctx, msg.JobID); err != nil {
		log.Warn().Err(err).Msg("Failed to mark job as completed in Redis")
	}

	// Record successful processing
	metrics.DocumentsProcessed.
		WithLabelValues(string(aiResult.DocumentType), "completed").Inc()

	// Decrement queue counter now that the job is done
	metrics.DocumentsInQueue.Dec()

	if doc.WebhookURL != nil && *doc.WebhookURL != "" {
		go w.fireWebhook(doc, aiResult)
	}

	return nil
}

// downloadFile fetches the raw bytes of a file from Supabase Storage.
// We generate a short-lived presigned URL and download via HTTP.
func (w *Worker) downloadFile(ctx context.Context, storagePath string) ([]byte, error) {
	signedURL, err := w.storageClient.GeneratePresignedURL(ctx, storagePath, 5*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("downloadFile generate URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, signedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("downloadFile create request: %w", err)
	}

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloadFile http do: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloadFile: unexpected status %d", resp.StatusCode)
	}

	// Cap read at 10MB to prevent memory exhaustion
	limited := io.LimitReader(resp.Body, 10*1024*1024)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("downloadFile read body: %w", err)
	}

	return data, nil
}

// requeueWithBackoff publishes the job back to the queue with an incremented
// attempt count. The AI layer already handles per-call retries with backoff —
// this handles full job-level retries for infrastructure failures.
func (w *Worker) requeueWithBackoff(ctx context.Context, msg *domain.QueueMessage, jobErr error) {
	log := w.log.WithDocumentID(msg.DocumentID)

	// Calculate backoff delay: attempt 1→2s, 2→4s, 3→8s
	delay := time.Duration(1<<uint(msg.Attempt)) * time.Second

	log.Warn().
		Err(jobErr).
		Int("attempt", msg.Attempt).
		Dur("backoff", delay).
		Msg("Requeuing job with backoff")

	// Apply the backoff delay before requeuing
	time.Sleep(delay)

	retryMsg := &domain.QueueMessage{
		JobID:      msg.JobID,
		DocumentID: msg.DocumentID,
		UserID:     msg.UserID,
		Attempt:    msg.Attempt + 1,
	}

	if err := w.queueClient.Publish(ctx, retryMsg); err != nil {
		log.Error().Err(err).Msg("Failed to requeue job — marking as failed")
		w.markJobFailed(ctx, msg, jobErr)
	}
}

// markJobFailed permanently fails a job after all retries are exhausted.
// Updates both Redis and PostgreSQL with the error message.
func (w *Worker) markJobFailed(ctx context.Context, msg *domain.QueueMessage, jobErr error) {
	errMsg := jobErr.Error()

	if err := w.processingUC.MarkFailed(ctx, msg.JobID, errMsg); err != nil {
		w.log.Error().Err(err).
			Str("job_id", msg.JobID).
			Msg("Failed to mark job as failed in Redis")
	}

	// Also update document error message in PostgreSQL
	if err := w.docRepo.UpdateError(ctx, msg.DocumentID, errMsg); err != nil {
		w.log.Error().Err(err).
			Str("document_id", msg.DocumentID).
			Msg("Failed to update document error in PostgreSQL")
	}

	w.log.Error().
		Str("job_id", msg.JobID).
		Str("document_id", msg.DocumentID).
		Int("attempts", msg.Attempt).
		Str("error", errMsg).
		Msg("Job permanently failed after max retries")
}

// webhookPayload is the JSON body sent to the user's webhook URL.
type webhookPayload struct {
	Event        string  `json:"event"`
	DocumentID   string  `json:"document_id"`
	DocumentType string  `json:"document_type"`
	Confidence   float64 `json:"confidence"`
	Summary      string  `json:"summary"`
	FieldCount   int     `json:"field_count"`
	ProcessedAt  string  `json:"processed_at"`
}

// fireWebhook sends a POST request to the user's registered webhook URL.
// Runs in a separate goroutine — failures are logged but do not affect
// the job completion status.
func (w *Worker) fireWebhook(doc *domain.Document, result *domain.AIResult) {
	ctx, cancel := context.WithTimeout(context.Background(), webhookTimeout)
	defer cancel()

	start := time.Now()

	payload := webhookPayload{
		Event:        "document.processed",
		DocumentID:   doc.ID,
		DocumentType: string(result.DocumentType),
		Confidence:   result.Confidence,
		Summary:      result.Summary,
		FieldCount:   len(result.Fields),
		ProcessedAt:  time.Now().UTC().Format(time.RFC3339),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		w.log.Error().Err(err).
			Str("document_id", doc.ID).
			Msg("Failed to marshal webhook payload")
		metrics.WebhookDeliveriesTotal.WithLabelValues("failed").Inc()
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, *doc.WebhookURL, bytes.NewReader(body))
	if err != nil {
		w.log.Error().Err(err).
			Str("document_id", doc.ID).
			Msg("Failed to create webhook request")
		metrics.WebhookDeliveriesTotal.WithLabelValues("failed").Inc()
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "SmartDocIntelligence/1.0")
	req.Header.Set("X-Webhook-Source", "smart-doc-intelligence")

	resp, err := w.httpClient.Do(req)

	// Record webhook delivery duration
	metrics.WebhookDuration.Observe(time.Since(start).Seconds())

	if err != nil {
		w.log.Warn().Err(err).
			Str("document_id", doc.ID).
			Str("webhook_url", *doc.WebhookURL).
			Msg("Webhook delivery failed")
		metrics.WebhookDeliveriesTotal.WithLabelValues("failed").Inc()
		return
	}
	defer func() { _ = resp.Body.Close() }()

	metrics.WebhookDeliveriesTotal.WithLabelValues("success").Inc()

	w.log.Info().
		Str("document_id", doc.ID).
		Str("webhook_url", *doc.WebhookURL).
		Int("status_code", resp.StatusCode).
		Msg("Webhook delivered")
}

// fieldsToMap converts []domain.Field to map[string]interface{}
// for use in embedding text construction.
func fieldsToMap(fields []domain.Field) map[string]interface{} {
	m := make(map[string]interface{}, len(fields))
	for _, f := range fields {
		m[f.Key] = f.Value
	}
	return m
}

// mimeTypeFromExt maps a file extension to its MIME type.
func mimeTypeFromExt(ext string) string {
	switch ext {
	case "pdf":
		return "application/pdf"
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "txt":
		return "text/plain"
	default:
		return "application/octet-stream"
	}
}
