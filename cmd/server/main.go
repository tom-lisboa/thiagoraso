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
	"mentoria-automation-server/internal/storage"
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

	var eventStore storage.Store
	if cfg.DatabaseURL != "" {
		storeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		store, err := storage.OpenPostgres(storeCtx, cfg.DatabaseURL)
		cancel()
		if err != nil {
			logger.Error("failed to connect postgres", "error", err)
			os.Exit(1)
		}
		defer func() {
			if err := store.Close(); err != nil {
				logger.Error("failed to close postgres", "error", err)
			}
		}()
		eventStore = store
		logger.Info("postgres event store enabled")
	}

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpapi.NewRouter(logger, workflowRunner, eventStore),
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
