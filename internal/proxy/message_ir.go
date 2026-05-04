package proxy

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ConvertAnthropicToIR converts an Anthropic Messages request to the internal
// Message IR understood by the body builder and transport layer.
func ConvertAnthropicToIR(req AnthropicMessagesRequest) ([]Message, error) {
	out := make([]Message, 0, len(req.Messages)+1)

	// Inject system prompt as the first message when present.
	if len(req.System) > 0 {
		sysMsg, err := parseSystemPrompt(req.System)
		if err != nil {
			return nil, fmt.Errorf("parse system: %w", err)
		}
		if sysMsg != nil {
			out = append(out, *sysMsg)
		}
	}

	for _, m := range req.Messages {
		ir, err := convertAnthropicMessage(m)
		if err != nil {
			return nil, err
		}
		out = append(out, ir...)
	}
	return out, nil
}

// ConvertIRToAnthropic converts IR messages back to Anthropic content blocks.
func ConvertIRToAnthropic(ir []Message) []ContentBlock {
	blocks := make([]ContentBlock, 0, len(ir))
	for _, m := range ir {
		switch m.Role {
		case "assistant":
			if len(m.ToolCalls) > 0 {
				for _, tc := range m.ToolCalls {
					input := json.RawMessage(tc.Function.Arguments)
					if !json.Valid(input) {
						input = json.RawMessage("{}")
					}
					blocks = append(blocks, ContentBlock{
						Type:  "tool_use",
						ID:    tc.ID,
						Name:  tc.Function.Name,
						Input: input,
					})
				}
			}
			if m.Content != "" {
				blocks = append(blocks, ContentBlock{
					Type: "text",
					Text: m.Content,
				})
			}
		case "tool":
			var content json.RawMessage
			if json.Valid([]byte(m.Content)) {
				content = json.RawMessage(m.Content)
			} else {
				escaped, _ := json.Marshal(m.Content)
				content = json.RawMessage(escaped)
			}
			blocks = append(blocks, ContentBlock{
				Type:      "tool_result",
				ToolUseID: m.ToolCallID,
				Content:   content,
			})
		case "user", "system":
			if m.Content != "" {
				blocks = append(blocks, ContentBlock{
					Type: "text",
					Text: m.Content,
				})
			}
		}
	}
	return blocks
}

func CanonicalizeOpenAIRequest(req OpenAIChatRequest, sessionID string) (CanonicalRequest, error) {
	if sessionID == "" {
		sessionID = strings.TrimSpace(req.ExtraBody.SessionID)
	}

	turns := make([]CanonicalTurn, 0, len(req.Messages))
	for _, message := range req.Messages {
		turn := CanonicalTurn{
			Role: message.Role,
			Name: message.Name,
		}
		switch message.Role {
		case "system", "user":
			if message.Content != "" {
				turn.Blocks = append(turn.Blocks, CanonicalContentBlock{
					Type: CanonicalBlockText,
					Text: message.Content,
				})
			}
		case "assistant":
			if message.Content != "" {
				turn.Blocks = append(turn.Blocks, CanonicalContentBlock{
					Type: CanonicalBlockText,
					Text: message.Content,
				})
			}
			for _, toolCall := range message.ToolCalls {
				turn.Blocks = append(turn.Blocks, CanonicalContentBlock{
					Type: CanonicalBlockToolCall,
					ToolCall: &CanonicalToolCall{
						ID:        toolCall.ID,
						Name:      toolCall.Function.Name,
						Arguments: toolCall.Function.Arguments,
					},
				})
			}
		case "tool":
			turn.Blocks = append(turn.Blocks, CanonicalContentBlock{
				Type: CanonicalBlockToolResult,
				ToolResult: &CanonicalToolResult{
					ToolCallID: message.ToolCallID,
					Content:    message.Content,
				},
			})
		default:
			return CanonicalRequest{}, fmt.Errorf("unsupported role %q", message.Role)
		}
		turns = append(turns, turn)
	}

	return CanonicalRequest{
		SchemaVersion: 1,
		Protocol:      CanonicalProtocolOpenAI,
		Model:         req.Model,
		Stream:        req.Stream,
		Temperature:   req.Temperature,
		Tools:         canonicalToolDefinitions(req.Tools),
		ToolChoice:    req.ToolChoice,
		HasTools:      len(req.Tools) > 0,
		HasReasoning:  req.Reasoning,
		SessionID:     sessionID,
		Turns:         turns,
	}, nil
}

