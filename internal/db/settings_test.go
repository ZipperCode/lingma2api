package db

import (
	"context"
	"testing"
)

func newSettingsTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestGetSettings_DefaultsIncludeVisionFallback(t *testing.T) {
	store := newSettingsTestStore(t)

	settings, err := store.GetSettings(context.Background())
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	got, ok := settings["vision_fallback_enabled"]
	if !ok {
		t.Fatalf("vision_fallback_enabled missing; defaults: %v", settings)
	}
	if got != "false" {
		t.Fatalf("vision_fallback_enabled = %q, want %q", got, "false")
	}
}

func TestUpdateSettings_AcceptsVisionFallback(t *testing.T) {
	store := newSettingsTestStore(t)

	if err := store.UpdateSettings(context.Background(), map[string]string{"vision_fallback_enabled": "true"}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	settings, err := store.GetSettings(context.Background())
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if settings["vision_fallback_enabled"] != "true" {
		t.Fatalf("vision_fallback_enabled = %q, want true", settings["vision_fallback_enabled"])
	}
}

func TestUpdateSettings_RejectsUnknownKey(t *testing.T) {
	store := newSettingsTestStore(t)

	err := store.UpdateSettings(context.Background(), map[string]string{"vision_unknown_key": "x"})
	if err == nil {
		t.Fatalf("expected error for unknown key")
	}
}
