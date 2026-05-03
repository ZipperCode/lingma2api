package proxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNativeTransport_StreamChat_Success(t *testing.T) {
	wantBody := `data: {"body":"{\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}","statusCodeValue":200}` + "\n\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}
		if auth := r.Header.Get("Authorization"); !strings.HasPrefix(auth, "Bearer COSY.") {
			t.Errorf("expected Authorization Bearer COSY..., got %s", auth)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(wantBody))
	}))
	defer server.Close()

	signer := NewSignatureEngine(SignatureOptions{CosyVersion: "2.11.2"})
	transport := NewNativeTransport(server.URL, signer, 30*time.Second)

	req := RemoteChatRequest{
		Path:     ChatPath,
		Query:    ChatQuery,
		BodyJSON: `{"model":"test"}`,
		Stream:   true,
	}
	cred := CredentialSnapshot{
		CosyKey:         "ck",
		EncryptUserInfo: "eui",
		UserID:          "uid",
		MachineID:       "mid",
	}

	reader, err := transport.StreamChat(context.Background(), req, cred)
	if err != nil {
		t.Fatalf("StreamChat() error = %v", err)
	}
	defer reader.Close()

	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(body) != wantBody {
		t.Fatalf("expected body %q, got %q", wantBody, string(body))
	}
}

func TestNativeTransport_StreamChat_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: ok\n\n"))
	}))
	defer server.Close()

	signer := NewSignatureEngine(SignatureOptions{CosyVersion: "2.11.2"})
	transport := NewNativeTransport(server.URL, signer, 100*time.Millisecond)

	req := RemoteChatRequest{
		Path:     ChatPath,
		Query:    ChatQuery,
		BodyJSON: `{"model":"test"}`,
		Stream:   true,
	}
	cred := CredentialSnapshot{
		CosyKey:         "ck",
		EncryptUserInfo: "eui",
		UserID:          "uid",
		MachineID:       "mid",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := transport.StreamChat(ctx, req, cred)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestNativeTransport_StreamChat_UpstreamError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer server.Close()

	signer := NewSignatureEngine(SignatureOptions{CosyVersion: "2.11.2"})
	transport := NewNativeTransport(server.URL, signer, 30*time.Second)

	req := RemoteChatRequest{
		Path:     ChatPath,
		Query:    ChatQuery,
		BodyJSON: `{"model":"test"}`,
		Stream:   true,
	}
	cred := CredentialSnapshot{
		CosyKey:         "ck",
		EncryptUserInfo: "eui",
		UserID:          "uid",
		MachineID:       "mid",
	}

	_, err := transport.StreamChat(context.Background(), req, cred)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	upstreamErr, ok := err.(*UpstreamHTTPError)
	if !ok {
		t.Fatalf("expected *UpstreamHTTPError, got %T", err)
	}
	if upstreamErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", upstreamErr.StatusCode)
	}
	if !strings.Contains(upstreamErr.Body, `{"error":"bad request"}`) {
		t.Fatalf("unexpected body: %s", upstreamErr.Body)
	}
}

