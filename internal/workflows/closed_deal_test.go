package workflows

import "testing"

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
