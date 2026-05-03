package middleware

// Deprecated: The Logging middleware is deprecated. Request logging is now handled
// by the canonical execution pipeline in internal/api/canonical_runtime.go.
// This middleware remains for backward compatibility but no longer writes to request_logs.

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"lingma2api/internal/db"
)

type LoggingConfig struct {
	StorageMode    string
	TruncateLength int
}

func Logging(dbInst *db.Store, cfg LoggingConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Deprecated: no longer intercepts or logs requests.
			// All request logging is handled by the canonical pipeline.
			next.ServeHTTP(w, r)
		})
	}
}

type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	buf        bytes.Buffer
}

func (r *responseRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	r.buf.Write(b)
	return r.ResponseWriter.Write(b)
}

func (r *responseRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func extractTokensFromResponse(log *db.RequestLog) {
	var resp struct {
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal([]byte(log.DownstreamResp), &resp) == nil && resp.Usage != nil {
		log.PromptTokens = resp.Usage.PromptTokens
		log.CompletionTokens = resp.Usage.CompletionTokens
		log.TotalTokens = resp.Usage.TotalTokens
		return
	}
	total := len(log.DownstreamResp) / 4
	if total > 0 {
		log.TotalTokens = total
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...[truncated]"
}

func generateID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b)
}
