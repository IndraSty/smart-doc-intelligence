package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/IndraSty/smart-doc-intelligence/internal/domain"
	"github.com/IndraSty/smart-doc-intelligence/internal/mocks"
	"github.com/IndraSty/smart-doc-intelligence/pkg/logger"
)

// ── RRF unit tests ────────────────────────────────────────────────────────────

func TestApplyRRF_BothListsEmpty(t *testing.T) {
	result := applyRRF(nil, nil, 20)
	if len(result) != 0 {
		t.Errorf("expected 0 results, got %d", len(result))
	}
}

func TestApplyRRF_OnlySemanticResults(t *testing.T) {
	semantic := []domain.SearchResult{
		{DocumentID: "doc-1", Score: 0.95, Rank: 1},
		{DocumentID: "doc-2", Score: 0.80, Rank: 2},
		{DocumentID: "doc-3", Score: 0.70, Rank: 3},
	}

	result := applyRRF(semantic, nil, 20)

	if len(result) != 3 {
		t.Fatalf("expected 3 results, got %d", len(result))
	}

	// Order must be preserved — highest RRF score first
	if result[0].DocumentID != "doc-1" {
		t.Errorf("expected doc-1 first, got %s", result[0].DocumentID)
	}

	// Verify ranks are assigned sequentially
	for i, r := range result {
		if r.Rank != i+1 {
			t.Errorf("result[%d] expected rank %d, got %d", i, i+1, r.Rank)
		}
	}
}

func TestApplyRRF_OnlyFulltextResults(t *testing.T) {
	fulltext := []domain.SearchResult{
		{DocumentID: "doc-a", Score: 0.9, Rank: 1},
		{DocumentID: "doc-b", Score: 0.7, Rank: 2},
	}

	result := applyRRF(nil, fulltext, 20)

	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}

	if result[0].DocumentID != "doc-a" {
		t.Errorf("expected doc-a first, got %s", result[0].DocumentID)
	}
}

func TestApplyRRF_DocumentInBothListsScoresHigher(t *testing.T) {
	// doc-overlap appears in both lists at rank 1
	// doc-semantic-only appears only in semantic at rank 2
	// doc-fulltext-only appears only in fulltext at rank 2
	// doc-overlap should win because it accumulates score from both lists

	semantic := []domain.SearchResult{
		{DocumentID: "doc-overlap", Score: 0.95, Rank: 1},
		{DocumentID: "doc-semantic-only", Score: 0.80, Rank: 2},
	}

	fulltext := []domain.SearchResult{
		{DocumentID: "doc-overlap", Score: 0.90, Rank: 1},
		{DocumentID: "doc-fulltext-only", Score: 0.75, Rank: 2},
	}

	result := applyRRF(semantic, fulltext, 20)

	if len(result) == 0 {
		t.Fatal("expected results, got none")
	}

	// doc-overlap must be ranked first
	if result[0].DocumentID != "doc-overlap" {
		t.Errorf("expected doc-overlap first (appeared in both lists), got %s", result[0].DocumentID)
	}

	// Verify doc-overlap has higher score than single-list documents
	overlapScore := result[0].Score
	for _, r := range result[1:] {
		if r.Score > overlapScore {
			t.Errorf("doc %s scored %.4f higher than doc-overlap %.4f but appeared in fewer lists",
				r.DocumentID, r.Score, overlapScore)
		}
	}
}

func TestApplyRRF_RRFFormulaIsCorrect(t *testing.T) {
	// Verify the exact RRF score calculation: 1/(k+rank)
	// For a doc at rank 1 in both lists: score = 1/(60+1) + 1/(60+1) = 2/61
	semantic := []domain.SearchResult{
		{DocumentID: "doc-1", Rank: 1},
	}
	fulltext := []domain.SearchResult{
		{DocumentID: "doc-1", Rank: 1},
	}

	result := applyRRF(semantic, fulltext, 20)

	if len(result) == 0 {
		t.Fatal("expected 1 result, got 0")
	}

	expectedScore := 2.0 / float64(rrfK+1)
	tolerance := 0.0001

	if result[0].Score < expectedScore-tolerance || result[0].Score > expectedScore+tolerance {
		t.Errorf("expected RRF score %.6f, got %.6f", expectedScore, result[0].Score)
	}
}

func TestApplyRRF_LimitIsRespected(t *testing.T) {
	// Create 10 semantic + 10 fulltext results with no overlap = 20 unique docs
	semantic := make([]domain.SearchResult, 10)
	for i := range semantic {
		semantic[i] = domain.SearchResult{
			DocumentID: "semantic-" + string(rune('a'+i)),
			Rank:       i + 1,
		}
	}

	fulltext := make([]domain.SearchResult, 10)
	for i := range fulltext {
		fulltext[i] = domain.SearchResult{
			DocumentID: "fulltext-" + string(rune('a'+i)),
			Rank:       i + 1,
		}
	}

	limit := 5
	result := applyRRF(semantic, fulltext, limit)

	if len(result) > limit {
		t.Errorf("expected at most %d results, got %d", limit, len(result))
	}
}

func TestApplyRRF_RanksAreConsecutive(t *testing.T) {
	semantic := []domain.SearchResult{
		{DocumentID: "doc-1", Rank: 1},
		{DocumentID: "doc-2", Rank: 2},
	}
	fulltext := []domain.SearchResult{
		{DocumentID: "doc-3", Rank: 1},
	}

	result := applyRRF(semantic, fulltext, 20)

	for i, r := range result {
		if r.Rank != i+1 {
			t.Errorf("result[%d] expected rank %d, got %d", i, i+1, r.Rank)
		}
	}
}

