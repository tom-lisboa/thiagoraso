package httpapi

import (
	"log/slog"
	"net/http"

	"mentoria-automation-server/internal/workflows"
)

func NewRouter(logger *slog.Logger, runner *workflows.Runner) http.Handler {
	mux := http.NewServeMux()

	api := API{
		logger: logger,
		runner: runner,
	}

	mux.HandleFunc("GET /healthz", api.health)
	mux.HandleFunc("GET /webhooks/n8n-replacement", api.verifyMetaWebhook)
	mux.HandleFunc("POST /webhooks/n8n-replacement", api.runN8NReplacement)
	mux.HandleFunc("POST /webhooks/negocio-fechado", api.runClosedDeal)
	mux.HandleFunc("POST /NEGOCIOFECHADO", api.runClosedDeal)
	mux.HandleFunc("GET /mentoria/healthz", api.health)
	mux.HandleFunc("GET /mentoria/webhooks/n8n-replacement", api.verifyMetaWebhook)
	mux.HandleFunc("POST /mentoria/webhooks/n8n-replacement", api.runN8NReplacement)
	mux.HandleFunc("POST /mentoria/webhooks/negocio-fechado", api.runClosedDeal)
	mux.HandleFunc("POST /mentoria/NEGOCIOFECHADO", api.runClosedDeal)

	return loggingMiddleware(logger, mux)
}

func loggingMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		logger.Info("request completed", "method", r.Method, "path", r.URL.Path, "remote_addr", r.RemoteAddr)
	})
}
