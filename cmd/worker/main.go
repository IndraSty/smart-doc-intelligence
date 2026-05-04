package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/IndraSty/smart-doc-intelligence/config"
	"github.com/IndraSty/smart-doc-intelligence/internal/ai/gemini"
	"github.com/IndraSty/smart-doc-intelligence/internal/repository/postgres"
	redisrepo "github.com/IndraSty/smart-doc-intelligence/internal/repository/redis"
	"github.com/IndraSty/smart-doc-intelligence/internal/usecase"
	"github.com/IndraSty/smart-doc-intelligence/internal/worker"
	"github.com/IndraSty/smart-doc-intelligence/pkg/database"
	"github.com/IndraSty/smart-doc-intelligence/pkg/embedding"
	"github.com/IndraSty/smart-doc-intelligence/pkg/logger"
	"github.com/IndraSty/smart-doc-intelligence/pkg/queue"
	"github.com/IndraSty/smart-doc-intelligence/pkg/storage"
)

func main() {
	// ── Bootstrap ────────────────────────────────────────────────────
	log := logger.New("development")
	log.Info().Msg("Starting Smart Document Intelligence Worker...")

	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load configuration", err)
	}

	log = logger.New(cfg.App.Env)
	log = log.WithService("worker")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ── Database ─────────────────────────────────────────────────────
	pool, err := database.NewPostgresPool(ctx, &cfg.Database, log)
	if err != nil {
		log.Fatal("Failed to connect to PostgreSQL", err)
	}
	defer pool.Close()

	// ── Redis ────────────────────────────────────────────────────────
	redisClient, err := redisrepo.NewRedisClient(cfg.Redis.URL, log)
	if err != nil {
		log.Fatal("Failed to connect to Redis", err)
	}
	defer func() { _ = redisClient.Close() }()

	// ── RabbitMQ ─────────────────────────────────────────────────────
	queueClient, err := queue.NewClient(&cfg.RabbitMQ, log)
	if err != nil {
		log.Fatal("Failed to connect to RabbitMQ", err)
	}
	defer queueClient.Close()

	// ── Storage ──────────────────────────────────────────────────────
	storageClient := storage.NewClient(&cfg.Supabase, log)

	// ── AI ───────────────────────────────────────────────────────────
	aiClient, err := gemini.NewClient(ctx, &cfg.Gemini, log)
	if err != nil {
		log.Fatal("Failed to initialize Gemini AI client", err)
	}

	embedder, err := embedding.NewGenerator(ctx, &cfg.Gemini, log)
	if err != nil {
		log.Fatal("Failed to initialize embedding generator", err)
	}
	defer func() { _ = embedder.Close() }()

	// ── Repositories ─────────────────────────────────────────────────
	docRepo := postgres.NewDocumentRepository(pool, log)
	extractionRepo := postgres.NewExtractionRepository(pool, log)
	jobRepo := redisrepo.NewJobRepository(redisClient, log)

	// ── Usecases ─────────────────────────────────────────────────────
	processingUC := usecase.NewProcessingUsecase(jobRepo, docRepo, queueClient, log)

	// ── Worker pool ──────────────────────────────────────────────────
	w := worker.NewWorker(
		aiClient,
		docRepo,
		extractionRepo,
		jobRepo,
		processingUC,
		storageClient,
		embedder,
		queueClient,
		worker.Config{
			PoolSize: cfg.Worker.PoolSize,
			RetryMax: cfg.Worker.RetryMax,
		},
		log,
	)

	// ── Graceful shutdown ────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		log.Info().Msg("Shutdown signal received")
		cancel() // cancel context to stop worker pool
	}()

	// Start blocks until ctx is cancelled
	if err := w.Start(ctx); err != nil {
		log.Fatal("Worker pool failed", err)
	}

	log.Info().Msg("Worker exited cleanly")
}
