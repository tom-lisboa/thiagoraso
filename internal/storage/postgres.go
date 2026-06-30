package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type Store interface {
	RecordInbound(ctx context.Context, event InboundEvent) (int64, error)
	MarkFinished(ctx context.Context, id int64, result EventResult) error
	DashboardMetrics(ctx context.Context, period DashboardPeriod) (DashboardMetrics, error)
	Close() error
}

type InboundEvent struct {
	Workflow   string
	Method     string
	Path       string
	RemoteAddr string
	Headers    http.Header
	Query      url.Values
	RawBody    []byte
}

type EventResult struct {
	Status       string
	HTTPStatus   int
	ResponseBody any
	ErrorMessage string
}

type DashboardMetrics struct {
	GeneratedAt    time.Time             `json:"generated_at"`
	Period         string                `json:"period"`
	PeriodLabel    string                `json:"period_label"`
	TotalEvents    int64                 `json:"total_events"`
	EventsToday    int64                 `json:"events_today"`
	EventsLast24h  int64                 `json:"events_last_24h"`
	Successful     int64                 `json:"successful"`
	Failed         int64                 `json:"failed"`
	InvalidJSON    int64                 `json:"invalid_json"`
	SuccessRate    float64               `json:"success_rate"`
	StatusCounts   []MetricCount         `json:"status_counts"`
	WorkflowCounts []MetricCount         `json:"workflow_counts"`
	HourlyEvents   []HourlyMetric        `json:"hourly_events"`
	RecentEvents   []WebhookEventSummary `json:"recent_events"`
}

type DashboardPeriod struct {
	Key   string
	Label string
}

type MetricCount struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

type HourlyMetric struct {
	Hour  time.Time `json:"hour"`
	Count int64     `json:"count"`
}

type WebhookEventSummary struct {
	ID           int64     `json:"id"`
	Workflow     string    `json:"workflow"`
	Status       string    `json:"status"`
	HTTPStatus   int       `json:"http_status,omitempty"`
	Path         string    `json:"path"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	ErrorMessage string    `json:"error_message,omitempty"`
}

type PostgresStore struct {
	db *sql.DB
}

func OpenPostgres(ctx context.Context, dsn string) (*PostgresStore, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}

	store := &PostgresStore{db: db}
	if err := store.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *PostgresStore) Close() error {
	return s.db.Close()
}

func (s *PostgresStore) migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS webhook_events (
	id BIGSERIAL PRIMARY KEY,
	workflow TEXT NOT NULL,
	method TEXT NOT NULL,
	path TEXT NOT NULL,
	remote_addr TEXT NOT NULL,
	headers JSONB NOT NULL DEFAULT '{}'::jsonb,
	query JSONB NOT NULL DEFAULT '{}'::jsonb,
	raw_body TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'received',
	http_status INTEGER,
	response_body JSONB,
	error_message TEXT,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS webhook_events_workflow_status_created_idx
	ON webhook_events (workflow, status, created_at DESC);
`)
	return err
}

func (s *PostgresStore) RecordInbound(ctx context.Context, event InboundEvent) (int64, error) {
	headers, err := json.Marshal(event.Headers)
	if err != nil {
		return 0, err
	}
	query, err := json.Marshal(event.Query)
	if err != nil {
		return 0, err
	}

	var id int64
	err = s.db.QueryRowContext(ctx, `
INSERT INTO webhook_events (workflow, method, path, remote_addr, headers, query, raw_body)
VALUES ($1, $2, $3, $4, $5::jsonb, $6::jsonb, $7)
RETURNING id
`, event.Workflow, event.Method, event.Path, event.RemoteAddr, string(headers), string(query), string(event.RawBody)).Scan(&id)
	return id, err
}

func (s *PostgresStore) MarkFinished(ctx context.Context, id int64, result EventResult) error {
	if id == 0 {
		return nil
	}

	var responseBody any
	if result.ResponseBody != nil {
		encoded, err := json.Marshal(result.ResponseBody)
		if err != nil {
			return err
		}
		responseBody = string(encoded)
	}

	_, err := s.db.ExecContext(ctx, `
UPDATE webhook_events
SET status = $2,
	http_status = $3,
	response_body = CASE WHEN $4::text IS NULL THEN NULL ELSE $4::jsonb END,
	error_message = $5,
	updated_at = $6
WHERE id = $1
`, id, result.Status, result.HTTPStatus, responseBody, emptyStringToNil(result.ErrorMessage), time.Now().UTC())
	return err
}

func NormalizeDashboardPeriod(value string) DashboardPeriod {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "today", "hoje":
		return DashboardPeriod{Key: "today", Label: "Hoje"}
	case "24h":
		return DashboardPeriod{Key: "24h", Label: "Últimas 24h"}
	case "7d", "7":
		return DashboardPeriod{Key: "7d", Label: "Últimos 7 dias"}
	case "30d", "30":
		return DashboardPeriod{Key: "30d", Label: "Últimos 30 dias"}
	case "all", "tudo":
		return DashboardPeriod{Key: "all", Label: "Tudo"}
	default:
		return DashboardPeriod{Key: "24h", Label: "Últimas 24h"}
	}
}

