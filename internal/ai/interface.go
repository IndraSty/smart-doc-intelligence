package ai

import (
	"context"

	"github.com/IndraSty/smart-doc-intelligence/internal/domain"
)

// AIProvider defines the contract for all AI processing operations.
// The Gemini implementation lives in internal/ai/gemini/client.go.
// This interface makes it easy to swap providers or mock in tests.
type AIProvider interface {
	// Process sends a document to the AI provider and returns the full
	// analysis result including classification, extracted fields, and summary.
	// It handles retries internally with exponential backoff.
	Process(ctx context.Context, input ProcessInput) (*domain.AIResult, error)
}

// ProcessInput holds everything the AI provider needs to analyze a document.
type ProcessInput struct {
	// DocumentID is used for logging and tracing only
	DocumentID string

	// FileData is the raw bytes of the document
	FileData []byte

	// FileType is the extension: pdf, png, jpg, txt
	FileType string

	// MIMEType is the full MIME type: application/pdf, image/png, etc.
	MIMEType string

	// Filename is the original filename, used only for context in prompts
	Filename string
}
