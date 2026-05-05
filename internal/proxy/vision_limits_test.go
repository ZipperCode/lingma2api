package proxy

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func makeBase64(size int) string {
	raw := make([]byte, size)
	for i := range raw {
		raw[i] = byte(i % 256)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func makeImageBlock(t *testing.T, sourceType, mediaType, data string) CanonicalContentBlock {
	t.Helper()
	src := ImageSource{Type: sourceType, MediaType: mediaType, Data: data}
	raw, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return CanonicalContentBlock{
		Type: CanonicalBlockImage,
		Data: raw,
		Metadata: map[string]any{
			"media_type":  mediaType,
			"source_type": sourceType,
			"byte_size":   approxByteSize(src),
			"index":       0,
		},
	}
}

func TestParseOpenAIImageURL_DataURI(t *testing.T) {
	src, err := parseOpenAIImageURL("data:image/png;base64,QUFB")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if src.Type != "base64" || src.MediaType != "image/png" || src.Data != "QUFB" {
		t.Fatalf("unexpected: %+v", src)
	}
}

func TestParseOpenAIImageURL_HTTP(t *testing.T) {
	src, err := parseOpenAIImageURL("https://example.com/x.png")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if src.Type != "url" || src.Data != "https://example.com/x.png" {
		t.Fatalf("unexpected: %+v", src)
	}
}

func TestParseOpenAIImageURL_Invalid(t *testing.T) {
	if _, err := parseOpenAIImageURL("ftp://x"); err == nil {
		t.Fatalf("expected error for ftp scheme")
	}
	if _, err := parseOpenAIImageURL("data:image/png;base64,!!"); err == nil {
		t.Fatalf("expected error for invalid base64")
	}
	if _, err := parseOpenAIImageURL(""); err == nil {
		t.Fatalf("expected error for empty url")
	}
}

func TestValidateVisionLimits_OK(t *testing.T) {
	req := CanonicalRequest{Turns: []CanonicalTurn{{
		Blocks: []CanonicalContentBlock{
			makeImageBlock(t, "base64", "image/png", makeBase64(1024)),
		},
	}}}
	if err := ValidateVisionLimits(req); err != nil {
		t.Fatalf("ValidateVisionLimits: %v", err)
	}
}

func TestValidateVisionLimits_TooManyImages(t *testing.T) {
	blocks := make([]CanonicalContentBlock, 5)
	for i := range blocks {
		blocks[i] = makeImageBlock(t, "url", "image/png", "https://example.com/")
	}
	req := CanonicalRequest{Turns: []CanonicalTurn{{Blocks: blocks}}}
	err := ValidateVisionLimits(req)
	if err == nil || !strings.Contains(err.Error(), "image count") {
		t.Fatalf("expected too-many-images error, got %v", err)
	}
}

func TestValidateVisionLimits_SingleImageTooLarge(t *testing.T) {
	tooLarge := makeBase64(6 * 1024 * 1024)
	req := CanonicalRequest{Turns: []CanonicalTurn{{
		Blocks: []CanonicalContentBlock{makeImageBlock(t, "base64", "image/png", tooLarge)},
	}}}
	err := ValidateVisionLimits(req)
	if err == nil || !strings.Contains(err.Error(), "exceeds 5 MB") {
		t.Fatalf("expected single-image limit error, got %v", err)
	}
}

func TestValidateVisionLimits_TotalBytesExceeded(t *testing.T) {
	big := makeBase64(4 * 1024 * 1024)
	req := CanonicalRequest{Turns: []CanonicalTurn{{
		Blocks: []CanonicalContentBlock{
			makeImageBlock(t, "base64", "image/png", big),
			makeImageBlock(t, "base64", "image/png", big),
			makeImageBlock(t, "base64", "image/png", big),
		},
	}}}
	err := ValidateVisionLimits(req)
	if err == nil || !strings.Contains(err.Error(), "total image bytes") {
		t.Fatalf("expected total-bytes error, got %v", err)
	}
}

func TestValidateVisionLimits_InvalidMediaType(t *testing.T) {
	req := CanonicalRequest{Turns: []CanonicalTurn{{
		Blocks: []CanonicalContentBlock{makeImageBlock(t, "base64", "image/bmp", makeBase64(64))},
	}}}
	err := ValidateVisionLimits(req)
	if err == nil || !strings.Contains(err.Error(), "unsupported media type") {
		t.Fatalf("expected media-type error, got %v", err)
	}
}
