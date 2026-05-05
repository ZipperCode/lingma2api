package api

import (
	"context"
	"errors"

	"lingma2api/internal/proxy"
)

// ErrVisionNotImplemented is returned by evaluateVisionGate when a request
// contains image/document blocks and the vision_fallback_enabled setting is
// false (the default).
var ErrVisionNotImplemented = errors.New("vision_not_implemented")

// SettingsStore is the minimal subset of *db.Store required by the gate.
// Defined here so tests can inject a fake without depending on the full DB.
type SettingsStore interface {
	GetSettings(ctx context.Context) (map[string]string, error)
}

// evaluateVisionGate decides whether a canonical request that contains
// image/document blocks should proceed via the soft-fallback path or be
// rejected with ErrVisionNotImplemented.
//
// Returns:
//   - (false, nil) when the request has no vision content; caller proceeds normally.
//   - (true, nil)  when fallback is enabled or no settings backend is wired
//     (lightweight/embedded use); caller proceeds and the existing
//     mediaBlockToText projection compresses images into text.
//   - (false, ErrVisionNotImplemented) when fallback is explicitly disabled OR
//     when the settings store fails (conservative).
func evaluateVisionGate(ctx context.Context, store SettingsStore, req proxy.CanonicalRequest) (bool, error) {
	if !canonicalRequestHasVisionContent(req) {
		return false, nil
	}
	if store == nil {
		// No settings backend wired (e.g., legacy/embedded callers, lightweight
		// tests). Preserve legacy soft-fallback semantics to avoid breaking
		// existing consumers; explicit production deployments wire a real store
		// and can opt into 501 by leaving vision_fallback_enabled=false.
		return true, nil
	}
	settings, err := store.GetSettings(ctx)
	if err != nil {
		return false, ErrVisionNotImplemented
	}
	if settings["vision_fallback_enabled"] == "true" {
		return true, nil
	}
	return false, ErrVisionNotImplemented
}

func canonicalRequestHasVisionContent(req proxy.CanonicalRequest) bool {
	for _, turn := range req.Turns {
		for _, block := range turn.Blocks {
			if block.Type == proxy.CanonicalBlockImage || block.Type == proxy.CanonicalBlockDocument {
				return true
			}
		}
	}
	return false
}
