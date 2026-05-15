package api

import (
	"context"
	"testing"

	"lingma2api/internal/proxy"
)

// TestEvaluateVisionGate_AlwaysPasses verifies that evaluateVisionGate always
// returns (false, nil) regardless of the request content, since the body
// builder now handles vision content directly.
func TestEvaluateVisionGate_AlwaysPasses(t *testing.T) {
	tests := []struct {
		name string
		req  proxy.CanonicalRequest
	}{
		{
			name: "no image",
			req:  proxy.CanonicalRequest{Turns: []proxy.CanonicalTurn{{Blocks: []proxy.CanonicalContentBlock{{Type: proxy.CanonicalBlockText, Text: "hello"}}}}},
		},
		{
			name: "with image",
			req:  proxy.CanonicalRequest{Turns: []proxy.CanonicalTurn{{Blocks: []proxy.CanonicalContentBlock{{Type: proxy.CanonicalBlockImage}}}}},
		},
		{
			name: "with document",
			req:  proxy.CanonicalRequest{Turns: []proxy.CanonicalTurn{{Blocks: []proxy.CanonicalContentBlock{{Type: proxy.CanonicalBlockDocument}}}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allow, err := evaluateVisionGate(context.Background(), nil, tt.req)
			if err != nil {
				t.Fatalf("evaluateVisionGate: %v", err)
			}
			if allow {
				t.Fatalf("allow = true, want false (gate always passes)")
			}
		})
	}
}
