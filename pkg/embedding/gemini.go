package embedding

import (
	"context"
	"fmt"

	"github.com/IndraSty/smart-doc-intelligence/config"
	"github.com/IndraSty/smart-doc-intelligence/pkg/logger"
	"google.golang.org/api/option"

	genai "github.com/google/generative-ai-go/genai"
)

const (
	// EmbeddingDimension is the output dimension of text-embedding-004.
	// Must match the vector(768) column in the extractions table.
	EmbeddingDimension = 768
)

// Generator wraps the Gemini embedding model to convert text into
// float32 vectors suitable for semantic search with pgvector.
type Generator struct {
	client *genai.Client
	model  string
	log    *logger.Logger
}

// NewGenerator creates a new embedding Generator using the Gemini API.
func NewGenerator(ctx context.Context, cfg *config.GeminiConfig, log *logger.Logger) (*Generator, error) {
	client, err := genai.NewClient(ctx, option.WithAPIKey(cfg.APIKey))
	if err != nil {
		return nil, fmt.Errorf("embedding.NewGenerator create client: %w", err)
	}

	log.Info().
		Str("model", cfg.EmbeddingModel).
		Int("dimension", EmbeddingDimension).
		Msg("Embedding generator initialized")

	return &Generator{
		client: client,
		model:  cfg.EmbeddingModel,
		log:    log,
	}, nil
}

// Generate converts a text string into a float32 vector using Gemini's
// text-embedding-004 model. The resulting vector has 768 dimensions.
//
// taskType should be one of:
//   - "RETRIEVAL_DOCUMENT" — for embedding documents at index time
//   - "RETRIEVAL_QUERY"    — for embedding search queries at query time
//
// Using the correct task type improves search relevance significantly.
func (g *Generator) Generate(ctx context.Context, text string, taskType string) ([]float32, error) {
	if text == "" {
		return nil, fmt.Errorf("embedding.Generate: empty text provided")
	}

	em := g.client.EmbeddingModel(g.model)

	// Set task type to improve retrieval accuracy
	switch taskType {
	case "RETRIEVAL_DOCUMENT":
		em.TaskType = genai.TaskTypeRetrievalDocument
	case "RETRIEVAL_QUERY":
		em.TaskType = genai.TaskTypeRetrievalQuery
	default:
		em.TaskType = genai.TaskTypeRetrievalDocument
	}

	res, err := em.EmbedContent(ctx, genai.Text(text))
	if err != nil {
		return nil, fmt.Errorf("embedding.Generate EmbedContent: %w", err)
	}

	if res == nil || res.Embedding == nil {
		return nil, fmt.Errorf("embedding.Generate: nil embedding returned")
	}

	vector := res.Embedding.Values

	// Validate output dimension matches our pgvector column
	if len(vector) != EmbeddingDimension {
		return nil, fmt.Errorf("embedding.Generate: expected %d dimensions, got %d",
			EmbeddingDimension, len(vector))
	}

	g.log.Debug().
		Int("dimension", len(vector)).
		Str("task_type", taskType).
		Msg("Embedding generated")

	return vector, nil
}

// GenerateForDocument is a convenience wrapper for indexing a document.
// Uses RETRIEVAL_DOCUMENT task type for better search performance.
func (g *Generator) GenerateForDocument(ctx context.Context, text string) ([]float32, error) {
	return g.Generate(ctx, text, "RETRIEVAL_DOCUMENT")
}

// GenerateForQuery is a convenience wrapper for embedding a search query.
// Uses RETRIEVAL_QUERY task type for better search performance.
func (g *Generator) GenerateForQuery(ctx context.Context, query string) ([]float32, error) {
	return g.Generate(ctx, query, "RETRIEVAL_QUERY")
}

// BuildDocumentText constructs a single string from document fields
// that will be embedded. We combine field keys and values to maximize
// semantic coverage of the document content.
func BuildDocumentText(summary string, fields map[string]interface{}) string {
	if len(fields) == 0 {
		return summary
	}

	var parts []string
	parts = append(parts, summary)

	for k, v := range fields {
		parts = append(parts, fmt.Sprintf("%s: %v", k, v))
	}

	result := ""
	for i, p := range parts {
		if i > 0 {
			result += ". "
		}
		result += p
	}

	return result
}

// Close releases the Gemini client resources.
func (g *Generator) Close() error {
	return g.client.Close()
}
