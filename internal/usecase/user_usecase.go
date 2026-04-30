package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/IndraSty/smart-doc-intelligence/config"
	"github.com/IndraSty/smart-doc-intelligence/internal/domain"
	"github.com/IndraSty/smart-doc-intelligence/pkg/logger"
)

type userUsecase struct {
	userRepo domain.UserRepository
	cfg      *config.Config
	log      *logger.Logger
}

// NewUserUsecase creates a new UserUsecase implementation.
func NewUserUsecase(
	userRepo domain.UserRepository,
	cfg *config.Config,
	log *logger.Logger,
) domain.UserUsecase {
	return &userUsecase{
		userRepo: userRepo,
		cfg:      cfg,
		log:      log,
	}
}

// Register creates a new user account.
// Returns auth tokens and the plaintext API key — shown only once.
func (u *userUsecase) Register(ctx context.Context, input domain.RegisterInput) (*domain.AuthTokens, string, error) {
	// Check if email is already taken
	_, err := u.userRepo.FindByEmail(ctx, input.Email)
	if err == nil {
		return nil, "", domain.ErrConflict
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return nil, "", fmt.Errorf("userUsecase.Register find email: %w", err)
	}

	// Hash password with bcrypt cost 12
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(input.Password), 12)
	if err != nil {
		return nil, "", fmt.Errorf("userUsecase.Register hash password: %w", err)
	}

	// Generate a random API key and store only its SHA-256 hash
	plaintextAPIKey, hashedAPIKey, err := generateAPIKey()
	if err != nil {
		return nil, "", fmt.Errorf("userUsecase.Register generate api key: %w", err)
	}

	user := &domain.User{
		Email:        input.Email,
		PasswordHash: string(passwordHash),
		APIKey:       hashedAPIKey, // only the hash is stored
	}

	created, err := u.userRepo.Create(ctx, user)
	if err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return nil, "", domain.ErrConflict
		}
		return nil, "", fmt.Errorf("userUsecase.Register create user: %w", err)
	}

	tokens, err := u.generateTokens(created.ID, created.Email)
	if err != nil {
		return nil, "", fmt.Errorf("userUsecase.Register generate tokens: %w", err)
	}

	u.log.Info().
		Str("user_id", created.ID).
		Str("email", created.Email).
		Msg("New user registered")

	// Return plaintext API key — this is the only time it will ever be shown
	return tokens, plaintextAPIKey, nil
}

// Login authenticates a user with email and password.
func (u *userUsecase) Login(ctx context.Context, input domain.LoginInput) (*domain.AuthTokens, error) {
	user, err := u.userRepo.FindByEmail(ctx, input.Email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			// Return generic error to prevent email enumeration
			return nil, domain.ErrUnauthorized
		}
		return nil, fmt.Errorf("userUsecase.Login find user: %w", err)
	}

	// Constant-time password comparison via bcrypt
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Password)); err != nil {
		return nil, domain.ErrUnauthorized
	}

	tokens, err := u.generateTokens(user.ID, user.Email)
	if err != nil {
		return nil, fmt.Errorf("userUsecase.Login generate tokens: %w", err)
	}

	u.log.Info().
		Str("user_id", user.ID).
		Msg("User logged in")

	return tokens, nil
}

// generateTokens creates a JWT access token and refresh token pair.
func (u *userUsecase) generateTokens(userID, email string) (*domain.AuthTokens, error) {
	accessExpire := time.Duration(u.cfg.JWT.AccessExpireMinutes) * time.Minute
	refreshExpire := time.Duration(u.cfg.JWT.RefreshExpireDays) * 24 * time.Hour

	// Access token — short lived, carries user identity
	accessClaims := jwt.MapClaims{
		"sub":   userID,
		"email": email,
		"type":  "access",
		"exp":   time.Now().Add(accessExpire).Unix(),
		"iat":   time.Now().Unix(),
	}

	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessSigned, err := accessToken.SignedString([]byte(u.cfg.JWT.Secret))
	if err != nil {
		return nil, fmt.Errorf("generateTokens sign access: %w", err)
	}

	// Refresh token — long lived, used only to get new access tokens
	refreshClaims := jwt.MapClaims{
		"sub":  userID,
		"type": "refresh",
		"exp":  time.Now().Add(refreshExpire).Unix(),
		"iat":  time.Now().Unix(),
	}

	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshSigned, err := refreshToken.SignedString([]byte(u.cfg.JWT.Secret))
	if err != nil {
		return nil, fmt.Errorf("generateTokens sign refresh: %w", err)
	}

	return &domain.AuthTokens{
		AccessToken:  accessSigned,
		RefreshToken: refreshSigned,
		ExpiresIn:    int(accessExpire.Seconds()),
	}, nil
}

// generateAPIKey creates a cryptographically random 32-byte API key.
// Returns both the plaintext (shown once) and SHA-256 hash (stored in DB).
func generateAPIKey() (plaintext, hashed string, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("generateAPIKey rand read: %w", err)
	}

	plaintext = hex.EncodeToString(raw) // 64-char hex string

	sum := sha256.Sum256([]byte(plaintext))
	hashed = hex.EncodeToString(sum[:])

	return plaintext, hashed, nil
}

// HashAPIKey hashes a plaintext API key for database lookup.
// Used by the auth middleware to look up a user by their API key.
func HashAPIKey(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}