func TestNativeTransport_ListModels_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if ct := r.Header.Get("Accept"); ct != "application/json" {
			t.Errorf("expected Accept application/json, got %s", ct)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"chat":[{"key":"k1","display_name":"Model 1","model":"m1","enable":true}],"inline":[]}`))
	}))
	defer server.Close()

	signer := NewSignatureEngine(SignatureOptions{CosyVersion: "2.11.2"})
	transport := NewNativeTransport(server.URL, signer, 30*time.Second)

	cred := CredentialSnapshot{
		CosyKey:         "ck",
		EncryptUserInfo: "eui",
		UserID:          "uid",
		MachineID:       "mid",
	}

	models, err := transport.ListModels(context.Background(), cred)
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}
	if models[0].Key != "k1" {
		t.Fatalf("expected key k1, got %s", models[0].Key)
	}
}

func TestNativeTransport_ListModels_UpstreamError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`unauthorized`))
	}))
	defer server.Close()

	signer := NewSignatureEngine(SignatureOptions{CosyVersion: "2.11.2"})
	transport := NewNativeTransport(server.URL, signer, 30*time.Second)

	cred := CredentialSnapshot{
		CosyKey:         "ck",
		EncryptUserInfo: "eui",
		UserID:          "uid",
		MachineID:       "mid",
	}

	_, err := transport.ListModels(context.Background(), cred)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	upstreamErr, ok := err.(*UpstreamHTTPError)
	if !ok {
		t.Fatalf("expected *UpstreamHTTPError, got %T", err)
	}
	if upstreamErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", upstreamErr.StatusCode)
	}
}

func TestNativeTransport_SetTimeout(t *testing.T) {
	signer := NewSignatureEngine(SignatureOptions{CosyVersion: "2.11.2"})
	transport := NewNativeTransport("http://example.com", signer, 30*time.Second)

	transport.SetTimeout(5 * time.Second)
	if transport.timeout != 5*time.Second {
		t.Fatalf("expected timeout 5s, got %v", transport.timeout)
	}
	if transport.client.Timeout != 5*time.Second {
		t.Fatalf("expected client.Timeout 5s, got %v", transport.client.Timeout)
	}
}

func TestNativeTransport_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Wait until context is cancelled
		select {
		case <-r.Context().Done():
			// client disconnected
		}
	}))
	defer server.Close()

	signer := NewSignatureEngine(SignatureOptions{CosyVersion: "2.11.2"})
	transport := NewNativeTransport(server.URL, signer, 30*time.Second)

	req := RemoteChatRequest{
		Path:     ChatPath,
		Query:    ChatQuery,
		BodyJSON: `{"model":"test"}`,
		Stream:   true,
	}
	cred := CredentialSnapshot{
		CosyKey:         "ck",
		EncryptUserInfo: "eui",
		UserID:          "uid",
		MachineID:       "mid",
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediately cancel

	_, err := transport.StreamChat(ctx, req, cred)
	if err == nil {
		t.Fatal("expected error due to cancelled context, got nil")
	}
}

func TestNativeTransport_StreamChat_RespectsPathAndQuery(t *testing.T) {
	var capturedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.RequestURI()
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: ok\n\n"))
	}))
	defer server.Close()

	signer := NewSignatureEngine(SignatureOptions{CosyVersion: "2.11.2"})
	transport := NewNativeTransport(server.URL, signer, 30*time.Second)

	req := RemoteChatRequest{
		Path:     "/algo/api/v2/service/pro/sse/agent_chat_generation",
		Query:    "?FetchKeys=llm_model_result&AgentId=agent_common",
		BodyJSON: `{"model":"test"}`,
		Stream:   true,
	}
	cred := CredentialSnapshot{
		CosyKey:         "ck",
		EncryptUserInfo: "eui",
		UserID:          "uid",
		MachineID:       "mid",
	}

	reader, err := transport.StreamChat(context.Background(), req, cred)
	if err != nil {
		t.Fatalf("StreamChat() error = %v", err)
	}
	defer reader.Close()

	expected := "/algo/api/v2/service/pro/sse/agent_chat_generation?FetchKeys=llm_model_result&AgentId=agent_common"
	if capturedPath != expected {
		t.Fatalf("expected path %q, got %q", expected, capturedPath)
	}
}

func TestNativeTransport_ListModels_RespectsModelListPath(t *testing.T) {
	var capturedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"chat":[],"inline":[]}`))
	}))
	defer server.Close()

	signer := NewSignatureEngine(SignatureOptions{CosyVersion: "2.11.2"})
	transport := NewNativeTransport(server.URL, signer, 30*time.Second)

	cred := CredentialSnapshot{
		CosyKey:         "ck",
		EncryptUserInfo: "eui",
		UserID:          "uid",
		MachineID:       "mid",
	}

	_, _ = transport.ListModels(context.Background(), cred)
	if capturedPath != ModelListPath {
		t.Fatalf("expected path %q, got %q", ModelListPath, capturedPath)
	}
}

func TestNativeTransport_StreamChat_ReadsAllSSEEvents(t *testing.T) {
	lines := []string{
		`data: {"body":"{\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}","statusCodeValue":200}`,
		"",
		`data: {"body":"{\"choices\":[{\"delta\":{\"content\":\" world\"}}]}","statusCodeValue":200}`,
		"",
		`data: [DONE]`,
		"",
	}
	body := strings.Join(lines, "\n") + "\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	signer := NewSignatureEngine(SignatureOptions{CosyVersion: "2.11.2"})
	transport := NewNativeTransport(server.URL, signer, 30*time.Second)

	req := RemoteChatRequest{
		Path:     ChatPath,
		Query:    ChatQuery,
		BodyJSON: `{"model":"test"}`,
		Stream:   true,
	}
	cred := CredentialSnapshot{
		CosyKey:         "ck",
		EncryptUserInfo: "eui",
		UserID:          "uid",
		MachineID:       "mid",
	}

	reader, err := transport.StreamChat(context.Background(), req, cred)
	if err != nil {
		t.Fatalf("StreamChat() error = %v", err)
	}
	defer reader.Close()

	// Use the existing SSE parser to verify the stream is readable
	content, err := CollectSSEContent(reader)
	if err != nil {
		t.Fatalf("CollectSSEContent() error = %v", err)
	}
	if content != "Hello world" {
		t.Fatalf("expected content %q, got %q", "Hello world", content)
	}
}

