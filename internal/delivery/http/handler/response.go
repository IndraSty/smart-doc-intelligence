package handler

// errorResponse is the standard error body returned on all failed requests.
// Used by swag to generate consistent error response schemas.
type errorResponse struct {
	Error   string `json:"error"             example:"Bad Request"`
	Message string `json:"message,omitempty" example:"email and password are required"`
}

var _ = errorResponse{}
