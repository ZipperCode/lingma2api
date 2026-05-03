package policy

import (
	"testing"

	"lingma2api/internal/db"
	"lingma2api/internal/proxy"
)

func TestEvaluateCanonicalRequestMergesActionsByCategory(t *testing.T) {
	rewriteOne := "model-a"
	rewriteTwo := "model-b"
	reasoningFalse := false
	reasoningTrue := true
	allowToolsFalse := false

	rules := []db.PolicyRule{
		{
			ID:       1,
			Priority: 10,
			Name:     "rewrite-first",
			Enabled:  true,
			Match: db.PolicyMatch{
				Protocol:       "anthropic",
				RequestedModel: "^claude",
			},
			Actions: db.PolicyActions{
				RewriteModel: &rewriteOne,
				AddTags:      []string{"rewrite"},
			},
		},
		{
			ID:       2,
			Priority: 20,
			Name:     "disable-tools",
			Enabled:  true,
			Match: db.PolicyMatch{
				Protocol: "anthropic",
			},
			Actions: db.PolicyActions{
				RewriteModel: &rewriteTwo,
				SetReasoning: &reasoningFalse,
				AllowTools:   &allowToolsFalse,
				AddTags:      []string{"tools-off"},
			},
		},
		{
			ID:       3,
			Priority: 30,
			Name:     "late-reasoning",
			Enabled:  true,
			Match: db.PolicyMatch{
				Protocol: "anthropic",
			},
			Actions: db.PolicyActions{
				SetReasoning: &reasoningTrue,
				AddTags:      []string{"late"},
			},
		},
	}

	input := proxy.CanonicalRequest{
		SchemaVersion: 1,
		Protocol:      proxy.CanonicalProtocolAnthropic,
		Model:         "claude-3-5-sonnet",
		Stream:        true,
		HasTools:      true,
		HasReasoning:  true,
		Tools: []proxy.CanonicalToolDefinition{{
			Name: "lookup_doc",
		}},
		ToolChoice: map[string]any{"type": "auto"},
		Metadata: map[string]any{
			"client_name": "anthropic-sdk",
		},
	}

	result, err := EvaluateCanonicalRequest(rules, input)
	if err != nil {
		t.Fatalf("EvaluateCanonicalRequest() error = %v", err)
	}
	if !result.Matched {
		t.Fatal("expected policy match")
	}
	if result.EffectiveActions.RewriteModel == nil || *result.EffectiveActions.RewriteModel != "model-a" {
		t.Fatalf("unexpected rewrite action: %#v", result.EffectiveActions.RewriteModel)
	}
	if result.EffectiveActions.SetReasoning == nil || *result.EffectiveActions.SetReasoning {
		t.Fatalf("unexpected reasoning action: %#v", result.EffectiveActions.SetReasoning)
	}
	if result.EffectiveActions.AllowTools == nil || *result.EffectiveActions.AllowTools {
		t.Fatalf("unexpected allow_tools action: %#v", result.EffectiveActions.AllowTools)
	}
	if got := result.EffectiveActions.AddTags; len(got) != 3 || got[0] != "rewrite" || got[1] != "tools-off" || got[2] != "late" {
		t.Fatalf("unexpected tags: %#v", got)
	}
	if len(result.MatchedRules) != 3 {
		t.Fatalf("expected 3 matched rules, got %d", len(result.MatchedRules))
	}
	if len(result.MatchedRules[1].Suppressed) != 1 || result.MatchedRules[1].Suppressed[0] != "rewrite_model" {
		t.Fatalf("unexpected suppressed actions: %#v", result.MatchedRules[1].Suppressed)
	}
	if len(result.MatchedRules[2].Suppressed) != 1 || result.MatchedRules[2].Suppressed[0] != "set_reasoning" {
		t.Fatalf("unexpected suppressed actions on rule 3: %#v", result.MatchedRules[2].Suppressed)
	}
	if result.PostPolicyRequest.Model != "model-a" {
		t.Fatalf("unexpected post-policy model %q", result.PostPolicyRequest.Model)
	}
	if result.PostPolicyRequest.HasReasoning {
		t.Fatalf("expected reasoning disabled in post-policy request: %#v", result.PostPolicyRequest)
	}
	if result.PostPolicyRequest.HasTools {
		t.Fatalf("expected tools disabled in post-policy request: %#v", result.PostPolicyRequest)
	}
	if len(result.PostPolicyRequest.Tools) != 0 || result.PostPolicyRequest.ToolChoice != nil {
		t.Fatalf("expected tool configuration cleared, got %#v", result.PostPolicyRequest)
	}
}
