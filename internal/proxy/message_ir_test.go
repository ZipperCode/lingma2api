package proxy

import (
	"encoding/json"
	"testing"
)

func TestCanonicalizeOpenAIRequestProjectsBackToLegacyMessages(t *testing.T) {
	req := OpenAIChatRequest{
		Model:      "auto",
		Stream:     true,
		Reasoning:  true,
		Tools:      []Tool{{Type: "function", Function: ToolFunction{Name: "read_file", Description: "Read a file", Parameters: map[string]any{"type": "object"}}}},
		ToolChoice: map[string]any{"type": "auto"},
		ExtraBody:  ExtraBody{SessionID: "body-session"},
		Messages: []Message{
			{Role: "system", Content: "follow instructions"},
			{Role: "user", Content: "read main.go"},
			{
				Role:    "assistant",
				Content: "calling tool",
				ToolCalls: []ToolCall{{
					Index: 0,
					ID:    "call_1",
					Type:  "function",
					Function: FunctionCall{
						Name:      "read_file",
						Arguments: `{"path":"main.go"}`,
					},
				}},
			},
			{Role: "tool", ToolCallID: "call_1", Content: `{"content":"package main"}`},
		},
	}

	canonical, err := CanonicalizeOpenAIRequest(req, "header-session")
	if err != nil {
		t.Fatalf("CanonicalizeOpenAIRequest() error = %v", err)
	}
	if canonical.Protocol != CanonicalProtocolOpenAI {
		t.Fatalf("expected openai protocol, got %q", canonical.Protocol)
	}
	if canonical.SessionID != "header-session" {
		t.Fatalf("expected header session override, got %q", canonical.SessionID)
	}
	if len(canonical.Tools) != 1 || canonical.Tools[0].Name != "read_file" {
		t.Fatalf("unexpected canonical tools: %#v", canonical.Tools)
	}
	if len(canonical.Turns) != 4 {
		t.Fatalf("expected 4 canonical turns, got %d", len(canonical.Turns))
	}
	if got := canonical.Turns[2].Blocks[1].Type; got != CanonicalBlockToolCall {
		t.Fatalf("expected assistant tool_call block, got %q", got)
	}
	if got := canonical.Turns[3].Blocks[0].Type; got != CanonicalBlockToolResult {
		t.Fatalf("expected tool result block, got %q", got)
	}

	projected, messages, err := ProjectCanonicalToOpenAIRequest(canonical)
	if err != nil {
		t.Fatalf("ProjectCanonicalToOpenAIRequest() error = %v", err)
	}
	if projected.Model != "auto" || !projected.Stream || !projected.Reasoning {
		t.Fatalf("unexpected projected request: %#v", projected)
	}
	if projected.ExtraBody.SessionID != "header-session" {
		t.Fatalf("unexpected projected session id %q", projected.ExtraBody.SessionID)
	}
	if len(projected.Tools) != 1 || projected.Tools[0].Function.Name != "read_file" {
		t.Fatalf("unexpected projected tools: %#v", projected.Tools)
	}
	if len(messages) != 4 {
		t.Fatalf("expected 4 projected messages, got %d", len(messages))
	}
	if len(messages[2].ToolCalls) != 1 || messages[2].ToolCalls[0].Function.Name != "read_file" {
		t.Fatalf("unexpected assistant tool calls: %#v", messages[2].ToolCalls)
	}
	if messages[3].Role != "tool" || messages[3].ToolCallID != "call_1" {
		t.Fatalf("unexpected projected tool message: %#v", messages[3])
	}
}

