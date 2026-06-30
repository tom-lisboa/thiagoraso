package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"mentoria-automation-server/internal/storage"
	"mentoria-automation-server/internal/workflows"
)

type API struct {
	logger     *slog.Logger
	runner     *workflows.Runner
	eventStore storage.Store
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
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	eventID, ok := api.recordWebhookEvent(w, r, "n8n-replacement", body)
	if !ok {
		return
	}

	var input workflows.N8NReplacementInput
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&input); err != nil {
		api.markWebhookEvent(r, eventID, storage.EventResult{
			Status:       "invalid_json",
			HTTPStatus:   http.StatusBadRequest,
			ResponseBody: map[string]string{"error": "invalid JSON body"},
			ErrorMessage: err.Error(),
		})
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	output, err := api.runner.RunN8NReplacement(r.Context(), input)
	if err != nil {
		api.logger.Error("workflow failed", "workflow", "n8n-replacement", "error", err)
		api.markWebhookEvent(r, eventID, storage.EventResult{
			Status:       "failed",
			HTTPStatus:   http.StatusInternalServerError,
			ResponseBody: map[string]string{"error": "workflow failed"},
			ErrorMessage: err.Error(),
		})
		writeError(w, http.StatusInternalServerError, "workflow failed")
		return
	}

	api.markWebhookEvent(r, eventID, storage.EventResult{
		Status:       output.Status,
		HTTPStatus:   http.StatusOK,
		ResponseBody: output,
	})
	writeJSON(w, http.StatusOK, output)
}

func (api API) runClosedDeal(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	eventID, ok := api.recordWebhookEvent(w, r, "closed-deal", body)
	if !ok {
		return
	}

	var input workflows.ClosedDealInput
	if err := json.NewDecoder(bytes.NewReader(body)).Decode(&input); err != nil {
		api.markWebhookEvent(r, eventID, storage.EventResult{
			Status:       "invalid_json",
			HTTPStatus:   http.StatusBadRequest,
			ResponseBody: map[string]string{"error": "invalid JSON body"},
			ErrorMessage: err.Error(),
		})
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	output, err := api.runner.RunClosedDeal(r.Context(), input)
	if err != nil {
		api.logger.Error("workflow failed", "workflow", "closed-deal", "error", err)
		api.markWebhookEvent(r, eventID, storage.EventResult{
			Status:       "failed",
			HTTPStatus:   http.StatusInternalServerError,
			ResponseBody: map[string]string{"error": "workflow failed"},
			ErrorMessage: err.Error(),
		})
		writeError(w, http.StatusInternalServerError, "workflow failed")
		return
	}

	api.markWebhookEvent(r, eventID, storage.EventResult{
		Status:       output.Status,
		HTTPStatus:   http.StatusOK,
		ResponseBody: output,
	})
	writeJSON(w, http.StatusOK, output)
}

func (api API) recordWebhookEvent(w http.ResponseWriter, r *http.Request, workflow string, body []byte) (int64, bool) {
	if api.eventStore == nil {
		return 0, true
	}

	id, err := api.eventStore.RecordInbound(r.Context(), storage.InboundEvent{
		Workflow:   workflow,
		Method:     r.Method,
		Path:       r.URL.Path,
		RemoteAddr: r.RemoteAddr,
		Headers:    r.Header,
		Query:      r.URL.Query(),
		RawBody:    body,
	})
	if err != nil {
		api.logger.Error("failed to persist webhook event", "workflow", workflow, "error", err)
		writeError(w, http.StatusInternalServerError, "event persistence failed")
		return 0, false
	}

	return id, true
}

func (api API) markWebhookEvent(r *http.Request, id int64, result storage.EventResult) {
	if api.eventStore == nil || id == 0 {
		return
	}
	if err := api.eventStore.MarkFinished(r.Context(), id, result); err != nil {
		api.logger.Error("failed to update webhook event", "event_id", id, "error", err)
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
