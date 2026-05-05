package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAnthropicMessagesVisionDefaultReturns501(t *testing.T) {
	store := newVisionTestStore(t)
	handler := newVisionHandler(t, store)

	body := `{"model":"claude-3-7-sonnet-20250219","max_tokens":1024,"messages":[{"role":"user","content":[{"type":"text","text":"see"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"QUFB"}}]}]}`
	request := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501; body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "not_supported_yet") {
		t.Fatalf("body = %s, want not_supported_yet", recorder.Body.String())
	}
}

func TestAnthropicMessagesVisionFallbackEnabledBypassesGate(t *testing.T) {
	store := newVisionTestStore(t)
	if err := store.UpdateSettings(context.Background(), map[string]string{"vision_fallback_enabled": "true"}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	handler := newVisionHandler(t, store)

	body := `{"model":"claude-3-7-sonnet-20250219","max_tokens":1024,"messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"QUFB"}}]}]}`
	request := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code == http.StatusNotImplemented {
		t.Fatalf("status = 501; expected fallback bypass. body=%s", recorder.Body.String())
	}
}
