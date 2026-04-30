package domain

import "fmt"

// Sentinel errors — use these for type-checking with errors.Is()
var (
	ErrNotFound          = fmt.Errorf("resource not found")
	ErrUnauthorized      = fmt.Errorf("unauthorized")
	ErrForbidden         = fmt.Errorf("access forbidden")
	ErrConflict          = fmt.Errorf("resource already exists")
	ErrInvalidInput      = fmt.Errorf("invalid input")
	ErrFileTooLarge      = fmt.Errorf("file size exceeds the allowed limit")
	ErrInvalidFileType   = fmt.Errorf("file type is not allowed")
	ErrStorageFailed     = fmt.Errorf("storage operation failed")
	ErrQueueFailed       = fmt.Errorf("queue operation failed")
	ErrAIFailed          = fmt.Errorf("AI processing failed")
	ErrProcessingTimeout = fmt.Errorf("document processing timed out")
	ErrWebhookInvalid    = fmt.Errorf("webhook URL must be a valid HTTPS URL")
)

// ValidationError holds field-level validation failures.
// Used when multiple fields fail validation simultaneously.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation failed on field '%s': %s", e.Field, e.Message)
}

// NotFoundError wraps ErrNotFound with a resource-specific message.
type NotFoundError struct {
	Resource string
	ID       string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s with id '%s' not found", e.Resource, e.ID)
}

func (e *NotFoundError) Unwrap() error {
	return ErrNotFound
}

// ForbiddenError wraps ErrForbidden when a user tries to access
// a resource that belongs to another user (multi-tenant violation).
type ForbiddenError struct {
	UserID     string
	ResourceID string
}

func (e *ForbiddenError) Error() string {
	return fmt.Sprintf("user '%s' does not have access to resource '%s'", e.UserID, e.ResourceID)
}

func (e *ForbiddenError) Unwrap() error {
	return ErrForbidden
}

// AIError wraps ErrAIFailed with attempt count and underlying cause.
type AIError struct {
	Attempts int
	Cause    error
}

func (e *AIError) Error() string {
	return fmt.Sprintf("AI processing failed after %d attempts: %v", e.Attempts, e.Cause)
}

func (e *AIError) Unwrap() error {
	return ErrAIFailed
}
