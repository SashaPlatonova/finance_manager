package internal_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"finance_manager/internal/api"
	"finance_manager/internal/domain"
	"finance_manager/internal/processor"
	"finance_manager/internal/repository/memory"
	"finance_manager/pkg/crypto"
	"finance_manager/pkg/metrics"
)

type testEnv struct {
	txRepo   *memory.TransactionRepository
	accRepo  *memory.AccountRepository
	ruleRepo *memory.RuleRepository

	processor *processor.TransactionProcessor
	handler   *api.APIHandler
	logger    *slog.Logger
}

func setup(t *testing.T) *testEnv {
	t.Helper()
	txRepo := memory.NewTransactionRepository()
	accRepo := memory.NewAccountRepository()
	ruleRepo := memory.NewRuleRepository()

	proc := processor.NewTransactionProcessor(txRepo, accRepo, ruleRepo, 4)

	metricsCollector := metrics.NewMetricsCollector(nil)
	signer := crypto.NewSigner("test-secret", nil)
	logger := slog.Default()

	handler := api.NewAPIHandler(proc, metricsCollector, signer, logger)

	return &testEnv{
		txRepo:    txRepo,
		accRepo:   accRepo,
		ruleRepo:  ruleRepo,
		processor: proc,
		handler:   handler,
		logger:    logger,
	}
}

func mustCreateAccount(t *testing.T, env *testEnv, id, currency string, balance float64) {
	t.Helper()
	acc := &domain.Account{
		ID:        id,
		UserID:    "user-" + id,
		Balance:   balance,
		Currency:  currency,
		Status:    domain.AccountActive,
		CreatedAt: time.Now(),
	}
	if err := env.accRepo.Save(context.Background(), acc); err != nil {
		t.Fatalf("save account failed: %v", err)
	}
}

