package usecase

import (
	"context"
	"fmt"
	"sort"

	"github.com/IndraSty/smart-doc-intelligence/internal/domain"
	"github.com/IndraSty/smart-doc-intelligence/pkg/embedding"
	"github.com/IndraSty/smart-doc-intelligence/pkg/logger"
)

const (
	// defaultSearchLimit is the maximum number of results returned per search
	defaultSearchLimit = 20

	// rrfK is the RRF constant — 60 is the standard value from the original paper
	// Higher values reduce the impact of rank differences
	rrfK = 60
)

type searchUsecase struct {
	extractionRepo domain.ExtractionRepository
	docRepo        domain.DocumentRepository
	embedder       *embedding.Generator
	log            *logger.Logger
}

// NewSearchUsecase creates a new SearchUsecase implementation.
func NewSearchUsecase(
	extractionRepo domain.ExtractionRepository,
	docRepo domain.DocumentRepository,
	embedder *embedding.Generator,
	log *logger.Logger,
) domain.SearchUsecase {
	return &searchUsecase{
		extractionRepo: extractionRepo,
		docRepo:        docRepo,
		embedder:       embedder,
		log:            log,
	}
}

// Search routes to the correct search strategy based on input.SearchType.
func (u *searchUsecase) Search(ctx context.Context, input domain.SearchInput) (*domain.SearchOutput, error) {
	if input.Query == "" {
		return nil, &domain.ValidationError{Field: "q", Message: "search query cannot be empty"}
	}

	limit := input.Limit
	if limit <= 0 || limit > defaultSearchLimit {
		limit = defaultSearchLimit
	}

	var results []domain.SearchResult
	var err error

	switch input.SearchType {
	case "semantic":
		results, err = u.searchSemantic(ctx, input.UserID, input.Query, limit)
	case "fulltext":
		results, err = u.searchFullText(ctx, input.UserID, input.Query, limit)
	case "hybrid":
		results, err = u.searchHybrid(ctx, input.UserID, input.Query, limit)
	default:
		// Default to hybrid for best results
		results, err = u.searchHybrid(ctx, input.UserID, input.Query, limit)
	}

	if err != nil {
		return nil, fmt.Errorf("searchUsecase.Search: %w", err)
	}

	// Enrich results with document metadata
	enriched, err := u.enrichResults(ctx, results)
	if err != nil {
		return nil, fmt.Errorf("searchUsecase.Search enrich: %w", err)
	}

	return &domain.SearchOutput{
		Results:    enriched,
		TotalCount: len(enriched),
		SearchType: input.SearchType,
	}, nil
}

// searchSemantic performs vector similarity search using pgvector.
// The query is embedded using RETRIEVAL_QUERY task type for accuracy.
func (u *searchUsecase) searchSemantic(
	ctx context.Context,
	userID string,
	query string,
	limit int,
) ([]domain.SearchResult, error) {
	// Embed the search query — use RETRIEVAL_QUERY for better asymmetric search
	queryVector, err := u.embedder.GenerateForQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("searchSemantic embed query: %w", err)
	}

	results, err := u.extractionRepo.SearchSemantic(ctx, userID, queryVector, limit)
	if err != nil {
		return nil, fmt.Errorf("searchSemantic query: %w", err)
	}

	u.log.Debug().
		Str("user_id", userID).
		Int("results", len(results)).
		Msg("Semantic search completed")

	return results, nil
}

// searchFullText performs PostgreSQL tsvector full-text search.
func (u *searchUsecase) searchFullText(
	ctx context.Context,
	userID string,
	query string,
	limit int,
) ([]domain.SearchResult, error) {
	results, err := u.extractionRepo.SearchFullText(ctx, userID, query, limit)
	if err != nil {
		return nil, fmt.Errorf("searchFullText query: %w", err)
	}

	u.log.Debug().
		Str("user_id", userID).
		Int("results", len(results)).
		Msg("Full-text search completed")

	return results, nil
}

