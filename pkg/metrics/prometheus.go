package metrics

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type MetricsCollector struct {
	registry              *prometheus.Registry
	transactionsProcessed prometheus.Counter
	transactionsFailed    prometheus.Counter
	transactionDuration   prometheus.Histogram
	riskScoreDistribution prometheus.Histogram
	accountBalance        *prometheus.GaugeVec
	mu                    sync.RWMutex
	logger                *slog.Logger
}

func NewMetricsCollector(logger *slog.Logger) *MetricsCollector {
	if logger == nil {
		logger = slog.Default()
	}

	registry := prometheus.NewRegistry()

	collector := &MetricsCollector{
		registry: registry,
		transactionsProcessed: promauto.With(registry).NewCounter(prometheus.CounterOpts{
			Name: "transactions_processed_total",
			Help: "Total number of processed transactions",
		}),
		transactionsFailed: promauto.With(registry).NewCounter(prometheus.CounterOpts{
			Name: "transactions_failed_total",
			Help: "Total number of failed transactions",
		}),
		transactionDuration: promauto.With(registry).NewHistogram(prometheus.HistogramOpts{
			Name:    "transaction_processing_duration_seconds",
			Help:    "Time taken to process a transaction",
			Buckets: prometheus.DefBuckets,
		}),
		riskScoreDistribution: promauto.With(registry).NewHistogram(prometheus.HistogramOpts{
			Name:    "transaction_risk_score_distribution",
			Help:    "Distribution of transaction risk scores",
			Buckets: []float64{0, 20, 40, 60, 80, 100},
		}),
		accountBalance: promauto.With(registry).NewGaugeVec(prometheus.GaugeOpts{
			Name: "account_balance",
			Help: "Current account balance",
		}, []string{"account_id", "currency"}),
		logger: logger,
	}

	return collector
}

func (m *MetricsCollector) RecordTransaction(duration time.Duration, riskScore int, success bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if success {
		m.transactionsProcessed.Inc()
	} else {
		m.transactionsFailed.Inc()
	}

	m.transactionDuration.Observe(duration.Seconds())
	m.riskScoreDistribution.Observe(float64(riskScore))
}

func (m *MetricsCollector) UpdateAccountBalance(accountID, currency string, balance float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.accountBalance.WithLabelValues(accountID, currency).Set(balance)
}

func (m *MetricsCollector) GetHandler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

func (m *MetricsCollector) StartMetricsServer(addr string) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", m.GetHandler())

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		m.logger.Info("Starting metrics server", slog.String("addr", addr))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			m.logger.Error("Metrics server failed", slog.String("error", err.Error()))
		}
	}()

	return server
}

func (m *MetricsCollector) Shutdown(ctx context.Context) error {
	m.logger.Info("Metrics collector shutdown complete")
	return nil
}
