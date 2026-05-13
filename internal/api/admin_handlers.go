package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"lingma2api/internal/db"
	"lingma2api/internal/policy"
	"lingma2api/internal/proxy"
)

const overviewRecentRequestLimit = 8
const overviewLatencySampleLimit = 120

type adminOverviewResponse struct {
	Healthy         bool                  `json:"healthy"`
	GeneratedAt     time.Time             `json:"generated_at"`
	Credential      proxy.CredentialStatus `json:"credential"`
	Models          proxy.ModelStatus     `json:"models"`
	SessionCount    int                   `json:"session_count"`
	TokenStats      map[string]int        `json:"token_stats"`
	Dashboard       db.DashboardData      `json:"dashboard"`
	Latency         adminLatencyStats     `json:"latency"`
	RecentRequests  []db.RequestLog       `json:"recent_requests"`
	AvailableModels []proxy.OpenAIModel   `json:"available_models"`
	Settings        map[string]string     `json:"settings"`
}

type adminLatencyStats struct {
	AvgMs       int `json:"avg_ms"`
	P50Ms       int `json:"p50_ms"`
	P95Ms       int `json:"p95_ms"`
	MaxMs       int `json:"max_ms"`
	SampleCount int `json:"sample_count"`
}

type adminModelsResponse struct {
	Items  []proxy.OpenAIModel `json:"items"`
	Status proxy.ModelStatus   `json:"status"`
}

func (server *Server) handleAdminOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	if !server.isAdminAuthorized(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	overview, err := server.buildAdminOverview(r)
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, overview)
}

func (server *Server) handleAdminModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeMethodNotAllowed(w, "GET, POST")
		return
	}
	if !server.isAdminAuthorized(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if r.Method == http.MethodPost {
		if err := server.deps.Models.Refresh(r.Context()); err != nil {
			writeMappedError(w, err)
			return
		}
	}

	models, err := server.deps.Models.ListModels(r.Context())
	if err != nil {
		writeMappedError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, adminModelsResponse{
		Items:  models,
		Status: server.deps.Models.Status(),
	})
}

