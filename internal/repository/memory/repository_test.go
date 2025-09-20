package memory

import (
	"context"
	"finance_manager/internal/domain"
	"testing"
	"time"
)

func TestAccountRepository_SaveAndGetByID(t *testing.T) {
	repo := NewAccountRepository()
	account := &domain.Account{
		ID:      "acc1",
		UserID:  "user1",
		Status:  domain.AccountActive,
		Balance: 100,
	}

	err := repo.Save(context.Background(), account)
	if err != nil {
		t.Fatalf("unexpected error on Save: %v", err)
	}
	got, err := repo.GetByID(context.Background(), "acc1")

	if err != nil {
		t.Fatalf("unexpected error on GetByID: %v", err)
	}
	if got.ID != account.ID || got.UserID != account.UserID || got.Balance != account.Balance {
		t.Errorf("expected account %+v, got %+v", account, got)
	}
}

func TestAccountRepository_UpdateBalance(t *testing.T) {
	repo := NewAccountRepository()
	account := &domain.Account{ID: "acc2", UserID: "user2", Balance: 50}
	_ = repo.Save(context.Background(), account)

	err := repo.UpdateBalance(context.Background(), "acc2", 25)
	got, _ := repo.GetByID(context.Background(), "acc2")

	if err != nil {
		t.Fatalf("unexpected error on UpdateBalance: %v", err)
	}
	if got.Balance != 75 {
		t.Errorf("expected balance 75, got %f", got.Balance)
	}
}

func TestTransactionRepository_SaveAndGetByID(t *testing.T) {
	repo := NewTransactionRepository()
	tx := &domain.Transaction{
		ID:            "tx1",
		FromAccountID: "acc1",
		ToAccountID:   "acc2",
		Amount:        100,
		Status:        domain.StatusCompleted,
		CreatedAt:     time.Now(),
	}

	err := repo.Save(context.Background(), tx)
	got, err := repo.GetByID(context.Background(), "tx1")

	if err != nil {
		t.Fatalf("unexpected error on GetByID: %v", err)
	}
	if got.Amount != 100 || got.Status != domain.StatusCompleted {
		t.Errorf("expected transaction %+v, got %+v", tx, got)
	}
}

func TestAccountRepository_GetAllActive(t *testing.T) {
	repo := NewAccountRepository()
	_ = repo.Save(context.Background(), &domain.Account{ID: "a1", Status: domain.AccountActive})
	_ = repo.Save(context.Background(), &domain.Account{ID: "a2", Status: domain.AccountSuspended})

	activeAccounts, err := repo.GetAllActive(context.Background())

	if err != nil {
		t.Fatalf("unexpected error on GetAllActive: %v", err)
	}
	if len(activeAccounts) != 1 || activeAccounts[0].ID != "a1" {
		t.Errorf("expected 1 active account with ID 'a1', got %+v", activeAccounts)
	}
}

func TestTransactionRepository_GetDailyVolume(t *testing.T) {
	repo := NewTransactionRepository()
	now := time.Now()
	tx1 := &domain.Transaction{ID: "tx1", FromAccountID: "acc1", Amount: 50, Status: domain.StatusCompleted, CreatedAt: now}
	tx2 := &domain.Transaction{ID: "tx2", FromAccountID: "acc1", Amount: 30, Status: domain.StatusCompleted, CreatedAt: now}
	_ = repo.Save(context.Background(), tx1)
	_ = repo.Save(context.Background(), tx2)

	total, err := repo.GetDailyVolume(context.Background(), "acc1", now)

	if err != nil {
		t.Fatalf("unexpected error on GetDailyVolume: %v", err)
	}
	if total != 80 {
		t.Errorf("expected total 80, got %f", total)
	}
}
