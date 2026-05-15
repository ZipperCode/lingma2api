package api

import (
	"context"

	"lingma2api/internal/proxy"
)

// SettingsStore is the minimal subset of *db.Store required by the gate.
type SettingsStore interface {
	GetSettings(ctx context.Context) (map[string]string, error)
}

// evaluateVisionGate always returns (false, nil). Vision content is handled
// by the body builder (uploader + parts injection), so no gating is needed.
func evaluateVisionGate(_ context.Context, _ SettingsStore, _ proxy.CanonicalRequest) (bool, error) {
	return false, nil
}
