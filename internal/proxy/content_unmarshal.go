package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// UnmarshalJSON accepts both legacy string form and OpenAI-style multimodal
// array form for the `content` field. Array form populates Parts and also
// concatenates text parts into Content for downstream code that still reads
// Content directly.
func (m *Message) UnmarshalJSON(data []byte) error {
	type messageAux struct {
		Role       string          `json:"role"`
		Content    json.RawMessage `json:"content"`
		Name       string          `json:"name,omitempty"`
		ToolCallID string          `json:"tool_call_id,omitempty"`
		ToolCalls  []ToolCall      `json:"tool_calls,omitempty"`
	}
	var aux messageAux
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	m.Role = aux.Role
	m.Name = aux.Name
	m.ToolCallID = aux.ToolCallID
	m.ToolCalls = aux.ToolCalls
	m.Content = ""
	m.Parts = nil

	raw := bytes.TrimSpace(aux.Content)
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return fmt.Errorf("content as string: %w", err)
		}
		m.Content = s
		return nil
	}
	if raw[0] == '[' {
		var parts []OpenAIContentPart
		if err := json.Unmarshal(raw, &parts); err != nil {
			return fmt.Errorf("content as array: %w", err)
		}
		texts := make([]string, 0, len(parts))
		for index, part := range parts {
			switch part.Type {
			case "text":
				texts = append(texts, part.Text)
			case "image_url":
				if part.ImageURL == nil || part.ImageURL.URL == "" {
					return fmt.Errorf("content[%d]: image_url.url is required", index)
				}
			default:
				return fmt.Errorf("content[%d]: unsupported part type %q", index, part.Type)
			}
		}
		m.Parts = parts
		m.Content = strings.Join(texts, "\n")
		return nil
	}
	return fmt.Errorf("content must be string or array, got %s", string(raw[:1]))
}
