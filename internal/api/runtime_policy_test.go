package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"lingma2api/internal/db"
	"lingma2api/internal/proxy"
)

type capturingModels struct {
	requested []string
}

func (models *capturingModels) ResolveChatModel(_ context.Context, requested string) (string, error) {
	models.requested = append(models.requested, requested)
	return requested + "-resolved", nil
}

func (*capturingModels) ListModels(context.Context) ([]proxy.OpenAIModel, error) {
	return []proxy.OpenAIModel{{ID: "auto", Object: "model", OwnedBy: "lingma"}}, nil
}

func (*capturingModels) Refresh(context.Context) error { return nil }

func (*capturingModels) Status() proxy.ModelStatus {
	return proxy.ModelStatus{Cached: true, Count: 1}
}

type capturingBuilder struct {
	request  proxy.CanonicalRequest
	messages []proxy.Message
	modelKey string
}

func (builder *capturingBuilder) BuildCanonical(request proxy.CanonicalRequest, modelKey string) (proxy.RemoteChatRequest, error) {
	builder.request = request
	_, messages, err := proxy.ProjectCanonicalToOpenAIRequest(request)
	if err != nil {
		return proxy.RemoteChatRequest{}, err
	}
	builder.messages = append([]proxy.Message(nil), messages...)
	builder.modelKey = modelKey
	return proxy.RemoteChatRequest{
		Path:      proxy.ChatPath,
		Query:     proxy.ChatQuery,
		RequestID: "req-1",
		ModelKey:  modelKey,
		Stream:    request.Stream,
	}, nil
}

func TestChatCompletionsAppliesRuntimePolicyBeforeBuild(t *testing.T) {
	store, cleanup := tempAPIStore(t)
	defer cleanup()

	rewriteModel := "rewritten-openai"
	allowTools := false
	reasoning := true
	policy := &db.PolicyRule{
		Priority: 1,
		Name:     "openai-runtime",
		Enabled:  true,
		Match: db.PolicyMatch{
			Protocol:       "openai",
			RequestedModel: "^auto$",
			HasTools:       boolPtr(true),
		},
		Actions: db.PolicyActions{
			RewriteModel: &rewriteModel,
			AllowTools:   &allowTools,
			SetReasoning: &reasoning,
		},
	}
	if err := store.CreatePolicy(context.Background(), policy); err != nil {
		t.Fatalf("CreatePolicy() error = %v", err)
	}

	models := &capturingModels{}
	builder := &capturingBuilder{}
	handler := NewServer(Dependencies{
		Credentials: fakeCredentials{},
		Models:      models,
		Sessions:    fakeSessions{},
		Transport: fakeTransport{
			lines: []string{
				`data:{"body":"{\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}","statusCodeValue":200}`,
				`data:[DONE]`,
			},
		},
		Builder: builder,
		Now:     func() time.Time { return time.Unix(1, 0) },
	}, store)

	body := `{"model":"auto","messages":[{"role":"user","content":"hi"}],"stream":false,"tools":[{"type":"function","function":{"name":"read_file","parameters":{"type":"object"}}}]}`
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if builder.request.Model != "rewritten-openai" {
		t.Fatalf("expected rewritten model, got %#v", builder.request)
	}
	if builder.request.HasReasoning != true {
		t.Fatalf("expected reasoning enabled, got %#v", builder.request)
	}
	if len(builder.request.Tools) != 0 || builder.request.ToolChoice != nil {
		t.Fatalf("expected tools cleared by policy, got %#v", builder.request)
	}
	if len(models.requested) != 1 || models.requested[0] != "rewritten-openai" {
		t.Fatalf("unexpected resolved model inputs: %#v", models.requested)
	}
	if !strings.Contains(recorder.Body.String(), `"model":"rewritten-openai"`) {
		t.Fatalf("expected rewritten model in response, got %s", recorder.Body.String())
	}
	records, err := store.ListCanonicalExecutionRecords(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListCanonicalExecutionRecords() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 canonical record, got %d", len(records))
	}
	if records[0].IngressProtocol != "openai" || records[0].IngressEndpoint != "/v1/chat/completions" {
		t.Fatalf("unexpected canonical record ingress: %#v", records[0])
	}
	if records[0].PrePolicyRequest.Model != "auto" || records[0].PostPolicyRequest.Model != "rewritten-openai" {
		t.Fatalf("unexpected canonical record models: %#v", records[0])
	}
	if records[0].SessionSnapshot == nil || len(records[0].SessionSnapshot.Turns) != 2 {
		t.Fatalf("unexpected session snapshot: %#v", records[0].SessionSnapshot)
	}
	if records[0].Sidecar == nil || len(records[0].Sidecar.RawSSELines) != 2 {
		t.Fatalf("expected raw SSE lines in sidecar, got %#v", records[0].Sidecar)
	}
}

