package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mentoria-automation-server/internal/storage"
	"mentoria-automation-server/internal/workflows"
)

type fakeEventStore struct {
	event        storage.InboundEvent
	result       storage.EventResult
	metricPeriod storage.DashboardPeriod
}

func (s *fakeEventStore) RecordInbound(_ context.Context, event storage.InboundEvent) (int64, error) {
	s.event = event
	return 123, nil
}

func (s *fakeEventStore) MarkFinished(_ context.Context, _ int64, result storage.EventResult) error {
	s.result = result
	return nil
}

func (s *fakeEventStore) DashboardMetrics(_ context.Context, period storage.DashboardPeriod) (storage.DashboardMetrics, error) {
	s.metricPeriod = period
	return storage.DashboardMetrics{Period: period.Key, PeriodLabel: period.Label}, nil
}

func (s *fakeEventStore) Close() error {
	return nil
}

func TestMetricsUsesRequestedPeriod(t *testing.T) {
	store := &fakeEventStore{}
	runner := workflows.NewRunner(slog.Default(), workflows.Config{})
	handler := NewRouter(slog.Default(), runner, store)

	req := httptest.NewRequest(http.MethodGet, "/mentoria/api/metrics?period=7d", nil)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}
	if store.metricPeriod.Key != "7d" {
		t.Fatalf("unexpected metric period: %q", store.metricPeriod.Key)
	}
}

func TestRunN8NReplacementPersistsInvalidJSON(t *testing.T) {
	store := &fakeEventStore{}
	runner := workflows.NewRunner(slog.Default(), workflows.Config{})
	handler := NewRouter(slog.Default(), runner, store)

	req := httptest.NewRequest(http.MethodPost, "/mentoria/webhooks/n8n-replacement", strings.NewReader("{bad-json"))
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, res.Code)
	}
	if store.event.Workflow != "n8n-replacement" {
		t.Fatalf("unexpected stored workflow: %q", store.event.Workflow)
	}
	if string(store.event.RawBody) != "{bad-json" {
		t.Fatalf("unexpected raw body: %q", string(store.event.RawBody))
	}
	if store.result.Status != "invalid_json" {
		t.Fatalf("unexpected stored result: %q", store.result.Status)
	}
}
