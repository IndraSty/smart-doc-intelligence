package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"

	"github.com/IndraSty/smart-doc-intelligence/config"
	"github.com/IndraSty/smart-doc-intelligence/internal/domain"
	"github.com/IndraSty/smart-doc-intelligence/pkg/logger"
)

// contextKey is an unexported type for context keys in this package.
// Prevents collisions with keys from other packages.
type contextKey string

const (
	// ContextKeyUserID is the key used to store the authenticated user ID in context.
	ContextKeyUserID contextKey = "user_id"

	// ContextKeyEmail is the key used to store the authenticated user email in context.
	ContextKeyEmail contextKey = "email"
)

// AuthMiddleware holds dependencies for authentication middleware.
type AuthMiddleware struct {
	userRepo domain.UserRepository
	cfg      *config.Config
	log      *logger.Logger
}

// NewAuthMiddleware creates a new AuthMiddleware instance.
func NewAuthMiddleware(
	userRepo domain.UserRepository,
	cfg *config.Config,
	log *logger.Logger,
) *AuthMiddleware {
	return &AuthMiddleware{
		userRepo: userRepo,
		cfg:      cfg,
		log:      log,
	}
}

// RequireAuth is an Echo middleware that accepts either:
//  1. Bearer JWT token in Authorization header
//  2. API key in X-API-Key header (SHA-256 hashed for DB lookup)
//
// On success, sets user_id and email in the Echo context.
func (m *AuthMiddleware) RequireAuth() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Try JWT first
			if token := extractBearerToken(c.Request()); token != "" {
				userID, email, err := m.validateJWT(token)
				if err != nil {
					m.log.Debug().Err(err).Msg("JWT validation failed")
					return echo.NewHTTPError(http.StatusUnauthorized, "invalid or expired token")
				}

				c.Set(string(ContextKeyUserID), userID)
				c.Set(string(ContextKeyEmail), email)
				return next(c)
			}

			// Try API key second
			if apiKey := c.Request().Header.Get("X-API-Key"); apiKey != "" {
				user, err := m.validateAPIKey(c.Request().Context(), apiKey)
				if err != nil {
					m.log.Debug().Err(err).Msg("API key validation failed")
					return echo.NewHTTPError(http.StatusUnauthorized, "invalid API key")
				}

				c.Set(string(ContextKeyUserID), user.ID)
				c.Set(string(ContextKeyEmail), user.Email)
				return next(c)
			}

			return echo.NewHTTPError(http.StatusUnauthorized,
				"authentication required: provide Bearer token or X-API-Key header")
		}
	}
}

// GetUserID extracts the authenticated user ID from Echo context.
// Panics if called outside of a RequireAuth-protected route.
func GetUserID(c echo.Context) string {
	return c.Get(string(ContextKeyUserID)).(string)
}

// GetEmail extracts the authenticated user email from Echo context.
func GetEmail(c echo.Context) string {
	return c.Get(string(ContextKeyEmail)).(string)
}

// validateJWT parses and validates a JWT access token.
// Returns the user ID and email on success.
func (m *AuthMiddleware) validateJWT(tokenString string) (userID, email string, err error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Enforce HS256 — reject tokens with unexpected signing methods
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(m.cfg.JWT.Secret), nil
	})
	if err != nil {
		return "", "", err
	}

	if !token.Valid {
		return "", "", jwt.ErrTokenInvalidClaims
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", "", jwt.ErrTokenInvalidClaims
	}

	// Ensure this is an access token, not a refresh token
	tokenType, _ := claims["type"].(string)
	if tokenType != "access" {
		return "", "", errors.New("token type must be 'access'")
	}

	userID, _ = claims["sub"].(string)
	email, _ = claims["email"].(string)

	if userID == "" {
		return "", "", errors.New("missing subject claim")
	}

	return userID, email, nil
}

// validateAPIKey hashes the provided key and looks it up in the database.
func (m *AuthMiddleware) validateAPIKey(ctx context.Context, plaintext string) (*domain.User, error) {
	hashed := hashAPIKeyInternal(plaintext)

	user, err := m.userRepo.FindByAPIKey(ctx, hashed)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrUnauthorized
		}
		return nil, err
	}

	return user, nil
}

// extractBearerToken pulls the token from "Authorization: Bearer <token>".
func extractBearerToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return ""
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}

	return strings.TrimSpace(parts[1])
}
