package mocks

import (
	"context"

	"github.com/IndraSty/smart-doc-intelligence/internal/domain"
)

// ── UserRepository mock ───────────────────────────────────────────────────────

type MockUserRepository struct {
	CreateFn       func(ctx context.Context, user *domain.User) (*domain.User, error)
	FindByEmailFn  func(ctx context.Context, email string) (*domain.User, error)
	FindByIDFn     func(ctx context.Context, id string) (*domain.User, error)
	FindByAPIKeyFn func(ctx context.Context, hashedKey string) (*domain.User, error)
}

func (m *MockUserRepository) Create(ctx context.Context, user *domain.User) (*domain.User, error) {
	return m.CreateFn(ctx, user)
}
func (m *MockUserRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	return m.FindByEmailFn(ctx, email)
}
func (m *MockUserRepository) FindByID(ctx context.Context, id string) (*domain.User, error) {
	return m.FindByIDFn(ctx, id)
}
func (m *MockUserRepository) FindByAPIKey(ctx context.Context, hashedKey string) (*domain.User, error) {
	return m.FindByAPIKeyFn(ctx, hashedKey)
}

// ── DocumentRepository mock ───────────────────────────────────────────────────

type MockDocumentRepository struct {
	CreateFn                func(ctx context.Context, doc *domain.Document) (*domain.Document, error)
	FindByIDFn              func(ctx context.Context, id string) (*domain.Document, error)
	FindByIDAndUserIDFn     func(ctx context.Context, id, userID string) (*domain.Document, error)
	ListByUserIDFn          func(ctx context.Context, userID string, filter domain.ListDocumentsFilter) ([]*domain.Document, int, error)
	UpdateStatusFn          func(ctx context.Context, id string, status domain.DocumentStatus) error
	UpdateAfterProcessingFn func(ctx context.Context, id string, docType domain.DocumentType, confidence float64, summary string) error
	UpdateErrorFn           func(ctx context.Context, id string, errMsg string) error
	DeleteFn                func(ctx context.Context, id, userID string) error
}

func (m *MockDocumentRepository) Create(ctx context.Context, doc *domain.Document) (*domain.Document, error) {
	return m.CreateFn(ctx, doc)
}
func (m *MockDocumentRepository) FindByID(ctx context.Context, id string) (*domain.Document, error) {
	return m.FindByIDFn(ctx, id)
}
func (m *MockDocumentRepository) FindByIDAndUserID(ctx context.Context, id, userID string) (*domain.Document, error) {
	return m.FindByIDAndUserIDFn(ctx, id, userID)
}
func (m *MockDocumentRepository) ListByUserID(ctx context.Context, userID string, filter domain.ListDocumentsFilter) ([]*domain.Document, int, error) {
	return m.ListByUserIDFn(ctx, userID, filter)
}
func (m *MockDocumentRepository) UpdateStatus(ctx context.Context, id string, status domain.DocumentStatus) error {
	return m.UpdateStatusFn(ctx, id, status)
}
func (m *MockDocumentRepository) UpdateAfterProcessing(ctx context.Context, id string, docType domain.DocumentType, confidence float64, summary string) error {
	return m.UpdateAfterProcessingFn(ctx, id, docType, confidence, summary)
}
func (m *MockDocumentRepository) UpdateError(ctx context.Context, id string, errMsg string) error {
	return m.UpdateErrorFn(ctx, id, errMsg)
}
func (m *MockDocumentRepository) Delete(ctx context.Context, id, userID string) error {
	return m.DeleteFn(ctx, id, userID)
}

// ── ExtractionRepository mock ─────────────────────────────────────────────────

type MockExtractionRepository struct {
	CreateFn           func(ctx context.Context, result *domain.ExtractionResult) (*domain.ExtractionResult, error)
	FindByDocumentIDFn func(ctx context.Context, documentID string) (*domain.ExtractionResult, error)
	SearchSemanticFn   func(ctx context.Context, userID string, embedding []float32, limit int) ([]domain.SearchResult, error)
	SearchFullTextFn   func(ctx context.Context, userID string, query string, limit int) ([]domain.SearchResult, error)
}

func (m *MockExtractionRepository) Create(ctx context.Context, result *domain.ExtractionResult) (*domain.ExtractionResult, error) {
	return m.CreateFn(ctx, result)
}
func (m *MockExtractionRepository) FindByDocumentID(ctx context.Context, documentID string) (*domain.ExtractionResult, error) {
	return m.FindByDocumentIDFn(ctx, documentID)
}
func (m *MockExtractionRepository) SearchSemantic(ctx context.Context, userID string, embedding []float32, limit int) ([]domain.SearchResult, error) {
	return m.SearchSemanticFn(ctx, userID, embedding, limit)
}
func (m *MockExtractionRepository) SearchFullText(ctx context.Context, userID string, query string, limit int) ([]domain.SearchResult, error) {
	return m.SearchFullTextFn(ctx, userID, query, limit)
}

// ── JobRepository mock ────────────────────────────────────────────────────────

type MockJobRepository struct {
	SetStatusFn             func(ctx context.Context, job *domain.ProcessingJob) error
	GetStatusFn             func(ctx context.Context, jobID string) (*domain.ProcessingJob, error)
	GetStatusByDocumentIDFn func(ctx context.Context, documentID string) (*domain.ProcessingJob, error)
	DeleteFn                func(ctx context.Context, jobID string) error
}

func (m *MockJobRepository) SetStatus(ctx context.Context, job *domain.ProcessingJob) error {
	return m.SetStatusFn(ctx, job)
}
func (m *MockJobRepository) GetStatus(ctx context.Context, jobID string) (*domain.ProcessingJob, error) {
	return m.GetStatusFn(ctx, jobID)
}
func (m *MockJobRepository) GetStatusByDocumentID(ctx context.Context, documentID string) (*domain.ProcessingJob, error) {
	return m.GetStatusByDocumentIDFn(ctx, documentID)
}
func (m *MockJobRepository) Delete(ctx context.Context, jobID string) error {
	return m.DeleteFn(ctx, jobID)
}

// ── ProcessingUsecase mock ────────────────────────────────────────────────────

type MockProcessingUsecase struct {
	EnqueueJobFn     func(ctx context.Context, documentID, userID string) (*domain.ProcessingJob, error)
	GetJobStatusFn   func(ctx context.Context, jobID, userID string) (*domain.ProcessingJob, error)
	MarkProcessingFn func(ctx context.Context, jobID string) error
	MarkCompletedFn  func(ctx context.Context, jobID string) error
	MarkFailedFn     func(ctx context.Context, jobID, errMsg string) error
}

func (m *MockProcessingUsecase) EnqueueJob(ctx context.Context, documentID, userID string) (*domain.ProcessingJob, error) {
	return m.EnqueueJobFn(ctx, documentID, userID)
}
func (m *MockProcessingUsecase) GetJobStatus(ctx context.Context, jobID, userID string) (*domain.ProcessingJob, error) {
	return m.GetJobStatusFn(ctx, jobID, userID)
}
func (m *MockProcessingUsecase) MarkProcessing(ctx context.Context, jobID string) error {
	return m.MarkProcessingFn(ctx, jobID)
}
func (m *MockProcessingUsecase) MarkCompleted(ctx context.Context, jobID string) error {
	return m.MarkCompletedFn(ctx, jobID)
}
func (m *MockProcessingUsecase) MarkFailed(ctx context.Context, jobID, errMsg string) error {
	return m.MarkFailedFn(ctx, jobID, errMsg)
}
