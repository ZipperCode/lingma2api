package db

import "lingma2api/internal/proxy"

func CanonicalRecordMappedModel(record CanonicalExecutionRecordRow) string {
	if record.PostPolicyRequest.Model != "" {
		return record.PostPolicyRequest.Model
	}
	return record.PrePolicyRequest.Model
}

func CanonicalRecordTokenCounts(record CanonicalExecutionRecordRow) (prompt, completion, total int) {
	if record.Sidecar == nil {
		return 0, 0, 0
	}
	prompt = canonicalMetadataInt(record.Sidecar.Metadata, "prompt_tokens")
	completion = canonicalMetadataInt(record.Sidecar.Metadata, "completion_tokens")
	total = canonicalMetadataInt(record.Sidecar.Metadata, "total_tokens")
	return prompt, completion, total
}

func CanonicalRecordUpstreamStatus(record CanonicalExecutionRecordRow) int {
	if record.Sidecar == nil {
		return 200
	}
	status := canonicalMetadataInt(record.Sidecar.Metadata, "upstream_status")
	if status == 0 {
		return 200
	}
	return status
}

func CanonicalRecordStatus(record CanonicalExecutionRecordRow) string {
	if CanonicalRecordUpstreamStatus(record) >= 400 {
		return "error"
	}
	if record.Sidecar != nil {
		if status, ok := record.Sidecar.Metadata["status"].(string); ok && status != "" {
			return status
		}
	}
	return "success"
}

func canonicalMetadataInt(metadata map[string]any, key string) int {
	if metadata == nil {
		return 0
	}
	switch value := metadata[key].(type) {
	case int:
		return value
	case int32:
		return int(value)
	case int64:
		return int(value)
	case float32:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func EstimateMessageTokens(messages []proxy.Message) int {
	totalChars := 0
	for _, message := range messages {
		totalChars += len(message.Role) + len(message.Name) + len(message.Content) + len(message.ToolCallID)
		for _, toolCall := range message.ToolCalls {
			totalChars += len(toolCall.ID) + len(toolCall.Type) + len(toolCall.Function.Name) + len(toolCall.Function.Arguments)
		}
	}
	if totalChars == 0 {
		return 0
	}
	return totalChars / 4
}
