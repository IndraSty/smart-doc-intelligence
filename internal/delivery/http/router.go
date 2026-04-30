package http

import (
	"net/http"

	"github.com/labstack/echo/v4"
	echomiddleware "github.com/labstack/echo/v4/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/IndraSty/smart-doc-intelligence/config"
	"github.com/IndraSty/smart-doc-intelligence/internal/delivery/http/handler"
	"github.com/IndraSty/smart-doc-intelligence/internal/delivery/http/middleware"
	"github.com/IndraSty/smart-doc-intelligence/pkg/logger"
)

// RouterDeps holds all handler and middleware dependencies for the router.
type RouterDeps struct {
	AuthHandler     *handler.AuthHandler
	DocumentHandler *handler.DocumentHandler
	SearchHandler   *handler.SearchHandler
	HealthHandler   *handler.HealthHandler
	AuthMiddleware  *middleware.AuthMiddleware
	RateLimiter     *middleware.RateLimiter
	Config          *config.Config
	Log             *logger.Logger
}

type httpErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// NewRouter creates and configures the Echo router with all routes,
// middleware, and handlers wired up.
func NewRouter(deps RouterDeps) *echo.Echo {
	e := echo.New()

	// Disable Echo's default error handler — we use our own
	e.HideBanner = true
	e.HidePort = true
	e.HTTPErrorHandler = customErrorHandler(deps.Log)

	// ---------------------------------------------------------------
	// Global middleware — applied to every request
	// ---------------------------------------------------------------

	// Recover from panics and return 500 instead of crashing
	e.Use(echomiddleware.Recover())

	// Attach request ID to every request for tracing
	e.Use(middleware.RequestID())

	// Set security headers on every response
	e.Use(middleware.SecurityHeaders())

	// Enforce HTTPS in production
	e.Use(middleware.RequireHTTPS(deps.Config))

	e.Use(middleware.PrometheusMetrics())

	// Structured request logging
	e.Use(echomiddleware.RequestLoggerWithConfig(echomiddleware.RequestLoggerConfig{
		LogURI:       true,
		LogStatus:    true,
		LogMethod:    true,
		LogLatency:   true,
		LogError:     true,
		LogRequestID: true,
		LogValuesFunc: func(c echo.Context, v echomiddleware.RequestLoggerValues) error {
			deps.Log.Info().
				Str("method", v.Method).
				Str("uri", v.URI).
				Int("status", v.Status).
				Dur("latency", v.Latency).
				Str("request_id", v.RequestID).
				Err(v.Error).
				Msg("request")
			return nil
		},
	}))

	// CORS — allow all origins in development, restrict in production
	e.Use(echomiddleware.CORSWithConfig(echomiddleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodDelete,
			http.MethodOptions,
		},
		AllowHeaders: []string{
			echo.HeaderOrigin,
			echo.HeaderContentType,
			echo.HeaderAccept,
			echo.HeaderAuthorization,
			"X-API-Key",
			echo.HeaderXRequestID,
		},
	}))

	// General rate limit — applied globally before route matching
	e.Use(deps.RateLimiter.Limit())

	// ---------------------------------------------------------------
	// Public routes — no authentication required
	// ---------------------------------------------------------------

	e.GET("/health", deps.HealthHandler.Health)
	e.GET("/metrics", echo.WrapHandler(promhttp.Handler()))

	// ---------------------------------------------------------------
	// API v1 routes
	// ---------------------------------------------------------------

	v1 := e.Group("/api/v1")

	// Auth routes — public
	auth := v1.Group("/auth")
	auth.POST("/register", deps.AuthHandler.Register)
	auth.POST("/login", deps.AuthHandler.Login)

	// Document routes — require authentication
	docs := v1.Group("/documents", deps.AuthMiddleware.RequireAuth())

	// Upload has a stricter rate limit than other document endpoints
	docs.POST("",
		deps.DocumentHandler.Upload,
		deps.RateLimiter.LimitUpload(),
		middleware.ValidateUpload(&deps.Config.Upload, deps.Log),
	)

	docs.GET("", deps.DocumentHandler.List)
	docs.GET("/:id", deps.DocumentHandler.GetByID)
	docs.GET("/:id/download", deps.DocumentHandler.GetDownloadURL)
	docs.GET("/:id/status", deps.DocumentHandler.GetStatus)
	docs.DELETE("/:id", deps.DocumentHandler.Delete)

	// Search routes — require authentication
	search := v1.Group("/search", deps.AuthMiddleware.RequireAuth())
	search.GET("", deps.SearchHandler.Search)

	return e
}

// customErrorHandler formats all errors — including Echo HTTP errors
// and unexpected panics — into a consistent JSON response shape.
func customErrorHandler(log *logger.Logger) echo.HTTPErrorHandler {
	return func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}

		code := http.StatusInternalServerError
		message := "internal server error"

		if he, ok := err.(*echo.HTTPError); ok {
			code = he.Code
			if msg, ok := he.Message.(string); ok {
				message = msg
			}
		}

		// Log 5xx errors only — 4xx are client mistakes
		if code >= 500 {
			requestID, _ := c.Get("request_id").(string)
			log.Error().
				Err(err).
				Str("request_id", requestID).
				Int("status", code).
				Str("path", c.Request().URL.Path).
				Msg("Internal server error")
		}

		_ = c.JSON(code, httpErrorResponse{
			Error:   http.StatusText(code),
			Message: message,
		})
	}
}
