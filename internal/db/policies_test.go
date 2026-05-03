package db

import (
	"context"
	"testing"
)

func TestPolicyCRUD(t *testing.T) {
	store, cleanup := tempStore(t)
	defer cleanup()

	ctx := context.Background()
	rewriteModel := "dashscope_qwen_max_latest"
	setReasoning := true
	policy := &PolicyRule{
		Priority: 10,
		Name:     "rewrite-openai-gpt4",
		Enabled:  true,
		Match: PolicyMatch{
			Protocol:       "openai",
			RequestedModel: "^gpt-4",
		},
		Actions: PolicyActions{
			RewriteModel: &rewriteModel,
			SetReasoning: &setReasoning,
			AddTags:      []string{"compat", "rewrite"},
		},
	}
	if err := store.CreatePolicy(ctx, policy); err != nil {
		t.Fatalf("CreatePolicy: %v", err)
	}
	if policy.ID == 0 {
		t.Fatal("expected created policy id")
	}

	items, err := store.ListPolicies(ctx)
	if err != nil {
		t.Fatalf("ListPolicies: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 policy, got %d", len(items))
	}
	if items[0].Match.Protocol != "openai" {
		t.Fatalf("unexpected protocol: %q", items[0].Match.Protocol)
	}
	if items[0].Actions.RewriteModel == nil || *items[0].Actions.RewriteModel != rewriteModel {
		t.Fatalf("unexpected rewrite_model: %#v", items[0].Actions.RewriteModel)
	}

	policy.Name = "rewrite-openai-gpt4-v2"
	policy.Enabled = false
	if err := store.UpdatePolicy(ctx, policy); err != nil {
		t.Fatalf("UpdatePolicy: %v", err)
	}
	enabled, err := store.GetEnabledPolicies(ctx)
	if err != nil {
		t.Fatalf("GetEnabledPolicies: %v", err)
	}
	if len(enabled) != 0 {
		t.Fatalf("expected no enabled policies, got %d", len(enabled))
	}

	if err := store.DeletePolicy(ctx, policy.ID); err != nil {
		t.Fatalf("DeletePolicy: %v", err)
	}
	items, err = store.ListPolicies(ctx)
	if err != nil {
		t.Fatalf("ListPolicies after delete: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 policies after delete, got %d", len(items))
	}
}
