package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"lingma2api/internal/db"
)

func tempAPIStore(t *testing.T) (*db.Store, func()) {
	t.Helper()
	f, err := os.CreateTemp("", "lingma2api-api-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	store, err := db.Open(f.Name())
	if err != nil {
		os.Remove(f.Name())
		t.Fatal(err)
	}
	if err := store.Migrate(); err != nil {
		store.Close()
		os.Remove(f.Name())
		t.Fatal(err)
	}
	return store, func() {
		store.Close()
		os.Remove(f.Name())
	}
}

func TestAdminPoliciesCRUDAndTest(t *testing.T) {
	store, cleanup := tempAPIStore(t)
	defer cleanup()

	handler := NewServer(Dependencies{
		Credentials: fakeCredentials{},
		Models:      fakeModels{},
		Sessions:    fakeSessions{},
		Transport:   fakeTransport{},
		Builder:     fakeBuilder{},
	}, store)

	createBody := map[string]any{
		"name":     "anthropic-tool-policy",
		"priority": 5,
		"enabled":  true,
		"match": map[string]any{
			"protocol":        "anthropic",
			"requested_model": "^claude-3",
			"has_tools":       true,
		},
		"actions": map[string]any{
			"rewrite_model": "dashscope_qwen3_coder",
			"allow_tools":   true,
			"add_tags":      []string{"anthropic", "tools"},
		},
	}
	bodyBytes, _ := json.Marshal(createBody)
	req := httptest.NewRequest(http.MethodPost, "/admin/policies", bytes.NewReader(bodyBytes))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 create, got %d: %s", rec.Code, rec.Body.String())
	}

	var created db.PolicyRule
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode created policy: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("expected created policy id")
	}

	req = httptest.NewRequest(http.MethodGet, "/admin/policies", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 list, got %d: %s", rec.Code, rec.Body.String())
	}
	var items []db.PolicyRule
	if err := json.NewDecoder(rec.Body).Decode(&items); err != nil {
		t.Fatalf("decode policies: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 policy, got %d", len(items))
	}

	testBody := map[string]any{
		"protocol":        "anthropic",
		"requested_model": "claude-3-opus",
		"stream":          true,
		"has_tools":       true,
		"has_reasoning":   false,
		"session_present": true,
	}
	bodyBytes, _ = json.Marshal(testBody)
	req = httptest.NewRequest(http.MethodPost, "/admin/policies/test", bytes.NewReader(bodyBytes))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 test, got %d: %s", rec.Code, rec.Body.String())
	}

	var result struct {
		Matched          bool             `json:"matched"`
		EffectiveActions db.PolicyActions `json:"effective_actions"`
		MatchedRules     []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"matched_rules"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode test result: %v", err)
	}
	if !result.Matched {
		t.Fatal("expected policy test to match")
	}
	if result.EffectiveActions.RewriteModel == nil || *result.EffectiveActions.RewriteModel != "dashscope_qwen3_coder" {
		t.Fatalf("unexpected effective rewrite model: %#v", result.EffectiveActions.RewriteModel)
	}
	if len(result.MatchedRules) != 1 || result.MatchedRules[0].ID != created.ID {
		t.Fatalf("unexpected matched rules: %#v", result.MatchedRules)
	}
}

