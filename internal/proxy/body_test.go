package proxy

import (
	"encoding/json"
	"testing"
	"time"
)

func TestBodyBuilderBuildsRemoteRequest(t *testing.T) {
	temperature := 0.2
	builder := NewBodyBuilder("2.11.2", func() time.Time { return time.UnixMilli(10) }, func() string {
		return "uuid-1"
	}, func() string {
		return "hex-1"
	})

	request, err := builder.BuildCanonical(CanonicalRequest{
		SchemaVersion: 1,
		Protocol:      CanonicalProtocolOpenAI,
		Model:         "auto",
		Stream:        true,
		Temperature:   &temperature,
		Turns: []CanonicalTurn{
			{Role: "user", Blocks: []CanonicalContentBlock{{Type: CanonicalBlockText, Text: "hi"}}},
		},
	}, "")
	if err != nil {
		t.Fatalf("BuildCanonical() error = %v", err)
	}

	if request.Path != ChatPath {
		t.Fatalf("expected chat path, got %q", request.Path)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(request.BodyJSON), &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if payload["request_id"] != "hex-1" {
		t.Fatalf("expected fixed request_id, got %#v", payload["request_id"])
	}
}

func TestBodyBuilderPreservesToolCallsInMessages(t *testing.T) {
	builder := NewBodyBuilder("2.11.2", func() time.Time { return time.UnixMilli(10) }, func() string {
		return "uuid-1"
	}, func() string {
		return "hex-1"
	})

	request, err := builder.BuildCanonical(CanonicalRequest{
		SchemaVersion: 1,
		Protocol:      CanonicalProtocolOpenAI,
		Model:         "auto",
		Stream:        true,
		Turns: []CanonicalTurn{
			{Role: "user", Blocks: []CanonicalContentBlock{{Type: CanonicalBlockText, Text: "read main.go"}}},
			{
				Role: "assistant",
				Blocks: []CanonicalContentBlock{{
					Type:     CanonicalBlockToolCall,
					ToolCall: &CanonicalToolCall{ID: "c1", Name: "read_file", Arguments: `{"path":"main.go"}`},
				}},
			},
			{
				Role: "tool",
				Blocks: []CanonicalContentBlock{{
					Type:       CanonicalBlockToolResult,
					ToolResult: &CanonicalToolResult{ToolCallID: "c1", Content: "package main\nfunc main() {}"},
				}},
			},
		},
	}, "")
	if err != nil {
		t.Fatalf("BuildCanonical() error = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(request.BodyJSON), &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	msgs, ok := payload["messages"].([]any)
	if !ok || len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %#v", payload["messages"])
	}

	assistant, ok := msgs[1].(map[string]any)
	if !ok {
		t.Fatal("assistant message not a map")
	}
	toolCalls, ok := assistant["tool_calls"].([]any)
	if !ok || len(toolCalls) != 1 {
		t.Fatalf("expected 1 tool_call in assistant message, got %#v", assistant["tool_calls"])
	}
	tc := toolCalls[0].(map[string]any)
	if tc["id"] != "c1" {
		t.Fatalf("expected tool_call id c1, got %q", tc["id"])
	}

	toolMsg, ok := msgs[2].(map[string]any)
	if !ok {
		t.Fatal("tool message not a map")
	}
	toolCallID, ok := toolMsg["tool_call_id"].(string)
	if !ok || toolCallID != "c1" {
		t.Fatalf("expected tool_call_id c1, got %#v", toolMsg["tool_call_id"])
	}
}

func TestBodyBuilderBuildCanonicalBuildsRemoteRequest(t *testing.T) {
	temperature := 0.2
	builder := NewBodyBuilder("2.11.2", func() time.Time { return time.UnixMilli(10) }, func() string {
		return "uuid-1"
	}, func() string {
		return "hex-1"
	})

	request, err := builder.BuildCanonical(CanonicalRequest{
		SchemaVersion: 1,
		Protocol:      CanonicalProtocolAnthropic,
		Model:         "auto",
		Stream:        true,
		Temperature:   &temperature,
		HasReasoning:  true,
		Turns: []CanonicalTurn{
			{Role: "user", Blocks: []CanonicalContentBlock{{Type: CanonicalBlockText, Text: "hi"}}},
		},
	}, "qwen-key")
	if err != nil {
		t.Fatalf("BuildCanonical() error = %v", err)
	}
	if request.Path != ChatPath {
		t.Fatalf("expected chat path, got %q", request.Path)
	}
	if request.ModelKey != "qwen-key" {
		t.Fatalf("expected model key qwen-key, got %q", request.ModelKey)
	}
	if !request.Stream {
		t.Fatal("expected stream request")
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(request.BodyJSON), &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if payload["request_id"] != "hex-1" {
		t.Fatalf("expected fixed request_id, got %#v", payload["request_id"])
	}
	modelConfig := payload["model_config"].(map[string]any)
	if modelConfig["key"] != "qwen-key" || modelConfig["is_reasoning"] != true {
		t.Fatalf("unexpected model_config: %#v", modelConfig)
	}
}

func TestBodyBuilderBuildCanonicalPreservesStructuredBlocksAndToolCalls(t *testing.T) {
	builder := NewBodyBuilder("2.11.2", func() time.Time { return time.UnixMilli(10) }, func() string {
		return "uuid-1"
	}, func() string {
		return "hex-1"
	})

	request, err := builder.BuildCanonical(CanonicalRequest{
		SchemaVersion: 1,
		Protocol:      CanonicalProtocolAnthropic,
		Model:         "auto",
		Stream:        true,
		Turns: []CanonicalTurn{
			{
				Role: "user",
				Blocks: []CanonicalContentBlock{
					{Type: CanonicalBlockImage, Data: mustMarshalRaw(ImageSource{Type: "base64", MediaType: "image/png", Data: "abc123"})},
				},
			},
			{
				Role: "assistant",
				Blocks: []CanonicalContentBlock{
					{Type: CanonicalBlockToolCall, ToolCall: &CanonicalToolCall{ID: "c1", Name: "read_file", Arguments: `{"path":"main.go"}`}},
				},
			},
			{
				Role: "tool",
				Blocks: []CanonicalContentBlock{
					{Type: CanonicalBlockToolResult, ToolResult: &CanonicalToolResult{ToolCallID: "c1", Content: "package main\nfunc main() {}"}},
				},
			},
		},
	}, "qwen-key")
	if err != nil {
		t.Fatalf("BuildCanonical() error = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(request.BodyJSON), &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	msgs, ok := payload["messages"].([]any)
	if !ok || len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %#v", payload["messages"])
	}
	user := msgs[0].(map[string]any)
	if content := user["content"].(string); content != "data:image/png;base64,abc123" {
		t.Fatalf("expected image data URL, got %q", content)
	}
	assistant := msgs[1].(map[string]any)
	toolCalls, ok := assistant["tool_calls"].([]any)
	if !ok || len(toolCalls) != 1 {
		t.Fatalf("expected 1 tool_call in assistant message, got %#v", assistant["tool_calls"])
	}
	toolMsg := msgs[2].(map[string]any)
	if toolMsg["tool_call_id"] != "c1" {
		t.Fatalf("expected tool_call_id c1, got %#v", toolMsg["tool_call_id"])
	}
}