func (server *Server) handleAdminDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	if !server.isAdminAuthorized(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	rangeParam := r.URL.Query().Get("range")
	if rangeParam == "" {
		rangeParam = "24h"
	}
	data, err := server.db.GetDashboardData(r.Context(), rangeParam)
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func (server *Server) buildAdminOverview(r *http.Request) (adminOverviewResponse, error) {
	overview := adminOverviewResponse{
		GeneratedAt: server.deps.Now(),
		Credential:  server.deps.Credentials.Status(),
		Models:      server.deps.Models.Status(),
		TokenStats: map[string]int{
			"today": 0,
			"week":  0,
			"total": 0,
		},
		Dashboard: db.DashboardData{
			SuccessRateSeries: []db.TimeSeriesPoint{},
			TokenSeries:       []db.TimeSeriesPoint{},
			ModelDistribution: []db.ModelDistPoint{},
		},
		RecentRequests:  []db.RequestLog{},
		AvailableModels: []proxy.OpenAIModel{},
		Settings:        map[string]string{},
	}

	if sessions, err := server.deps.Sessions.List(r.Context()); err == nil {
		overview.SessionCount = len(sessions)
	}

	if server.db != nil {
		if settings, err := server.db.GetSettings(r.Context()); err == nil && settings != nil {
			overview.Settings = settings
		}
		if today, week, total, err := server.db.GetTokenStats(r.Context()); err == nil {
			overview.TokenStats["today"] = today
			overview.TokenStats["week"] = week
			overview.TokenStats["total"] = total
		}
		if dashboard, err := server.db.GetDashboardData(r.Context(), "24h"); err == nil {
			overview.Dashboard = dashboard
		}
		recent, err := server.listAdminLogs(r.Context(), db.LogFilter{}, 1, overviewRecentRequestLimit)
		if err == nil {
			overview.RecentRequests = recent.Items
		}
		latencySample, err := server.exportAdminLogs(r.Context(), db.LogFilter{})
		if err == nil {
			if len(latencySample) > overviewLatencySampleLimit {
				latencySample = latencySample[:overviewLatencySampleLimit]
			}
			overview.Latency = buildLatencyStats(latencySample)
		}
	}

	models, modelErr := server.deps.Models.ListModels(r.Context())
	if modelErr == nil {
		overview.AvailableModels = models
		overview.Models = server.deps.Models.Status()
	}

	overview.Healthy = overview.Credential.Loaded && overview.Credential.HasCredentials && (overview.Models.Cached || len(overview.AvailableModels) > 0)
	return overview, nil
}

func buildLatencyStats(logs []db.RequestLog) adminLatencyStats {
	values := make([]int, 0, len(logs))
	for _, log := range logs {
		if ms := logLatencyMs(log); ms > 0 {
			values = append(values, ms)
		}
	}
	if len(values) == 0 {
		return adminLatencyStats{}
	}
	sort.Ints(values)
	sum := 0
	for _, value := range values {
		sum += value
	}
	return adminLatencyStats{
		AvgMs:       int(math.Round(float64(sum) / float64(len(values)))),
		P50Ms:       percentileValue(values, 0.50),
		P95Ms:       percentileValue(values, 0.95),
		MaxMs:       values[len(values)-1],
		SampleCount: len(values),
	}
}

func percentileValue(sorted []int, p float64) int {
	if len(sorted) == 0 {
		return 0
	}
	index := int(math.Ceil(float64(len(sorted))*p)) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func logLatencyMs(log db.RequestLog) int {
	switch {
	case log.UpstreamMs > 0:
		return log.UpstreamMs
	case log.DownstreamMs > 0:
		return log.DownstreamMs
	case log.TTFTMs > 0:
		return log.TTFTMs
	default:
		return 0
	}
}

func (server *Server) handleAdminAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	if !server.isAdminAuthorized(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	cred, _ := server.deps.Credentials.Current(r.Context())
	today, week, total, _ := server.db.GetTokenStats(r.Context())
	storedMeta := server.deps.Credentials.StoredMeta()
	hasAT, hasRT := server.deps.Credentials.HasOAuth()
	writeJSON(w, http.StatusOK, map[string]any{
		"credential":   cred,
		"status":       server.deps.Credentials.Status(),
		"token_stats": map[string]int{
			"today": today,
			"week":  week,
			"total": total,
		},
		"stored_meta": storedMeta,
		"oauth": map[string]bool{
			"has_access_token":  hasAT,
			"has_refresh_token": hasRT,
		},
	})
}

func (server *Server) handleAdminAccountTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, http.MethodPost)
		return
	}
	if !server.isAdminAuthorized(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	cred, err := server.deps.Credentials.Current(r.Context())
	if err != nil {
		log.Printf("[account-test] credential load failed: %v", err)
		writeJSON(w, http.StatusOK, map[string]any{
			"success":      false,
			"status_code":  0,
			"response_preview": "",
			"error":        err.Error(),
			"credential_snapshot": map[string]bool{
				"has_cosy_key":         false,
				"has_encrypt_user_info": false,
				"has_user_id":          false,
				"has_machine_id":       false,
			},
			"timestamp": server.deps.Now().Format(time.RFC3339),
		})
		return
	}

	snapshot := map[string]bool{
		"has_cosy_key":          cred.CosyKey != "",
		"has_encrypt_user_info": cred.EncryptUserInfo != "",
		"has_user_id":           cred.UserID != "",
		"has_machine_id":        cred.MachineID != "",
	}

	// Log credential details for diagnosis
	log.Printf("[account-test] credential: user_id=%s machine_id=%s cosy_key_len=%d encrypt_info_len=%d source=%s",
		cred.UserID, cred.MachineID, len(cred.CosyKey), len(cred.EncryptUserInfo), cred.Source)

	models, testErr := server.deps.Models.ListModels(r.Context())
	if testErr != nil {
		var upstream *proxy.UpstreamHTTPError
		statusCode := 0
		responsePreview := ""
		if errors.As(testErr, &upstream) {
			statusCode = upstream.StatusCode
			responsePreview = upstream.Body
		}
		log.Printf("[account-test] ListModels failed: %v (status=%d)", testErr, statusCode)
		writeJSON(w, http.StatusOK, map[string]any{
			"success":           false,
			"status_code":       statusCode,
			"response_preview":  responsePreview,
			"error":             testErr.Error(),
			"credential_snapshot": snapshot,
			"cosy_key_prefix":   safePrefix(cred.CosyKey, 20),
			"user_id":           cred.UserID,
			"timestamp":         server.deps.Now().Format(time.RFC3339),
		})
		return
	}

	log.Printf("[account-test] ListModels success: %d models", len(models))
	writeJSON(w, http.StatusOK, map[string]any{
		"success":           true,
		"status_code":       200,
		"response_preview":  fmt.Sprintf("ListModels returned %d models", len(models)),
		"error":             "",
		"credential_snapshot": snapshot,
		"cosy_key_prefix":   safePrefix(cred.CosyKey, 20),
		"user_id":           cred.UserID,
		"timestamp":         server.deps.Now().Format(time.RFC3339),
	})
}

func safePrefix(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func (server *Server) handleAdminAccountRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, http.MethodPost)
		return
	}
	if !server.isAdminAuthorized(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	cred, err := server.deps.Credentials.Refresh(r.Context())
	if err != nil {
		writeMappedError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"credential": cred})
}

