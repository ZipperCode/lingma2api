package db

import (
	"context"
	"testing"
	"time"
)

func TestMigrateBackfillsModelMappingsIntoPolicyRules(t *testing.T) {
	store, cleanup := tempStore(t)
	defer cleanup()

	ctx := context.Background()
	legacyCreated := time.Unix(100, 0)
	if _, err := store.db.ExecContext(ctx,
		`INSERT INTO model_mappings (priority,name,pattern,target,enabled,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?)`,
		7, "legacy auto", "^auto$", "qwen3-coder", 1, legacyCreated, legacyCreated,
	); err != nil {
		t.Fatalf("insert legacy mapping: %v", err)
	}

	if err := store.Migrate(); err != nil {
		t.Fatalf("second Migrate() error = %v", err)
	}

	policies, err := store.ListPolicies(ctx)
	if err != nil {
		t.Fatalf("ListPolicies() error = %v", err)
	}
	if len(policies) != 1 {
		t.Fatalf("expected 1 backfilled policy, got %#v", policies)
	}
	policy := policies[0]
	if policy.Source != "model_mapping" {
		t.Fatalf("Source = %q, want model_mapping", policy.Source)
	}
	if policy.Priority != 7 || policy.Name != "legacy auto" || !policy.Enabled {
		t.Fatalf("unexpected policy metadata: %#v", policy)
	}
	if policy.Match.RequestedModel != "^auto$" {
		t.Fatalf("RequestedModel = %q, want ^auto$", policy.Match.RequestedModel)
	}
	if policy.Actions.RewriteModel == nil || *policy.Actions.RewriteModel != "qwen3-coder" {
		t.Fatalf("unexpected rewrite model: %#v", policy.Actions.RewriteModel)
	}

	if err := store.Migrate(); err != nil {
		t.Fatalf("third Migrate() error = %v", err)
	}
	policies, err = store.ListPolicies(ctx)
	if err != nil {
		t.Fatalf("ListPolicies() after third migrate error = %v", err)
	}
	if len(policies) != 1 {
		t.Fatalf("expected idempotent backfill, got %#v", policies)
	}
}