func (s *PostgresStore) DashboardMetrics(ctx context.Context, period DashboardPeriod) (DashboardMetrics, error) {
	period = NormalizeDashboardPeriod(period.Key)
	metrics := DashboardMetrics{
		GeneratedAt: time.Now().UTC(),
		Period:      period.Key,
		PeriodLabel: period.Label,
	}

	err := s.db.QueryRowContext(ctx, `
SELECT
	count(*),
	count(*) FILTER (WHERE created_at::date = current_date),
	count(*) FILTER (WHERE created_at >= now() - interval '24 hours'),
	count(*) FILTER (WHERE status IN ('processed', 'duplicate')),
	count(*) FILTER (WHERE status = 'failed'),
	count(*) FILTER (WHERE status = 'invalid_json')
FROM webhook_events
WHERE `+dashboardPeriodSQL("created_at")+`
`, period.Key).Scan(
		&metrics.TotalEvents,
		&metrics.EventsToday,
		&metrics.EventsLast24h,
		&metrics.Successful,
		&metrics.Failed,
		&metrics.InvalidJSON,
	)
	if err != nil {
		return DashboardMetrics{}, err
	}
	if metrics.TotalEvents > 0 {
		metrics.SuccessRate = float64(metrics.Successful) / float64(metrics.TotalEvents)
	}

	statusCounts, err := s.metricCounts(ctx, `
SELECT status, count(*)
FROM webhook_events
WHERE `+dashboardPeriodSQL("created_at")+`
GROUP BY status
ORDER BY count(*) DESC, status ASC
`, period.Key)
	if err != nil {
		return DashboardMetrics{}, err
	}
	metrics.StatusCounts = statusCounts

	workflowCounts, err := s.metricCounts(ctx, `
SELECT workflow, count(*)
FROM webhook_events
WHERE `+dashboardPeriodSQL("created_at")+`
GROUP BY workflow
ORDER BY count(*) DESC, workflow ASC
`, period.Key)
	if err != nil {
		return DashboardMetrics{}, err
	}
	metrics.WorkflowCounts = workflowCounts

	hourlyEvents, err := s.hourlyMetrics(ctx, period)
	if err != nil {
		return DashboardMetrics{}, err
	}
	metrics.HourlyEvents = hourlyEvents

	recentEvents, err := s.recentEvents(ctx, period)
	if err != nil {
		return DashboardMetrics{}, err
	}
	metrics.RecentEvents = recentEvents

	return metrics, nil
}

func (s *PostgresStore) metricCounts(ctx context.Context, query string, args ...any) ([]MetricCount, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var counts []MetricCount
	for rows.Next() {
		var item MetricCount
		if err := rows.Scan(&item.Name, &item.Count); err != nil {
			return nil, err
		}
		counts = append(counts, item)
	}
	return counts, rows.Err()
}

func (s *PostgresStore) hourlyMetrics(ctx context.Context, period DashboardPeriod) ([]HourlyMetric, error) {
	bucket := "hour"
	if period.Key == "7d" || period.Key == "30d" || period.Key == "all" {
		bucket = "day"
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT date_trunc($2, created_at) AS hour, count(*)
FROM webhook_events
WHERE `+dashboardPeriodSQL("created_at")+`
GROUP BY hour
ORDER BY hour ASC
`, period.Key, bucket)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var metrics []HourlyMetric
	for rows.Next() {
		var item HourlyMetric
		if err := rows.Scan(&item.Hour, &item.Count); err != nil {
			return nil, err
		}
		metrics = append(metrics, item)
	}
	return metrics, rows.Err()
}

func (s *PostgresStore) recentEvents(ctx context.Context, period DashboardPeriod) ([]WebhookEventSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, workflow, status, coalesce(http_status, 0), path, created_at, updated_at, coalesce(error_message, '')
FROM webhook_events
WHERE `+dashboardPeriodSQL("created_at")+`
ORDER BY id DESC
LIMIT 25
`, period.Key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []WebhookEventSummary
	for rows.Next() {
		var event WebhookEventSummary
		if err := rows.Scan(
			&event.ID,
			&event.Workflow,
			&event.Status,
			&event.HTTPStatus,
			&event.Path,
			&event.CreatedAt,
			&event.UpdatedAt,
			&event.ErrorMessage,
		); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func dashboardPeriodSQL(column string) string {
	return `CASE $1
	WHEN 'today' THEN ` + column + ` >= date_trunc('day', now())
	WHEN '24h' THEN ` + column + ` >= now() - interval '24 hours'
	WHEN '7d' THEN ` + column + ` >= now() - interval '7 days'
	WHEN '30d' THEN ` + column + ` >= now() - interval '30 days'
	ELSE true
END`
}

func emptyStringToNil(value string) any {
	if value == "" {
		return nil
	}
	return value
}