func (server *Server) handleAdminSettingsGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	if !server.isAdminAuthorized(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	settings, err := server.db.GetSettings(r.Context())
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (server *Server) handleAdminSettingsUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeMethodNotAllowed(w, http.MethodPut)
		return
	}
	if !server.isAdminAuthorized(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var settings map[string]string
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := server.db.UpdateSettings(r.Context(), settings); err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (server *Server) handleAdminLogsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	if !server.isAdminAuthorized(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit < 1 || limit > 200 {
		limit = 50
	}

	filter := db.LogFilter{
		Status: q.Get("status"),
		Model:  q.Get("model"),
	}
	if from := q.Get("from"); from != "" {
		filter.From, _ = parseTime(from)
	}
	if to := q.Get("to"); to != "" {
		filter.To, _ = parseTime(to)
	}

	result, err := server.listAdminLogs(r.Context(), filter, page, limit)
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (server *Server) handleAdminLogsGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	if !server.isAdminAuthorized(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/admin/logs/")
	id = strings.TrimSuffix(id, "/replay")
	if id == "" {
		writeOpenAIError(w, http.StatusBadRequest, "missing log id")
		return
	}
	log, _, err := server.getAdminLog(r.Context(), id)
	if err != nil {
		writeOpenAIError(w, http.StatusNotFound, "log not found")
		return
	}
	writeJSON(w, http.StatusOK, log)
}

func (server *Server) handleAdminLogsReplay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, http.MethodPost)
		return
	}
	if !server.isAdminAuthorized(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/admin/logs/")
	id = strings.TrimSuffix(id, "/replay")
	bodyBytes, _ := io.ReadAll(r.Body)
	var replayBody io.ReadCloser
	replayPath := "/v1/chat/completions"
	if len(bodyBytes) > 0 {
		replayBody = io.NopCloser(bytes.NewReader(bodyBytes))
	} else {
		record, err := server.db.GetCanonicalExecutionRecord(r.Context(), id)
		if err == nil {
			canonicalRequest := canonicalReplayRequestForMode(record, r.URL.Query().Get("mode"))
			marshaled, marshalErr := marshalReplayBodyFromCanonical(canonicalRequest)
			if marshalErr != nil {
				writeOpenAIError(w, http.StatusInternalServerError, marshalErr.Error())
				return
			}
			replayBody = io.NopCloser(bytes.NewReader(marshaled))
			replayPath = replayEndpointForCanonicalRequest(canonicalRequest, record.IngressEndpoint)
			if isHistoricalReplayMode(r.URL.Query().Get("mode")) {
				r.Header.Set("X-Replay-Mode", "historical")
			}
		} else {
			replayReq, getErr := server.db.GetLog(r.Context(), id)
			if getErr != nil {
				writeOpenAIError(w, http.StatusNotFound, "log not found")
				return
			}
			replayBody = io.NopCloser(strings.NewReader(replayReq.DownstreamReq))
		}
	}
	newReq := r.Clone(r.Context())
	newReq.Method = http.MethodPost
	newReq.URL.Path = replayPath
	newReq.Body = replayBody
	if replayPath == "/v1/messages" {
		server.handleAnthropicMessages(w, newReq)
		return
	}
	server.handleChatCompletions(w, newReq)
}

func (server *Server) handleAdminLogsCleanup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, http.MethodPost)
		return
	}
	if !server.isAdminAuthorized(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	settings, _ := server.db.GetSettings(r.Context())
	days := 30
	if d, err := strconv.Atoi(settings["retention_days"]); err == nil {
		days = d
	}
	affected, err := server.db.CleanupExpiredLogs(r.Context(), days)
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": affected})
}

func (server *Server) handleAdminLogsExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	if !server.isAdminAuthorized(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	q := r.URL.Query()
	filter := db.LogFilter{Status: q.Get("status"), Model: q.Get("model")}
	if from := q.Get("from"); from != "" {
		filter.From, _ = parseTime(from)
	}
	if to := q.Get("to"); to != "" {
		filter.To, _ = parseTime(to)
	}

	logs, err := server.exportAdminLogs(r.Context(), filter)
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	format := q.Get("format")
	if format == "csv" {
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=logs.csv")
		w.Write([]byte("id,created_at,model,status,prompt_tokens,completion_tokens,total_tokens,ttft_ms\n"))
		for _, l := range logs {
			w.Write([]byte(l.ID + "," + l.CreatedAt.Format("2006-01-02T15:04:05Z") + "," + l.Model + "," + l.Status + "," +
				strconv.Itoa(l.PromptTokens) + "," + strconv.Itoa(l.CompletionTokens) + "," + strconv.Itoa(l.TotalTokens) + "," + strconv.Itoa(l.TTFTMs) + "\n"))
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=logs.json")
	json.NewEncoder(w).Encode(logs)
}

func (server *Server) handleAdminStatsExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	if !server.isAdminAuthorized(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	rangeParam := r.URL.Query().Get("range")
	if rangeParam == "" {
		rangeParam = "24h"
	}
	data, err := server.db.GetDashboardData(r.Context(), rangeParam)
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=stats.json")
	json.NewEncoder(w).Encode(data)
}

func (server *Server) handleAdminPoliciesList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	if !server.isAdminAuthorized(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	items, err := server.db.ListPolicies(r.Context())
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (server *Server) handleAdminPoliciesCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, http.MethodPost)
		return
	}
	if !server.isAdminAuthorized(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var policy db.PolicyRule
	if err := json.NewDecoder(r.Body).Decode(&policy); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(policy.Name) == "" {
		writeOpenAIError(w, http.StatusBadRequest, "policy name is required")
		return
	}
	if err := server.db.CreatePolicy(r.Context(), &policy); err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, policy)
}

