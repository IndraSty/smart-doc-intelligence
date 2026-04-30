package domain

import (
	"context"
	"time"
)

// User represents an authenticated user of the system.
// API keys are stored as SHA-256 hashes — the plaintext key
// is only returned once at registration time.
type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"` // never serialized to JSON
	APIKey       string    `json:"-"` // stored as SHA-256 hash, never serialized
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// RegisterInput holds the data required to create a new user.
type RegisterInput struct {
	Email    string `json:"email"    validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
}

// LoginInput holds credentials for authentication.
type LoginInput struct {
	Email    string `json:"email"    validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

// AuthTokens holds the JWT access token and refresh token pair
// returned after a successful login or registration.
type AuthTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"` // seconds until access token expires
}

// UserRepository defines all database operations for the users table.
// Implemented in internal/repository/postgres/user_repo.go
type UserRepository interface {
	// Create inserts a new user and returns the created record.
	Create(ctx context.Context, user *User) (*User, error)

	// FindByEmail returns a user by email address.
	// Returns ErrNotFound if the user does not exist.
	FindByEmail(ctx context.Context, email string) (*User, error)

	// FindByID returns a user by their UUID.
	// Returns ErrNotFound if the user does not exist.
	FindByID(ctx context.Context, id string) (*User, error)

	// FindByAPIKey returns a user by their hashed API key.
	// Returns ErrNotFound if no matching key exists.
	FindByAPIKey(ctx context.Context, hashedKey string) (*User, error)
}

// UserUsecase defines all business logic operations for user management.
// Implemented in internal/usecase/user_usecase.go
type UserUsecase interface {
	// Register creates a new user account.
	// Returns the auth tokens and the plaintext API key (shown once only).
	Register(ctx context.Context, input RegisterInput) (*AuthTokens, string, error)

	// Login authenticates a user and returns new auth tokens.
	Login(ctx context.Context, input LoginInput) (*AuthTokens, error)
}
