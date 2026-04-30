package usecase

import (
	"testing"
)

// ── Magic byte file type detection tests ──────────────────────────────────────

func TestDetectFileType_ValidPDF(t *testing.T) {
	// Real PDF magic bytes: %PDF
	data := []byte{0x25, 0x50, 0x44, 0x46, 0x2D, 0x31, 0x2E, 0x34}

	fileType, err := detectFileType(data, "document.pdf")
	if err != nil {
		t.Fatalf("expected no error for valid PDF, got: %v", err)
	}

	if fileType != "pdf" {
		t.Errorf("expected file type 'pdf', got '%s'", fileType)
	}
}

func TestDetectFileType_ValidPNG(t *testing.T) {
	// Real PNG magic bytes: \x89PNG
	data := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

	fileType, err := detectFileType(data, "image.png")
	if err != nil {
		t.Fatalf("expected no error for valid PNG, got: %v", err)
	}

	if fileType != "png" {
		t.Errorf("expected file type 'png', got '%s'", fileType)
	}
}

func TestDetectFileType_ValidJPEG(t *testing.T) {
	// Real JPEG magic bytes: \xFF\xD8\xFF
	data := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46}

	fileType, err := detectFileType(data, "photo.jpg")
	if err != nil {
		t.Fatalf("expected no error for valid JPEG, got: %v", err)
	}

	if fileType != "jpg" {
		t.Errorf("expected file type 'jpg', got '%s'", fileType)
	}
}

func TestDetectFileType_ValidJPEGWithJpegExtension(t *testing.T) {
	data := []byte{0xFF, 0xD8, 0xFF, 0xE1, 0x00, 0x10, 0x45, 0x78}

	fileType, err := detectFileType(data, "photo.jpeg")
	if err != nil {
		t.Fatalf("expected no error for valid JPEG, got: %v", err)
	}

	// Both jpg and jpeg magic bytes are identical — returns "jpg" (first match)
	if fileType != "jpg" && fileType != "jpeg" {
		t.Errorf("expected file type 'jpg' or 'jpeg', got '%s'", fileType)
	}
}

func TestDetectFileType_ValidTXT(t *testing.T) {
	// Plain UTF-8 text — no magic bytes, detected by content analysis
	data := []byte("This is a plain text document with UTF-8 content.")

	fileType, err := detectFileType(data, "notes.txt")
	if err != nil {
		t.Fatalf("expected no error for valid TXT, got: %v", err)
	}

	if fileType != "txt" {
		t.Errorf("expected file type 'txt', got '%s'", fileType)
	}
}

func TestDetectFileType_PDFMasqueradingAsTXT(t *testing.T) {
	// Attacker renames a PDF to .txt — magic bytes must catch this
	// The PDF magic bytes should be detected regardless of filename
	data := []byte{0x25, 0x50, 0x44, 0x46, 0x2D, 0x31, 0x2E, 0x37}

	fileType, err := detectFileType(data, "notavirus.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be detected as PDF, not TXT, because magic bytes take precedence
	if fileType != "pdf" {
		t.Errorf("expected 'pdf' (magic bytes take precedence), got '%s'", fileType)
	}
}

func TestDetectFileType_UnknownBinaryRejected(t *testing.T) {
	// Random binary data with no recognized magic bytes
	data := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07}

	_, err := detectFileType(data, "malware.exe")
	if err == nil {
		t.Error("expected error for unknown binary, got nil")
	}
}

func TestDetectFileType_EmptyFileRejected(t *testing.T) {
	data := []byte{}

	_, err := detectFileType(data, "empty.pdf")
	if err == nil {
		t.Error("expected error for empty file, got nil")
	}
}

func TestDetectFileType_TinyFileRejected(t *testing.T) {
	// File with fewer than 4 bytes cannot have valid magic bytes
	data := []byte{0x25, 0x50}

	_, err := detectFileType(data, "tiny.pdf")
	if err == nil {
		t.Error("expected error for file smaller than 4 bytes, got nil")
	}
}