func TestNativeTransport_DefaultTimeout(t *testing.T) {
	signer := NewSignatureEngine(SignatureOptions{CosyVersion: "2.11.2"})
	transport := NewNativeTransport("http://example.com", signer, 0)
	if transport.timeout != 90*time.Second {
		t.Fatalf("expected default timeout 90s, got %v", transport.timeout)
	}
	if transport.client.Timeout != 90*time.Second {
		t.Fatalf("expected default client.Timeout 90s, got %v", transport.client.Timeout)
	}
}

func TestNativeTransport_StreamChat_EmptyBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if len(body) != 0 {
			t.Errorf("expected empty body, got %q", string(body))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: ok\n\n"))
	}))
	defer server.Close()

	signer := NewSignatureEngine(SignatureOptions{CosyVersion: "2.11.2"})
	transport := NewNativeTransport(server.URL, signer, 30*time.Second)

	req := RemoteChatRequest{
		Path:   ChatPath,
		Query:  ChatQuery,
		Stream: true,
	}
	cred := CredentialSnapshot{
		CosyKey:         "ck",
		EncryptUserInfo: "eui",
		UserID:          "uid",
		MachineID:       "mid",
	}

	reader, err := transport.StreamChat(context.Background(), req, cred)
	if err != nil {
		t.Fatalf("StreamChat() error = %v", err)
	}
	defer reader.Close()
}

func TestNativeTransport_ListModels_MergesChatAndInline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"chat":[{"key":"chat1","display_name":"Chat Model","model":"cm1","enable":true}],
			"inline":[{"key":"inline1","display_name":"Inline Model","model":"im1","enable":true}]
		}`))
	}))
	defer server.Close()

	signer := NewSignatureEngine(SignatureOptions{CosyVersion: "2.11.2"})
	transport := NewNativeTransport(server.URL, signer, 30*time.Second)

	cred := CredentialSnapshot{
		CosyKey:         "ck",
		EncryptUserInfo: "eui",
		UserID:          "uid",
		MachineID:       "mid",
	}

	models, err := transport.ListModels(context.Background(), cred)
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0].Key != "chat1" {
		t.Fatalf("expected first model key chat1, got %s", models[0].Key)
	}
	if models[1].Key != "inline1" {
		t.Fatalf("expected second model key inline1, got %s", models[1].Key)
	}
}

func TestNativeTransport_SignerError(t *testing.T) {
	// Use a signer that will fail because credential is empty
	signer := NewSignatureEngine(SignatureOptions{CosyVersion: "2.11.2"})
	transport := NewNativeTransport("http://example.com", signer, 30*time.Second)

	req := RemoteChatRequest{
		Path:     ChatPath,
		Query:    ChatQuery,
		BodyJSON: `{"model":"test"}`,
		Stream:   true,
	}
	// Empty credential should cause BuildHeaders to fail
	cred := CredentialSnapshot{}

	_, err := transport.StreamChat(context.Background(), req, cred)
	if err == nil {
		t.Fatal("expected error from signer, got nil")
	}

	_, err = transport.ListModels(context.Background(), cred)
	if err == nil {
		t.Fatal("expected error from signer, got nil")
	}
}

func TestNativeTransport_StreamChat_ContextTimeoutDuringBodyRead(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected flusher")
		}
		_, _ = w.Write([]byte("data: ok\n\n"))
		flusher.Flush()
		// Sleep longer than the client context timeout
		time.Sleep(2 * time.Second)
		_, _ = w.Write([]byte("data: done\n\n"))
		flusher.Flush()
	}))
	defer server.Close()

	signer := NewSignatureEngine(SignatureOptions{CosyVersion: "2.11.2"})
	transport := NewNativeTransport(server.URL, signer, 30*time.Second)

	req := RemoteChatRequest{
		Path:     ChatPath,
		Query:    ChatQuery,
		BodyJSON: `{"model":"test"}`,
		Stream:   true,
	}
	cred := CredentialSnapshot{
		CosyKey:         "ck",
		EncryptUserInfo: "eui",
		UserID:          "uid",
		MachineID:       "mid",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	reader, err := transport.StreamChat(ctx, req, cred)
	if err != nil {
		t.Fatalf("StreamChat() error = %v", err)
	}
	defer reader.Close()

	// Read the first chunk
	buf := make([]byte, 64)
	_, err = reader.Read(buf)
	if err != nil {
		t.Fatalf("first Read() error = %v", err)
	}

	// The second read should fail due to context timeout
	_, err = reader.Read(buf)
	if err == nil {
		t.Fatal("expected timeout error on second read, got nil")
	}
}

func TestNativeTransport_ListModels_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not json`))
	}))
	defer server.Close()

	signer := NewSignatureEngine(SignatureOptions{CosyVersion: "2.11.2"})
	transport := NewNativeTransport(server.URL, signer, 30*time.Second)

	cred := CredentialSnapshot{
		CosyKey:         "ck",
		EncryptUserInfo: "eui",
		UserID:          "uid",
		MachineID:       "mid",
	}

	_, err := transport.ListModels(context.Background(), cred)
	if err == nil {
		t.Fatal("expected JSON parse error, got nil")
	}
}

