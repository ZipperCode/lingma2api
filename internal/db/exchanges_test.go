package db

import (
	"context"
	"testing"
	"time"
)

func TestInsertAndGetHTTPExchanges(t *testing.T) {
	store, cleanup := tempStore(t)
	defer cleanup()
	ctx := context.Background()

	exchanges := []HTTPExchange{
		{LogID: "log-1", Direction: "downstream", Phase: "request", Timestamp: time.Now(), Method: "POST", Path: "/v1/chat/completions", Headers: `{"Content-Type":"application/json"}`, Body: `{"model":"gpt-4"}`},
		{LogID: "log-1", Direction: "upstream", Phase: "request", Timestamp: time.Now(), Method: "POST", URL: "https://example.com/chat", Headers: `{"Authorization":"Bearer xxx"}`, Body: `{"model":"qwen"}`},
		{LogID: "log-1", Direction: "upstream", Phase: "response", Timestamp: time.Now(), StatusCode: 200, Headers: `{"Content-Type":"application/json"}`, Body: `{"choices":[]}`, DurationMs: 1500, RawStream: "data: {}\n\n"},
	}

	for _, e := range exchanges {
		if err := store.InsertHTTPExchange(ctx, &e); err != nil {
			t.Fatalf("InsertHTTPExchange: %v", err)
		}
	}

	result, err := store.GetHTTPExchangesByLogID(ctx, "log-1")
	if err != nil {
		t.Fatalf("GetHTTPExchangesByLogID: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 exchanges, got %d", len(result))
	}
	if result[0].Method != "POST" {
		t.Fatalf("expected method POST, got %s", result[0].Method)
	}
}

func TestGetHTTPExchangesByLogID_NotFound(t *testing.T) {
	store, cleanup := tempStore(t)
	defer cleanup()
	ctx := context.Background()

	result, err := store.GetHTTPExchangesByLogID(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected 0 exchanges, got %d", len(result))
	}
}