func (server *Server) handleAdminPoliciesUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeMethodNotAllowed(w, http.MethodPut)
		return
	}
	if !server.isAdminAuthorized(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	idStr := strings.TrimPrefix(r.URL.Path, "/admin/policies/")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var policy db.PolicyRule
	if err := json.NewDecoder(r.Body).Decode(&policy); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid json")
		return
	}
	policy.ID = id
	if strings.TrimSpace(policy.Name) == "" {
		writeOpenAIError(w, http.StatusBadRequest, "policy name is required")
		return
	}
	if err := server.db.UpdatePolicy(r.Context(), &policy); err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, policy)
}

func (server *Server) handleAdminPoliciesDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeMethodNotAllowed(w, http.MethodDelete)
		return
	}
	if !server.isAdminAuthorized(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	idStr := strings.TrimPrefix(r.URL.Path, "/admin/policies/")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := server.db.DeletePolicy(r.Context(), id); err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (server *Server) handleAdminPoliciesTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, http.MethodPost)
		return
	}
	if !server.isAdminAuthorized(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req struct {
		Protocol       string `json:"protocol"`
		RequestedModel string `json:"requested_model"`
		Stream         bool   `json:"stream"`
		HasTools       bool   `json:"has_tools"`
		HasReasoning   bool   `json:"has_reasoning"`
		SessionPresent bool   `json:"session_present"`
		ClientName     string `json:"client_name"`
		IngressTag     string `json:"ingress_tag"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid json")
		return
	}
	policies, err := server.db.GetEnabledPolicies(r.Context())
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	evaluated, err := policy.EvaluateMatchAttributes(policies, policy.MatchAttributes{
		Protocol:       req.Protocol,
		RequestedModel: req.RequestedModel,
		Stream:         req.Stream,
		HasTools:       req.HasTools,
		HasReasoning:   req.HasReasoning,
		SessionPresent: req.SessionPresent,
		ClientName:     req.ClientName,
		IngressTag:     req.IngressTag,
	})
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, err.Error())
		return
	}
	if evaluated.EffectiveActions.RewriteModel == nil && req.RequestedModel != "" {
		evaluated.EffectiveActions.RewriteModel = &req.RequestedModel
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"matched":           evaluated.Matched,
		"effective_actions": evaluated.EffectiveActions,
		"matched_rules":     evaluated.MatchedRules,
	})
}

func policyMatchesRequest(match db.PolicyMatch, req struct {
	Protocol       string `json:"protocol"`
	RequestedModel string `json:"requested_model"`
	Stream         bool   `json:"stream"`
	HasTools       bool   `json:"has_tools"`
	HasReasoning   bool   `json:"has_reasoning"`
	SessionPresent bool   `json:"session_present"`
	ClientName     string `json:"client_name"`
	IngressTag     string `json:"ingress_tag"`
}) bool {
	if match.Protocol != "" && match.Protocol != req.Protocol {
		return false
	}
	if match.RequestedModel != "" {
		ok, err := matchRegex(match.RequestedModel, req.RequestedModel)
		if err != nil || !ok {
			return false
		}
	}
	if match.Stream != nil && *match.Stream != req.Stream {
		return false
	}
	if match.HasTools != nil && *match.HasTools != req.HasTools {
		return false
	}
	if match.HasReasoning != nil && *match.HasReasoning != req.HasReasoning {
		return false
	}
	if match.SessionPresent != nil && *match.SessionPresent != req.SessionPresent {
		return false
	}
	if match.ClientName != "" && match.ClientName != req.ClientName {
		return false
	}
	if match.IngressTag != "" && match.IngressTag != req.IngressTag {
		return false
	}
	return true
}

func parseTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}

func matchRegex(pattern, input string) (bool, error) {
	if len(pattern) > 1024 {
		return false, fmt.Errorf("pattern too long")
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false, err
	}
	return re.MatchString(input), nil
}
