package policy

import (
	"fmt"
	"regexp"

	"lingma2api/internal/db"
	"lingma2api/internal/proxy"
)

type MatchAttributes struct {
	Protocol       string `json:"protocol"`
	RequestedModel string `json:"requested_model"`
	Stream         bool   `json:"stream"`
	HasTools       bool   `json:"has_tools"`
	HasReasoning   bool   `json:"has_reasoning"`
	SessionPresent bool   `json:"session_present"`
	ClientName     string `json:"client_name"`
	IngressTag     string `json:"ingress_tag"`
}

type EvaluatedRule struct {
	ID         int              `json:"id"`
	Name       string           `json:"name"`
	Priority   int              `json:"priority"`
	Applied    db.PolicyActions `json:"applied"`
	Suppressed []string         `json:"suppressed,omitempty"`
}

type EvaluationResult struct {
	Matched           bool                   `json:"matched"`
	EffectiveActions  db.PolicyActions       `json:"effective_actions"`
	MatchedRules      []EvaluatedRule        `json:"matched_rules"`
	PostPolicyRequest proxy.CanonicalRequest `json:"post_policy_request"`
}

func AttributesFromCanonicalRequest(req proxy.CanonicalRequest) MatchAttributes {
	return MatchAttributes{
		Protocol:       string(req.Protocol),
		RequestedModel: req.Model,
		Stream:         req.Stream,
		HasTools:       req.HasTools,
		HasReasoning:   req.HasReasoning,
		SessionPresent: req.SessionID != "",
		ClientName:     metadataString(req.Metadata, "client_name"),
		IngressTag:     metadataString(req.Metadata, "ingress_tag"),
	}
}

func EvaluateCanonicalRequest(rules []db.PolicyRule, req proxy.CanonicalRequest) (EvaluationResult, error) {
	result, err := EvaluateMatchAttributes(rules, AttributesFromCanonicalRequest(req))
	if err != nil {
		return EvaluationResult{}, err
	}
	result.PostPolicyRequest = applyActions(req, result.EffectiveActions)
	return result, nil
}

func EvaluateMatchAttributes(rules []db.PolicyRule, attrs MatchAttributes) (EvaluationResult, error) {
	result := EvaluationResult{
		EffectiveActions: db.PolicyActions{},
	}
	rewriteTaken := false
	reasoningTaken := false
	toolsTaken := false

	for _, rule := range rules {
		matched, err := ruleMatches(rule.Match, attrs)
		if err != nil {
			return EvaluationResult{}, fmt.Errorf("policy %q match failed: %w", rule.Name, err)
		}
		if !matched {
			continue
		}

		entry := EvaluatedRule{
			ID:       rule.ID,
			Name:     rule.Name,
			Priority: rule.Priority,
		}
		if rule.Actions.RewriteModel != nil {
			if !rewriteTaken {
				rewriteTaken = true
				result.EffectiveActions.RewriteModel = rule.Actions.RewriteModel
				entry.Applied.RewriteModel = rule.Actions.RewriteModel
			} else {
				entry.Suppressed = append(entry.Suppressed, "rewrite_model")
			}
		}
		if rule.Actions.SetReasoning != nil {
			if !reasoningTaken {
				reasoningTaken = true
				result.EffectiveActions.SetReasoning = rule.Actions.SetReasoning
				entry.Applied.SetReasoning = rule.Actions.SetReasoning
			} else {
				entry.Suppressed = append(entry.Suppressed, "set_reasoning")
			}
		}
		if rule.Actions.AllowTools != nil {
			if !toolsTaken {
				toolsTaken = true
				result.EffectiveActions.AllowTools = rule.Actions.AllowTools
				entry.Applied.AllowTools = rule.Actions.AllowTools
			} else {
				entry.Suppressed = append(entry.Suppressed, "allow_tools")
			}
		}
		if len(rule.Actions.AddTags) > 0 {
			result.EffectiveActions.AddTags = append(result.EffectiveActions.AddTags, rule.Actions.AddTags...)
			entry.Applied.AddTags = append(entry.Applied.AddTags, rule.Actions.AddTags...)
		}
		result.MatchedRules = append(result.MatchedRules, entry)
	}

	result.Matched = len(result.MatchedRules) > 0
	return result, nil
}

func ruleMatches(match db.PolicyMatch, attrs MatchAttributes) (bool, error) {
	if match.Protocol != "" && match.Protocol != attrs.Protocol {
		return false, nil
	}
	if match.RequestedModel != "" {
		ok, err := regexp.MatchString(match.RequestedModel, attrs.RequestedModel)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	if match.Stream != nil && *match.Stream != attrs.Stream {
		return false, nil
	}
	if match.HasTools != nil && *match.HasTools != attrs.HasTools {
		return false, nil
	}
	if match.HasReasoning != nil && *match.HasReasoning != attrs.HasReasoning {
		return false, nil
	}
	if match.SessionPresent != nil && *match.SessionPresent != attrs.SessionPresent {
		return false, nil
	}
	if match.ClientName != "" && match.ClientName != attrs.ClientName {
		return false, nil
	}
	if match.IngressTag != "" && match.IngressTag != attrs.IngressTag {
		return false, nil
	}
	return true, nil
}

func applyActions(req proxy.CanonicalRequest, actions db.PolicyActions) proxy.CanonicalRequest {
	cloned := req
	cloned.Metadata = cloneMetadata(req.Metadata)
	cloned.Tools = append([]proxy.CanonicalToolDefinition(nil), req.Tools...)

	if actions.RewriteModel != nil {
		cloned.Model = *actions.RewriteModel
	}
	if actions.SetReasoning != nil {
		cloned.HasReasoning = *actions.SetReasoning
	}
	if actions.AllowTools != nil {
		cloned.HasTools = *actions.AllowTools
		if !*actions.AllowTools {
			cloned.Tools = nil
			cloned.ToolChoice = nil
		}
	}
	if len(actions.AddTags) > 0 {
		existing := stringSliceMetadata(cloned.Metadata, "policy_tags")
		cloned.Metadata["policy_tags"] = append(existing, actions.AddTags...)
	}
	return cloned
}

func cloneMetadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

func stringSliceMetadata(metadata map[string]any, key string) []string {
	value, ok := metadata[key]
	if !ok {
		return nil
	}
	raw, ok := value.([]string)
	if ok {
		return append([]string(nil), raw...)
	}
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if ok {
			out = append(out, text)
		}
	}
	return out
}

func metadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	value, _ := metadata[key].(string)
	return value
}