func TestCanonicalizeAnthropicRequestPreservesOrderedBlocks(t *testing.T) {
	req := AnthropicMessagesRequest{
		Model:  "claude-3-5-sonnet",
		Stream: false,
		Tools:  []AnthropicTool{{Name: "lookup_doc", InputSchema: json.RawMessage(`{"type":"object"}`)}},
		System: json.RawMessage(`[{"type":"text","text":"be precise"}]`),
		Thinking: &ThinkingConfig{
			Type:         "enabled",
			BudgetTokens: intPtr(512),
		},
		Messages: []AnthropicMessage{
			{
				Role: "user",
				Content: []ContentBlock{
					{Type: "text", Text: "look at this"},
					{Type: "image", Source: &ImageSource{Type: "base64", MediaType: "image/png", Data: "aGVsbG8="}},
					{Type: "document", Source: &ImageSource{Type: "base64", MediaType: "application/pdf", Data: "cGRm"}},
					{Type: "tool_result", ToolUseID: "tool_1", Content: json.RawMessage(`{"ok":true}`)},
				},
			},
			{
				Role: "assistant",
				Content: []ContentBlock{
					{Type: "thinking", Thinking: "step one", Signature: "sig-1"},
					{Type: "text", Text: "done"},
					{Type: "tool_use", ID: "tool_2", Name: "lookup_doc", Input: json.RawMessage(`{"id":"42"}`)},
				},
			},
		},
	}

	canonical, err := CanonicalizeAnthropicRequest(req, "sess-1")
	if err != nil {
		t.Fatalf("CanonicalizeAnthropicRequest() error = %v", err)
	}
	if canonical.Protocol != CanonicalProtocolAnthropic {
		t.Fatalf("expected anthropic protocol, got %q", canonical.Protocol)
	}
	if !canonical.HasReasoning || !canonical.HasTools {
		t.Fatalf("unexpected reasoning/tools flags: %#v", canonical)
	}
	if len(canonical.Tools) != 1 || canonical.Tools[0].Name != "lookup_doc" {
		t.Fatalf("unexpected canonical tools: %#v", canonical.Tools)
	}
	if len(canonical.Turns) != 3 {
		t.Fatalf("expected 3 turns including system, got %d", len(canonical.Turns))
	}
	userBlocks := canonical.Turns[1].Blocks
	if len(userBlocks) != 4 {
		t.Fatalf("expected 4 user blocks, got %d", len(userBlocks))
	}
	if userBlocks[0].Type != CanonicalBlockText || userBlocks[1].Type != CanonicalBlockImage || userBlocks[2].Type != CanonicalBlockDocument || userBlocks[3].Type != CanonicalBlockToolResult {
		t.Fatalf("unexpected user block ordering: %#v", userBlocks)
	}
	assistantBlocks := canonical.Turns[2].Blocks
	if len(assistantBlocks) != 3 {
		t.Fatalf("expected 3 assistant blocks, got %d", len(assistantBlocks))
	}
	if assistantBlocks[0].Type != CanonicalBlockReasoning || assistantBlocks[2].Type != CanonicalBlockToolCall {
		t.Fatalf("unexpected assistant blocks: %#v", assistantBlocks)
	}

	projected, messages, err := ProjectCanonicalToOpenAIRequest(canonical)
	if err != nil {
		t.Fatalf("ProjectCanonicalToOpenAIRequest() error = %v", err)
	}
	if projected.Model != "claude-3-5-sonnet" || projected.ExtraBody.SessionID != "sess-1" {
		t.Fatalf("unexpected projected request: %#v", projected)
	}
	if len(projected.Tools) != 1 || projected.Tools[0].Function.Name != "lookup_doc" {
		t.Fatalf("unexpected projected tools: %#v", projected.Tools)
	}
	if len(messages) != 4 {
		t.Fatalf("expected 4 legacy messages, got %d", len(messages))
	}
	if messages[1].Role != "user" || messages[1].Content != "look at this\ndata:application/pdf;base64,cGRm" {
		t.Fatalf("unexpected projected user message: %#v", messages[1])
	}
	if messages[2].Role != "tool" || messages[2].ToolCallID != "tool_1" {
		t.Fatalf("unexpected projected tool message: %#v", messages[2])
	}
	if messages[3].Role != "assistant" || messages[3].Content != "[thinking]step one[/thinking]done" {
		t.Fatalf("unexpected projected assistant message: %#v", messages[3])
	}
	if len(messages[3].ToolCalls) != 1 || messages[3].ToolCalls[0].ID != "tool_2" {
		t.Fatalf("unexpected projected assistant tool calls: %#v", messages[3].ToolCalls)
	}
}

func intPtr(v int) *int {
	return &v
}