// searchHybrid combines semantic and full-text results using
// Reciprocal Rank Fusion (RRF). RRF is a rank-based fusion algorithm
// that is robust to score scale differences between search methods.
//
// RRF formula: score(d) = Σ 1 / (k + rank(d))
// where k=60 is the standard smoothing constant.
//
// Documents that rank highly in BOTH lists get the highest combined score.
func (u *searchUsecase) searchHybrid(
	ctx context.Context,
	userID string,
	query string,
	limit int,
) ([]domain.SearchResult, error) {
	// Run both searches in parallel using goroutines
	type searchRes struct {
		results []domain.SearchResult
		err     error
	}

	semanticCh := make(chan searchRes, 1)
	fulltextCh := make(chan searchRes, 1)

	go func() {
		res, err := u.searchSemantic(ctx, userID, query, limit)
		semanticCh <- searchRes{res, err}
	}()

	go func() {
		res, err := u.searchFullText(ctx, userID, query, limit)
		fulltextCh <- searchRes{res, err}
	}()

	semanticResult := <-semanticCh
	fulltextResult := <-fulltextCh

	// If both fail, return error
	if semanticResult.err != nil && fulltextResult.err != nil {
		return nil, fmt.Errorf("hybrid search both failed: semantic=%v, fulltext=%v",
			semanticResult.err, fulltextResult.err)
	}

	// If only one fails, log a warning and continue with the other
	if semanticResult.err != nil {
		u.log.Warn().Err(semanticResult.err).Msg("Semantic search failed in hybrid mode, using fulltext only")
		return fulltextResult.results, nil
	}

	if fulltextResult.err != nil {
		u.log.Warn().Err(fulltextResult.err).Msg("Full-text search failed in hybrid mode, using semantic only")
		return semanticResult.results, nil
	}

	// Apply Reciprocal Rank Fusion
	fused := applyRRF(semanticResult.results, fulltextResult.results, limit)

	u.log.Debug().
		Str("user_id", userID).
		Int("semantic_hits", len(semanticResult.results)).
		Int("fulltext_hits", len(fulltextResult.results)).
		Int("fused_results", len(fused)).
		Msg("Hybrid search completed")

	return fused, nil
}

// applyRRF implements Reciprocal Rank Fusion to merge two ranked lists.
// Each document's RRF score is the sum of 1/(k+rank) across all lists it appears in.
// Documents appearing in both lists score higher than those in only one.
func applyRRF(
	semanticResults []domain.SearchResult,
	fulltextResults []domain.SearchResult,
	limit int,
) []domain.SearchResult {
	// Map of documentID → accumulated RRF score
	rrfScores := make(map[string]float64)

	// Accumulate scores from semantic results
	for _, r := range semanticResults {
		rrfScores[r.DocumentID] += 1.0 / float64(rrfK+r.Rank)
	}

	// Accumulate scores from full-text results
	for _, r := range fulltextResults {
		rrfScores[r.DocumentID] += 1.0 / float64(rrfK+r.Rank)
	}

	// Convert map to slice for sorting
	fused := make([]domain.SearchResult, 0, len(rrfScores))
	for docID, score := range rrfScores {
		fused = append(fused, domain.SearchResult{
			DocumentID: docID,
			Score:      score,
		})
	}

	// Sort by RRF score descending
	sort.Slice(fused, func(i, j int) bool {
		return fused[i].Score > fused[j].Score
	})

	// Apply limit
	if len(fused) > limit {
		fused = fused[:limit]
	}

	// Assign final ranks
	for i := range fused {
		fused[i].Rank = i + 1
	}

	return fused
}

// enrichResults fetches document metadata for each search result.
// Results whose document no longer exists are silently dropped.
func (u *searchUsecase) enrichResults(
	ctx context.Context,
	results []domain.SearchResult,
) ([]domain.SearchResultDetail, error) {
	enriched := make([]domain.SearchResultDetail, 0, len(results))

	for _, r := range results {
		doc, err := u.docRepo.FindByID(ctx, r.DocumentID)
		if err != nil {
			// Document may have been deleted after search index was built
			u.log.Warn().
				Str("document_id", r.DocumentID).
				Msg("Search result references deleted document, skipping")
			continue
		}

		enriched = append(enriched, domain.SearchResultDetail{
			Document: doc,
			Score:    r.Score,
			Rank:     r.Rank,
		})
	}

	return enriched, nil
}
