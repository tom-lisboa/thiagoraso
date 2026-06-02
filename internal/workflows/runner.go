package workflows

import "log/slog"

type Config struct {
	ClickUpToken     string
	ClickUpListID    string
	MetaVerifyToken  string
	GoogleWebhookURL string
}

type Runner struct {
	logger *slog.Logger
	config Config
}

func NewRunner(logger *slog.Logger, config Config) *Runner {
	return &Runner{
		logger: logger,
		config: config,
	}
}
