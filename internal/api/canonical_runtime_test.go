package api

import (
	"encoding/json"
	"testing"
	"time"

	"lingma2api/internal/proxy"
)

func TestBuildCanonicalSessionSnapshotPreservesCanonicalBlocks(t *testing.T) {
	now := time.Unix(100, 0)
	postPolicyRequest := proxy.CanonicalRequest{
		SchemaVersion: 1,
		Protocol:      proxy.CanonicalProtocolAnthropic,
		SessionID:     "session-1",
		Metadata: map[string]any{
			"client_name": "anthropic-client",
		},
		Turns: []proxy.CanonicalTurn{
			{
				Role: "user",
				Blocks: []proxy.CanonicalContentBlock{
					{
						Type: proxy.CanonicalBlockImage,
						Data: mustRawMessage(map[string]any{
							"media_type": "image/png",
							"data":       "abc123",
						}),
					},
				},
			},
		},
	}

	snapshot := buildCanonicalSessionSnapshot(
		proxy.CanonicalProtocolAnthropic,
		postPolicyRequest,
		proxy.Message{Role: "assistant", Content: "done"},
		now,
	)

	if snapshot == nil {
		t.Fatal("expected snapshot")
	}
	if snapshot.SessionID != "session-1" {
		t.Fatalf("SessionID = %q, want session-1", snapshot.SessionID)
	}
	if snapshot.IngressProtocol != proxy.CanonicalProtocolAnthropic {
		t.Fatalf("IngressProtocol = %q, want anthropic", snapshot.IngressProtocol)
	}
	if len(snapshot.Turns) != 2 {
		t.Fatalf("expected 2 turns, got %d", len(snapshot.Turns))
	}
	if got := snapshot.Turns[0].Blocks[0].Type; got != proxy.CanonicalBlockImage {
		t.Fatalf("first block type = %q, want image", got)
	}
	if got := snapshot.Turns[1].Blocks[0].Type; got != proxy.CanonicalBlockText {
		t.Fatalf("assistant block type = %q, want text", got)
	}
	if snapshot.UpdatedAt != now {
		t.Fatalf("UpdatedAt = %v, want %v", snapshot.UpdatedAt, now)
	}
}

func mustRawMessage(value any) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}
