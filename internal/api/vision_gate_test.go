package api

import (
	"context"
	"errors"
	"testing"

	"lingma2api/internal/proxy"
)

type fakeSettingsStore struct {
	settings map[string]string
	err      error
}

func (f *fakeSettingsStore) GetSettings(ctx context.Context) (map[string]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make(map[string]string, len(f.settings))
	for k, v := range f.settings {
		out[k] = v
	}
	return out, nil
}

func TestEvaluateVisionGate_NoImage(t *testing.T) {
	store := &fakeSettingsStore{settings: map[string]string{"vision_fallback_enabled": "false"}}
	req := proxy.CanonicalRequest{Turns: []proxy.CanonicalTurn{{Blocks: []proxy.CanonicalContentBlock{
		{Type: proxy.CanonicalBlockText, Text: "hello"},
	}}}}
	allow, err := evaluateVisionGate(context.Background(), store, req)
	if err != nil {
		t.Fatalf("evaluateVisionGate: %v", err)
	}
	if allow {
		t.Fatalf("allowFallback = true, want false (no image)")
	}
}

func TestEvaluateVisionGate_ImageDefaultDenies(t *testing.T) {
	store := &fakeSettingsStore{settings: map[string]string{"vision_fallback_enabled": "false"}}
	req := proxy.CanonicalRequest{Turns: []proxy.CanonicalTurn{{Blocks: []proxy.CanonicalContentBlock{
		{Type: proxy.CanonicalBlockImage},
	}}}}
	_, err := evaluateVisionGate(context.Background(), store, req)
	if !errors.Is(err, ErrVisionNotImplemented) {
		t.Fatalf("err = %v, want ErrVisionNotImplemented", err)
	}
}

func TestEvaluateVisionGate_ImageFallbackAllows(t *testing.T) {
	store := &fakeSettingsStore{settings: map[string]string{"vision_fallback_enabled": "true"}}
	req := proxy.CanonicalRequest{Turns: []proxy.CanonicalTurn{{Blocks: []proxy.CanonicalContentBlock{
		{Type: proxy.CanonicalBlockImage},
	}}}}
	allow, err := evaluateVisionGate(context.Background(), store, req)
	if err != nil {
		t.Fatalf("evaluateVisionGate: %v", err)
	}
	if !allow {
		t.Fatalf("allowFallback = false, want true")
	}
}

func TestEvaluateVisionGate_StoreErrorIsConservative(t *testing.T) {
	store := &fakeSettingsStore{err: errors.New("db down")}
	req := proxy.CanonicalRequest{Turns: []proxy.CanonicalTurn{{Blocks: []proxy.CanonicalContentBlock{
		{Type: proxy.CanonicalBlockImage},
	}}}}
	_, err := evaluateVisionGate(context.Background(), store, req)
	if !errors.Is(err, ErrVisionNotImplemented) {
		t.Fatalf("err = %v, want ErrVisionNotImplemented (conservative path)", err)
	}
}

func TestEvaluateVisionGate_DocumentTreatedAsVision(t *testing.T) {
	store := &fakeSettingsStore{settings: map[string]string{"vision_fallback_enabled": "false"}}
	req := proxy.CanonicalRequest{Turns: []proxy.CanonicalTurn{{Blocks: []proxy.CanonicalContentBlock{
		{Type: proxy.CanonicalBlockDocument},
	}}}}
	_, err := evaluateVisionGate(context.Background(), store, req)
	if !errors.Is(err, ErrVisionNotImplemented) {
		t.Fatalf("err = %v, want ErrVisionNotImplemented for document block", err)
	}
}