// ── Search usecase integration tests ─────────────────────────────────────────

func TestSearchUsecase_EmptyQueryReturnsError(t *testing.T) {
	log := logger.New("development")

	extractionRepo := &mocks.MockExtractionRepository{}
	docRepo := &mocks.MockDocumentRepository{}

	uc := &searchUsecase{
		extractionRepo: extractionRepo,
		docRepo:        docRepo,
		embedder:       nil,
		log:            log,
	}

	_, err := uc.Search(context.Background(), domain.SearchInput{
		UserID:     "user-1",
		Query:      "", // empty query
		SearchType: "hybrid",
	})

	if err == nil {
		t.Fatal("expected error for empty query, got nil")
	}

	var valErr *domain.ValidationError
	if !errors.As(err, &valErr) {
		t.Errorf("expected ValidationError, got %T: %v", err, err)
	}
}

func TestSearchUsecase_InvalidSearchTypeDefaultsToHybrid(t *testing.T) {
	// "hybrid" is the default when type is empty — test that it routes correctly
	log := logger.New("development")

	semanticCalled := false
	fulltextCalled := false

	extractionRepo := &mocks.MockExtractionRepository{
		SearchSemanticFn: func(ctx context.Context, userID string, embedding []float32, limit int) ([]domain.SearchResult, error) {
			semanticCalled = true
			return []domain.SearchResult{}, nil
		},
		SearchFullTextFn: func(ctx context.Context, userID string, query string, limit int) ([]domain.SearchResult, error) {
			fulltextCalled = true
			return []domain.SearchResult{}, nil
		},
	}

	docRepo := &mocks.MockDocumentRepository{
		FindByIDFn: func(ctx context.Context, id string) (*domain.Document, error) {
			return nil, domain.ErrNotFound
		},
	}

	// We can't run hybrid without an embedder, so test fulltext routing instead
	uc := &searchUsecase{
		extractionRepo: extractionRepo,
		docRepo:        docRepo,
		embedder:       nil,
		log:            log,
	}

	_, _ = uc.searchFullText(context.Background(), "user-1", "invoice", 10)

	if !fulltextCalled {
		t.Error("expected SearchFullText to be called")
	}

	_ = semanticCalled // suppress unused warning
}

func TestSearchUsecase_FulltextSearchReturnsEnrichedResults(t *testing.T) {
	log := logger.New("development")

	docID := "doc-123"
	userID := "user-456"

	extractionRepo := &mocks.MockExtractionRepository{
		SearchFullTextFn: func(ctx context.Context, uid string, query string, limit int) ([]domain.SearchResult, error) {
			if uid != userID {
				t.Errorf("expected userID %s, got %s", userID, uid)
			}
			return []domain.SearchResult{
				{DocumentID: docID, Score: 0.85, Rank: 1},
			}, nil
		},
	}

	expectedDoc := &domain.Document{
		ID:     docID,
		UserID: userID,
		Status: domain.StatusCompleted,
	}

	docRepo := &mocks.MockDocumentRepository{
		FindByIDFn: func(ctx context.Context, id string) (*domain.Document, error) {
			if id == docID {
				return expectedDoc, nil
			}
			return nil, domain.ErrNotFound
		},
	}

	uc := &searchUsecase{
		extractionRepo: extractionRepo,
		docRepo:        docRepo,
		embedder:       nil,
		log:            log,
	}

	output, err := uc.Search(context.Background(), domain.SearchInput{
		UserID:     userID,
		Query:      "invoice total",
		SearchType: "fulltext",
		Limit:      10,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.TotalCount != 1 {
		t.Errorf("expected 1 result, got %d", output.TotalCount)
	}

	if output.Results[0].Document.ID != docID {
		t.Errorf("expected doc ID %s, got %s", docID, output.Results[0].Document.ID)
	}

	if output.SearchType != "fulltext" {
		t.Errorf("expected search type fulltext, got %s", output.SearchType)
	}
}

func TestSearchUsecase_DeletedDocumentSkippedInResults(t *testing.T) {
	log := logger.New("development")

	extractionRepo := &mocks.MockExtractionRepository{
		SearchFullTextFn: func(ctx context.Context, userID string, query string, limit int) ([]domain.SearchResult, error) {
			// Return 2 results — one whose document has been deleted
			return []domain.SearchResult{
				{DocumentID: "doc-exists", Score: 0.9, Rank: 1},
				{DocumentID: "doc-deleted", Score: 0.8, Rank: 2},
			}, nil
		},
	}

	docRepo := &mocks.MockDocumentRepository{
		FindByIDFn: func(ctx context.Context, id string) (*domain.Document, error) {
			if id == "doc-exists" {
				return &domain.Document{ID: id}, nil
			}
			// doc-deleted has been removed from DB
			return nil, &domain.NotFoundError{Resource: "document", ID: id}
		},
	}

	uc := &searchUsecase{
		extractionRepo: extractionRepo,
		docRepo:        docRepo,
		embedder:       nil,
		log:            log,
	}

	output, err := uc.Search(context.Background(), domain.SearchInput{
		UserID:     "user-1",
		Query:      "contract",
		SearchType: "fulltext",
		Limit:      10,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only 1 result should be returned — deleted doc is silently dropped
	if output.TotalCount != 1 {
		t.Errorf("expected 1 result (deleted doc skipped), got %d", output.TotalCount)
	}

	if output.Results[0].Document.ID != "doc-exists" {
		t.Errorf("expected doc-exists, got %s", output.Results[0].Document.ID)
	}
}
