package config

import "os"

type Config struct {
	HTTPAddr             string
	ClickUpToken         string
	ClickUpListID        string
	MetaVerifyToken      string
	GoogleWebhookURL     string
	OnboardingListID     string
	OnboardingAssigneeID string
}

func Load() (Config, error) {
	httpAddr := os.Getenv("HTTP_ADDR")
	if httpAddr == "" {
		httpAddr = ":8080"
	}

	return Config{
		HTTPAddr:             httpAddr,
		ClickUpToken:         os.Getenv("CLICKUP_TOKEN"),
		ClickUpListID:        os.Getenv("CLICKUP_LIST_ID"),
		MetaVerifyToken:      os.Getenv("META_VERIFY_TOKEN"),
		GoogleWebhookURL:     os.Getenv("GOOGLE_WEBHOOK_URL"),
		OnboardingListID:     os.Getenv("ONBOARDING_LIST_ID"),
		OnboardingAssigneeID: os.Getenv("ONBOARDING_ASSIGNEE_ID"),
	}, nil
}
