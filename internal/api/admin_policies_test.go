package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

func TestAdminMappingsCompatibilityBackedByPolicies(t *testing.T) {
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
		"name":     "legacy auto mapping",
		"priority": 9,
		"pattern":  "^auto$",
		"target":   "qwen3-coder",
		"enabled":  true,
	}
	bodyBytes, _ := json.Marshal(createBody)
	req := httptest.NewRequest(http.MethodPost, "/admin/mappings", bytes.NewReader(bodyBytes))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 create mapping, got %d: %s", rec.Code, rec.Body.String())
	}

	var created db.ModelMapping
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode created mapping: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("expected compatibility mapping id from policy")
	}

	policies, err := store.ListPolicies(context.Background())
	if err != nil {
		t.Fatalf("ListPolicies() error = %v", err)
	}
	if len(policies) != 1 {
		t.Fatalf("expected 1 backing policy, got %#v", policies)
	}
	if policies[0].Source != "model_mapping" {
		t.Fatalf("expected model_mapping policy source, got %q", policies[0].Source)
	}
	if policies[0].Match.RequestedModel != "^auto$" {
		t.Fatalf("unexpected policy match: %#v", policies[0].Match)
	}
	if policies[0].Actions.RewriteModel == nil || *policies[0].Actions.RewriteModel != "qwen3-coder" {
		t.Fatalf("unexpected policy actions: %#v", policies[0].Actions)
	}

	req = httptest.NewRequest(http.MethodGet, "/admin/mappings", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 list mappings, got %d: %s", rec.Code, rec.Body.String())
	}
	var mappings []db.ModelMapping
	if err := json.NewDecoder(rec.Body).Decode(&mappings); err != nil {
		t.Fatalf("decode mappings: %v", err)
	}
	if len(mappings) != 1 || mappings[0].ID != policies[0].ID || mappings[0].Target != "qwen3-coder" {
		t.Fatalf("unexpected compatibility mappings: %#v", mappings)
	}

	testBody := map[string]any{"model": "auto"}
	bodyBytes, _ = json.Marshal(testBody)
	req = httptest.NewRequest(http.MethodPost, "/admin/mappings/test", bytes.NewReader(bodyBytes))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 mapping test, got %d: %s", rec.Code, rec.Body.String())
	}
	var testResult struct {
		Matched bool   `json:"matched"`
		Target  string `json:"target"`
		RuleID  int    `json:"rule_id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&testResult); err != nil {
		t.Fatalf("decode mapping test: %v", err)
	}
	if !testResult.Matched || testResult.Target != "qwen3-coder" || testResult.RuleID != policies[0].ID {
		t.Fatalf("unexpected mapping test result: %#v", testResult)
	}

	updateBody := map[string]any{
		"name":     "legacy auto mapping updated",
		"priority": 3,
		"pattern":  "^auto2$",
		"target":   "qwen-max",
		"enabled":  true,
	}
	bodyBytes, _ = json.Marshal(updateBody)
	req = httptest.NewRequest(http.MethodPut, fmt.Sprintf("/admin/mappings/%d", policies[0].ID), bytes.NewReader(bodyBytes))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 update mapping, got %d: %s", rec.Code, rec.Body.String())
	}
	policies, err = store.ListPolicies(context.Background())
	if err != nil {
		t.Fatalf("ListPolicies() after update error = %v", err)
	}
	if len(policies) != 1 || policies[0].Priority != 3 || policies[0].Match.RequestedModel != "^auto2$" ||
		policies[0].Actions.RewriteModel == nil || *policies[0].Actions.RewriteModel != "qwen-max" {
		t.Fatalf("unexpected updated backing policy: %#v", policies)
	}

	req = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/admin/mappings/%d", policies[0].ID), nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 delete mapping, got %d: %s", rec.Code, rec.Body.String())
	}
	policies, err = store.ListPolicies(context.Background())
	if err != nil {
		t.Fatalf("ListPolicies() after delete error = %v", err)
	}
	if len(policies) != 0 {
		t.Fatalf("expected backing policy deleted, got %#v", policies)
	}
}

func TestAdminMappingsCompatibilityDoesNotExposeOrDeleteNativePolicies(t *testing.T) {
	store, cleanup := tempAPIStore(t)
	defer cleanup()

	rewriteModel := "native-target"
	nativePolicy := &db.PolicyRule{
		Priority: 1,
		Name:     "native rewrite",
		Enabled:  true,
		Match: db.PolicyMatch{
			RequestedModel: "^native$",
		},
		Actions: db.PolicyActions{RewriteModel: &rewriteModel},
		Source:  "native",
	}
	if err := store.CreatePolicy(context.Background(), nativePolicy); err != nil {
		t.Fatalf("CreatePolicy() error = %v", err)
	}

	handler := NewServer(Dependencies{
		Credentials: fakeCredentials{},
		Models:      fakeModels{},
		Sessions:    fakeSessions{},
		Transport:   fakeTransport{},
		Builder:     fakeBuilder{},
	}, store)

	req := httptest.NewRequest(http.MethodGet, "/admin/mappings", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 list mappings, got %d: %s", rec.Code, rec.Body.String())
	}
	var mappings []db.ModelMapping
	if err := json.NewDecoder(rec.Body).Decode(&mappings); err != nil {
		t.Fatalf("decode mappings: %v", err)
	}
	if len(mappings) != 0 {
		t.Fatalf("expected native policy hidden from mappings view, got %#v", mappings)
	}

	req = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/admin/mappings/%d", nativePolicy.ID), nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 deleting native policy through mappings, got %d: %s", rec.Code, rec.Body.String())
	}
	policies, err := store.ListPolicies(context.Background())
	if err != nil {
		t.Fatalf("ListPolicies() error = %v", err)
	}
	if len(policies) != 1 || policies[0].ID != nativePolicy.ID {
		t.Fatalf("expected native policy preserved, got %#v", policies)
	}
}