func TestDetectFileType_TXTWithNullByteRejected(t *testing.T) {
	// TXT file containing a null byte is treated as binary — rejected
	data := append([]byte("legitimate text content"), 0x00)
	data = append(data, []byte(" more text")...)

	_, err := detectFileType(data, "suspicious.txt")
	if err == nil {
		t.Error("expected error for TXT with null byte (binary content), got nil")
	}
}

// ── Webhook URL validation tests ──────────────────────────────────────────────

func TestValidateWebhookURL_ValidHTTPS(t *testing.T) {
	urls := []string{
		"https://example.com/webhook",
		"https://api.myapp.com/callbacks/document",
		"https://hooks.zapier.com/hooks/catch/123/abc",
	}

	for _, u := range urls {
		if err := validateWebhookURL(u); err != nil {
			t.Errorf("expected valid URL '%s' to pass, got error: %v", u, err)
		}
	}
}

func TestValidateWebhookURL_HTTPRejected(t *testing.T) {
	// Plain HTTP is not allowed — webhooks must use HTTPS
	err := validateWebhookURL("http://example.com/webhook")
	if err == nil {
		t.Error("expected error for HTTP URL, got nil")
	}
}

func TestValidateWebhookURL_EmptyURLRejected(t *testing.T) {
	err := validateWebhookURL("")
	if err == nil {
		t.Error("expected error for empty URL, got nil")
	}
}

func TestValidateWebhookURL_URLWithSpaceRejected(t *testing.T) {
	err := validateWebhookURL("https://example.com/web hook")
	if err == nil {
		t.Error("expected error for URL with space, got nil")
	}
}

// ── Filename sanitization tests ───────────────────────────────────────────────

func TestSanitizeFilename_RemovesPathTraversal(t *testing.T) {
	cases := []struct {
		input    string
		expected string // should NOT contain path separators or ..
	}{
		{"../../../etc/passwd", "______etc_passwd"},
		{"folder/subfolder/file.pdf", "folder_subfolder_file.pdf"},
		{"C:\\Windows\\System32\\file.txt", "C:_Windows_System32_file.txt"},
		{"normal_file.pdf", "normal_file.pdf"},
	}

	for _, tc := range cases {
		result := sanitizeFilename(tc.input)

		// Must not contain path separators
		for _, c := range result {
			if c == '/' || c == '\\' {
				t.Errorf("sanitized filename '%s' still contains path separator", result)
			}
		}

		// Must not contain null bytes
		for _, b := range []byte(result) {
			if b == 0x00 {
				t.Errorf("sanitized filename '%s' contains null byte", result)
			}
		}
	}
}

func TestSanitizeFilename_TruncatesLongFilename(t *testing.T) {
	// Generate a 300-character filename
	long := make([]byte, 300)
	for i := range long {
		long[i] = 'a'
	}

	result := sanitizeFilename(string(long))

	if len(result) > 255 {
		t.Errorf("expected filename truncated to 255 chars, got %d", len(result))
	}
}

func TestSanitizeFilename_NormalFilenameUnchanged(t *testing.T) {
	input := "invoice_2024_Q3.pdf"
	result := sanitizeFilename(input)

	if result != input {
		t.Errorf("expected unchanged filename '%s', got '%s'", input, result)
	}
}

// ── isValidUTF8Text tests ─────────────────────────────────────────────────────

func TestIsValidUTF8Text_PlainTextPasses(t *testing.T) {
	data := []byte("Hello, this is a valid UTF-8 text document.")
	if !isValidUTF8Text(data) {
		t.Error("expected plain text to be valid UTF-8")
	}
}

func TestIsValidUTF8Text_NullByteFailes(t *testing.T) {
	data := []byte("text with \x00 null byte")
	if isValidUTF8Text(data) {
		t.Error("expected text with null byte to fail UTF-8 validation")
	}
}

func TestIsValidUTF8Text_BinaryDataFails(t *testing.T) {
	// Simulate a binary file header
	data := []byte{0x89, 0x50, 0x4E, 0x47, 0x00, 0x00, 0x00}
	if isValidUTF8Text(data) {
		t.Error("expected binary data with null byte to fail")
	}
}
