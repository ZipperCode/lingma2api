package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"lingma2api/internal/db"
	"lingma2api/internal/proxy"
)

func TestAdminLogsListUsesCanonicalExecutionRecords(t *testing.T) {
	store, cleanup := tempAPIStore(t)
	defer cleanup()

	if err := store.InsertCanonicalExecutionRecord(context.Background(), &db.CanonicalExecutionRecordRow{
		ID:              "cer-openai",
		CreatedAt:       time.Unix(100, 0),
		IngressProtocol: "openai",
		IngressEndpoint: "/v1/chat/completions",
		SessionID:       "sess-1",
		PrePolicyRequest: proxy.CanonicalRequest{
			SchemaVersion: 1,
			Protocol:      proxy.CanonicalProtocolOpenAI,
			Model:         "gpt-4o",
			Stream:        true,
			SessionID:     "sess-1",
		},
		PostPolicyRequest: proxy.CanonicalRequest{
			SchemaVersion: 1,
			Protocol:      proxy.CanonicalProtocolOpenAI,
			Model:         "qwen-max",
			Stream:        true,
			SessionID:     "sess-1",
		},
		SouthboundRequest: `{"request_id":"req-1"}`,
		Sidecar: &proxy.CanonicalExecutionSidecar{
			SchemaVersion: 1,
			TTFTMs:        321,
		},
	}); err != nil {
		t.Fatalf("InsertCanonicalExecutionRecord() error = %v", err)
	}

	handler := NewServer(Dependencies{
		Credentials: fakeCredentials{},
		Models:      fakeModels{},
		Sessions:    fakeSessions{},
		Transport:   fakeTransport{},
		Builder:     fakeBuilder{},
	}, store)

	req := httptest.NewRequest(http.MethodGet, "/admin/logs?page=1&limit=50", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result db.LogListResult
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode list result: %v", err)
	}
	if result.Total != 1 || len(result.Items) != 1 {
		t.Fatalf("unexpected list result: %#v", result)
	}
	item := result.Items[0]
	if item.ID != "cer-openai" || item.Model != "gpt-4o" || item.MappedModel != "qwen-max" {
		t.Fatalf("unexpected projected item: %#v", item)
	}
	if item.DownstreamPath != "/v1/chat/completions" || item.TTFTMs != 321 {
		t.Fatalf("unexpected projected path/ttft: %#v", item)
	}
}

