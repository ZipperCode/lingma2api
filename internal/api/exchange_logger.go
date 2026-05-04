package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"lingma2api/internal/db"
)

// Body size limits for truncation
const (
	maxDownstreamReqBody  = 2 << 20  // 2MB
	maxDownstreamRespBody = 1 << 20  // 1MB
	maxUpstreamReqBody    = 2 << 20  // 2MB
	maxUpstreamRespBody   = 2 << 20  // 2MB
	maxRawStreamSize      = 10 << 20 // 10MB
)

// ExchangeLogger wraps exchange recording for a single request lifecycle.
type ExchangeLogger struct {
	store     *db.Store
	logID     string
	startTime time.Time
}

func NewExchangeLogger(store *db.Store, logID string) *ExchangeLogger {
	return &ExchangeLogger{store: store, logID: logID, startTime: time.Now()}
}

func (el *ExchangeLogger) RecordDownstreamRequest(r *http.Request, body []byte) {
	if el.store == nil {
		return
	}
	_ = el.store.InsertHTTPExchange(context.Background(), &db.HTTPExchange{
		LogID:     el.logID,
		Direction: "downstream",
		Phase:     "request",
		Timestamp: time.Now(),
		Method:    r.Method,
		Path:      r.URL.Path,
		Headers:   headersToJSON(r.Header),
		Body:      truncateBody(string(body), maxDownstreamReqBody),
	})
}

func (el *ExchangeLogger) RecordUpstreamRequest(method, url string, headers http.Header, body string) {
	if el.store == nil {
		return
	}
	el.startTime = time.Now()
	_ = el.store.InsertHTTPExchange(context.Background(), &db.HTTPExchange{
		LogID:     el.logID,
		Direction: "upstream",
		Phase:     "request",
		Timestamp: time.Now(),
		Method:    method,
		URL:       url,
		Headers:   headersToJSON(headers),
		Body:      truncateBody(body, maxUpstreamReqBody),
	})
}

func (el *ExchangeLogger) RecordUpstreamResponse(statusCode int, headers http.Header, body string, rawStream string, err error) {
	if el.store == nil {
		return
	}
	duration := int(time.Since(el.startTime).Milliseconds())
	exchange := &db.HTTPExchange{
		LogID:      el.logID,
		Direction:  "upstream",
		Phase:      "response",
		Timestamp:  time.Now(),
		StatusCode: statusCode,
		Headers:    headersToJSON(headers),
		DurationMs: duration,
	}
	if err != nil {
		exchange.Error = err.Error()
	} else {
		exchange.Body = truncateBody(body, maxUpstreamRespBody)
		exchange.RawStream = truncateBody(rawStream, maxRawStreamSize)
	}
	_ = el.store.InsertHTTPExchange(context.Background(), exchange)
}

func (el *ExchangeLogger) RecordDownstreamResponse(statusCode int, headers http.Header, body string, start time.Time) {
	if el.store == nil {
		return
	}
	duration := int(time.Since(start).Milliseconds())
	_ = el.store.InsertHTTPExchange(context.Background(), &db.HTTPExchange{
		LogID:      el.logID,
		Direction:  "downstream",
		Phase:      "response",
		Timestamp:  time.Now(),
		StatusCode: statusCode,
		Headers:    headersToJSON(headers),
		Body:       truncateBody(body, maxDownstreamRespBody),
		DurationMs: duration,
	})
}

func headersToJSON(h http.Header) string {
	m := make(map[string][]string, len(h))
	for k, v := range h {
		m[k] = v
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func truncateBody(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + fmt.Sprintf("\n...[truncated, total: %d bytes]", len(s))
}