func TestNativeTransport_StreamChat_HeadersIncludeExpectedValues(t *testing.T) {
	var headers http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = r.Header
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: ok\n\n"))
	}))
	defer server.Close()

	signer := NewSignatureEngine(SignatureOptions{CosyVersion: "2.11.2"})
	transport := NewNativeTransport(server.URL, signer, 30*time.Second)

	req := RemoteChatRequest{
		Path:     ChatPath,
		Query:    ChatQuery,
		BodyJSON: `{"model":"test"}`,
		Stream:   true,
	}
	cred := CredentialSnapshot{
		CosyKey:         "ck",
		EncryptUserInfo: "eui",
		UserID:          "uid",
		MachineID:       "mid",
	}

	reader, err := transport.StreamChat(context.Background(), req, cred)
	if err != nil {
		t.Fatalf("StreamChat() error = %v", err)
	}
	defer reader.Close()

	if headers.Get("Appcode") != "cosy" {
		t.Fatalf("expected Appcode=cosy, got %s", headers.Get("Appcode"))
	}
	if headers.Get("Cosy-Key") != "ck" {
		t.Fatalf("expected Cosy-Key=ck, got %s", headers.Get("Cosy-Key"))
	}
	if headers.Get("Cosy-User") != "uid" {
		t.Fatalf("expected Cosy-User=uid, got %s", headers.Get("Cosy-User"))
	}
	if headers.Get("Cosy-Machineid") != "mid" {
		t.Fatalf("expected Cosy-Machineid=mid, got %s", headers.Get("Cosy-Machineid"))
	}
	if headers.Get("Accept") != "text/event-stream" {
		t.Fatalf("expected Accept=text/event-stream, got %s", headers.Get("Accept"))
	}
	if headers.Get("Cache-Control") != "no-cache" {
		t.Fatalf("expected Cache-Control=no-cache, got %s", headers.Get("Cache-Control"))
	}
}

func TestNativeTransport_ListModels_HeadersIncludeExpectedValues(t *testing.T) {
	var headers http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = r.Header
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"chat":[],"inline":[]}`))
	}))
	defer server.Close()

	signer := NewSignatureEngine(SignatureOptions{CosyVersion: "2.11.2"})
	transport := NewNativeTransport(server.URL, signer, 30*time.Second)

	cred := CredentialSnapshot{
		CosyKey:         "ck",
		EncryptUserInfo: "eui",
		UserID:          "uid",
		MachineID:       "mid",
	}

	_, _ = transport.ListModels(context.Background(), cred)

	if headers.Get("Accept") != "application/json" {
		t.Fatalf("expected Accept=application/json, got %s", headers.Get("Accept"))
	}
	// When body is empty, there should be no Cache-Control header
	if headers.Get("Cache-Control") != "" {
		t.Fatalf("expected no Cache-Control header for empty body, got %s", headers.Get("Cache-Control"))
	}
}

func TestNativeTransport_StreamChat_RequestID(t *testing.T) {
	var capturedReqID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// We can't directly capture requestID, but we can verify the request succeeds
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fmt.Sprintf("data: ok\n\n")))
		_ = capturedReqID
	}))
	defer server.Close()

	signer := NewSignatureEngine(SignatureOptions{CosyVersion: "2.11.2"})
	transport := NewNativeTransport(server.URL, signer, 30*time.Second)

	req := RemoteChatRequest{
		Path:      ChatPath,
		Query:     ChatQuery,
		BodyJSON:  `{"model":"test"}`,
		RequestID: "req-123",
		Stream:    true,
	}
	cred := CredentialSnapshot{
		CosyKey:         "ck",
		EncryptUserInfo: "eui",
		UserID:          "uid",
		MachineID:       "mid",
	}

	reader, err := transport.StreamChat(context.Background(), req, cred)
	if err != nil {
		t.Fatalf("StreamChat() error = %v", err)
	}
	defer reader.Close()
}