func TestAnthropicMessagesAppliesRuntimePolicyBeforeBuild(t *testing.T) {
	store, cleanup := tempAPIStore(t)
	defer cleanup()

	rewriteModel := "rewritten-anthropic"
	allowTools := false
	reasoning := false
	policy := &db.PolicyRule{
		Priority: 1,
		Name:     "anthropic-runtime",
		Enabled:  true,
		Match: db.PolicyMatch{
			Protocol:       "anthropic",
			RequestedModel: "^claude-3-5-sonnet$",
			HasTools:       boolPtr(true),
		},
		Actions: db.PolicyActions{
			RewriteModel: &rewriteModel,
			AllowTools:   &allowTools,
			SetReasoning: &reasoning,
		},
	}
	if err := store.CreatePolicy(context.Background(), policy); err != nil {
		t.Fatalf("CreatePolicy() error = %v", err)
	}

	models := &capturingModels{}
	builder := &capturingBuilder{}
	handler := NewServer(Dependencies{
		Credentials: fakeCredentials{},
		Models:      models,
		Sessions:    fakeSessions{},
		Transport: fakeTransport{
			lines: []string{
				`data:{"body":"{\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}","statusCodeValue":200}`,
				`data:[DONE]`,
			},
		},
		Builder: builder,
		Now:     func() time.Time { return time.Unix(1, 0) },
	}, store)

	body := `{"model":"claude-3-5-sonnet","stream":false,"thinking":{"type":"enabled","budget_tokens":128},"tools":[{"type":"function","function":{"name":"lookup_doc","parameters":{"type":"object"}}}],"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`
	request := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if builder.request.Model != "rewritten-anthropic" {
		t.Fatalf("expected rewritten model, got %#v", builder.request)
	}
	if builder.request.HasReasoning {
		t.Fatalf("expected reasoning disabled, got %#v", builder.request)
	}
	if len(builder.request.Tools) != 0 || builder.request.ToolChoice != nil {
		t.Fatalf("expected tools cleared, got %#v", builder.request)
	}
	if len(models.requested) != 1 || models.requested[0] != "rewritten-anthropic" {
		t.Fatalf("unexpected resolved model inputs: %#v", models.requested)
	}
	if !strings.Contains(recorder.Body.String(), `"model":"rewritten-anthropic"`) {
		t.Fatalf("expected rewritten model in anthropic response, got %s", recorder.Body.String())
	}
	records, err := store.ListCanonicalExecutionRecords(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListCanonicalExecutionRecords() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 canonical record, got %d", len(records))
	}
	if records[0].IngressProtocol != "anthropic" || records[0].IngressEndpoint != "/v1/messages" {
		t.Fatalf("unexpected canonical record ingress: %#v", records[0])
	}
	if records[0].PrePolicyRequest.Model != "claude-3-5-sonnet" || records[0].PostPolicyRequest.Model != "rewritten-anthropic" {
		t.Fatalf("unexpected canonical record models: %#v", records[0])
	}
	if records[0].SessionSnapshot == nil || len(records[0].SessionSnapshot.Turns) != 2 {
		t.Fatalf("unexpected anthropic session snapshot: %#v", records[0].SessionSnapshot)
	}
	if records[0].Sidecar == nil || len(records[0].Sidecar.RawSSELines) != 2 {
		t.Fatalf("expected anthropic raw SSE lines in sidecar, got %#v", records[0].Sidecar)
	}
}

func TestCanonicalSessionSharedAcrossAnthropicAndOpenAI(t *testing.T) {
	sessionStore := proxy.NewSessionStore(30*time.Minute, 100, func() time.Time {
		return time.Unix(1, 0)
	})
	builder := &capturingBuilder{}
	handler := NewServer(Dependencies{
		Credentials: fakeCredentials{},
		Models:      &capturingModels{},
		Sessions:    sessionStore,
		Transport: fakeTransport{
			lines: []string{
				`data:{"body":"{\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}","statusCodeValue":200}`,
				`data:[DONE]`,
			},
		},
		Builder: builder,
		Now:     func() time.Time { return time.Unix(1, 0) },
	}, nil)

	anthropicBody := `{"model":"claude-3-5-sonnet","messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"abc123"}}]}]}`
	anthropicRequest := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(anthropicBody))
	anthropicRequest.Header.Set("Content-Type", "application/json")
	anthropicRequest.Header.Set("X-Session-Id", "shared-session")
	anthropicRecorder := httptest.NewRecorder()
	handler.ServeHTTP(anthropicRecorder, anthropicRequest)
	if anthropicRecorder.Code != http.StatusOK {
		t.Fatalf("expected anthropic 200, got %d: %s", anthropicRecorder.Code, anthropicRecorder.Body.String())
	}

	openAIBody := `{"model":"qwen","messages":[{"role":"user","content":"next"}]}`
	openAIRequest := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(openAIBody))
	openAIRequest.Header.Set("Content-Type", "application/json")
	openAIRequest.Header.Set("X-Session-Id", "shared-session")
	openAIRecorder := httptest.NewRecorder()
	handler.ServeHTTP(openAIRecorder, openAIRequest)
	if openAIRecorder.Code != http.StatusOK {
		t.Fatalf("expected openai 200, got %d: %s", openAIRecorder.Code, openAIRecorder.Body.String())
	}

	if len(builder.messages) != 3 {
		t.Fatalf("expected 3 southbound messages, got %#v", builder.messages)
	}
	if builder.messages[0].Role != "user" || !strings.Contains(builder.messages[0].Content, "data:image/png;base64,abc123") {
		t.Fatalf("expected prior anthropic image in southbound history, got %#v", builder.messages[0])
	}
	if builder.messages[2].Role != "user" || builder.messages[2].Content != "next" {
		t.Fatalf("expected latest openai turn last, got %#v", builder.messages[2])
	}
}

func boolPtr(v bool) *bool {
	return &v
}
