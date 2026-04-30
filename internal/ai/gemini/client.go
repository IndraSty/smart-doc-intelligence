package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"

	"github.com/IndraSty/smart-doc-intelligence/config"
	internali "github.com/IndraSty/smart-doc-intelligence/internal/ai"
	"github.com/IndraSty/smart-doc-intelligence/internal/domain"
	"github.com/IndraSty/smart-doc-intelligence/pkg/logger"
)

// client implements the AIProvider interface using Google Gemini.
type client struct {
	genaiClient *genai.Client
	cfg         *config.GeminiConfig
	log         *logger.Logger
}

// classificationResponse mirrors the JSON Gemini returns for classification.
type classificationResponse struct {
	DocumentType string  `json:"document_type"`
	Confidence   float64 `json:"confidence"`
	Reasoning    string  `json:"reasoning"`
}

// extractionResponse mirrors the JSON Gemini returns for field extraction.
type extractionResponse struct {
	Fields []domain.Field `json:"fields"`
}

// summaryResponse mirrors the JSON Gemini returns for summarization.
type summaryResponse struct {
	Summary string `json:"summary"`
}

// NewClient creates a new Gemini AIProvider.
func NewClient(ctx context.Context, cfg *config.GeminiConfig, log *logger.Logger) (internali.AIProvider, error) {
	genaiClient, err := genai.NewClient(ctx, option.WithAPIKey(cfg.APIKey))
	if err != nil {
		return nil, fmt.Errorf("gemini.NewClient: %w", err)
	}

	log.Info().
		Str("model", cfg.Model).
		Int("max_retries", cfg.MaxRetries).
		Msg("Gemini AI client initialized")

	return &client{
		genaiClient: genaiClient,
		cfg:         cfg,
		log:         log,
	}, nil
}

// Process runs the full AI pipeline for a document:
// 1. Classify the document type
// 2. Extract fields using the type-specific prompt
// 3. Generate a summary
// Each step retries up to MaxRetries times with exponential backoff.
func (c *client) Process(ctx context.Context, input internali.ProcessInput) (*domain.AIResult, error) {
	log := c.log.WithDocumentID(input.DocumentID)

	// Step 1: Classify document type
	log.Info().Msg("Starting document classification")
	classification, err := c.withRetry(ctx, func() (*classificationResponse, error) {
		return c.classify(ctx, input)
	})
	if err != nil {
		return nil, &domain.AIError{
			Attempts: c.cfg.MaxRetries,
			Cause:    fmt.Errorf("classification failed: %w", err),
		}
	}

	docType := domain.DocumentType(classification.DocumentType)
	log.Info().
		Str("type", string(docType)).
		Float64("confidence", classification.Confidence).
		Msg("Document classified")

	// Step 2: Extract fields using type-specific prompt
	log.Info().Msg("Starting field extraction")
	extraction, err := c.withRetryExtraction(ctx, func() (*extractionResponse, error) {
		return c.extract(ctx, input, docType)
	})
	if err != nil {
		return nil, &domain.AIError{
			Attempts: c.cfg.MaxRetries,
			Cause:    fmt.Errorf("extraction failed: %w", err),
		}
	}

	log.Info().
		Int("field_count", len(extraction.Fields)).
		Msg("Fields extracted")

	// Step 3: Generate summary
	log.Info().Msg("Starting summarization")
	summary, err := c.withRetrySummary(ctx, func() (*summaryResponse, error) {
		return c.summarize(ctx, input, extraction.Fields)
	})
	if err != nil {
		// Summary failure is non-fatal — use a fallback
		log.Warn().Err(err).Msg("Summary generation failed, using fallback")
		summary = &summaryResponse{
			Summary: fmt.Sprintf("A %s document processed by Smart Document Intelligence.", docType),
		}
	}

	// Build the raw AI response string for debugging storage
	rawResponse, _ := json.Marshal(map[string]interface{}{
		"classification": classification,
		"extraction":     extraction,
		"summary":        summary,
	})

	return &domain.AIResult{
		DocumentType: docType,
		Confidence:   classification.Confidence,
		Summary:      summary.Summary,
		Fields:       extraction.Fields,
		RawResponse:  string(rawResponse),
	}, nil
}

