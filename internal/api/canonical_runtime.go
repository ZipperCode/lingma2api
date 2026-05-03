package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"lingma2api/internal/db"
	"lingma2api/internal/policy"
	"lingma2api/internal/proxy"
)

func requestSessionID(request *http.Request, bodySessionID string) string {
	sessionID := strings.TrimSpace(bodySessionID)
	if headerSession := strings.TrimSpace(request.Header.Get("X-Session-Id")); headerSession != "" {
		sessionID = headerSession
	}
	return sessionID
}

func attachCanonicalRequestMetadata(canonical *proxy.CanonicalRequest, headers http.Header) {
	if canonical.Metadata == nil {
		canonical.Metadata = map[string]any{}
	}
	if clientName := strings.TrimSpace(headers.Get("X-Client-Name")); clientName != "" {
		canonical.Metadata["client_name"] = clientName
	}
	if ingressTag := strings.TrimSpace(headers.Get("X-Ingress-Tag")); ingressTag != "" {
		canonical.Metadata["ingress_tag"] = ingressTag
	}
	if replayMode := strings.TrimSpace(headers.Get("X-Replay-Mode")); replayMode != "" {
		canonical.Metadata["replay_mode"] = replayMode
	}
}

func (server *Server) evaluateCanonicalRequest(ctx context.Context, canonical proxy.CanonicalRequest) (policy.EvaluationResult, error) {
	if replayMode, _ := canonical.Metadata["replay_mode"].(string); strings.EqualFold(replayMode, "historical") {
		return policy.EvaluationResult{
			Matched:           false,
			EffectiveActions:  policy.EvaluationResult{}.EffectiveActions,
			PostPolicyRequest: canonical,
		}, nil
	}
	if server.db == nil {
		return policy.EvaluationResult{
			Matched:           false,
			EffectiveActions:  policy.EvaluationResult{}.EffectiveActions,
			PostPolicyRequest: canonical,
		}, nil
	}

	rules, err := server.db.GetEnabledPolicies(ctx)
	if err != nil {
		return policy.EvaluationResult{}, err
	}
	return policy.EvaluateCanonicalRequest(rules, canonical)
}

func (server *Server) persistCanonicalExecutionRecord(
	ctx context.Context,
	ingressProtocol proxy.CanonicalProtocol,
	ingressEndpoint string,
	prePolicyRequest proxy.CanonicalRequest,
	postPolicyRequest proxy.CanonicalRequest,
	sessionCanonicalRequest proxy.CanonicalRequest,
	projectedRequest proxy.OpenAIChatRequest,
	requestMessages []proxy.Message,
	assistant proxy.Message,
	remoteRequest proxy.RemoteChatRequest,
	rawSSELines []string,
) {
	if server.db == nil {
		return
	}

	record := &db.CanonicalExecutionRecordRow{
		ID:                proxy.NewUUID(),
		CreatedAt:         server.deps.Now(),
		IngressProtocol:   string(ingressProtocol),
		IngressEndpoint:   ingressEndpoint,
		SessionID:         postPolicyRequest.SessionID,
		PrePolicyRequest:  prePolicyRequest,
		PostPolicyRequest: postPolicyRequest,
		SessionSnapshot: buildCanonicalSessionSnapshot(
			ingressProtocol,
			sessionCanonicalRequest,
			assistant,
			server.deps.Now(),
		),
		SouthboundRequest: remoteRequest.BodyJSON,
		Sidecar: &proxy.CanonicalExecutionSidecar{
			SchemaVersion: 1,
			RawSSELines:   rawSSELines,
			Metadata: map[string]any{
				"request_id":        remoteRequest.RequestID,
				"model_key":         remoteRequest.ModelKey,
				"stream":            remoteRequest.Stream,
				"upstream_status":   200,
				"prompt_tokens":     db.EstimateMessageTokens(requestMessages),
				"completion_tokens": db.EstimateMessageTokens([]proxy.Message{assistant}),
				"total_tokens":      db.EstimateMessageTokens(requestMessages) + db.EstimateMessageTokens([]proxy.Message{assistant}),
			},
		},
	}
	_ = server.db.InsertCanonicalExecutionRecord(ctx, record)
}

func buildCanonicalSessionSnapshot(
	ingressProtocol proxy.CanonicalProtocol,
	postPolicyRequest proxy.CanonicalRequest,
	assistant proxy.Message,
	now time.Time,
) *proxy.CanonicalSessionSnapshot {
	assistantCanonical, err := proxy.CanonicalizeOpenAIRequest(proxy.OpenAIChatRequest{
		Messages: []proxy.Message{assistant},
	}, postPolicyRequest.SessionID)
	if err != nil {
		return nil
	}
	turns := make([]proxy.CanonicalTurn, 0, len(postPolicyRequest.Turns)+len(assistantCanonical.Turns))
	turns = append(turns, postPolicyRequest.Turns...)
	turns = append(turns, assistantCanonical.Turns...)

	return &proxy.CanonicalSessionSnapshot{
		SchemaVersion:   1,
		SessionID:       postPolicyRequest.SessionID,
		IngressProtocol: ingressProtocol,
		Turns:           turns,
		UpdatedAt:       now,
	}
}

func cloneMetadataMap(metadata map[string]any) map[string]any {
	if metadata == nil {
		return nil
	}
	cloned := make(map[string]any, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}
