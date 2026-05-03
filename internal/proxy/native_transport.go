package proxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// NativeTransport is a native HTTP client transport that replaces CurlTransport.
// It uses http.Client for northbound requests instead of spawning curl subprocesses.
type NativeTransport struct {
	baseURL string
	signer  *SignatureEngine
	timeout time.Duration
	client  *http.Client
	mu      sync.RWMutex
}

// NewNativeTransport creates a new NativeTransport.
// If timeout is <= 0, it defaults to 90 seconds.
func NewNativeTransport(baseURL string, signer *SignatureEngine, timeout time.Duration) *NativeTransport {
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	return &NativeTransport{
		baseURL: strings.TrimRight(baseURL, "/"),
		signer:  signer,
		timeout: timeout,
		client:  &http.Client{Timeout: timeout},
	}
}

// SetTimeout updates the transport timeout dynamically.
// It updates both the stored timeout and the http.Client timeout.
func (t *NativeTransport) SetTimeout(timeout time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	t.timeout = timeout
	t.client.Timeout = timeout
}

// StreamChat implements the ChatTransport interface.
// It sends a POST request with the given body and returns the response body as an io.ReadCloser.
// The caller is responsible for closing the returned io.ReadCloser.
// On HTTP status >= 400, it reads the response body and returns an UpstreamHTTPError.
func (t *NativeTransport) StreamChat(ctx context.Context, request RemoteChatRequest, credential CredentialSnapshot) (io.ReadCloser, error) {
	headers, err := t.signer.BuildHeaders(ctx, credential, request.Path, request.BodyJSON)
	if err != nil {
		return nil, err
	}

	var bodyReader io.Reader
	if request.BodyJSON != "" {
		bodyReader = strings.NewReader(request.BodyJSON)
	}

	url := t.baseURL + request.Path
	if request.Query != "" {
		url += request.Query
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bodyReader)
	if err != nil {
		return nil, err
	}

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	t.mu.RLock()
	client := t.client
	t.mu.RUnlock()

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, &UpstreamHTTPError{
			StatusCode: resp.StatusCode,
			Body:       strings.TrimSpace(string(bodyBytes)),
		}
	}

	return resp.Body, nil
}

// ListModels fetches the list of available models from the upstream.
// It sends a GET request to the model list endpoint and parses the JSON response.
// On HTTP status >= 400, it reads the response body and returns an UpstreamHTTPError.
func (t *NativeTransport) ListModels(ctx context.Context, credential CredentialSnapshot) ([]RemoteModel, error) {
	headers, err := t.signer.BuildHeaders(ctx, credential, ModelListPath, "")
	if err != nil {
		return nil, err
	}

	url := t.baseURL + ModelListPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	t.mu.RLock()
	client := t.client
	t.mu.RUnlock()

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, &UpstreamHTTPError{
			StatusCode: resp.StatusCode,
			Body:       strings.TrimSpace(string(bodyBytes)),
		}
	}

	var payload struct {
		Chat   []RemoteModel `json:"chat"`
		Inline []RemoteModel `json:"inline"`
	}
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		return nil, err
	}
	return append(payload.Chat, payload.Inline...), nil
}
