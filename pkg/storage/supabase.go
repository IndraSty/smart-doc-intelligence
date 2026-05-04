package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/IndraSty/smart-doc-intelligence/config"
	"github.com/IndraSty/smart-doc-intelligence/pkg/logger"
)

// Client wraps Supabase Storage REST API.
// Files are stored under UUID-based paths — never original filenames.
// All download access goes through presigned URLs with 15-minute expiry.
type Client struct {
	cfg        *config.SupabaseConfig
	httpClient *http.Client
	log        *logger.Logger
	baseURL    string // e.g. https://xyz.supabase.co/storage/v1
}

// UploadResult holds the storage path after a successful upload.
type UploadResult struct {
	StoragePath string // UUID-based path inside the bucket
	PublicURL   string // direct URL (not used for download — presigned only)
}

// NewClient creates a new Supabase Storage client.
func NewClient(cfg *config.SupabaseConfig, log *logger.Logger) *Client {
	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 60 * time.Second, // generous timeout for large file uploads
		},
		log:     log,
		baseURL: strings.TrimRight(cfg.URL, "/") + "/storage/v1",
	}
}

// Upload stores a file in Supabase Storage under a UUID-based path.
// The storagePath parameter must be a UUID-based path, never the original filename.
// Example storagePath: "user-uuid/document-uuid.pdf"
func (c *Client) Upload(ctx context.Context, storagePath string, fileData []byte, contentType string) (*UploadResult, error) {
	uploadURL := fmt.Sprintf("%s/object/%s/%s",
		c.baseURL,
		c.cfg.Bucket,
		storagePath,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, bytes.NewReader(fileData))
	if err != nil {
		return nil, fmt.Errorf("storage.Upload create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.cfg.ServiceKey)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("x-upsert", "false") // reject duplicate uploads

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("storage.Upload http do: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("storage.Upload failed with status %d: %s",
			resp.StatusCode, string(body))
	}

	c.log.Debug().
		Str("path", storagePath).
		Str("bucket", c.cfg.Bucket).
		Int("size_bytes", len(fileData)).
		Msg("File uploaded to Supabase Storage")

	return &UploadResult{
		StoragePath: storagePath,
		PublicURL: fmt.Sprintf("%s/object/public/%s/%s",
			c.baseURL, c.cfg.Bucket, storagePath),
	}, nil
}

// GeneratePresignedURL creates a time-limited signed URL for downloading a file.
// The URL expires after the configured duration (default 15 minutes).
// Files are never served directly through our API server.
func (c *Client) GeneratePresignedURL(ctx context.Context, storagePath string, expiresIn time.Duration) (string, error) {
	signURL := fmt.Sprintf("%s/object/sign/%s/%s",
		c.baseURL,
		c.cfg.Bucket,
		storagePath,
	)

	expirySeconds := int(expiresIn.Seconds())

	payload, err := json.Marshal(map[string]int{"expiresIn": expirySeconds})
	if err != nil {
		return "", fmt.Errorf("storage.GeneratePresignedURL marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, signURL, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("storage.GeneratePresignedURL create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.cfg.ServiceKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("storage.GeneratePresignedURL http do: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("storage.GeneratePresignedURL failed with status %d: %s",
			resp.StatusCode, string(body))
	}

	var result struct {
		SignedURL string `json:"signedURL"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("storage.GeneratePresignedURL unmarshal: %w", err)
	}

	if result.SignedURL == "" {
		return "", fmt.Errorf("storage.GeneratePresignedURL: empty signed URL in response")
	}

	// Build the full download URL
	fullURL := strings.TrimRight(c.cfg.URL, "/") + "/storage/v1/object/sign/" +
		c.cfg.Bucket + "/" + storagePath + "?token=" +
		extractTokenFromSignedURL(result.SignedURL)

	c.log.Debug().
		Str("path", storagePath).
		Int("expires_seconds", expirySeconds).
		Msg("Presigned URL generated")

	return fullURL, nil
}

// Delete removes a file from Supabase Storage.
// Called when a document is deleted by the user.
func (c *Client) Delete(ctx context.Context, storagePath string) error {
	deleteURL := fmt.Sprintf("%s/object/%s/%s",
		c.baseURL,
		c.cfg.Bucket,
		storagePath,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, deleteURL, nil)
	if err != nil {
		return fmt.Errorf("storage.Delete create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.cfg.ServiceKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("storage.Delete http do: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// 200 or 404 are both acceptable — 404 means already deleted
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("storage.Delete failed with status %d: %s",
			resp.StatusCode, string(body))
	}

	c.log.Debug().
		Str("path", storagePath).
		Msg("File deleted from Supabase Storage")

	return nil
}

// HealthCheck verifies the Supabase Storage bucket is accessible.
func (c *Client) HealthCheck(ctx context.Context) error {
	checkURL := fmt.Sprintf("%s/bucket/%s", c.baseURL, c.cfg.Bucket)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, checkURL, nil)
	if err != nil {
		return fmt.Errorf("storage.HealthCheck create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.cfg.ServiceKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("storage.HealthCheck http do: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("storage.HealthCheck: bucket not accessible, status %d",
			resp.StatusCode)
	}

	return nil
}

// BuildStoragePath constructs a UUID-based storage path for a new document.
// Format: {userID}/{documentID}.{ext}
// Original filename is never used as the storage path.
func BuildStoragePath(userID, documentID, fileExt string) string {
	ext := strings.ToLower(strings.TrimPrefix(fileExt, "."))
	return path.Join(userID, documentID+"."+ext)
}

// ContentTypeFromExtension maps file extensions to MIME types.
func ContentTypeFromExtension(ext string) string {
	switch strings.ToLower(strings.TrimPrefix(ext, ".")) {
	case "pdf":
		return "application/pdf"
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "txt":
		return "text/plain"
	default:
		return "application/octet-stream"
	}
}

// extractTokenFromSignedURL pulls the token query param from a Supabase signed URL.
func extractTokenFromSignedURL(signedURL string) string {
	parsed, err := url.Parse(signedURL)
	if err != nil {
		return signedURL
	}
	token := parsed.Query().Get("token")
	if token == "" {
		// Some Supabase versions return the token as the path suffix
		return path.Base(parsed.Path)
	}
	return token
}
