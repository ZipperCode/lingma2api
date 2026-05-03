package api

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"lingma2api/internal/db"
	"lingma2api/internal/proxy"
)

const canonicalAdminLogFetchLimit = 5000

func (server *Server) listAdminLogs(ctx context.Context, filter db.LogFilter, page, limit int) (db.LogListResult, error) {
	if server.db == nil {
		return db.LogListResult{Items: []db.RequestLog{}, Total: 0, Page: page, Limit: limit}, nil
	}

	canonicalRecords, err := server.db.ListCanonicalExecutionRecords(ctx, canonicalAdminLogFetchLimit)
	if err != nil {
		return db.LogListResult{}, err
	}
	if len(canonicalRecords) == 0 {
		return server.db.ListLogs(ctx, filter, page, limit)
	}

	items := projectAndFilterCanonicalLogs(canonicalRecords, filter)
	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})

	total := len(items)
	start := (page - 1) * limit
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
	items = items[start:end]
	if items == nil {
		items = []db.RequestLog{}
	}
	return db.LogListResult{Items: items, Total: total, Page: page, Limit: limit}, nil
}

func (server *Server) getAdminLog(ctx context.Context, id string) (db.RequestLog, bool, error) {
	if server.db == nil {
		return db.RequestLog{}, false, nil
	}
	record, err := server.db.GetCanonicalExecutionRecord(ctx, id)
	if err == nil {
		return projectCanonicalRecordToLog(record), true, nil
	}
	log, err := server.db.GetLog(ctx, id)
	if err != nil {
		return db.RequestLog{}, false, err
	}
	return log, false, nil
}

func (server *Server) exportAdminLogs(ctx context.Context, filter db.LogFilter) ([]db.RequestLog, error) {
	if server.db == nil {
		return []db.RequestLog{}, nil
	}
	canonicalRecords, err := server.db.ListCanonicalExecutionRecords(ctx, canonicalAdminLogFetchLimit)
	if err != nil {
		return nil, err
	}
	if len(canonicalRecords) == 0 {
		return server.db.ExportLogs(ctx, filter)
	}
	items := projectAndFilterCanonicalLogs(canonicalRecords, filter)
	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	return items, nil
}

func projectAndFilterCanonicalLogs(records []db.CanonicalExecutionRecordRow, filter db.LogFilter) []db.RequestLog {
	items := make([]db.RequestLog, 0, len(records))
	for _, record := range records {
		log := projectCanonicalRecordToLog(record)
		if !matchesLogFilter(log, filter) {
			continue
		}
		items = append(items, log)
	}
	return items
}

func matchesLogFilter(log db.RequestLog, filter db.LogFilter) bool {
	if filter.Status != "" && log.Status != filter.Status {
		return false
	}
	if filter.Model != "" && log.Model != filter.Model && log.MappedModel != filter.Model {
		return false
	}
	if !filter.From.IsZero() && log.CreatedAt.Before(filter.From) {
		return false
	}
	if !filter.To.IsZero() && log.CreatedAt.After(filter.To) {
		return false
	}
	return true
}

func projectCanonicalRecordToLog(record db.CanonicalExecutionRecordRow) db.RequestLog {
	projectedRequest, _, err := proxy.ProjectCanonicalToOpenAIRequest(record.PrePolicyRequest)
	downstreamReq := ""
	stream := record.PrePolicyRequest.Stream
	if err == nil {
		if body, marshalErr := json.Marshal(projectedRequest); marshalErr == nil {
			downstreamReq = string(body)
		}
		stream = projectedRequest.Stream
	}

	upstreamResp := ""
	if record.Sidecar != nil {
		upstreamResp = strings.Join(record.Sidecar.RawSSELines, "\n")
	}

	mappedModel := db.CanonicalRecordMappedModel(record)
	upstreamStatus := db.CanonicalRecordUpstreamStatus(record)
	promptTokens, completionTokens, totalTokens := db.CanonicalRecordTokenCounts(record)
	ttftMs := 0
	if record.Sidecar != nil {
		ttftMs = record.Sidecar.TTFTMs
	}

	return db.RequestLog{
		ID:                record.ID,
		CreatedAt:         record.CreatedAt,
		SessionID:         record.SessionID,
		Model:             record.PrePolicyRequest.Model,
		MappedModel:       mappedModel,
		Stream:            stream,
		Status:            db.CanonicalRecordStatus(record),
		ErrorMsg:          "",
		DownstreamMethod:  "POST",
		DownstreamPath:    record.IngressEndpoint,
		DownstreamReq:     downstreamReq,
		DownstreamResp:    "",
		UpstreamReq:       record.SouthboundRequest,
		UpstreamResp:      upstreamResp,
		UpstreamStatus:    upstreamStatus,
		PromptTokens:      promptTokens,
		CompletionTokens:  completionTokens,
		TotalTokens:       totalTokens,
		TTFTMs:            ttftMs,
		UpstreamMs:        0,
		DownstreamMs:      0,
		CanonicalRecord:   true,
		IngressProtocol:   record.IngressProtocol,
		IngressEndpoint:   record.IngressEndpoint,
		PrePolicyRequest:  marshalPrettyJSON(record.PrePolicyRequest),
		PostPolicyRequest: marshalPrettyJSON(record.PostPolicyRequest),
		SessionSnapshot:   marshalPrettyJSON(record.SessionSnapshot),
		ExecutionSidecar:  marshalPrettyJSON(record.Sidecar),
	}
}

