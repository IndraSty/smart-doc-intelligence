package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// HTTP metrics
var (
	// HTTPRequestsTotal counts every HTTP request by method, path, and status code.
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "smartdoc",
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "Total number of HTTP requests by method, path, and status code.",
		},
		[]string{"method", "path", "status"},
	)

	// HTTPRequestDuration measures HTTP request latency by method and path.
	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "smartdoc",
			Subsystem: "http",
			Name:      "request_duration_seconds",
			Help:      "HTTP request duration in seconds.",
			// Buckets tuned for an API: 10ms to 10s
			Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"method", "path"},
	)

	// HTTPRequestsInFlight tracks currently active HTTP requests.
	HTTPRequestsInFlight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "smartdoc",
			Subsystem: "http",
			Name:      "requests_in_flight",
			Help:      "Number of HTTP requests currently being processed.",
		},
	)

	// HTTPResponseSize measures the size of HTTP responses in bytes.
	HTTPResponseSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "smartdoc",
			Subsystem: "http",
			Name:      "response_size_bytes",
			Help:      "HTTP response size in bytes.",
			Buckets:   prometheus.ExponentialBuckets(100, 10, 7), // 100B to 100MB
		},
		[]string{"method", "path"},
	)
)

// Document processing metrics
var (
	// DocumentsUploaded counts total document uploads by file type.
	DocumentsUploaded = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "smartdoc",
			Subsystem: "documents",
			Name:      "uploaded_total",
			Help:      "Total number of documents uploaded by file type.",
		},
		[]string{"file_type"},
	)

	// DocumentsProcessed counts completed AI processing jobs by document type and status.
	DocumentsProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "smartdoc",
			Subsystem: "documents",
			Name:      "processed_total",
			Help:      "Total number of documents processed by document type and status.",
		},
		[]string{"document_type", "status"}, // status: completed | failed
	)

	// DocumentProcessingDuration measures how long AI processing takes per document type.
	DocumentProcessingDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "smartdoc",
			Subsystem: "documents",
			Name:      "processing_duration_seconds",
			Help:      "Time taken to process a document including AI calls.",
			// AI processing can take anywhere from 2s to 60s
			Buckets: []float64{1, 2, 5, 10, 20, 30, 45, 60, 90, 120},
		},
		[]string{"document_type"},
	)

	// DocumentsInQueue tracks the current number of documents waiting to be processed.
	DocumentsInQueue = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "smartdoc",
			Subsystem: "documents",
			Name:      "in_queue",
			Help:      "Number of documents currently waiting in the processing queue.",
		},
	)

	// DocumentFileSizeBytes measures the size of uploaded documents.
	DocumentFileSizeBytes = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "smartdoc",
			Subsystem: "documents",
			Name:      "file_size_bytes",
			Help:      "Size of uploaded document files in bytes.",
			Buckets:   prometheus.ExponentialBuckets(1024, 4, 8), // 1KB to ~16MB
		},
		[]string{"file_type"},
	)
)

// AI provider metrics
var (
	// AIRequestsTotal counts Gemini API calls by operation and status.
	AIRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "smartdoc",
			Subsystem: "ai",
			Name:      "requests_total",
			Help:      "Total number of AI provider API calls by operation and status.",
		},
		[]string{"operation", "status"}, // operation: classify|extract|summarize|embed
	)

	// AIRequestDuration measures Gemini API call latency by operation.
	AIRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "smartdoc",
			Subsystem: "ai",
			Name:      "request_duration_seconds",
			Help:      "Gemini API call duration in seconds.",
			Buckets:   []float64{0.5, 1, 2, 5, 10, 20, 30, 60},
		},
		[]string{"operation"},
	)

	// AIRetryTotal counts retry attempts by operation.
	AIRetryTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "smartdoc",
			Subsystem: "ai",
			Name:      "retries_total",
			Help:      "Total number of AI API retry attempts.",
		},
		[]string{"operation"},
	)
)

// Worker metrics
var (
	// WorkerJobsProcessed counts jobs processed per worker by status.
	WorkerJobsProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "smartdoc",
			Subsystem: "worker",
			Name:      "jobs_processed_total",
			Help:      "Total number of jobs processed by worker ID and status.",
		},
		[]string{"worker_id", "status"}, // status: success | failed | retried
	)

	// WorkerActiveJobs tracks how many jobs are currently being processed.
	WorkerActiveJobs = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "smartdoc",
			Subsystem: "worker",
			Name:      "active_jobs",
			Help:      "Number of jobs currently being processed by the worker pool.",
		},
	)

	// WorkerJobDuration measures end-to-end job processing time.
	WorkerJobDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "smartdoc",
			Subsystem: "worker",
			Name:      "job_duration_seconds",
			Help:      "End-to-end duration of a job from queue pickup to completion.",
			Buckets:   []float64{1, 2, 5, 10, 20, 30, 60, 90, 120},
		},
	)
)

// Search metrics
var (
	// SearchRequestsTotal counts search queries by type and status.
	SearchRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "smartdoc",
			Subsystem: "search",
			Name:      "requests_total",
			Help:      "Total number of search requests by type and status.",
		},
		[]string{"type", "status"}, // type: semantic|fulltext|hybrid
	)

	// SearchDuration measures search query latency by type.
	SearchDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "smartdoc",
			Subsystem: "search",
			Name:      "duration_seconds",
			Help:      "Search query duration in seconds.",
			Buckets:   []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5},
		},
		[]string{"type"},
	)

	// SearchResultCount measures how many results are returned per search.
	SearchResultCount = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "smartdoc",
			Subsystem: "search",
			Name:      "result_count",
			Help:      "Number of results returned per search query.",
			Buckets:   []float64{0, 1, 2, 5, 10, 15, 20},
		},
		[]string{"type"},
	)
)

// Webhook metrics
var (
	// WebhookDeliveriesTotal counts webhook delivery attempts by status.
	WebhookDeliveriesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "smartdoc",
			Subsystem: "webhook",
			Name:      "deliveries_total",
			Help:      "Total number of webhook delivery attempts.",
		},
		[]string{"status"}, // status: success | failed
	)

	// WebhookDuration measures webhook HTTP call latency.
	WebhookDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "smartdoc",
			Subsystem: "webhook",
			Name:      "duration_seconds",
			Help:      "Webhook HTTP call duration in seconds.",
			Buckets:   []float64{0.1, 0.25, 0.5, 1, 2, 5, 10},
		},
	)
)