func TestCanonicalizeOpenAIRequest_ImagePartHTTP(t *testing.T) {
	req := OpenAIChatRequest{
		Model: "auto",
		Messages: []Message{{
			Role: "user",
			Parts: []OpenAIContentPart{
				{Type: "text", Text: "see this:"},
				{Type: "image_url", ImageURL: &OpenAIContentImageURL{URL: "https://example.com/x.png"}},
			},
			Content: "see this:",
		}},
	}
	canonical, err := CanonicalizeOpenAIRequest(req, "")
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	if len(canonical.Turns) != 1 {
		t.Fatalf("turns = %d, want 1", len(canonical.Turns))
	}
	blocks := canonical.Turns[0].Blocks
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d, want 2", len(blocks))
	}
	if blocks[0].Type != CanonicalBlockText || blocks[0].Text != "see this:" {
		t.Fatalf("first block: %+v", blocks[0])
	}
	if blocks[1].Type != CanonicalBlockImage {
		t.Fatalf("second block type = %v, want image", blocks[1].Type)
	}
	if blocks[1].Metadata["source_type"] != "url" {
		t.Fatalf("source_type = %v, want url", blocks[1].Metadata["source_type"])
	}
}

func TestCanonicalizeOpenAIRequest_ImagePartDataURI(t *testing.T) {
	req := OpenAIChatRequest{
		Model: "auto",
		Messages: []Message{{
			Role: "user",
			Parts: []OpenAIContentPart{
				{Type: "image_url", ImageURL: &OpenAIContentImageURL{URL: "data:image/png;base64,QUFB"}},
			},
		}},
	}
	canonical, err := CanonicalizeOpenAIRequest(req, "")
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	blocks := canonical.Turns[0].Blocks
	if len(blocks) != 1 || blocks[0].Type != CanonicalBlockImage {
		t.Fatalf("blocks: %+v", blocks)
	}
	if blocks[0].Metadata["media_type"] != "image/png" {
		t.Fatalf("media_type = %v", blocks[0].Metadata["media_type"])
	}
	if blocks[0].Metadata["source_type"] != "base64" {
		t.Fatalf("source_type = %v", blocks[0].Metadata["source_type"])
	}
}

func TestCanonicalizeOpenAIRequest_TextOnlyParts(t *testing.T) {
	req := OpenAIChatRequest{
		Model: "auto",
		Messages: []Message{{
			Role: "user",
			Parts: []OpenAIContentPart{
				{Type: "text", Text: "hi"},
				{Type: "text", Text: "again"},
			},
			Content: "hi\nagain",
		}},
	}
	canonical, err := CanonicalizeOpenAIRequest(req, "")
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	blocks := canonical.Turns[0].Blocks
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d, want 2", len(blocks))
	}
	if blocks[0].Type != CanonicalBlockText || blocks[0].Text != "hi" {
		t.Fatalf("block[0]: %+v", blocks[0])
	}
	if blocks[1].Type != CanonicalBlockText || blocks[1].Text != "again" {
		t.Fatalf("block[1]: %+v", blocks[1])
	}
}

func TestCanonicalizeAnthropicRequest_ImageBlockHasMetadata(t *testing.T) {
	req := AnthropicMessagesRequest{
		Model:     "claude-3-7-sonnet-20250219",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{{
			Role: "user",
			Content: []ContentBlock{
				{Type: "text", Text: "see"},
				{Type: "image", Source: &ImageSource{Type: "base64", MediaType: "image/jpeg", Data: "QUFB"}},
			},
		}},
	}
	canonical, err := CanonicalizeAnthropicRequest(req, "")
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	if len(canonical.Turns) != 1 {
		t.Fatalf("turns = %d", len(canonical.Turns))
	}
	blocks := canonical.Turns[0].Blocks
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d, want 2", len(blocks))
	}
	if blocks[1].Type != CanonicalBlockImage {
		t.Fatalf("blocks[1].Type = %v", blocks[1].Type)
	}
	if blocks[1].Metadata == nil {
		t.Fatalf("blocks[1].Metadata is nil; want populated")
	}
	if blocks[1].Metadata["media_type"] != "image/jpeg" {
		t.Fatalf("media_type = %v", blocks[1].Metadata["media_type"])
	}
	if blocks[1].Metadata["source_type"] != "base64" {
		t.Fatalf("source_type = %v", blocks[1].Metadata["source_type"])
	}
}
