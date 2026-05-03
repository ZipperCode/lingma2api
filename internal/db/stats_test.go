package db

import (
	"context"
	"testing"
	"time"

	"lingma2api/internal/proxy"
)

func TestGetDashboardDataPrefersCanonicalExecutionRecords(t *testing.T) {
	store, cleanup := tempStore(t)
	defer cleanup()

	now := time.Now()
	insertCanonicalRecordForStats(t, store, "r1", now.Add(-2*time.Hour), "openai", "gpt-4o", "qwen-max", 100, 40, 60, 321)
	insertCanonicalRecordForStats(t, store, "r2", now.Add(-1*time.Hour), "anthropic", "claude-3-5-sonnet", "qwen-max", 80, 30, 50, 123)
	insertCanonicalRecordForStats(t, store, "old", now.Add(-48*time.Hour), "openai", "old-model", "old-model", 999, 1, 1, 1)

	data, err := store.GetDashboardData(context.Background(), "24h")
	if err != nil {
		t.Fatalf("GetDashboardData() error = %v", err)
	}
	if data.Stats.TotalRequests != 2 {
		t.Fatalf("expected 2 requests, got %d", data.Stats.TotalRequests)
	}
	if data.Stats.SuccessRate != 100 {
		t.Fatalf("expected 100 success rate, got %v", data.Stats.SuccessRate)
	}
	if data.Stats.AvgTTFTMs != 222 {
		t.Fatalf("expected avg ttft 222, got %d", data.Stats.AvgTTFTMs)
	}
	if data.Stats.TotalTokens != 180 {
		t.Fatalf("expected total tokens 180, got %d", data.Stats.TotalTokens)
	}
	if len(data.SuccessRateSeries) == 0 {
		t.Fatal("expected success rate series")
	}
	if len(data.TokenSeries) == 0 {
		t.Fatal("expected token series")
	}
	if len(data.ModelDistribution) != 1 || data.ModelDistribution[0].Model != "qwen-max" || data.ModelDistribution[0].Count != 2 {
		t.Fatalf("unexpected model distribution: %#v", data.ModelDistribution)
	}
}

func TestGetTokenStatsPrefersCanonicalExecutionRecords(t *testing.T) {
	store, cleanup := tempStore(t)
	defer cleanup()

	now := time.Now()
	insertCanonicalRecordForStats(t, store, "today", now.Add(-2*time.Hour), "openai", "gpt-4o", "qwen-max", 90, 20, 30, 50)
	insertCanonicalRecordForStats(t, store, "week", now.AddDate(0, 0, -3), "openai", "gpt-4o", "qwen-max", 60, 10, 20, 40)
	insertCanonicalRecordForStats(t, store, "old", now.AddDate(0, 0, -10), "openai", "gpt-4o", "qwen-max", 30, 5, 10, 20)

	today, week, total, err := store.GetTokenStats(context.Background())
	if err != nil {
		t.Fatalf("GetTokenStats() error = %v", err)
	}
	if today != 90 {
		t.Fatalf("expected today 90, got %d", today)
	}
	if week != 150 {
		t.Fatalf("expected week 150, got %d", week)
	}
	if total != 180 {
		t.Fatalf("expected total 180, got %d", total)
	}
}

func insertCanonicalRecordForStats(
	t *testing.T,
	store *Store,
	id string,
	createdAt time.Time,
	protocol string,
	model string,
	mappedModel string,
	totalTokens int,
	promptTokens int,
	completionTokens int,
	ttftMs int,
) {
	t.Helper()
	err := store.InsertCanonicalExecutionRecord(context.Background(), &CanonicalExecutionRecordRow{
		ID:              id,
		CreatedAt:       createdAt,
		IngressProtocol: protocol,
		IngressEndpoint: "/v1/chat/completions",
		PrePolicyRequest: proxy.CanonicalRequest{
			SchemaVersion: 1,
			Protocol:      proxy.CanonicalProtocol(protocol),
			Model:         model,
			Turns:         []proxy.CanonicalTurn{},
		},
		PostPolicyRequest: proxy.CanonicalRequest{
			SchemaVersion: 1,
			Protocol:      proxy.CanonicalProtocol(protocol),
			Model:         mappedModel,
			Turns:         []proxy.CanonicalTurn{},
		},
		Sidecar: &proxy.CanonicalExecutionSidecar{
			SchemaVersion: 1,
			TTFTMs:        ttftMs,
			Metadata: map[string]any{
				"prompt_tokens":     promptTokens,
				"completion_tokens": completionTokens,
				"total_tokens":      totalTokens,
				"upstream_status":   200,
			},
		},
	})
	if err != nil {
		t.Fatalf("InsertCanonicalExecutionRecord(%s) error = %v", id, err)
	}
}