func CanonicalizeAnthropicRequest(req AnthropicMessagesRequest, sessionID string) (CanonicalRequest, error) {
	if req.Model == "" {
		return CanonicalRequest{}, fmt.Errorf("model is required")
	}
	if sessionID == "" {
		sessionID = metadataString(parseMetadataMap(req.Metadata), "session_id")
	}

	turns := make([]CanonicalTurn, 0, len(req.Messages)+1)
	systemTurn, err := canonicalSystemTurn(req.System)
	if err != nil {
		return CanonicalRequest{}, fmt.Errorf("parse system: %w", err)
	}
	if systemTurn != nil {
		turns = append(turns, *systemTurn)
	}

	for _, message := range req.Messages {
		turn, err := canonicalizeAnthropicTurn(message)
		if err != nil {
			return CanonicalRequest{}, err
		}
		turns = append(turns, turn)
	}

	return CanonicalRequest{
		SchemaVersion: 1,
		Protocol:      CanonicalProtocolAnthropic,
		Model:         req.Model,
		Stream:        req.Stream,
		Temperature:   req.Temperature,
		Tools:         canonicalizeAnthropicTools(req.Tools),
		ToolChoice:    req.ToolChoice,
		HasTools:      len(req.Tools) > 0,
		HasReasoning:  req.Thinking == nil || req.Thinking.Type != "disabled",
		SessionID:     sessionID,
		Metadata:      parseMetadataMap(req.Metadata),
		Turns:         turns,
	}, nil
}

func ProjectCanonicalToOpenAIRequest(req CanonicalRequest) (OpenAIChatRequest, []Message, error) {
	messages, err := projectCanonicalTurnsToLegacyMessages(req.Turns)
	if err != nil {
		return OpenAIChatRequest{}, nil, err
	}

	projected := OpenAIChatRequest{
		Model:       req.Model,
		Messages:    cloneMessages(messages),
		Stream:      req.Stream,
		Temperature: req.Temperature,
		ExtraBody: ExtraBody{
			SessionID: req.SessionID,
		},
		Tools:      projectCanonicalToolDefinitions(req.Tools),
		ToolChoice: req.ToolChoice,
		Reasoning:  req.HasReasoning,
	}
	if !req.HasTools {
		projected.Tools = nil
		projected.ToolChoice = nil
	}
	return projected, messages, nil
}

func canonicalSystemTurn(raw json.RawMessage) (*CanonicalTurn, error) {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 {
		return nil, nil
	}
	if raw[0] == '"' {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return nil, err
		}
		if text == "" {
			return nil, nil
		}
		return &CanonicalTurn{
			Role: "system",
			Blocks: []CanonicalContentBlock{{
				Type: CanonicalBlockText,
				Text: text,
			}},
		}, nil
	}
	if raw[0] != '[' {
		return nil, fmt.Errorf("unsupported system format: %s", string(raw[:min(60, len(raw))]))
	}

	var blocks []SystemBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, fmt.Errorf("unmarshal system blocks: %w", err)
	}
	turn := &CanonicalTurn{Role: "system"}
	for _, block := range blocks {
		if block.Type != "text" || block.Text == "" {
			continue
		}
		turn.Blocks = append(turn.Blocks, CanonicalContentBlock{
			Type: CanonicalBlockText,
			Text: block.Text,
		})
	}
	if len(turn.Blocks) == 0 {
		return nil, nil
	}
	return turn, nil
}

