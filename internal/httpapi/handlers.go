package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"mentoria-automation-server/internal/workflows"
)

type API struct {
	logger *slog.Logger
	runner *workflows.Runner
}

func (api API) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (api API) verifyMetaWebhook(w http.ResponseWriter, r *http.Request) {
	challenge, ok := api.runner.VerifyMetaWebhook(r.URL.Query())
	if !ok {
		writeError(w, http.StatusForbidden, "invalid verification token")
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(challenge))
}

func (api API) runN8NReplacement(w http.ResponseWriter, r *http.Request) {
	var input workflows.N8NReplacementInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	output, err := api.runner.RunN8NReplacement(r.Context(), input)
	if err != nil {
		api.logger.Error("workflow failed", "workflow", "n8n-replacement", "error", err)
		writeError(w, http.StatusInternalServerError, "workflow failed")
		return
	}

	writeJSON(w, http.StatusOK, output)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
