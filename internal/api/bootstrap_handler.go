package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

func (server *Server) handleAdminAccountBootstrap(w http.ResponseWriter, r *http.Request) {
	if !server.isAdminAuthorized(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	bootstrap := server.deps.Bootstrap
	if bootstrap == nil {
		writeOpenAIError(w, http.StatusInternalServerError, "bootstrap not configured")
		return
	}

	switch r.Method {
	case http.MethodPost:
		var body struct {
			Method string `json:"method"`
		}
		if r.ContentLength > 0 {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeOpenAIError(w, http.StatusBadRequest, "invalid json: "+err.Error())
				return
			}
		}
		sess, err := bootstrap.Start(body.Method)
		if err != nil {
			status := http.StatusBadRequest
			if strings.Contains(err.Error(), "in progress") {
				status = http.StatusConflict
			}
			writeOpenAIError(w, status, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, sess)

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			writeOpenAIError(w, http.StatusBadRequest, "missing id parameter")
			return
		}
		if err := bootstrap.Cancel(id); err != nil {
			status := http.StatusBadRequest
			switch {
			case err.Error() == "session not found":
				status = http.StatusNotFound
			case strings.HasPrefix(err.Error(), "session already"):
				status = http.StatusConflict
			}
			writeOpenAIError(w, status, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})

	default:
		writeMethodNotAllowed(w, "POST, DELETE")
	}
}

func (server *Server) handleAdminAccountBootstrapStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	if !server.isAdminAuthorized(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	bootstrap := server.deps.Bootstrap
	if bootstrap == nil {
		writeOpenAIError(w, http.StatusInternalServerError, "bootstrap not configured")
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		writeOpenAIError(w, http.StatusBadRequest, "missing id parameter")
		return
	}

	sess := bootstrap.GetStatus(id)
	if sess == nil {
		writeOpenAIError(w, http.StatusNotFound, "session not found")
		return
	}

	writeJSON(w, http.StatusOK, sess)
}

func (server *Server) handleAdminAccountBootstrapSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, http.MethodPost)
		return
	}
	if !server.isAdminAuthorized(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	bootstrap := server.deps.Bootstrap
	if bootstrap == nil {
		writeOpenAIError(w, http.StatusInternalServerError, "bootstrap not configured")
		return
	}

	var body struct {
		ID          string `json:"id"`
		CallbackURL string `json:"callback_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}
	if body.ID == "" {
		writeOpenAIError(w, http.StatusBadRequest, "missing id")
		return
	}
	if body.CallbackURL == "" {
		writeOpenAIError(w, http.StatusBadRequest, "missing callback_url")
		return
	}

	sess, err := bootstrap.SubmitCallbackURL(body.ID, body.CallbackURL)
	if err != nil {
		status := http.StatusBadRequest
		switch {
		case strings.Contains(err.Error(), "session not found"):
			status = http.StatusNotFound
		case strings.Contains(err.Error(), "already"):
			status = http.StatusConflict
		}
		writeOpenAIError(w, status, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, sess)
}
