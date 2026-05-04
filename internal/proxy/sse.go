package proxy

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type outerSSEPayload struct {
	Body            string `json:"body"`
	StatusCodeValue int    `json:"statusCodeValue"`
}

type innerSSEPayload struct {
	Choices []struct {
		Delta struct {
			Content          string          `json:"content"`
			ReasoningContent string          `json:"reasoning_content"`
			ToolCalls        []toolCallDelta `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage,omitempty"`
}

type toolCallDelta struct {
	Index    int               `json:"index"`
	ID       string            `json:"id,omitempty"`
	Type     string            `json:"type,omitempty"`
	Function functionCallDelta `json:"function,omitempty"`
}

type functionCallDelta struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

func ParseSSELine(line string) (SSEEvent, bool, error) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || !strings.HasPrefix(trimmed, "data:") {
		return SSEEvent{}, false, nil
	}

	payload := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
	if payload == "[DONE]" {
		return SSEEvent{Done: true}, true, nil
	}

	var outer outerSSEPayload
	if err := json.Unmarshal([]byte(payload), &outer); err != nil {
		return SSEEvent{}, false, err
	}
	if outer.StatusCodeValue >= 400 {
		return SSEEvent{}, false, fmt.Errorf("upstream sse status %d", outer.StatusCodeValue)
	}
	if outer.Body == "" {
		return SSEEvent{}, false, nil
	}
	if outer.Body == "[DONE]" {
		return SSEEvent{Done: true}, true, nil
	}

	var inner innerSSEPayload
	if err := json.Unmarshal([]byte(outer.Body), &inner); err != nil {
		return SSEEvent{}, false, err
	}

	var contentBuilder strings.Builder
	var reasoningBuilder strings.Builder
	for _, choice := range inner.Choices {
		contentBuilder.WriteString(choice.Delta.Content)
		reasoningBuilder.WriteString(choice.Delta.ReasoningContent)
	}
	var toolCalls []ToolCall
	for _, choice := range inner.Choices {
		for _, tc := range choice.Delta.ToolCalls {
			toolCalls = append(toolCalls, ToolCall{
				Index: tc.Index,
				ID:    tc.ID,
				Type:  tc.Type,
				Function: FunctionCall{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			})
		}
	}
	var usage *SSEUsage
	if inner.Usage != nil {
		usage = &SSEUsage{
			PromptTokens:     inner.Usage.PromptTokens,
			CompletionTokens: inner.Usage.CompletionTokens,
			TotalTokens:      inner.Usage.TotalTokens,
		}
	}
	return SSEEvent{
		Content:          contentBuilder.String(),
		ReasoningContent: reasoningBuilder.String(),
		ToolCalls:        toolCalls,
		Usage:            usage,
	}, true, nil
}

func ScanSSE(reader io.Reader, onEvent func(SSEEvent) error) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		event, ok, err := ParseSSELine(scanner.Text())
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if err := onEvent(event); err != nil {
			return err
		}
		if event.Done {
			return nil
		}
	}
	return scanner.Err()
}

func ScanSSEWithLines(reader io.Reader, onLine func(string) error, onEvent func(SSEEvent) error) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if onLine != nil {
			if err := onLine(line); err != nil {
				return err
			}
		}
		event, ok, err := ParseSSELine(line)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if err := onEvent(event); err != nil {
			return err
		}
		if event.Done {
			return nil
		}
	}
	return scanner.Err()
}

func CollectSSEContent(reader io.Reader) (string, error) {
	var builder strings.Builder
	err := ScanSSE(reader, func(event SSEEvent) error {
		builder.WriteString(event.Content)
		return nil
	})
	if err != nil {
		return "", err
	}
	return builder.String(), nil
}

func CollectSSEContentWithLines(reader io.Reader) (string, []string, error) {
	var builder strings.Builder
	var lines []string
	err := ScanSSEWithLines(reader, func(line string) error {
		lines = append(lines, line)
		return nil
	}, func(event SSEEvent) error {
		builder.WriteString(event.Content)
		return nil
	})
	if err != nil {
		return "", nil, err
	}
	if lines == nil {
		lines = []string{}
	}
	return builder.String(), lines, nil
}
