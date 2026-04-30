package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"golang.org/x/time/rate"

	"github.com/IndraSty/smart-doc-intelligence/config"
	"github.com/IndraSty/smart-doc-intelligence/pkg/logger"
)

// visitor holds a rate limiter and last seen time per user/IP.
type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimiter provides per-user and per-IP rate limiting using the
// token bucket algorithm via golang.org/x/time/rate.
type RateLimiter struct {
	visitors map[string]*visitor
	mu       sync.RWMutex
	cfg      *config.RateLimitConfig
	log      *logger.Logger
}

// NewRateLimiter creates a RateLimiter and starts the cleanup goroutine.
func NewRateLimiter(cfg *config.RateLimitConfig, log *logger.Logger) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		cfg:      cfg,
		log:      log,
	}

	// Cleanup stale visitor entries every 3 minutes
	go rl.cleanupLoop()

	return rl
}

// Limit returns a general-purpose rate limiting middleware.
// Uses RPS from config.RateLimit.GeneralRPS.
// Key is user ID if authenticated, otherwise client IP.
func (rl *RateLimiter) Limit() echo.MiddlewareFunc {
	return rl.limitWithRPS(rl.cfg.GeneralRPS)
}

// LimitUpload returns a stricter rate limiting middleware for the upload endpoint.
// Upload triggers AI processing, so it gets a lower RPS limit.
func (rl *RateLimiter) LimitUpload() echo.MiddlewareFunc {
	return rl.limitWithRPS(rl.cfg.UploadRPS)
}

// limitWithRPS creates a middleware that enforces a specific RPS limit.
func (rl *RateLimiter) limitWithRPS(rps float64) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Use user ID if authenticated, otherwise fall back to IP
			key := c.RealIP()
			if userID := c.Get(string(ContextKeyUserID)); userID != nil {
				key = "user:" + userID.(string)
			}

			limiter := rl.getOrCreateLimiter(key, rps)

			if !limiter.Allow() {
				rl.log.Debug().
					Str("key", key).
					Float64("rps", rps).
					Msg("Rate limit exceeded")

				return echo.NewHTTPError(
					http.StatusTooManyRequests,
					"rate limit exceeded — please slow down",
				)
			}

			return next(c)
		}
	}
}

// getOrCreateLimiter retrieves an existing limiter or creates a new one for the key.
func (rl *RateLimiter) getOrCreateLimiter(key string, rps float64) *rate.Limiter {
	rl.mu.RLock()
	v, exists := rl.visitors[key]
	rl.mu.RUnlock()

	if exists {
		v.lastSeen = time.Now()
		return v.limiter
	}

	// Create new limiter: allow burst of 2x the RPS for short spikes
	burst := int(rps * 2)
	if burst < 1 {
		burst = 1
	}

	limiter := rate.NewLimiter(rate.Limit(rps), burst)

	rl.mu.Lock()
	rl.visitors[key] = &visitor{
		limiter:  limiter,
		lastSeen: time.Now(),
	}
	rl.mu.Unlock()

	return limiter
}

// cleanupLoop removes visitor entries that have been inactive for 5 minutes.
// This prevents the visitors map from growing unbounded.
func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(3 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		for key, v := range rl.visitors {
			if time.Since(v.lastSeen) > 5*time.Minute {
				delete(rl.visitors, key)
			}
		}
		rl.mu.Unlock()
	}
}
