package handler

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/IndraSty/smart-doc-intelligence/internal/domain"
	"github.com/IndraSty/smart-doc-intelligence/pkg/logger"
)

// AuthHandler handles user registration and login endpoints.
type AuthHandler struct {
	userUsecase domain.UserUsecase
	log         *logger.Logger
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(userUsecase domain.UserUsecase, log *logger.Logger) *AuthHandler {
	return &AuthHandler{
		userUsecase: userUsecase,
		log:         log,
	}
}

// registerRequest is the expected JSON body for POST /auth/register.
type registerRequest struct {
	Email    string `json:"email"    example:"user@example.com"`
	Password string `json:"password" example:"securepassword123"`
}

// loginRequest is the expected JSON body for POST /auth/login.
type loginRequest struct {
	Email    string `json:"email"    example:"user@example.com"`
	Password string `json:"password" example:"securepassword123"`
}

// registerResponse is the JSON body returned after successful registration.
type registerResponse struct {
	Tokens *domain.AuthTokens `json:"tokens"`
	APIKey string             `json:"api_key" example:"a3f1c2...64hexchars"`
	Notice string             `json:"notice"  example:"Save your API key now — it will not be shown again."`
}

// loginResponse is the JSON body returned after successful login.
type loginResponse struct {
	Tokens *domain.AuthTokens `json:"tokens"`
}

// Register godoc
// @Summary      Register a new user
// @Description  Creates a new user account. Returns JWT tokens and a one-time plaintext API key.
// @Description  The API key is shown exactly once — store it securely.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body body registerRequest true "Registration credentials"
// @Success      201  {object} registerResponse
// @Failure      400  {object} errorResponse "Missing or invalid fields"
// @Failure      409  {object} errorResponse "Email already registered"
// @Failure      500  {object} errorResponse "Internal server error"
// @Router       /api/v1/auth/register [post]
func (h *AuthHandler) Register(c echo.Context) error {
	var req registerRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if req.Email == "" || req.Password == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "email and password are required")
	}

	if len(req.Password) < 8 {
		return echo.NewHTTPError(http.StatusBadRequest, "password must be at least 8 characters")
	}

	input := domain.RegisterInput{
		Email:    req.Email,
		Password: req.Password,
	}

	tokens, plaintextAPIKey, err := h.userUsecase.Register(c.Request().Context(), input)
	if err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return echo.NewHTTPError(http.StatusConflict, "email already registered")
		}
		h.log.Error().Err(err).Str("email", req.Email).Msg("Register failed")
		return echo.NewHTTPError(http.StatusInternalServerError, "registration failed")
	}

	return c.JSON(http.StatusCreated, registerResponse{
		Tokens: tokens,
		APIKey: plaintextAPIKey,
		Notice: "Save your API key now — it will not be shown again.",
	})
}

// Login godoc
// @Summary      Login
// @Description  Authenticates a user with email and password.
// @Description  Returns a short-lived JWT access token (15 min) and a long-lived refresh token (7 days).
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body body loginRequest true "Login credentials"
// @Success      200  {object} loginResponse
// @Failure      400  {object} errorResponse "Missing fields"
// @Failure      401  {object} errorResponse "Invalid credentials"
// @Failure      500  {object} errorResponse "Internal server error"
// @Router       /api/v1/auth/login [post]
func (h *AuthHandler) Login(c echo.Context) error {
	var req loginRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	if req.Email == "" || req.Password == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "email and password are required")
	}

	input := domain.LoginInput{
		Email:    req.Email,
		Password: req.Password,
	}

	tokens, err := h.userUsecase.Login(c.Request().Context(), input)
	if err != nil {
		if errors.Is(err, domain.ErrUnauthorized) {
			return echo.NewHTTPError(http.StatusUnauthorized, "invalid email or password")
		}
		h.log.Error().Err(err).Str("email", req.Email).Msg("Login failed")
		return echo.NewHTTPError(http.StatusInternalServerError, "login failed")
	}

	return c.JSON(http.StatusOK, loginResponse{Tokens: tokens})
}
