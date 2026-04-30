package middleware

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/IndraSty/smart-doc-intelligence/config"
	"github.com/IndraSty/smart-doc-intelligence/pkg/logger"
)

// ValidateUpload returns a middleware that enforces file upload constraints
// before the file bytes ever reach the handler or storage layer.
// This is the first line of defense against oversized or invalid uploads.
func ValidateUpload(cfg *config.UploadConfig, log *logger.Logger) echo.MiddlewareFunc {
	maxBytes := cfg.MaxFileSizeMB * 1024 * 1024

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			r := c.Request()

			// Only validate multipart POST requests
			if r.Method != http.MethodPost {
				return next(c)
			}

			// Reject Content-Length header that exceeds the limit before reading body.
			// This prevents large uploads from consuming server memory.
			if r.ContentLength > maxBytes {
				log.Debug().
					Int64("content_length", r.ContentLength).
					Int64("max_bytes", maxBytes).
					Msg("Upload rejected: Content-Length exceeds limit")

				return echo.NewHTTPError(
					http.StatusRequestEntityTooLarge,
					"file size exceeds the 10MB limit",
				)
			}

			// Wrap the request body with a size-limited reader.
			// This catches cases where Content-Length is not set or is lying.
			r.Body = http.MaxBytesReader(c.Response().Writer, r.Body, maxBytes)

			// Parse multipart form with the same size limit
			if err := r.ParseMultipartForm(maxBytes); err != nil {
				if err.Error() == "http: request body too large" {
					return echo.NewHTTPError(
						http.StatusRequestEntityTooLarge,
						"file size exceeds the 10MB limit",
					)
				}
				// Not a multipart form — let the handler deal with it
				return next(c)
			}

			// Validate file field is present
			file, header, err := r.FormFile("file")
			if err != nil {
				if err == http.ErrMissingFile {
					return echo.NewHTTPError(
						http.StatusBadRequest,
						"request must include a 'file' field",
					)
				}
				return next(c) // let handler handle other errors
			}
			defer file.Close()

			// Validate file size from the header
			if header.Size > maxBytes {
				return echo.NewHTTPError(
					http.StatusRequestEntityTooLarge,
					"file size exceeds the 10MB limit",
				)
			}

			// Validate file extension against allowlist
			if !isAllowedExtension(header.Filename, cfg.AllowedFileTypes) {
				log.Debug().
					Str("filename", header.Filename).
					Strs("allowed", cfg.AllowedFileTypes).
					Msg("Upload rejected: file extension not allowed")

				return echo.NewHTTPError(
					http.StatusUnsupportedMediaType,
					"file type not allowed — accepted types: pdf, png, jpg, jpeg, txt",
				)
			}

			return next(c)
		}
	}
}

// isAllowedExtension checks if the filename has an allowed extension.
// This is a pre-check only — the handler does the authoritative magic byte check.
func isAllowedExtension(filename string, allowed []string) bool {
	if filename == "" {
		return false
	}

	dotIdx := -1
	for i := len(filename) - 1; i >= 0; i-- {
		if filename[i] == '.' {
			dotIdx = i
			break
		}
	}

	if dotIdx == -1 || dotIdx == len(filename)-1 {
		return false
	}

	ext := toLower(filename[dotIdx+1:])
	for _, a := range allowed {
		if toLower(a) == ext {
			return true
		}
	}

	return false
}

// toLower converts a string to lowercase without importing strings package.
func toLower(s string) string {
	b := make([]byte, len(s))
	for i := range s {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}
