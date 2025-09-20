package main

import (
	"context"
	"finance_manager/internal/api"
	"finance_manager/internal/processor"
	"finance_manager/internal/repository/memory"
	"finance_manager/internal/service"
	"finance_manager/pkg/crypto"
	"finance_manager/pkg/metrics"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	appName = "finance_manager"
)

func main() {
	logger := setupLogger()
	logger.Info("Starting application",
		slog.String("name", appName))

	metricsCollector := metrics.NewMetricsCollector(logger)
	signer := crypto.NewSigner("your-secret-key-here", logger)
	txRepo := memory.NewTransactionRepository()
	accountRepo := memory.NewAccountRepository()
	ruleRepo := memory.NewRuleRepository()
	txProcessor := processor.NewTransactionProcessor(txRepo, accountRepo, ruleRepo, 10)
	notificationService := setupNotificationService(logger)
	apiHandler := api.NewAPIHandler(txProcessor, metricsCollector, signer, logger)
	metricsServer := metricsCollector.StartMetricsServer(":9090")
	httpServer := startHTTPServer(apiHandler, logger)
	waitForShutdown(logger, httpServer, metricsServer, notificationService)
	logger.Info("Application shutdown complete")
}

func setupLogger() *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}

	handler := slog.NewJSONHandler(os.Stdout, opts)
	return slog.New(handler)
}

func setupNotificationService(logger *slog.Logger) *service.NotificationService {
	emailService := &service.MockEmailService{}
	smsService := &service.MockSMSService{}

	return service.NewNotificationService(
		emailService,
		smsService,
		nil,
		nil,
		3,
		logger,
	)
}

func startHTTPServer(apiHandler *api.APIHandler, logger *slog.Logger) *http.Server {
	mux := http.NewServeMux()

	apiHandler.RegisterRoutes(mux)

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"name": "%s", "status": "ok"}`, appName)
	})

	server := &http.Server{
		Addr:         ":8080",
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("Starting HTTP server", slog.String("addr", server.Addr))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	return server
}

func waitForShutdown(
	logger *slog.Logger,
	httpServer *http.Server,
	metricsServer *http.Server,
	notificationService *service.NotificationService,
) {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	<-stop
	logger.Info("Shutdown signal received")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Error("HTTP server shutdown failed", slog.String("error", err.Error()))
	}

	if err := metricsServer.Shutdown(ctx); err != nil {
		logger.Error("Metrics server shutdown failed", slog.String("error", err.Error()))
	}

	if err := notificationService.Shutdown(ctx); err != nil {
		logger.Error("Notification service shutdown failed", slog.String("error", err.Error()))
	}
	if err := metrics.NewMetricsCollector(logger).Shutdown(ctx); err != nil {
		logger.Error("Metrics collector shutdown failed", slog.String("error", err.Error()))
	}
}