// classify sends the document to Gemini with the classification prompt.
func (c *client) classify(ctx context.Context, input internali.ProcessInput) (*classificationResponse, error) {
	model := c.genaiClient.GenerativeModel(c.cfg.Model)
	model.SystemInstruction = &genai.Content{
		Parts: []genai.Part{genai.Text(systemPrompt)},
	}

	// Build the content parts depending on file type
	parts, err := c.buildContentParts(input, classificationPrompt)
	if err != nil {
		return nil, err
	}

	resp, err := model.GenerateContent(ctx, parts...)
	if err != nil {
		return nil, fmt.Errorf("classify GenerateContent: %w", err)
	}

	text := extractTextFromResponse(resp)
	if text == "" {
		return nil, fmt.Errorf("classify: empty response from Gemini")
	}

	var result classificationResponse
	if err := parseJSON(text, &result); err != nil {
		return nil, fmt.Errorf("classify parseJSON: %w", err)
	}

	// Validate the returned document type
	if !isValidDocumentType(result.DocumentType) {
		result.DocumentType = string(domain.TypeOther)
		result.Confidence = 0.5
	}

	return &result, nil
}

// extract sends the document to Gemini with the type-specific extraction prompt.
func (c *client) extract(
	ctx context.Context,
	input internali.ProcessInput,
	docType domain.DocumentType,
) (*extractionResponse, error) {
	model := c.genaiClient.GenerativeModel(c.cfg.Model)
	model.SystemInstruction = &genai.Content{
		Parts: []genai.Part{genai.Text(systemPrompt)},
	}

	extractPrompt := GetExtractionPrompt(docType)
	parts, err := c.buildContentParts(input, extractPrompt)
	if err != nil {
		return nil, err
	}

	resp, err := model.GenerateContent(ctx, parts...)
	if err != nil {
		return nil, fmt.Errorf("extract GenerateContent: %w", err)
	}

	text := extractTextFromResponse(resp)
	if text == "" {
		return nil, fmt.Errorf("extract: empty response from Gemini")
	}

	var result extractionResponse
	if err := parseJSON(text, &result); err != nil {
		return nil, fmt.Errorf("extract parseJSON: %w", err)
	}

	return &result, nil
}

// summarize sends the document + extracted fields to Gemini to generate a summary.
func (c *client) summarize(
	ctx context.Context,
	input internali.ProcessInput,
	fields []domain.Field,
) (*summaryResponse, error) {
	model := c.genaiClient.GenerativeModel(c.cfg.Model)
	model.SystemInstruction = &genai.Content{
		Parts: []genai.Part{genai.Text(systemPrompt)},
	}

	// Include extracted fields in the summary prompt for better context
	fieldsJSON, _ := json.Marshal(fields)
	fullPrompt := fmt.Sprintf("%s\n\nExtracted fields for context:\n%s",
		summaryPrompt, string(fieldsJSON))

	parts, err := c.buildContentParts(input, fullPrompt)
	if err != nil {
		return nil, err
	}

	resp, err := model.GenerateContent(ctx, parts...)
	if err != nil {
		return nil, fmt.Errorf("summarize GenerateContent: %w", err)
	}

	text := extractTextFromResponse(resp)
	if text == "" {
		return nil, fmt.Errorf("summarize: empty response from Gemini")
	}

	var result summaryResponse
	if err := parseJSON(text, &result); err != nil {
		return nil, fmt.Errorf("summarize parseJSON: %w", err)
	}

	return &result, nil
}

// buildContentParts constructs the genai.Part slice for a request.
// For images and PDFs we send the raw bytes as inline data.
// For text files we send the content as plain text.
func (c *client) buildContentParts(input internali.ProcessInput, prompt string) ([]genai.Part, error) {
	var parts []genai.Part

	switch strings.ToLower(input.FileType) {
	case "txt":
		// Text files are sent as plain text — no inline data needed
		parts = append(parts,
			genai.Text(fmt.Sprintf("Document filename: %s\n\nDocument content:\n%s",
				input.Filename, string(input.FileData))),
			genai.Text(prompt),
		)

	case "pdf", "png", "jpg", "jpeg":
		// Binary files sent as inline data with correct MIME type
		parts = append(parts,
			genai.ImageData(input.MIMEType, input.FileData),
			genai.Text(fmt.Sprintf("Document filename: %s", input.Filename)),
			genai.Text(prompt),
		)

	default:
		return nil, fmt.Errorf("buildContentParts: unsupported file type '%s'", input.FileType)
	}

	return parts, nil
}