func marshalReplayBodyFromCanonical(request proxy.CanonicalRequest) ([]byte, error) {
	if request.Protocol == proxy.CanonicalProtocolAnthropic {
		return marshalAnthropicReplayBodyFromCanonical(request)
	}
	projectedRequest, _, err := proxy.ProjectCanonicalToOpenAIRequest(request)
	if err != nil {
		return nil, err
	}
	return json.Marshal(projectedRequest)
}

func canonicalReplayRequestForMode(record db.CanonicalExecutionRecordRow, mode string) proxy.CanonicalRequest {
	if isHistoricalReplayMode(mode) {
		return record.PostPolicyRequest
	}
	return record.PrePolicyRequest
}

func isHistoricalReplayMode(mode string) bool {
	return strings.EqualFold(strings.TrimSpace(mode), "historical")
}

func replayEndpointForCanonicalRequest(request proxy.CanonicalRequest, ingressEndpoint string) string {
	if request.Protocol == proxy.CanonicalProtocolAnthropic || ingressEndpoint == "/v1/messages" {
		return "/v1/messages"
	}
	return "/v1/chat/completions"
}

func marshalAnthropicReplayBodyFromCanonical(request proxy.CanonicalRequest) ([]byte, error) {
	anthropicRequest := proxy.AnthropicMessagesRequest{
		Model:     request.Model,
		Messages:  make([]proxy.AnthropicMessage, 0, len(request.Turns)),
		Tools:     canonicalToolsToAnthropicTools(request.Tools),
		MaxTokens: 4096,
		Stream:    request.Stream,
		Metadata:  canonicalReplayMetadata(request),
	}
	if !request.HasTools {
		anthropicRequest.Tools = nil
	}
	if !request.HasReasoning {
		anthropicRequest.Thinking = &proxy.ThinkingConfig{Type: "disabled"}
	}

	systemText := strings.Builder{}
	for _, turn := range request.Turns {
		blocks := canonicalTurnToAnthropicBlocks(turn)
		if len(blocks) == 0 {
			continue
		}
		if turn.Role == "system" {
			for _, block := range blocks {
				if block.Text == "" {
					continue
				}
				if systemText.Len() > 0 {
					systemText.WriteString("\n")
				}
				systemText.WriteString(block.Text)
			}
			continue
		}
		if turn.Role != "user" && turn.Role != "assistant" {
			continue
		}
		anthropicRequest.Messages = append(anthropicRequest.Messages, proxy.AnthropicMessage{
			Role:    turn.Role,
			Content: blocks,
		})
	}
	if systemText.Len() > 0 {
		systemRaw, err := json.Marshal(systemText.String())
		if err != nil {
			return nil, err
		}
		anthropicRequest.System = systemRaw
	}
	return json.Marshal(anthropicRequest)
}

func canonicalToolsToAnthropicTools(canonicalTools []proxy.CanonicalToolDefinition) []proxy.Tool {
	tools := make([]proxy.Tool, 0, len(canonicalTools))
	for _, tool := range canonicalTools {
		tools = append(tools, proxy.Tool{
			Type: tool.Type,
			Function: proxy.ToolFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.Parameters,
			},
		})
	}
	return tools
}

func canonicalReplayMetadata(request proxy.CanonicalRequest) json.RawMessage {
	if request.SessionID == "" {
		return nil
	}
	data, err := json.Marshal(map[string]string{"session_id": request.SessionID})
	if err != nil {
		return nil
	}
	return data
}

func canonicalTurnToAnthropicBlocks(turn proxy.CanonicalTurn) []proxy.ContentBlock {
	blocks := make([]proxy.ContentBlock, 0, len(turn.Blocks))
	for _, block := range turn.Blocks {
		switch block.Type {
		case proxy.CanonicalBlockText:
			blocks = append(blocks, proxy.ContentBlock{Type: "text", Text: block.Text})
		case proxy.CanonicalBlockReasoning:
			contentBlock := proxy.ContentBlock{Type: "thinking", Thinking: block.Text}
			if signature, ok := block.Metadata["signature"].(string); ok {
				contentBlock.Signature = signature
			}
			blocks = append(blocks, contentBlock)
		case proxy.CanonicalBlockToolCall:
			if block.ToolCall == nil {
				continue
			}
			input := json.RawMessage(block.ToolCall.Arguments)
			if !json.Valid(input) {
				input = json.RawMessage("{}")
			}
			blocks = append(blocks, proxy.ContentBlock{
				Type:  "tool_use",
				ID:    block.ToolCall.ID,
				Name:  block.ToolCall.Name,
				Input: input,
			})
		case proxy.CanonicalBlockToolResult:
			if block.ToolResult == nil {
				continue
			}
			content, err := json.Marshal(block.ToolResult.Content)
			if err != nil {
				continue
			}
			blocks = append(blocks, proxy.ContentBlock{
				Type:      "tool_result",
				ToolUseID: block.ToolResult.ToolCallID,
				Content:   content,
			})
		case proxy.CanonicalBlockImage:
			blocks = append(blocks, proxy.ContentBlock{Type: "image", Source: canonicalImageSource(block.Data)})
		case proxy.CanonicalBlockDocument:
			blocks = append(blocks, proxy.ContentBlock{Type: "document", Source: canonicalImageSource(block.Data)})
		}
	}
	return blocks
}

func canonicalImageSource(raw json.RawMessage) *proxy.ImageSource {
	if len(raw) == 0 {
		return nil
	}
	source := &proxy.ImageSource{}
	if err := json.Unmarshal(raw, source); err != nil {
		return nil
	}
	return source
}

func marshalPrettyJSON(value any) string {
	if value == nil {
		return ""
	}
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return ""
	}
	return string(body)
}