func canonicalizeAnthropicTurn(message AnthropicMessage) (CanonicalTurn, error) {
	if message.Role != "user" && message.Role != "assistant" {
		return CanonicalTurn{}, fmt.Errorf("unsupported role %q", message.Role)
	}

	turn := CanonicalTurn{
		Role:   message.Role,
		Blocks: make([]CanonicalContentBlock, 0, len(message.Content)),
	}
	for _, block := range message.Content {
		switch block.Type {
		case "text":
			turn.Blocks = append(turn.Blocks, CanonicalContentBlock{
				Type: CanonicalBlockText,
				Text: block.Text,
			})
		case "thinking":
			metadata := map[string]any{}
			if block.Signature != "" {
				metadata["signature"] = block.Signature
			}
			if len(metadata) == 0 {
				metadata = nil
			}
			turn.Blocks = append(turn.Blocks, CanonicalContentBlock{
				Type:     CanonicalBlockReasoning,
				Text:     block.Thinking,
				Metadata: metadata,
			})
		case "tool_use":
			args := string(block.Input)
			if !json.Valid(block.Input) {
				args = "{}"
			}
			turn.Blocks = append(turn.Blocks, CanonicalContentBlock{
				Type: CanonicalBlockToolCall,
				ToolCall: &CanonicalToolCall{
					ID:        block.ID,
					Name:      block.Name,
					Arguments: args,
				},
			})
		case "tool_result":
			turn.Blocks = append(turn.Blocks, CanonicalContentBlock{
				Type: CanonicalBlockToolResult,
				ToolResult: &CanonicalToolResult{
					ToolCallID: block.ToolUseID,
					Content:    toolResultString(block.Content),
				},
			})
		case "image":
			turn.Blocks = append(turn.Blocks, CanonicalContentBlock{
				Type: CanonicalBlockImage,
				Data: mustMarshalRaw(block.Source),
			})
		case "document":
			turn.Blocks = append(turn.Blocks, CanonicalContentBlock{
				Type: CanonicalBlockDocument,
				Data: mustMarshalRaw(block.Source),
			})
		}
	}
	return turn, nil
}

func canonicalizeAnthropicTools(tools []AnthropicTool) []CanonicalToolDefinition {
	if len(tools) == 0 {
		return nil
	}
	out := make([]CanonicalToolDefinition, 0, len(tools))
	for _, tool := range tools {
		out = append(out, CanonicalToolDefinition{
			Type:        "function",
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  json.RawMessage(tool.InputSchema),
		})
	}
	return out
}

func canonicalToolDefinitions(tools []Tool) []CanonicalToolDefinition {
	if len(tools) == 0 {
		return nil
	}
	out := make([]CanonicalToolDefinition, 0, len(tools))
	for _, tool := range tools {
		out = append(out, CanonicalToolDefinition{
			Type:        tool.Type,
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			Parameters:  mustMarshalRaw(tool.Function.Parameters),
		})
	}
	return out
}

func projectCanonicalToolDefinitions(tools []CanonicalToolDefinition) []Tool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]Tool, 0, len(tools))
	for _, tool := range tools {
		out = append(out, Tool{
			Type: toolTypeOrDefault(tool.Type),
			Function: ToolFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  unmarshalRawAny(tool.Parameters),
			},
		})
	}
	return out
}

func projectCanonicalTurnsToLegacyMessages(turns []CanonicalTurn) ([]Message, error) {
	out := make([]Message, 0, len(turns))
	for _, turn := range turns {
		messages, err := projectCanonicalTurn(turn)
		if err != nil {
			return nil, err
		}
		out = append(out, messages...)
	}
	return out, nil
}

