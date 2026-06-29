package workflows

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClosedDealInputTaskID(t *testing.T) {
	tests := []struct {
		name     string
		input    ClosedDealInput
		expected string
	}{
		{
			name:     "top level payload",
			input:    ClosedDealInput{Payload: closedDealPayload{ID: "86afkfpwd"}},
			expected: "86afkfpwd",
		},
		{
			name:     "body payload",
			input:    ClosedDealInput{Body: closedDealBody{Payload: closedDealPayload{ID: "86ag5jzhg"}}},
			expected: "86ag5jzhg",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.input.taskID(); got != test.expected {
				t.Fatalf("taskID() = %q, want %q", got, test.expected)
			}
		})
	}
}

func TestCustomFieldValue(t *testing.T) {
	fields := []clickUpCustomField{
		{ID: "other", Value: "ignored"},
		{ID: emailCustomFieldID, Value: "lead@example.com"},
	}

	if got := customFieldValue(fields, emailCustomFieldID); got != "lead@example.com" {
		t.Fatalf("customFieldValue() = %#v", got)
	}
}

func TestClickUpRequestReadsLargeResponses(t *testing.T) {
	largeName := strings.Repeat("a", 9000)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"task_123","name":"` + largeName + `","custom_fields":[{"id":"field","name":"Field","value":null}]}`))
	}))
	defer server.Close()

	runner := NewRunner(slog.Default(), Config{ClickUpToken: "token"})

	var task clickUpTaskDetails
	if err := runner.clickUpRequest(context.Background(), http.MethodGet, server.URL, nil, &task); err != nil {
		t.Fatalf("clickUpRequest returned error: %v", err)
	}
	if task.ID != "task_123" {
		t.Fatalf("unexpected task id: %q", task.ID)
	}
	if task.Name != largeName {
		t.Fatal("large response body was not decoded completely")
	}
}