func callCreateTransaction(t *testing.T, env *testEnv, req api.CreateTransactionRequest) (*api.TransactionResponse, int) {
	t.Helper()
	b, _ := json.Marshal(req)
	r := httptest.NewRequest("POST", "/api/v1/transactions", bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	env.handler.CreateTransactionHandler(w, r)
	respCode := w.Result().StatusCode

	if respCode >= 200 && respCode < 300 {
		var tr api.TransactionResponse
		if err := json.NewDecoder(w.Body).Decode(&tr); err != nil {
			t.Fatalf("decode success response failed: %v", err)
		}
		return &tr, respCode
	}
	return nil, respCode
}

func TestIntegration_DepositSuccess(t *testing.T) {
	env := setup(t)

	mustCreateAccount(t, env, "A1", "USD", 0)

	req := api.CreateTransactionRequest{
		Type:        domain.TypeDeposit,
		Amount:      150.0,
		Currency:    "USD",
		ToAccountID: "A1",
		Description: "test deposit",
	}

	resp, code := callCreateTransaction(t, env, req)
	if code != 201 && code != 200 {
		t.Fatalf("expected 2xx, got %d", code)
	}
	if resp == nil {
		t.Fatalf("expected response body")
	}
	tx, err := env.txRepo.GetByID(context.Background(), resp.ID)
	if err != nil {
		t.Fatalf("tx not found: %v", err)
	}
	if tx.Status != domain.StatusCompleted {
		t.Fatalf("expected status completed, got %s", tx.Status)
	}
	acc, _ := env.accRepo.GetByID(context.Background(), "A1")
	if acc.Balance != 150.0 {
		t.Fatalf("expected balance 150, got %v", acc.Balance)
	}
}

func TestIntegration_WithdrawalInsufficientFunds(t *testing.T) {
	env := setup(t)

	mustCreateAccount(t, env, "A2", "USD", 10.0)

	req := api.CreateTransactionRequest{
		Type:          domain.TypeWithdrawal,
		Amount:        50.0,
		Currency:      "USD",
		FromAccountID: "A2",
		Description:   "attempt overdraw",
	}

	_, code := callCreateTransaction(t, env, req)
	if code < 400 || code >= 500 && code != 500 {
		t.Fatalf("expected client error (4xx) or server error for insufficient funds, got %d", code)
	}

	txs, _ := env.txRepo.GetByAccountID(context.Background(), "A2", 10, 0)
	if len(txs) != 0 {
		for _, tx := range txs {
			if tx.Status == domain.StatusCompleted {
				t.Fatalf("withdrawal should not complete when insufficient funds")
			}
		}
	}
}

func TestIntegration_TransferCurrencyMismatch(t *testing.T) {
	env := setup(t)

	mustCreateAccount(t, env, "A3", "USD", 1000)
	mustCreateAccount(t, env, "A4", "EUR", 1000)

	req := api.CreateTransactionRequest{
		Type:          domain.TypeTransfer,
		Amount:        100.0,
		Currency:      "USD",
		FromAccountID: "A3",
		ToAccountID:   "A4",
		Description:   "cross-currency transfer attempt",
	}

	_, code := callCreateTransaction(t, env, req)
	if code < 400 || code >= 500 && code != 500 {
		t.Fatalf("expected error due to currency mismatch, got %d", code)
	}
}

func TestIntegration_FraudHighAmount(t *testing.T) {
	env := setup(t)

	mustCreateAccount(t, env, "A5", "USD", 10_000_000)

	req := api.CreateTransactionRequest{
		Type:          domain.TypeWithdrawal,
		Amount:        9_000_000,
		Currency:      "USD",
		FromAccountID: "A5",
		Description:   "big suspicious withdrawal",
	}

	resp, code := callCreateTransaction(t, env, req)
	if code != 200 && code != 201 && code != 202 && code != 500 {
		t.Fatalf("unexpected response code %d", code)
	}

	if resp != nil {
		tx, _ := env.txRepo.GetByID(context.Background(), resp.ID)
		if tx == nil {
			t.Fatalf("expected tx persisted")
		}
		if !(tx.Status == domain.StatusSuspicious || tx.Status == domain.StatusPending || tx.Status == domain.StatusCompleted) {
			t.Fatalf("unexpected tx status for high-risk tx: %s", tx.Status)
		}
	}
}

func TestIntegration_RuleEngineBlocks(t *testing.T) {
	env := setup(t)

	cond := `{"field":"amount","operator":">","value":5000}`
	action := `{"type":"block","params":{},"message":"block large tx"}`
	rule := &domain.Rule{
		ID:        "r-block-large",
		Name:      "Block large tx",
		Type:      domain.RuleType("validation"),
		Condition: cond,
		Action:    action,
		IsActive:  true,
		Priority:  10,
	}
	if err := env.ruleRepo.Save(context.Background(), rule); err != nil {
		t.Fatalf("save rule failed: %v", err)
	}

	mustCreateAccount(t, env, "A6", "USD", 10_000)

	req := api.CreateTransactionRequest{
		Type:          domain.TypeWithdrawal,
		Amount:        6000,
		Currency:      "USD",
		FromAccountID: "A6",
		Description:   "should be blocked by rule",
	}

	_, code := callCreateTransaction(t, env, req)
	if code >= 200 && code < 300 {
		txs, _ := env.txRepo.GetByAccountID(context.Background(), "A6", 10, 0)
		found := false
		for _, tx := range txs {
			if tx.Amount == 6000 {
				found = true
				if tx.Status == domain.StatusCompleted {
					t.Fatalf("transaction should be blocked by rule but is completed")
				}
			}
		}
		if !found {
			t.Fatalf("expected a transaction record (blocked) for account A6")
		}
	}
}

func TestIntegration_GetTransactionByID(t *testing.T) {
	env := setup(t)
	mustCreateAccount(t, env, "A7", "USD", 500)
	tx := domain.NewTransaction(domain.TypeDeposit, 100.0, "USD").WithAccounts("", "A7")
	if err := env.processor.ProcessTransaction(context.Background(), tx); err != nil {
		t.Fatalf("process tx failed: %v", err)
	}

	r := httptest.NewRequest("GET", "/api/v1/transactions?id="+tx.ID, nil)
	w := httptest.NewRecorder()
	env.handler.GetTransactionHandler(w, r)
	if w.Result().StatusCode != 200 {
		t.Fatalf("expected 200, got %d", w.Result().StatusCode)
	}
	var got domain.Transaction
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if got.ID != tx.ID {
		t.Fatalf("expected id %s, got %s", tx.ID, got.ID)
	}
}

func TestIntegration_GetTransactionMissingID(t *testing.T) {
	env := setup(t)
	r := httptest.NewRequest("GET", "/api/v1/transactions", nil)
	w := httptest.NewRecorder()
	env.handler.GetTransactionHandler(w, r)
	if w.Result().StatusCode != 400 {
		t.Fatalf("expected 400 for missing id, got %d", w.Result().StatusCode)
	}
}

func TestIntegration_EventOnSuspiciousTransaction(t *testing.T) {
	env := setup(t)

	mustCreateAccount(t, env, "A8", "USD", 9_000_000)
	req := api.CreateTransactionRequest{
		Type:          domain.TypeWithdrawal,
		Amount:        8_000_000,
		Currency:      "USD",
		FromAccountID: "A8",
		Description:   "trigger daily limit block",
	}

	_, code := callCreateTransaction(t, env, req)

	if code != 500 {
		t.Fatalf("expected 500 due to daily withdrawal limit exceeded, got %d", code)
	}
}

func TestIntegration_ConcurrentTransfers(t *testing.T) {
	env := setup(t)
	mustCreateAccount(t, env, "A9", "USD", 1000.0)
	mustCreateAccount(t, env, "A10", "USD", 0)
	mustCreateAccount(t, env, "A11", "USD", 0)

	n := 10
	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			req := api.CreateTransactionRequest{
				Type:          domain.TypeTransfer,
				Amount:        10.0,
				Currency:      "USD",
				FromAccountID: "A9",
				ToAccountID:   "A10",
				Description:   fmt.Sprintf("concurrent transfer %d", i),
			}
			_, _ = callCreateTransaction(t, env, req)
		}(i)
	}
	wg.Wait()

	acc9, _ := env.accRepo.GetByID(context.Background(), "A9")
	acc10, _ := env.accRepo.GetByID(context.Background(), "A10")
	acc11, _ := env.accRepo.GetByID(context.Background(), "A11")

	total := acc9.Balance + acc10.Balance + acc11.Balance
	if int(total) != 1000 {
		t.Fatalf("expected total 1000 after concurrent transfers, got %v", total)
	}
}

func TestIntegration_InvalidRequestValidation(t *testing.T) {
	env := setup(t)

	raw := []byte(`{"type":"deposit","currency":"USD"}`)
	r := httptest.NewRequest("POST", "/api/v1/transactions", bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	env.handler.CreateTransactionHandler(w, r)
	if w.Result().StatusCode != 400 {
		t.Fatalf("expected 400 for invalid request, got %d", w.Result().StatusCode)
	}
}
