package main

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	// maxImageSize is the maximum permitted upload size (20 MB).
	maxImageSize = 20 << 20 // 20 MB

	// uploadsDir is where uploaded images are stored on disk.
	uploadsDir = "static/uploads"

	// uploadsRoute is the URL prefix used to serve uploaded images.
	uploadsRoute = "/static/uploads/"
)

// allowedMIMETypes maps each permitted MIME type to its canonical file extension.
var allowedMIMETypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/gif":  ".gif",
}

// EnsureUploadsDir creates the uploads directory if it does not already exist.
func EnsureUploadsDir() error {
	return os.MkdirAll(uploadsDir, 0755)
}

// ValidateImageFile inspects the multipart file for size and MIME type.
// It returns the detected MIME type string on success, or a user-facing error.
func ValidateImageFile(file multipart.File, header *multipart.FileHeader) (string, error) {
	// Primary size guard: the multipart header reports the declared content length.
	if header.Size > maxImageSize {
		return "", errors.New("Image is too large. Maximum size allowed is 20MB.")
	}

	// Read the first 512 bytes for MIME-type sniffing.
	buf := make([]byte, 512)
	n, err := file.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", errors.New("Failed to read image file.")
	}
	buf = buf[:n]

	// Seek back to the beginning so the full file can still be copied later.
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return "", errors.New("Failed to process image file.")
	}

	// Detect MIME type from the magic bytes.
	mimeType := http.DetectContentType(buf)

	// Trim any ";charset=..." suffix that DetectContentType may append.
	if idx := strings.Index(mimeType, ";"); idx != -1 {
		mimeType = strings.TrimSpace(mimeType[:idx])
	}

	if _, ok := allowedMIMETypes[mimeType]; !ok {
		return "", errors.New("Invalid image format. Only JPEG, PNG, and GIF are allowed.")
	}

	return mimeType, nil
}

// SaveImageFile writes the multipart file to uploadsDir with a UUID-based filename.
// It returns the web-accessible URL path (e.g. /static/uploads/<uuid>.jpg).
func SaveImageFile(file multipart.File, mimeType string) (string, error) {
	if err := EnsureUploadsDir(); err != nil {
		return "", fmt.Errorf("failed to create uploads directory: %w", err)
	}

	ext := allowedMIMETypes[mimeType]
	filename := newUUID() + ext
	destPath := filepath.Join(uploadsDir, filename)

	dst, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("failed to create file on disk: %w", err)
	}
	defer dst.Close()

	if _, err = io.Copy(dst, file); err != nil {
		os.Remove(destPath) // clean up partial file
		return "", fmt.Errorf("failed to write image data: %w", err)
	}

	return uploadsRoute + filename, nil
}

// newUUID generates a random RFC-4122 version-4 UUID string.
func newUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Extremely unlikely; fall back to a pid-based name so the upload still works.
		return fmt.Sprintf("fallback-%d", os.Getpid())
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant bits (RFC 4122)
	return fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16],
	)
}