package processor

import (
	"context"
	"errors"
	"finance_manager/internal/domain"
	"finance_manager/internal/repository"
	"finance_manager/internal/repository/memory"
	"testing"
)

func TestTransactionProcessor_ProcessTransaction_TransferSuccess(t *testing.T) {
	ctx := context.Background()
	accRepo := memory.NewAccountRepository()
	txRepo := memory.NewTransactionRepository()
	ruleRepo := memory.NewRuleRepository()

	fromAcc := &domain.Account{ID: "a1", UserID: "u1", Balance: 1000, Status: domain.AccountActive, Currency: "USD"}
	toAcc := &domain.Account{ID: "a2", UserID: "u2", Balance: 500, Status: domain.AccountActive, Currency: "USD"}
	_ = accRepo.Save(ctx, fromAcc)
	_ = accRepo.Save(ctx, toAcc)

	proc := NewTransactionProcessor(txRepo, accRepo, ruleRepo, 1)
	tx := &domain.Transaction{ID: "tx1", Type: domain.TypeTransfer, FromAccountID: "a1", ToAccountID: "a2", Amount: 200, Currency: "USD"}

	err := proc.ProcessTransaction(ctx, tx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fromAccUpdated, _ := accRepo.GetByID(ctx, "a1"); fromAccUpdated.Balance != 800 {
		t.Errorf("expected 800, got %f", fromAccUpdated.Balance)
	}
	if toAccUpdated, _ := accRepo.GetByID(ctx, "a2"); toAccUpdated.Balance != 700 {
		t.Errorf("expected 700, got %f", toAccUpdated.Balance)
	}
	if tx.Status != domain.StatusCompleted {
		t.Errorf("expected transaction status completed, got %s", tx.Status)
	}
}

func TestFraudDetector_AnalyzeTransaction_LargeAmount(t *testing.T) {
	tx := &domain.Transaction{Amount: 20000}
	fd := NewFraudDetector()

	score, flags := fd.AnalyzeTransaction(tx)

	if score == 0 || len(flags) == 0 {
		t.Errorf("expected fraud score > 0 and flags not empty, got score=%d flags=%v", score, flags)
	}
	found := false
	for _, f := range flags {
		if f == "large_amount" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'large_amount' flag, got %v", flags)
	}
}

func TestTransactionProcessor_ProcessTransaction_Deposit(t *testing.T) {
	ctx := context.Background()
	accRepo := memory.NewAccountRepository()
	txRepo := memory.NewTransactionRepository()
	ruleRepo := memory.NewRuleRepository()

	account := &domain.Account{ID: "a1", UserID: "u1", Balance: 100, Status: domain.AccountActive, Currency: "USD"}
	_ = accRepo.Save(ctx, account)

	processor := NewTransactionProcessor(txRepo, accRepo, ruleRepo, 1)
	tx := &domain.Transaction{ID: "tx1", Type: domain.TypeDeposit, ToAccountID: "a1", Amount: 150, Currency: "USD"}

	err := processor.ProcessTransaction(ctx, tx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	accUpdated, _ := accRepo.GetByID(ctx, "a1")
	if accUpdated.Balance != 250 {
		t.Errorf("expected 250, got %f", accUpdated.Balance)
	}
	if tx.Status != domain.StatusCompleted {
		t.Errorf("expected transaction status completed, got %s", tx.Status)
	}
}

func TestRuleEngine_EvaluateRules_AmountCondition(t *testing.T) {
	ctx := context.Background()
	ruleRepo := memory.NewRuleRepository()
	engine := NewRuleEngine(ruleRepo, nil)

	rule := &domain.Rule{
		ID:        "r1",
		Name:      "high_amount",
		IsActive:  true,
		Priority:  10,
		Condition: `{"field":"amount","operator":">","value":1000}`,
		Action:    `{"type":"flag_transaction","params":{"reason":"high_amount"},"message":"High transaction amount"}`,
	}
	_ = ruleRepo.Save(ctx, rule)

	tx := &domain.Transaction{ID: "tx1", Amount: 1500}

	results, err := engine.EvaluateRules(ctx, tx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || !results[0].Triggered {
		t.Errorf("expected rule to be triggered, got results: %+v", results)
	}
	if results[0].Action.Type != "flag_transaction" {
		t.Errorf("expected action type 'flag_transaction', got %s", results[0].Action.Type)
	}
}

func TestTransactionProcessor_ProcessTransaction_WithdrawalInsufficientFunds(t *testing.T) {
	ctx := context.Background()
	accRepo := memory.NewAccountRepository()
	txRepo := memory.NewTransactionRepository()
	ruleRepo := memory.NewRuleRepository()

	account := &domain.Account{ID: "a1", UserID: "u1", Balance: 100, Status: domain.AccountActive, Currency: "USD"}
	_ = accRepo.Save(ctx, account)

	processor := NewTransactionProcessor(txRepo, accRepo, ruleRepo, 1)
	tx := &domain.Transaction{ID: "tx1", Type: domain.TypeWithdrawal, FromAccountID: "a1", Amount: 200, Currency: "USD"}

	err := processor.ProcessTransaction(ctx, tx)

	if !errors.Is(err, repository.ErrInsufficientFunds) {
		t.Errorf("expected ErrInsufficientFunds, got %v", err)
	}
	accUpdated, _ := accRepo.GetByID(ctx, "a1")
	if accUpdated.Balance != 100 {
		t.Errorf("expected 100, got %f", accUpdated.Balance)
	}
}
