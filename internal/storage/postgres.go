package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type Store interface {
	RecordInbound(ctx context.Context, event InboundEvent) (int64, error)
	MarkFinished(ctx context.Context, id int64, result EventResult) error
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

func emptyStringToNil(value string) any {
	if value == "" {
		return nil
	}
	return value
}
