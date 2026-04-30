package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/IndraSty/smart-doc-intelligence/internal/domain"
	"github.com/IndraSty/smart-doc-intelligence/pkg/logger"
)

type userRepository struct {
	pool *pgxpool.Pool
	log  *logger.Logger
}

// NewUserRepository creates a new PostgreSQL-backed UserRepository.
func NewUserRepository(pool *pgxpool.Pool, log *logger.Logger) domain.UserRepository {
	return &userRepository{pool: pool, log: log}
}

func (r *userRepository) Create(ctx context.Context, user *domain.User) (*domain.User, error) {
	query := `
		INSERT INTO users (email, password_hash, api_key)
		VALUES ($1, $2, $3)
		RETURNING id, email, password_hash, api_key, created_at, updated_at
	`

	created := &domain.User{}
	err := r.pool.QueryRow(ctx, query,
		user.Email,
		user.PasswordHash,
		user.APIKey,
	).Scan(
		&created.ID,
		&created.Email,
		&created.PasswordHash,
		&created.APIKey,
		&created.CreatedAt,
		&created.UpdatedAt,
	)
	if err != nil {
		// Check for unique constraint violation (duplicate email or api_key)
		if isPgUniqueViolation(err) {
			return nil, domain.ErrConflict
		}
		return nil, fmt.Errorf("userRepository.Create: %w", err)
	}

	return created, nil
}

func (r *userRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	query := `
		SELECT id, email, password_hash, api_key, created_at, updated_at
		FROM users
		WHERE email = $1
		LIMIT 1
	`

	user := &domain.User{}
	err := r.pool.QueryRow(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.APIKey,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &domain.NotFoundError{Resource: "user", ID: email}
		}
		return nil, fmt.Errorf("userRepository.FindByEmail: %w", err)
	}

	return user, nil
}

func (r *userRepository) FindByID(ctx context.Context, id string) (*domain.User, error) {
	query := `
		SELECT id, email, password_hash, api_key, created_at, updated_at
		FROM users
		WHERE id = $1
		LIMIT 1
	`

	user := &domain.User{}
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.APIKey,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &domain.NotFoundError{Resource: "user", ID: id}
		}
		return nil, fmt.Errorf("userRepository.FindByID: %w", err)
	}

	return user, nil
}

func (r *userRepository) FindByAPIKey(ctx context.Context, hashedKey string) (*domain.User, error) {
	query := `
		SELECT id, email, password_hash, api_key, created_at, updated_at
		FROM users
		WHERE api_key = $1
		LIMIT 1
	`

	user := &domain.User{}
	err := r.pool.QueryRow(ctx, query, hashedKey).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.APIKey,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &domain.NotFoundError{Resource: "user", ID: hashedKey}
		}
		return nil, fmt.Errorf("userRepository.FindByAPIKey: %w", err)
	}

	return user, nil
}

// isPgUniqueViolation checks if the error is a PostgreSQL unique constraint violation.
// pgx wraps the error so we check the string code directly.
func isPgUniqueViolation(err error) bool {
	return err != nil && len(err.Error()) > 0 &&
		containsAny(err.Error(), "23505", "unique constraint", "duplicate key")
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}
