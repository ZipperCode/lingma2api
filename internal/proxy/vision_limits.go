package proxy

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// Vision skeleton limits. Values intentionally conservative; tune via Settings
// once reverse-engineering of the upstream image_urls field is complete.
const (
	visionMaxImagesPerRequest = 4
	visionMaxImageBytes       = 5 * 1024 * 1024  // 5 MB
	visionMaxTotalImageBytes  = 10 * 1024 * 1024 // 10 MB
)

var visionAllowedMediaTypes = map[string]struct{}{
	"image/png":  {},
	"image/jpeg": {},
	"image/gif":  {},
	"image/webp": {},
}

// parseOpenAIImageURL turns an OpenAI image_url.url into the canonical
// ImageSource shape used in IR. Supports `https?://...` and
// `data:<media>;base64,<payload>`.
func parseOpenAIImageURL(raw string) (ImageSource, error) {
	if raw == "" {
		return ImageSource{}, fmt.Errorf("image_url.url must not be empty")
	}
	if strings.HasPrefix(raw, "data:") {
		comma := strings.Index(raw, ",")
		if comma < 0 {
			return ImageSource{}, fmt.Errorf("invalid data uri")
		}
		header := raw[len("data:"):comma]
		payload := raw[comma+1:]
		// header expected like "image/png;base64"
		semi := strings.Index(header, ";")
		if semi < 0 || header[semi+1:] != "base64" {
			return ImageSource{}, fmt.Errorf("data uri must use base64 encoding")
		}
		mediaType := header[:semi]
		if _, err := base64.StdEncoding.DecodeString(payload); err != nil {
			return ImageSource{}, fmt.Errorf("invalid base64 image payload: %w", err)
		}
		return ImageSource{Type: "base64", MediaType: mediaType, Data: payload}, nil
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ImageSource{}, fmt.Errorf("invalid image url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ImageSource{}, fmt.Errorf("unsupported image url scheme %q", parsed.Scheme)
	}
	return ImageSource{Type: "url", MediaType: "", Data: raw}, nil
}

// approxByteSize returns the approximate raw byte count for an image source.
// For base64 it inverts standard base64 padding; for url it returns the URL
// string length as a proxy (only used for log metadata, not for limit checks).
func approxByteSize(src ImageSource) int {
	if src.Type == "base64" {
		stripped := strings.TrimRight(src.Data, "=")
		return len(stripped) * 3 / 4
	}
	return len(src.Data)
}

// ValidateVisionLimits scans every block of every turn and enforces the
// vision skeleton limits. Returns the first violation as a user-facing error.
func ValidateVisionLimits(req CanonicalRequest) error {
	totalBytes := 0
	imageCount := 0
	for turnIdx, turn := range req.Turns {
		for blockIdx, block := range turn.Blocks {
			if block.Type != CanonicalBlockImage && block.Type != CanonicalBlockDocument {
				continue
			}
			imageCount++
			if imageCount > visionMaxImagesPerRequest {
				return fmt.Errorf("image count exceeds %d per request", visionMaxImagesPerRequest)
			}
			var src ImageSource
			if err := json.Unmarshal(block.Data, &src); err != nil {
				return fmt.Errorf("turn[%d].block[%d]: invalid image source: %w", turnIdx, blockIdx, err)
			}
			if src.Type == "base64" {
				if _, ok := visionAllowedMediaTypes[src.MediaType]; !ok {
					return fmt.Errorf("turn[%d].block[%d]: unsupported media type %q", turnIdx, blockIdx, src.MediaType)
				}
				size := approxByteSize(src)
				if size > visionMaxImageBytes {
					return fmt.Errorf("turn[%d].block[%d]: image exceeds 5 MB limit (%d bytes)", turnIdx, blockIdx, size)
				}
				totalBytes += size
			} else if src.Type == "url" {
				// http(s) URL: do not fetch in the skeleton stage. media_type
				// may be empty and is allowed; size budget not consumed.
				if src.Data == "" {
					return fmt.Errorf("turn[%d].block[%d]: empty image url", turnIdx, blockIdx)
				}
			} else {
				return fmt.Errorf("turn[%d].block[%d]: unsupported image source type %q", turnIdx, blockIdx, src.Type)
			}
			if totalBytes > visionMaxTotalImageBytes {
				return fmt.Errorf("total image bytes exceed 10 MB per request")
			}
		}
	}
	return nil
}
