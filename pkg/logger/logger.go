package logger

import (
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Logger wraps zerolog.Logger to provide structured logging
// with consistent fields across the application.
type Logger struct {
	zerolog.Logger
}

// New creates and configures a new Logger instance.
// In development, it uses a human-readable console writer.
// In production, it outputs structured JSON for log aggregators.
func New(env string) *Logger {
	// Set global log level
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	// Use millisecond precision for timestamps
	zerolog.TimeFieldFormat = time.RFC3339Nano

	var logger zerolog.Logger

	if env == "development" {
		// Pretty console output for local development
		output := zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: "15:04:05",
		}
		logger = zerolog.New(output).
			With().
			Timestamp().
			Caller().
			Logger()

		// Enable debug level in development
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		// JSON output for production — compatible with Grafana Loki
		logger = zerolog.New(os.Stdout).
			With().
			Timestamp().
			Logger()
	}

	// Set as global logger so log.Info(), log.Error() etc. work anywhere
	log.Logger = logger

	return &Logger{Logger: logger}
}

// WithService returns a logger with a "service" field pre-attached.
// Use this to differentiate logs from api vs worker processes.
func (l *Logger) WithService(name string) *Logger {
	return &Logger{
		Logger: l.Logger.With().Str("service", name).Logger(),
	}
}

// WithRequestID returns a logger with a "request_id" field pre-attached.
// Used inside HTTP middleware to correlate logs per request.
func (l *Logger) WithRequestID(requestID string) *Logger {
	return &Logger{
		Logger: l.Logger.With().Str("request_id", requestID).Logger(),
	}
}

// WithDocumentID returns a logger with a "document_id" field pre-attached.
// Used inside worker and usecase layers for document processing logs.
func (l *Logger) WithDocumentID(documentID string) *Logger {
	return &Logger{
		Logger: l.Logger.With().Str("document_id", documentID).Logger(),
	}
}

// WithUserID returns a logger with a "user_id" field pre-attached.
// Used to trace all actions performed by a specific user.
func (l *Logger) WithUserID(userID string) *Logger {
	return &Logger{
		Logger: l.Logger.With().Str("user_id", userID).Logger(),
	}
}

// Fatal logs a fatal message and exits the process.
// Use only during initialization failures.
func (l *Logger) Fatal(msg string, err error) {
	l.Logger.Fatal().Err(err).Msg(msg)
}