// withRetry executes a classification function with exponential backoff.
// Delays: attempt 1 = 1s, attempt 2 = 2s, attempt 3 = 4s
func (c *client) withRetry(
	ctx context.Context,
	fn func() (*classificationResponse, error),
) (*classificationResponse, error) {
	var lastErr error

	for attempt := 1; attempt <= c.cfg.MaxRetries; attempt++ {
		result, err := fn()
		if err == nil {
			return result, nil
		}

		lastErr = err
		c.log.Warn().
			Err(err).
			Int("attempt", attempt).
			Int("max", c.cfg.MaxRetries).
			Msg("Gemini request failed, retrying")

		if attempt < c.cfg.MaxRetries {
			delay := time.Duration(1<<uint(attempt-1)) * time.Second // 1s, 2s, 4s
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}
	}

	return nil, lastErr
}

// withRetryExtraction is the same retry logic for extraction responses.
func (c *client) withRetryExtraction(
	ctx context.Context,
	fn func() (*extractionResponse, error),
) (*extractionResponse, error) {
	var lastErr error

	for attempt := 1; attempt <= c.cfg.MaxRetries; attempt++ {
		result, err := fn()
		if err == nil {
			return result, nil
		}

		lastErr = err
		c.log.Warn().
			Err(err).
			Int("attempt", attempt).
			Int("max", c.cfg.MaxRetries).
			Msg("Gemini extraction failed, retrying")

		if attempt < c.cfg.MaxRetries {
			delay := time.Duration(1<<uint(attempt-1)) * time.Second
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}
	}

	return nil, lastErr
}

// withRetrySummary is the same retry logic for summary responses.
func (c *client) withRetrySummary(
	ctx context.Context,
	fn func() (*summaryResponse, error),
) (*summaryResponse, error) {
	var lastErr error

	for attempt := 1; attempt <= c.cfg.MaxRetries; attempt++ {
		result, err := fn()
		if err == nil {
			return result, nil
		}

		lastErr = err
		c.log.Warn().
			Err(err).
			Int("attempt", attempt).
			Int("max", c.cfg.MaxRetries).
			Msg("Gemini summary failed, retrying")

		if attempt < c.cfg.MaxRetries {
			delay := time.Duration(1<<uint(attempt-1)) * time.Second
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}
	}

	return nil, lastErr
}

// extractTextFromResponse pulls the text content from a Gemini response.
func extractTextFromResponse(resp *genai.GenerateContentResponse) string {
	if resp == nil || len(resp.Candidates) == 0 {
		return ""
	}

	candidate := resp.Candidates[0]
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, part := range candidate.Content.Parts {
		if text, ok := part.(genai.Text); ok {
			sb.WriteString(string(text))
		}
	}

	return sb.String()
}

// parseJSON cleans markdown fences from Gemini responses and unmarshals JSON.
// Gemini sometimes wraps JSON in ```json ... ``` despite the system prompt.
func parseJSON(raw string, target interface{}) error {
	cleaned := strings.TrimSpace(raw)

	// Strip markdown code fences if present
	if strings.HasPrefix(cleaned, "```") {
		lines := strings.Split(cleaned, "\n")
		if len(lines) > 2 {
			// Remove first line (```json or ```) and last line (```)
			cleaned = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}

	cleaned = strings.TrimSpace(cleaned)

	if err := json.Unmarshal([]byte(cleaned), target); err != nil {
		return fmt.Errorf("parseJSON unmarshal: %w\nraw input: %s", err, raw)
	}

	return nil
}

// isValidDocumentType checks if the returned type is one of the known types.
func isValidDocumentType(t string) bool {
	valid := map[string]bool{
		"invoice":   true,
		"contract":  true,
		"identity":  true,
		"financial": true,
		"receipt":   true,
		"other":     true,
	}
	return valid[t]
}

// Close releases the Gemini client resources.
func (c *client) Close() error {
	return c.genaiClient.Close()
}
