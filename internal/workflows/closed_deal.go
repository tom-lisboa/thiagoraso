package workflows

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const emailCustomFieldID = "0d4f8ffe-adeb-404d-a47d-8e0c8f9bdc6a"

type ClosedDealInput struct {
	Body    closedDealBody    `json:"body"`
	Payload closedDealPayload `json:"payload"`
}

type closedDealBody struct {
	Payload closedDealPayload `json:"payload"`
}

type closedDealPayload struct {
	ID string `json:"id"`
}

type ClosedDealOutput struct {
	Workflow          string    `json:"workflow"`
	Status            string    `json:"status"`
	SourceTaskID      string    `json:"source_task_id"`
	SourceTaskURL     string    `json:"source_task_url,omitempty"`
	OnboardingTaskID  string    `json:"onboarding_task_id"`
	OnboardingTaskURL string    `json:"onboarding_task_url,omitempty"`
	EmailCopied       bool      `json:"email_copied"`
	Handled           time.Time `json:"handled_at"`
}

type clickUpTaskDetails struct {
	ID           string               `json:"id"`
	Name         string               `json:"name"`
	URL          string               `json:"url"`
	CustomFields []clickUpCustomField `json:"custom_fields"`
}

type clickUpCustomField struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Value any    `json:"value"`
}

func (input ClosedDealInput) taskID() string {
	if input.Body.Payload.ID != "" {
		return input.Body.Payload.ID
	}
	return input.Payload.ID
}

func (r *Runner) RunClosedDeal(ctx context.Context, input ClosedDealInput) (ClosedDealOutput, error) {
	if r.config.ClickUpToken == "" {
		return ClosedDealOutput{}, errors.New("CLICKUP_TOKEN is required")
	}
	if r.config.OnboardingListID == "" {
		return ClosedDealOutput{}, errors.New("ONBOARDING_LIST_ID is required")
	}

	sourceTaskID := strings.TrimSpace(input.taskID())
	if sourceTaskID == "" {
		return ClosedDealOutput{}, errors.New("payload.id is required")
	}

	sourceTask, err := r.getClickUpTask(ctx, sourceTaskID)
	if err != nil {
		return ClosedDealOutput{}, err
	}

	onboardingTask, err := r.createOnboardingTask(ctx, sourceTask)
	if err != nil {
		return ClosedDealOutput{}, err
	}

	email := customFieldValue(sourceTask.CustomFields, emailCustomFieldID)
	emailCopied := false
	if email != nil {
		if err := r.setClickUpCustomField(ctx, onboardingTask.ID, emailCustomFieldID, email); err != nil {
			return ClosedDealOutput{}, err
		}
		emailCopied = true
	}

	return ClosedDealOutput{
		Workflow:          "closed-deal",
		Status:            "processed",
		SourceTaskID:      sourceTask.ID,
		SourceTaskURL:     sourceTask.URL,
		OnboardingTaskID:  onboardingTask.ID,
		OnboardingTaskURL: onboardingTask.URL,
		EmailCopied:       emailCopied,
		Handled:           time.Now().UTC(),
	}, nil
}

func (r *Runner) getClickUpTask(ctx context.Context, taskID string) (clickUpTaskDetails, error) {
	var task clickUpTaskDetails
	if err := r.clickUpRequest(ctx, http.MethodGet, "https://api.clickup.com/api/v2/task/"+taskID, nil, &task); err != nil {
		return clickUpTaskDetails{}, err
	}
	return task, nil
}

func (r *Runner) createOnboardingTask(ctx context.Context, source clickUpTaskDetails) (clickUpTask, error) {
	dueDate := time.Now().AddDate(0, 0, 4).UnixMilli()
	startDate := time.Now().UnixMilli()

	payload := map[string]any{
		"name":            source.Name + " - Onboarding (Checklist e elaboração do plano)",
		"description":     "Plano:\nMetodologia:\nInício:",
		"due_date":        dueDate,
		"due_date_time":   true,
		"start_date":      startDate,
		"start_date_time": true,
	}

	if r.config.OnboardingAssigneeID != "" {
		payload["assignees"] = []string{r.config.OnboardingAssigneeID}
	}

	var task clickUpTask
	if err := r.clickUpRequest(ctx, http.MethodPost, "https://api.clickup.com/api/v2/list/"+r.config.OnboardingListID+"/task", payload, &task); err != nil {
		return clickUpTask{}, err
	}
	return task, nil
}

func (r *Runner) setClickUpCustomField(ctx context.Context, taskID string, fieldID string, value any) error {
	payload := map[string]any{"value": value}
	return r.clickUpRequest(ctx, http.MethodPost, "https://api.clickup.com/api/v2/task/"+taskID+"/field/"+fieldID, payload, nil)
}

func customFieldValue(fields []clickUpCustomField, fieldID string) any {
	for _, field := range fields {
		if field.ID == fieldID && field.Value != nil {
			return field.Value
		}
	}
	return nil
}

func (r *Runner) clickUpRequest(ctx context.Context, method string, url string, payload any, output any) error {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", r.config.ClickUpToken)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	responseBody, _ := io.ReadAll(io.LimitReader(res.Body, 8192))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("clickup returned %s: %s", res.Status, string(responseBody))
	}
	if output == nil || len(bytes.TrimSpace(responseBody)) == 0 {
		return nil
	}
	if err := json.Unmarshal(responseBody, output); err != nil {
		if errors.Is(err, io.EOF) || strings.Contains(err.Error(), "unexpected end of JSON input") {
			return nil
		}
		return err
	}
	return nil
}
