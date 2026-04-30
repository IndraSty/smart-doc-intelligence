package handler

import (
	"context"
	"net/http"
	"runtime"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"

	"github.com/IndraSty/smart-doc-intelligence/pkg/logger"
	"github.com/IndraSty/smart-doc-intelligence/pkg/queue"
	"github.com/IndraSty/smart-doc-intelligence/pkg/storage"
)

// HealthHandler checks the status of all system dependencies.
type HealthHandler struct {
	db        *pgxpool.Pool
	redis     *redis.Client
	queue     *queue.Client
	storage   *storage.Client
	log       *logger.Logger
	startTime time.Time
}

// NewHealthHandler creates a new HealthHandler.
func NewHealthHandler(
	db *pgxpool.Pool,
	redisClient *redis.Client,
	queueClient *queue.Client,
	storageClient *storage.Client,
	log *logger.Logger,
) *HealthHandler {
	return &HealthHandler{
		db:        db,
		redis:     redisClient,
		queue:     queueClient,
		storage:   storageClient,
		log:       log,
		startTime: time.Now(),
	}
}

// dependencyStatus holds the health status of a single dependency.
type dependencyStatus struct {
	Status  string `json:"status"`
	Latency string `json:"latency,omitempty"`
	Error   string `json:"error,omitempty"`
}

// dbPoolStats holds PostgreSQL connection pool statistics.
type dbPoolStats struct {
	TotalConns    int32 `json:"total_conns"`
	IdleConns     int32 `json:"idle_conns"`
	AcquiredConns int32 `json:"acquired_conns"`
}

// systemInfo holds basic runtime information.
type systemInfo struct {
	GoVersion     string  `json:"go_version"`
	NumGoroutine  int     `json:"goroutines"`
	NumCPU        int     `json:"cpus"`
	UptimeSeconds float64 `json:"uptime_seconds"`
}

// healthResponse is the full health check response body.
type healthResponse struct {
	Status       string                      `json:"status"`
	Version      string                      `json:"version"`
	Timestamp    string                      `json:"timestamp"`
	System       systemInfo                  `json:"system"`
	Database     dbPoolStats                 `json:"db_pool"`
	Dependencies map[string]dependencyStatus `json:"dependencies"`
}

// Health godoc
// @Summary      Health check
// @Description  Returns health status of the API and all its dependencies
// @Tags         system
// @Produce      json
// @Success      200 {object} healthResponse
// @Failure      503 {object} healthResponse
// @Router       /health [get]
func (h *HealthHandler) Health(c echo.Context) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), 5*time.Second)
	defer cancel()

	deps := make(map[string]dependencyStatus)

	// Check PostgreSQL
	deps["postgres"] = checkDependency(func() error {
		return h.db.Ping(ctx)
	})

	// Check Redis
	deps["redis"] = checkDependency(func() error {
		return h.redis.Ping(ctx).Err()
	})

	// Check RabbitMQ
	deps["rabbitmq"] = checkDependency(func() error {
		return h.queue.HealthCheck()
	})

	// Check Supabase Storage
	deps["storage"] = checkDependency(func() error {
		return h.storage.HealthCheck(ctx)
	})

	// Determine overall status
	allHealthy := true
	for _, dep := range deps {
		if dep.Status != "ok" {
			allHealthy = false
			break
		}
	}

	status := "ok"
	httpStatus := http.StatusOK
	if !allHealthy {
		status = "degraded"
		httpStatus = http.StatusServiceUnavailable
	}

	// Collect DB pool stats
	poolStats := h.db.Stat()

	return c.JSON(httpStatus, healthResponse{
		Status:    status,
		Version:   "1.0.0",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		System: systemInfo{
			GoVersion:     runtime.Version(),
			NumGoroutine:  runtime.NumGoroutine(),
			NumCPU:        runtime.NumCPU(),
			UptimeSeconds: time.Since(h.startTime).Seconds(),
		},
		Database: dbPoolStats{
			TotalConns:    poolStats.TotalConns(),
			IdleConns:     poolStats.IdleConns(),
			AcquiredConns: poolStats.AcquiredConns(),
		},
		Dependencies: deps,
	})
}

// checkDependency runs a health check function and measures its latency.
func checkDependency(fn func() error) dependencyStatus {
	start := time.Now()
	err := fn()
	latency := time.Since(start)

	if err != nil {
		return dependencyStatus{
			Status: "error",
			Error:  err.Error(),
		}
	}

	return dependencyStatus{
		Status:  "ok",
		Latency: latency.String(),
	}
}