func TestAdminLogGetUsesCanonicalExecutionRecordProjection(t *testing.T) {
	store, cleanup := tempAPIStore(t)
	defer cleanup()

	if err := store.InsertCanonicalExecutionRecord(context.Background(), &db.CanonicalExecutionRecordRow{
		ID:              "cer-anthropic",
		CreatedAt:       time.Unix(200, 0),
		IngressProtocol: "anthropic",
		IngressEndpoint: "/v1/messages",
		SessionID:       "sess-a",
		PrePolicyRequest: proxy.CanonicalRequest{
			SchemaVersion: 1,
			Protocol:      proxy.CanonicalProtocolAnthropic,
			Model:         "claude-3-5-sonnet",
			Stream:        false,
			SessionID:     "sess-a",
			Turns: []proxy.CanonicalTurn{{
				Role: "user",
				Blocks: []proxy.CanonicalContentBlock{{
					Type: proxy.CanonicalBlockText,
					Text: "hello",
				}},
			}},
		},
		PostPolicyRequest: proxy.CanonicalRequest{
			SchemaVersion: 1,
			Protocol:      proxy.CanonicalProtocolAnthropic,
			Model:         "qwen-plus",
			Stream:        false,
			SessionID:     "sess-a",
		},
		SessionSnapshot: &proxy.CanonicalSessionSnapshot{
			SchemaVersion:   1,
			SessionID:       "sess-a",
			IngressProtocol: proxy.CanonicalProtocolAnthropic,
			UpdatedAt:       time.Unix(201, 0),
			Turns: []proxy.CanonicalTurn{
				{
					Role: "user",
					Blocks: []proxy.CanonicalContentBlock{{
						Type: proxy.CanonicalBlockText,
						Text: "hello",
					}},
				},
				{
					Role: "assistant",
					Blocks: []proxy.CanonicalContentBlock{{
						Type: proxy.CanonicalBlockText,
						Text: "hi",
					}},
				},
			},
		},
		SouthboundRequest: `{"request_id":"req-2"}`,
		Sidecar: &proxy.CanonicalExecutionSidecar{
			SchemaVersion: 1,
			RawSSELines: []string{
				`data:{"body":"{\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}","statusCodeValue":200}`,
				`data:[DONE]`,
			},
			TTFTMs: 12,
			Metadata: map[string]any{
				"request_id":      "req-2",
				"upstream_status": 200,
			},
		},
	}); err != nil {
		t.Fatalf("InsertCanonicalExecutionRecord() error = %v", err)
	}

	handler := NewServer(Dependencies{
		Credentials: fakeCredentials{},
		Models:      fakeModels{},
		Sessions:    fakeSessions{},
		Transport:   fakeTransport{},
		Builder:     fakeBuilder{},
	}, store)

	req := httptest.NewRequest(http.MethodGet, "/admin/logs/cer-anthropic", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var log db.RequestLog
	if err := json.NewDecoder(rec.Body).Decode(&log); err != nil {
		t.Fatalf("decode log: %v", err)
	}
	if log.Model != "claude-3-5-sonnet" || log.MappedModel != "qwen-plus" {
		t.Fatalf("unexpected projected models: %#v", log)
	}
	if !log.CanonicalRecord || log.IngressProtocol != "anthropic" || log.IngressEndpoint != "/v1/messages" {
		t.Fatalf("expected canonical metadata fields, got %#v", log)
	}
	if log.DownstreamPath != "/v1/messages" {
		t.Fatalf("unexpected downstream path: %#v", log)
	}
	if !strings.Contains(log.DownstreamReq, `"model":"claude-3-5-sonnet"`) {
		t.Fatalf("expected canonical replay body in downstream_req, got %s", log.DownstreamReq)
	}
	if !strings.Contains(log.PrePolicyRequest, `"model": "claude-3-5-sonnet"`) {
		t.Fatalf("expected pre_policy_request payload, got %s", log.PrePolicyRequest)
	}
	if !strings.Contains(log.PostPolicyRequest, `"model": "qwen-plus"`) {
		t.Fatalf("expected post_policy_request payload, got %s", log.PostPolicyRequest)
	}
	if !strings.Contains(log.SessionSnapshot, `"session_id": "sess-a"`) {
		t.Fatalf("expected session_snapshot payload, got %s", log.SessionSnapshot)
	}
	if !strings.Contains(log.ExecutionSidecar, `"raw_sse_lines"`) {
		t.Fatalf("expected execution_sidecar payload, got %s", log.ExecutionSidecar)
	}
	if log.UpstreamResp == "" || !strings.Contains(log.UpstreamResp, "data:[DONE]") {
		t.Fatalf("expected raw SSE evidence in upstream_resp, got %q", log.UpstreamResp)
	}
}

func TestAdminLogsReplayDefaultsToPrePolicyCanonicalRequest(t *testing.T) {
	store, cleanup := tempAPIStore(t)
	defer cleanup()

	if err := store.InsertCanonicalExecutionRecord(context.Background(), &db.CanonicalExecutionRecordRow{
		ID:              "cer-replay",
		CreatedAt:       time.Unix(300, 0),
		IngressProtocol: "anthropic",
		IngressEndpoint: "/v1/messages",
		SessionID:       "sess-r",
		PrePolicyRequest: proxy.CanonicalRequest{
			SchemaVersion: 1,
			Protocol:      proxy.CanonicalProtocolAnthropic,
			Model:         "claude-3-5-sonnet",
			Stream:        false,
			SessionID:     "sess-r",
			Turns: []proxy.CanonicalTurn{{
				Role: "user",
				Blocks: []proxy.CanonicalContentBlock{{
					Type: proxy.CanonicalBlockText,
					Text: "replay me",
				}},
			}},
		},
		PostPolicyRequest: proxy.CanonicalRequest{
			SchemaVersion: 1,
			Protocol:      proxy.CanonicalProtocolAnthropic,
			Model:         "rewritten-model",
			Stream:        false,
			SessionID:     "sess-r",
		},
	}); err != nil {
		t.Fatalf("InsertCanonicalExecutionRecord() error = %v", err)
	}

	models := &capturingModels{}
	builder := &capturingBuilder{}
	handler := NewServer(Dependencies{
		Credentials: fakeCredentials{},
		Models:      models,
		Sessions:    fakeSessions{},
		Transport: fakeTransport{
			lines: []string{
				`data:{"body":"{\"choices\":[{\"delta\":{\"content\":\"Replay\"}}]}","statusCodeValue":200}`,
				`data:[DONE]`,
			},
		},
		Builder: builder,
		Now:     func() time.Time { return time.Unix(400, 0) },
	}, store)

	req := httptest.NewRequest(http.MethodPost, "/admin/logs/cer-replay/replay", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if builder.request.Model != "claude-3-5-sonnet" {
		t.Fatalf("expected replay from pre-policy model, got %#v", builder.request)
	}
	if len(builder.messages) != 1 || builder.messages[0].Content != "replay me" {
		t.Fatalf("unexpected replay messages: %#v", builder.messages)
	}
}

func TestAdminLogsReplayHistoricalUsesPostPolicyCanonicalRequest(t *testing.T) {
	store, cleanup := tempAPIStore(t)
	defer cleanup()

	if err := store.InsertCanonicalExecutionRecord(context.Background(), &db.CanonicalExecutionRecordRow{
		ID:              "cer-historical",
		CreatedAt:       time.Unix(300, 0),
		IngressProtocol: "openai",
		IngressEndpoint: "/v1/chat/completions",
		SessionID:       "sess-h",
		PrePolicyRequest: proxy.CanonicalRequest{
			SchemaVersion: 1,
			Protocol:      proxy.CanonicalProtocolOpenAI,
			Model:         "pre-model",
			Stream:        false,
			SessionID:     "sess-h",
			Turns: []proxy.CanonicalTurn{{
				Role:   "user",
				Blocks: []proxy.CanonicalContentBlock{{Type: proxy.CanonicalBlockText, Text: "pre body"}},
			}},
		},
		PostPolicyRequest: proxy.CanonicalRequest{
			SchemaVersion: 1,
			Protocol:      proxy.CanonicalProtocolOpenAI,
			Model:         "post-model",
			Stream:        false,
			SessionID:     "sess-h",
			Turns: []proxy.CanonicalTurn{{
				Role:   "user",
				Blocks: []proxy.CanonicalContentBlock{{Type: proxy.CanonicalBlockText, Text: "post body"}},
			}},
		},
	}); err != nil {
		t.Fatalf("InsertCanonicalExecutionRecord() error = %v", err)
	}

	builder := &capturingBuilder{}
	handler := NewServer(Dependencies{
		Credentials: fakeCredentials{},
		Models:      &capturingModels{},
		Sessions:    fakeSessions{},
		Transport: fakeTransport{
			lines: []string{
				`data:{"body":"{\"choices\":[{\"delta\":{\"content\":\"Replay\"}}]}","statusCodeValue":200}`,
				`data:[DONE]`,
			},
		},
		Builder: builder,
		Now:     func() time.Time { return time.Unix(400, 0) },
	}, store)

	req := httptest.NewRequest(http.MethodPost, "/admin/logs/cer-historical/replay?mode=historical", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if builder.request.Model != "post-model" {
		t.Fatalf("expected replay from post-policy model, got %#v", builder.request)
	}
	if len(builder.messages) != 1 || builder.messages[0].Content != "post body" {
		t.Fatalf("unexpected historical replay messages: %#v", builder.messages)
	}
}

func TestAdminLogsReplayHistoricalSkipsCurrentPolicy(t *testing.T) {
	store, cleanup := tempAPIStore(t)
	defer cleanup()

	rewriteAgain := "current-policy-model"
	if err := store.CreatePolicy(context.Background(), &db.PolicyRule{
		Priority: 1,
		Name:     "rewrite historical if policy reruns",
		Enabled:  true,
		Match: db.PolicyMatch{
			Protocol:       "openai",
			RequestedModel: "^post-model$",
		},
		Actions: db.PolicyActions{RewriteModel: &rewriteAgain},
	}); err != nil {
		t.Fatalf("CreatePolicy() error = %v", err)
	}

	if err := store.InsertCanonicalExecutionRecord(context.Background(), &db.CanonicalExecutionRecordRow{
		ID:              "cer-historical-no-policy",
		CreatedAt:       time.Unix(300, 0),
		IngressProtocol: "openai",
		IngressEndpoint: "/v1/chat/completions",
		SessionID:       "sess-hp",
		PrePolicyRequest: proxy.CanonicalRequest{
			SchemaVersion: 1,
			Protocol:      proxy.CanonicalProtocolOpenAI,
			Model:         "pre-model",
			Stream:        false,
			SessionID:     "sess-hp",
			Turns: []proxy.CanonicalTurn{{
				Role:   "user",
				Blocks: []proxy.CanonicalContentBlock{{Type: proxy.CanonicalBlockText, Text: "pre body"}},
			}},
		},
		PostPolicyRequest: proxy.CanonicalRequest{
			SchemaVersion: 1,
			Protocol:      proxy.CanonicalProtocolOpenAI,
			Model:         "post-model",
			Stream:        false,
			SessionID:     "sess-hp",
			Turns: []proxy.CanonicalTurn{{
				Role:   "user",
				Blocks: []proxy.CanonicalContentBlock{{Type: proxy.CanonicalBlockText, Text: "post body"}},
			}},
		},
	}); err != nil {
		t.Fatalf("InsertCanonicalExecutionRecord() error = %v", err)
	}

	builder := &capturingBuilder{}
	handler := NewServer(Dependencies{
		Credentials: fakeCredentials{},
		Models:      &capturingModels{},
		Sessions:    fakeSessions{},
		Transport: fakeTransport{
			lines: []string{
				`data:{"body":"{\"choices\":[{\"delta\":{\"content\":\"Replay\"}}]}","statusCodeValue":200}`,
				`data:[DONE]`,
			},
		},
		Builder: builder,
		Now:     func() time.Time { return time.Unix(400, 0) },
	}, store)

	req := httptest.NewRequest(http.MethodPost, "/admin/logs/cer-historical-no-policy/replay?mode=historical", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if builder.request.Model != "post-model" {
		t.Fatalf("expected historical replay to skip current policy, got %#v", builder.request)
	}
}

func TestAdminLogsReplayUsesOriginalAnthropicIngressEndpoint(t *testing.T) {
	store, cleanup := tempAPIStore(t)
	defer cleanup()

	if err := store.InsertCanonicalExecutionRecord(context.Background(), &db.CanonicalExecutionRecordRow{
		ID:              "cer-anthropic-replay",
		CreatedAt:       time.Unix(300, 0),
		IngressProtocol: "anthropic",
		IngressEndpoint: "/v1/messages",
		SessionID:       "sess-a",
		PrePolicyRequest: proxy.CanonicalRequest{
			SchemaVersion: 1,
			Protocol:      proxy.CanonicalProtocolAnthropic,
			Model:         "claude-3-5-sonnet",
			Stream:        false,
			SessionID:     "sess-a",
			Turns: []proxy.CanonicalTurn{{
				Role:   "user",
				Blocks: []proxy.CanonicalContentBlock{{Type: proxy.CanonicalBlockText, Text: "anthropic replay"}},
			}},
		},
		PostPolicyRequest: proxy.CanonicalRequest{
			SchemaVersion: 1,
			Protocol:      proxy.CanonicalProtocolAnthropic,
			Model:         "rewritten-anthropic",
			Stream:        false,
			SessionID:     "sess-a",
		},
	}); err != nil {
		t.Fatalf("InsertCanonicalExecutionRecord() error = %v", err)
	}

	handler := NewServer(Dependencies{
		Credentials: fakeCredentials{},
		Models:      &capturingModels{},
		Sessions:    fakeSessions{},
		Transport: fakeTransport{
			lines: []string{
				`data:{"body":"{\"choices\":[{\"delta\":{\"content\":\"Replay\"}}]}","statusCodeValue":200}`,
				`data:[DONE]`,
			},
		},
		Builder: &capturingBuilder{},
		Now:     func() time.Time { return time.Unix(400, 0) },
	}, store)

	req := httptest.NewRequest(http.MethodPost, "/admin/logs/cer-anthropic-replay/replay", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"type":"message"`) {
		t.Fatalf("expected anthropic message response, got %s", rec.Body.String())
	}
}
