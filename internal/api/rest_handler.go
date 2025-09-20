package api

import (
	"context"
	"encoding/json"
	"finance_manager/internal/domain"
	"finance_manager/internal/processor"
	"finance_manager/internal/repository"
	"finance_manager/pkg/crypto"
	"finance_manager/pkg/metrics"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

type APIHandler struct {
	processor      *processor.TransactionProcessor
	metrics        *metrics.MetricsCollector
	signer         *crypto.Signer
	logger         *slog.Logger
	requestTimeout time.Duration
}

func NewAPIHandler(
	processor *processor.TransactionProcessor,
	metrics *metrics.MetricsCollector,
	signer *crypto.Signer,
	logger *slog.Logger,
) *APIHandler {
	if logger == nil {
		logger = slog.Default()
	}

	return &APIHandler{
		processor:      processor,
		metrics:        metrics,
		signer:         signer,
		logger:         logger,
		requestTimeout: 30 * time.Second,
	}
}

type CreateTransactionRequest struct {
	Type          domain.TransactionType `json:"type"`
	Amount        float64                `json:"amount"`
	Currency      string                 `json:"currency"`
	FromAccountID string                 `json:"from_account_id,omitempty"`
	ToAccountID   string                 `json:"to_account_id,omitempty"`
	Description   string                 `json:"description,omitempty"`
	Metadata      map[string]string      `json:"metadata,omitempty"`
	Signature     string                 `json:"signature,omitempty"`
}

type TransactionResponse struct {
	ID         string                   `json:"id"`
	Status     domain.TransactionStatus `json:"status"`
	RiskScore  int                      `json:"risk_score"`
	FraudFlags []string                 `json:"fraud_flags,omitempty"`
	Message    string                   `json:"message,omitempty"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
	Details string `json:"details,omitempty"`
}

func (h *APIHandler) CreateTransactionHandler(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	ctx, cancel := context.WithTimeout(r.Context(), h.requestTimeout)
	defer cancel()

	var req CreateTransactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendError(w, "Invalid request body", http.StatusBadRequest, "INVALID_REQUEST")
		return
	}

	if err := h.validateTransactionRequest(req); err != nil {
		h.sendError(w, err.Error(), http.StatusBadRequest, "VALIDATION_ERROR")
		return
	}

	if req.Signature != "" {
		if valid, err := h.signer.VerifyTransaction(
			"",
			req.Amount,
			req.Currency,
			time.Now().Unix(),
			req.Signature,
		); !valid || err != nil {
			h.sendError(w, "Invalid signature", http.StatusUnauthorized, "INVALID_SIGNATURE")
			return
		}
	}

	tx := domain.NewTransaction(req.Type, req.Amount, req.Currency).
		WithDescription(req.Description).
		WithAccounts(req.FromAccountID, req.ToAccountID)

	for k, v := range req.Metadata {
		tx.AddMetadata(k, v)
	}

	err := h.processor.ProcessTransaction(ctx, tx)
	duration := time.Since(startTime)

	success := err == nil
	h.metrics.RecordTransaction(duration, tx.RiskScore, success)

	if err != nil {
		h.logger.Error("Transaction processing failed",
			slog.String("error", err.Error()),
			slog.String("transaction_id", tx.ID))
		h.sendError(w, fmt.Sprintf("Transaction failed: %v", err), http.StatusInternalServerError, "PROCESSING_ERROR")
		return
	}

	response := TransactionResponse{
		ID:         tx.ID,
		Status:     tx.Status,
		RiskScore:  tx.RiskScore,
		FraudFlags: tx.FraudFlags,
		Message:    "Transaction processed successfully",
	}

	h.sendJSON(w, response, http.StatusCreated)
	h.logger.Info("Transaction processed successfully",
		slog.String("transaction_id", tx.ID),
		slog.String("status", string(tx.Status)),
		slog.Int("risk_score", tx.RiskScore))
}

func (h *APIHandler) GetTransactionHandler(w http.ResponseWriter, r *http.Request) {
	transactionID := r.URL.Query().Get("id")
	if transactionID == "" {
		h.sendError(w, "Transaction ID is required", http.StatusBadRequest, "MISSING_ID")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.requestTimeout)
	defer cancel()

	tx, err := h.processor.GetTransaction(ctx, transactionID)
	if err != nil {
		if err == repository.ErrNotFound {
			h.sendError(w, "Transaction not found", http.StatusNotFound, "NOT_FOUND")
		} else {
			h.sendError(w, "Failed to get transaction", http.StatusInternalServerError, "SERVER_ERROR")
		}
		return
	}

	h.sendJSON(w, tx, http.StatusOK)
}

func (h *APIHandler) HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().UTC(),
		"version":   "1.0.0",
	}
	h.sendJSON(w, response, http.StatusOK)
}

func (h *APIHandler) validateTransactionRequest(req CreateTransactionRequest) error {
	if req.Amount <= 0 {
		return fmt.Errorf("amount must be positive")
	}
	if req.Currency == "" {
		return fmt.Errorf("currency is required")
	}
	if len(req.Currency) != 3 {
		return fmt.Errorf("currency must be 3 letters")
	}

	switch req.Type {
	case domain.TypeTransfer:
		if req.FromAccountID == "" || req.ToAccountID == "" {
			return fmt.Errorf("from_account_id and to_account_id are required for transfers")
		}
	case domain.TypeWithdrawal:
		if req.FromAccountID == "" {
			return fmt.Errorf("from_account_id is required for withdrawals")
		}
	case domain.TypeDeposit:
		if req.ToAccountID == "" {
			return fmt.Errorf("to_account_id is required for deposits")
		}
	default:
		return fmt.Errorf("unknown transaction type: %s", req.Type)
	}

	return nil
}

func (h *APIHandler) sendJSON(w http.ResponseWriter, data interface{}, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("Failed to encode JSON response", slog.String("error", err.Error()))
	}
}

func (h *APIHandler) sendError(w http.ResponseWriter, message string, statusCode int, code string) {
	errorResponse := ErrorResponse{
		Error: message,
		Code:  code,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(errorResponse)

	h.logger.Warn("API error response",
		slog.String("message", message),
		slog.String("code", code),
		slog.Int("status", statusCode))
}

func (h *APIHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/transactions", h.CreateTransactionHandler)
	mux.HandleFunc("GET /api/v1/transactions", h.GetTransactionHandler)
	mux.HandleFunc("GET /api/health", h.HealthCheckHandler)
}
