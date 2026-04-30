package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/IndraSty/smart-doc-intelligence/pkg/metrics"
)

// PrometheusMetrics returns a middleware that records HTTP request metrics
// for every route. Attach this after RequestID middleware so the request
// ID is available in the context.
func PrometheusMetrics() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()

			// Track in-flight requests
			metrics.HTTPRequestsInFlight.Inc()
			defer metrics.HTTPRequestsInFlight.Dec()

			// Call the actual handler
			err := next(c)

			// Collect metrics after handler returns
			duration := time.Since(start).Seconds()
			status := c.Response().Status
			method := c.Request().Method

			// Use route path pattern instead of full URL to avoid
			// high cardinality from path params like /documents/uuid-here
			path := c.Path()
			if path == "" {
				path = c.Request().URL.Path
			}

			statusStr := strconv.Itoa(status)

			// Record request count
			metrics.HTTPRequestsTotal.WithLabelValues(method, path, statusStr).Inc()

			// Record request duration
			metrics.HTTPRequestDuration.WithLabelValues(method, path).Observe(duration)

			// Record response size
			responseSize := float64(c.Response().Size)
			if responseSize > 0 {
				metrics.HTTPResponseSize.WithLabelValues(method, path).Observe(responseSize)
			}

			return err
		}
	}
}

// RecordUpload records document upload metrics.
// Call this from the document handler after a successful upload.
func RecordUpload(fileType string, fileSizeBytes int64) {
	metrics.DocumentsUploaded.WithLabelValues(fileType).Inc()
	metrics.DocumentFileSizeBytes.WithLabelValues(fileType).Observe(float64(fileSizeBytes))
	metrics.DocumentsInQueue.Inc()
}

// RecordSearchRequest records search query metrics.
// Call this from the search handler after a completed search.
func RecordSearchRequest(searchType string, resultCount int, duration time.Duration, success bool) {
	status := "success"
	if !success {
		status = "failed"
	}

	metrics.SearchRequestsTotal.WithLabelValues(searchType, status).Inc()
	metrics.SearchDuration.WithLabelValues(searchType).Observe(duration.Seconds())

	if success {
		metrics.SearchResultCount.WithLabelValues(searchType).Observe(float64(resultCount))
	}
}

// NormalizeHTTPStatus buckets status codes into 2xx, 4xx, 5xx strings
// for lower-cardinality metrics when needed.
func NormalizeHTTPStatus(status int) string {
	switch {
	case status >= http.StatusOK && status < http.StatusMultipleChoices:
		return "2xx"
	case status >= http.StatusBadRequest && status < http.StatusInternalServerError:
		return "4xx"
	case status >= http.StatusInternalServerError:
		return "5xx"
	default:
		return "other"
	}
}