func projectCanonicalTurn(turn CanonicalTurn) ([]Message, error) {
	switch turn.Role {
	case "system", "user":
		return projectCanonicalUserLikeTurn(turn), nil
	case "assistant":
		message := Message{Role: "assistant", Name: turn.Name}
		for _, block := range turn.Blocks {
			switch block.Type {
			case CanonicalBlockText:
				appendInlineText(&message.Content, block.Text)
			case CanonicalBlockReasoning:
				appendInlineText(&message.Content, "[thinking]"+block.Text+"[/thinking]")
			case CanonicalBlockImage:
				appendStructuredText(&message.Content, mediaBlockToText(block.Type, block.Data))
			case CanonicalBlockDocument:
				appendStructuredText(&message.Content, mediaBlockToText(block.Type, block.Data))
			case CanonicalBlockToolCall:
				if block.ToolCall == nil {
					continue
				}
				message.ToolCalls = append(message.ToolCalls, ToolCall{
					Index: len(message.ToolCalls),
					ID:    block.ToolCall.ID,
					Type:  "function",
					Function: FunctionCall{
						Name:      block.ToolCall.Name,
						Arguments: canonicalToolArguments(block.ToolCall),
					},
				})
			case CanonicalBlockToolResult:
				if block.ToolResult != nil {
					appendStructuredText(&message.Content, block.ToolResult.Content)
				}
			}
		}
		if message.Content == "" && len(message.ToolCalls) == 0 {
			return nil, nil
		}
		return []Message{message}, nil
	case "tool":
		out := make([]Message, 0, len(turn.Blocks))
		for _, block := range turn.Blocks {
			if block.Type != CanonicalBlockToolResult || block.ToolResult == nil {
				continue
			}
			out = append(out, Message{
				Role:       "tool",
				ToolCallID: block.ToolResult.ToolCallID,
				Content:    block.ToolResult.Content,
			})
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported role %q", turn.Role)
	}
}

func projectCanonicalUserLikeTurn(turn CanonicalTurn) []Message {
	current := Message{Role: turn.Role, Name: turn.Name}
	out := make([]Message, 0, len(turn.Blocks)+1)
	flushCurrent := func() {
		if current.Content == "" {
			return
		}
		out = append(out, current)
		current = Message{Role: turn.Role, Name: turn.Name}
	}

	for _, block := range turn.Blocks {
		switch block.Type {
		case CanonicalBlockText:
			appendInlineText(&current.Content, block.Text)
		case CanonicalBlockReasoning:
			appendInlineText(&current.Content, "[thinking]"+block.Text+"[/thinking]")
		case CanonicalBlockImage:
			appendStructuredText(&current.Content, mediaBlockToText(block.Type, block.Data))
		case CanonicalBlockDocument:
			appendStructuredText(&current.Content, mediaBlockToText(block.Type, block.Data))
		case CanonicalBlockToolResult:
			if block.ToolResult == nil {
				continue
			}
			flushCurrent()
			out = append(out, Message{
				Role:       "tool",
				ToolCallID: block.ToolResult.ToolCallID,
				Content:    block.ToolResult.Content,
			})
		}
	}
	flushCurrent()
	return out
}

func appendInlineText(current *string, next string) {
	if next == "" {
		return
	}
	*current += next
}

func appendStructuredText(current *string, next string) {
	if next == "" {
		return
	}
	if *current != "" {
		*current += "\n"
	}
	*current += next
}

func mediaBlockToText(kind CanonicalBlockType, raw json.RawMessage) string {
	if len(raw) == 0 {
		if kind == CanonicalBlockDocument {
			return "[document]"
		}
		return "[image]"
	}
	var source ImageSource
	if err := json.Unmarshal(raw, &source); err != nil {
		if kind == CanonicalBlockDocument {
			return "[document]"
		}
		return "[image]"
	}
	if kind == CanonicalBlockDocument {
		return documentToText(&source)
	}
	return imageToText(&source)
}

func canonicalToolArguments(toolCall *CanonicalToolCall) string {
	if toolCall == nil || toolCall.Arguments == "" {
		return "{}"
	}
	return toolCall.Arguments
}

func parseMetadataMap(raw json.RawMessage) map[string]any {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return nil
	}
	return metadata
}

func metadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	value, _ := metadata[key].(string)
	return strings.TrimSpace(value)
}

func mustMarshalRaw(value any) json.RawMessage {
	if value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return json.RawMessage(raw)
}

func unmarshalRawAny(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil
	}
	return value
}

func toolTypeOrDefault(toolType string) string {
	if toolType == "" {
		return "function"
	}
	return toolType
}

func parseSystemPrompt(raw json.RawMessage) (*Message, error) {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 {
		return nil, nil
	}

	// Case 1: plain string
	if raw[0] == '"' {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return nil, err
		}
		if text == "" {
			return nil, nil
		}
		return &Message{Role: "system", Content: text}, nil
	}

	// Case 2: array of {type:"text", text:"..."}
	if raw[0] == '[' {
		var blocks []SystemBlock
		if err := json.Unmarshal(raw, &blocks); err != nil {
			return nil, fmt.Errorf("unmarshal system blocks: %w", err)
		}
		var builder strings.Builder
		for _, b := range blocks {
			if b.Type == "text" {
				builder.WriteString(b.Text)
				builder.WriteByte('\n')
			}
		}
		text := strings.TrimRight(builder.String(), "\n")
		if text == "" {
			return nil, nil
		}
		return &Message{Role: "system", Content: text}, nil
	}

	return nil, fmt.Errorf("unsupported system format: %s", string(raw[:min(60, len(raw))]))
}

func convertAnthropicMessage(m AnthropicMessage) ([]Message, error) {
	if m.Role != "user" && m.Role != "assistant" {
		return nil, fmt.Errorf("unsupported role %q", m.Role)
	}

	out := make([]Message, 0, 1)
	switch m.Role {
	case "user":
		msg := Message{Role: "user"}
		for _, block := range m.Content {
			switch block.Type {
			case "text":
				msg.Content += block.Text
			case "tool_result":
				out = append(out, Message{
					Role:       "tool",
					ToolCallID: block.ToolUseID,
					Content:    toolResultString(block.Content),
				})
				continue
			case "image":
				if msg.Content != "" {
					msg.Content += "\n"
				}
				msg.Content += imageToText(block.Source)
			case "document":
				if msg.Content != "" {
					msg.Content += "\n"
				}
				msg.Content += documentToText(block.Source)
			default:
				continue
			}
		}
		if msg.Content != "" || len(out) == 0 {
			out = append([]Message{msg}, out...)
		}

	case "assistant":
		msg := Message{Role: "assistant"}
		for _, block := range m.Content {
			switch block.Type {
			case "text":
				msg.Content += block.Text
			case "thinking":
				msg.Content += "[thinking]" + block.Thinking + "[/thinking]"
			case "tool_use":
				args := string(block.Input)
				if !json.Valid(block.Input) {
					args = "{}"
				}
				msg.ToolCalls = append(msg.ToolCalls, ToolCall{
					Index: len(msg.ToolCalls),
					ID:    block.ID,
					Type:  "function",
					Function: FunctionCall{
						Name:      block.Name,
						Arguments: args,
					},
				})
			}
		}
		out = append(out, msg)
	}
	return out, nil
}

func toolResultString(content json.RawMessage) string {
	if len(content) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(content, &s); err == nil {
		return s
	}
	return string(content)
}

func imageToText(source *ImageSource) string {
	if source == nil || source.Data == "" {
		return "[image]"
	}
	keep := source.Data
	if len(keep) > 256<<10 {
		keep = keep[:256<<10]
	}
	return fmt.Sprintf("data:%s;base64,%s", source.MediaType, keep)
}

func documentToText(source *ImageSource) string {
	if source == nil || source.Data == "" {
		return "[document]"
	}
	keep := source.Data
	if len(keep) > 256<<10 {
		keep = keep[:256<<10]
	}
	return fmt.Sprintf("data:%s;base64,%s", source.MediaType, keep)
}
