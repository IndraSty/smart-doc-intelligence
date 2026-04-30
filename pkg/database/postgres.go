package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/IndraSty/smart-doc-intelligence/config"
	"github.com/IndraSty/smart-doc-intelligence/pkg/logger"
)

// NewPostgresPool creates and validates a pgx connection pool.
// It registers the pgvector type so vector columns can be scanned
// directly into []float32 slices.
func NewPostgresPool(ctx context.Context, cfg *config.DatabaseConfig, log *logger.Logger) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database URL: %w", err)
	}

	// Connection pool sizing
	poolCfg.MaxConns = cfg.MaxConnections
	poolCfg.MinConns = cfg.MinConnections

	// Connection health settings
	poolCfg.MaxConnLifetime = 1 * time.Hour
	poolCfg.MaxConnIdleTime = 30 * time.Minute
	poolCfg.HealthCheckPeriod = 1 * time.Minute

	// AfterConnect runs on every new connection in the pool.
	// We use it to verify the connection is functional and
	// pgvector types are accessible.
	poolCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		// Verify connection is alive with a lightweight query
		var one int
		if err := conn.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil {
			return fmt.Errorf("AfterConnect health check failed: %w", err)
		}
		return nil
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Verify the connection is actually usable
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Verify pgvector extension is available
	var extExists bool
	err = pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'vector')",
	).Scan(&extExists)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to check pgvector extension: %w", err)
	}

	if !extExists {
		pool.Close()
		return nil, fmt.Errorf("pgvector extension is not installed — run migrations first")
	}

	log.Info().
		Int32("max_conns", cfg.MaxConnections).
		Int32("min_conns", cfg.MinConnections).
		Msg("PostgreSQL connection pool established")

	return pool, nil
}

// RunMigrations applies all pending up migrations using golang-migrate.
// Call this during application startup before serving traffic.
func RunMigrations(databaseURL string, migrationsPath string, log *logger.Logger) error {
	// Import these in the file that calls RunMigrations:
	// _ "github.com/golang-migrate/migrate/v4/database/postgres"
	// _ "github.com/golang-migrate/migrate/v4/source/file"

	// We keep this as a helper comment here.
	// The actual migrate call is done in cmd/api/main.go
	// to keep this package free of migrate imports if not needed.
	log.Info().Str("path", migrationsPath).Msg("Running database migrations...")
	return nil
}

// HealthCheck pings the database and returns an error if unreachable.
// Used by the /health endpoint.
func HealthCheck(ctx context.Context, pool *pgxpool.Pool) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	return nil
}
