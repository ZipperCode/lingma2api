package db

import (
	"context"
	"testing"
	"time"
)

func TestCleanupExpiredCanonicalRecords(t *testing.T) {
	store, cleanup := tempStore(t)
	defer cleanup()
	ctx := context.Background()

	// Insert old record
	_, err := store.db.ExecContext(ctx,
		`INSERT INTO canonical_execution_records (id, created_at, ingress_protocol, ingress_endpoint, pre_policy_json, post_policy_json)
		 VALUES ('old1', ?, 'openai', '/v1/chat/completions', '{}', '{}')`,
		time.Now().AddDate(0, 0, -10))
	if err != nil {
		t.Fatalf("insert old: %v", err)
	}
	// Insert recent record
	_, err = store.db.ExecContext(ctx,
		`INSERT INTO canonical_execution_records (id, created_at, ingress_protocol, ingress_endpoint, pre_policy_json, post_policy_json)
		 VALUES ('new1', ?, 'openai', '/v1/chat/completions', '{}', '{}')`,
		time.Now())
	if err != nil {
		t.Fatalf("insert new: %v", err)
	}

	affected, err := store.CleanupExpiredCanonicalRecords(ctx, 5)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if affected != 1 {
		t.Errorf("expected 1 deleted, got %d", affected)
	}

	var count int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM canonical_execution_records`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 remaining, got %d", count)
	}
}
