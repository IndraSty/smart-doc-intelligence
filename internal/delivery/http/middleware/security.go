package middleware

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/IndraSty/smart-doc-intelligence/config"
)

// SecurityHeaders returns a middleware that sets security-related HTTP headers
// on every response as required by the project spec.
func SecurityHeaders() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			h := c.Response().Header()

			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			h.Set("X-XSS-Protection", "1; mode=block")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
			h.Set("Cache-Control", "no-store")

			return next(c)
		}
	}
}

// RequireHTTPS enforces HTTPS in production environments.
// In development this middleware is a no-op to allow plain HTTP locally.
func RequireHTTPS(cfg *config.Config) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if cfg.IsDevelopment() {
				return next(c)
			}

			proto := c.Request().Header.Get("X-Forwarded-Proto")
			if proto == "http" {
				httpsURL := "https://" + c.Request().Host + c.Request().RequestURI
				return c.Redirect(http.StatusMovedPermanently, httpsURL)
			}

			return next(c)
		}
	}
}

// RequestID generates a unique ID for every request and attaches it to
// both the request context and response header for distributed tracing.
func RequestID() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			requestID := c.Request().Header.Get(echo.HeaderXRequestID)
			if requestID == "" {
				requestID = generateRequestID()
			}

			c.Request().Header.Set(echo.HeaderXRequestID, requestID)
			c.Response().Header().Set(echo.HeaderXRequestID, requestID)
			c.Set("request_id", requestID)

			return next(c)
		}
	}
}
