package main

import (
	"bytes"
	"mime/multipart"
	"net/textproto"
	"strings"
	"testing"
)

// ── test helpers ──────────────────────────────────────────────────────────────

// mockFile wraps a bytes.Reader to satisfy the multipart.File interface.
type mockFile struct{ *bytes.Reader }

func (m *mockFile) Close() error { return nil }

// newMockFile builds a multipart.File + FileHeader pair for use in tests.
// content is the raw bytes; declaredSize is written into the FileHeader.Size field
// (it may differ from len(content) to simulate a client-declared size).
func newMockFile(content []byte, declaredSize int64) (multipart.File, *multipart.FileHeader) {
	header := &multipart.FileHeader{
		Filename: "test",
		Header:   make(textproto.MIMEHeader),
		Size:     declaredSize,
	}
	return &mockFile{bytes.NewReader(content)}, header
}

// magic byte sequences recognised by http.DetectContentType
var (
	jpegMagic = []byte{0xFF, 0xD8, 0xFF}
	pngMagic  = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	gifMagic  = []byte("GIF89a")
)

func makeJPEG(pad int) []byte { return append(jpegMagic, make([]byte, pad)...) }
func makePNG(pad int) []byte  { return append(pngMagic, make([]byte, pad)...) }
func makeGIF(pad int) []byte  { return append(gifMagic, make([]byte, pad)...) }

// ── ValidateImageFile ─────────────────────────────────────────────────────────

func TestValidateImageFile_ValidJPEG(t *testing.T) {
	f, h := newMockFile(makeJPEG(100), 200)
	mime, err := ValidateImageFile(f, h)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mime != "image/jpeg" {
		t.Errorf("want image/jpeg, got %s", mime)
	}
}

func TestValidateImageFile_ValidPNG(t *testing.T) {
	f, h := newMockFile(makePNG(100), 200)
	mime, err := ValidateImageFile(f, h)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mime != "image/png" {
		t.Errorf("want image/png, got %s", mime)
	}
}

func TestValidateImageFile_ValidGIF(t *testing.T) {
	f, h := newMockFile(makeGIF(100), 200)
	mime, err := ValidateImageFile(f, h)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mime != "image/gif" {
		t.Errorf("want image/gif, got %s", mime)
	}
}

func TestValidateImageFile_ExactMaxSize(t *testing.T) {
	// Exactly at the limit should pass.
	f, h := newMockFile(makeJPEG(100), maxImageSize)
	if _, err := ValidateImageFile(f, h); err != nil {
		t.Fatalf("expected no error at exact limit, got: %v", err)
	}
}

func TestValidateImageFile_TooLarge(t *testing.T) {
	f, h := newMockFile(makeJPEG(100), maxImageSize+1)
	_, err := ValidateImageFile(f, h)
	if err == nil {
		t.Fatal("expected error for oversized file, got nil")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestValidateImageFile_InvalidFormat(t *testing.T) {
	f, h := newMockFile([]byte("this is plain text, not an image"), 32)
	_, err := ValidateImageFile(f, h)
	if err == nil {
		t.Fatal("expected error for invalid format, got nil")
	}
	if !strings.Contains(err.Error(), "Invalid image format") {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestValidateImageFile_EmptyFile(t *testing.T) {
	f, h := newMockFile([]byte{}, 0)
	_, err := ValidateImageFile(f, h)
	if err == nil {
		t.Fatal("expected error for empty file, got nil")
	}
}

// ── newUUID ───────────────────────────────────────────────────────────────────

func TestNewUUID_Format(t *testing.T) {
	id := newUUID()
	// Expected layout: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
	parts := strings.Split(id, "-")
	if len(parts) != 5 {
		t.Fatalf("expected 5 dash-separated parts, got %d in %q", len(parts), id)
	}
	expected := []int{8, 4, 4, 4, 12}
	for i, p := range parts {
		if len(p) != expected[i] {
			t.Errorf("part[%d]: want len %d, got %d (%q)", i, expected[i], len(p), p)
		}
	}
}

func TestNewUUID_Uniqueness(t *testing.T) {
	seen := make(map[string]struct{}, 200)
	for i := 0; i < 200; i++ {
		id := newUUID()
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate UUID on iteration %d: %s", i, id)
		}
		seen[id] = struct{}{}
	}
}

func TestNewUUID_Version4Bits(t *testing.T) {
	id := newUUID()
	// Third group starts with '4' for version 4.
	parts := strings.Split(id, "-")
	if parts[2][0] != '4' {
		t.Errorf("version nibble should be '4', got %q in UUID %q", string(parts[2][0]), id)
	}
	// Fourth group high nibble must be 8, 9, a, or b (RFC 4122 variant).
	v := parts[3][0]
	if v != '8' && v != '9' && v != 'a' && v != 'b' {
		t.Errorf("variant nibble %q not in {8,9,a,b} for UUID %q", string(v), id)
	}
}

// ── allowedMIMETypes ──────────────────────────────────────────────────────────

func TestAllowedMIMETypes_Present(t *testing.T) {
	for _, mime := range []string{"image/jpeg", "image/png", "image/gif"} {
		if _, ok := allowedMIMETypes[mime]; !ok {
			t.Errorf("allowedMIMETypes missing %q", mime)
		}
	}
}

func TestAllowedMIMETypes_Extensions(t *testing.T) {
	cases := map[string]string{
		"image/jpeg": ".jpg",
		"image/png":  ".png",
		"image/gif":  ".gif",
	}
	for mime, want := range cases {
		if got := allowedMIMETypes[mime]; got != want {
			t.Errorf("allowedMIMETypes[%q]: want %q, got %q", mime, want, got)
		}
	}
}

func TestAllowedMIMETypes_NoExtra(t *testing.T) {
	// Reject unknown types like webp or bmp
	for _, bad := range []string{"image/webp", "image/bmp", "image/tiff", "text/plain"} {
		if _, ok := allowedMIMETypes[bad]; ok {
			t.Errorf("allowedMIMETypes should NOT contain %q", bad)
		}
	}
}