package db

import (
	"context"
	"testing"
	"time"

	"lingma2api/internal/proxy"
)

func TestInsertAndGetCanonicalExecutionRecord(t *testing.T) {
	store, cleanup := tempStore(t)
	defer cleanup()

	ctx := context.Background()
	record := &CanonicalExecutionRecordRow{
		ID:              "cer-1",
		CreatedAt:       time.Unix(100, 0),
		IngressProtocol: "openai",
		IngressEndpoint: "/v1/chat/completions",
		SessionID:       "s1",
		PrePolicyRequest: proxy.CanonicalRequest{
			SchemaVersion: 1,
			Protocol:      proxy.CanonicalProtocolOpenAI,
			Model:         "auto",
			Stream:        false,
			SessionID:     "s1",
			Turns: []proxy.CanonicalTurn{{
				Role: "user",
				Blocks: []proxy.CanonicalContentBlock{{
					Type: proxy.CanonicalBlockText,
					Text: "hi",
				}},
			}},
		},
		PostPolicyRequest: proxy.CanonicalRequest{
			SchemaVersion: 1,
			Protocol:      proxy.CanonicalProtocolOpenAI,
			Model:         "rewritten",
			Stream:        false,
			SessionID:     "s1",
			Turns: []proxy.CanonicalTurn{{
				Role: "user",
				Blocks: []proxy.CanonicalContentBlock{{
					Type: proxy.CanonicalBlockText,
					Text: "hi",
				}},
			}},
		},
		SessionSnapshot: &proxy.CanonicalSessionSnapshot{
			SchemaVersion:   1,
			SessionID:       "s1",
			IngressProtocol: proxy.CanonicalProtocolOpenAI,
			UpdatedAt:       time.Unix(101, 0),
			Turns: []proxy.CanonicalTurn{
				{
					Role: "user",
					Blocks: []proxy.CanonicalContentBlock{{
						Type: proxy.CanonicalBlockText,
						Text: "hi",
					}},
				},
				{
					Role: "assistant",
					Blocks: []proxy.CanonicalContentBlock{{
						Type: proxy.CanonicalBlockText,
						Text: "hello",
					}},
				},
			},
		},
		SouthboundRequest: `{"request_id":"req-1"}`,
		Sidecar: &proxy.CanonicalExecutionSidecar{
			SchemaVersion: 1,
			Metadata: map[string]any{
				"request_id": "req-1",
			},
		},
	}
	if err := store.InsertCanonicalExecutionRecord(ctx, record); err != nil {
		t.Fatalf("InsertCanonicalExecutionRecord: %v", err)
	}

	got, err := store.GetCanonicalExecutionRecord(ctx, "cer-1")
	if err != nil {
		t.Fatalf("GetCanonicalExecutionRecord: %v", err)
	}
	if got.PostPolicyRequest.Model != "rewritten" {
		t.Fatalf("unexpected post-policy model %q", got.PostPolicyRequest.Model)
	}
	if got.SessionSnapshot == nil || len(got.SessionSnapshot.Turns) != 2 {
		t.Fatalf("unexpected session snapshot: %#v", got.SessionSnapshot)
	}
	if got.Sidecar == nil {
		t.Fatal("expected sidecar")
	}
}

func TestListCanonicalExecutionRecordsNewestFirst(t *testing.T) {
	store, cleanup := tempStore(t)
	defer cleanup()

	ctx := context.Background()
	for _, item := range []struct {
		id        string
		createdAt time.Time
		model     string
	}{
		{id: "older", createdAt: time.Unix(100, 0), model: "m1"},
		{id: "newer", createdAt: time.Unix(200, 0), model: "m2"},
	} {
		if err := store.InsertCanonicalExecutionRecord(ctx, &CanonicalExecutionRecordRow{
			ID:              item.id,
			CreatedAt:       item.createdAt,
			IngressProtocol: "openai",
			IngressEndpoint: "/v1/chat/completions",
			PrePolicyRequest: proxy.CanonicalRequest{
				SchemaVersion: 1,
				Protocol:      proxy.CanonicalProtocolOpenAI,
				Model:         item.model,
				Turns:         []proxy.CanonicalTurn{},
			},
			PostPolicyRequest: proxy.CanonicalRequest{
				SchemaVersion: 1,
				Protocol:      proxy.CanonicalProtocolOpenAI,
				Model:         item.model,
				Turns:         []proxy.CanonicalTurn{},
			},
		}); err != nil {
			t.Fatalf("InsertCanonicalExecutionRecord(%s): %v", item.id, err)
		}
	}

	items, err := store.ListCanonicalExecutionRecords(ctx, 10)
	if err != nil {
		t.Fatalf("ListCanonicalExecutionRecords: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 canonical records, got %d", len(items))
	}
	if items[0].ID != "newer" || items[1].ID != "older" {
		t.Fatalf("unexpected order: %#v", items)
	}
}
