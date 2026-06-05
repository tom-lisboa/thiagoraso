package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mentoria-automation-server/internal/config"
	"mentoria-automation-server/internal/httpapi"
	"mentoria-automation-server/internal/workflows"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	workflowRunner := workflows.NewRunner(logger, workflows.Config{
		ClickUpToken:         cfg.ClickUpToken,
		ClickUpListID:        cfg.ClickUpListID,
		MetaVerifyToken:      cfg.MetaVerifyToken,
		GoogleWebhookURL:     cfg.GoogleWebhookURL,
		OnboardingListID:     cfg.OnboardingListID,
		OnboardingAssigneeID: cfg.OnboardingAssigneeID,
	})
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpapi.NewRouter(logger, workflowRunner),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("server listening", "addr", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}

	logger.Info("server stopped")
}
