package validator

import (
	"testing"
	"time"

	"finance_manager/internal/domain"
)

func TestTransactionValidator_ValidTransaction(t *testing.T) {
	v := NewTransactionValidator()
	tx := &domain.Transaction{
		ID:          "tx1",
		Amount:      100,
		Currency:    "USD",
		Type:        domain.TypeDeposit,
		ToAccountID: "A2",
		CreatedAt:   time.Now(),
	}

	err := v.ValidateTransaction(tx)

	if err != nil {
		t.Fatalf("expected valid transaction, got err=%v", err)
	}
}

func TestTransactionValidator_InvalidAmount(t *testing.T) {
	v := NewTransactionValidator()
	tx := &domain.Transaction{
		ID:          "tx2",
		Amount:      0,
		Currency:    "USD",
		Type:        domain.TypeDeposit,
		ToAccountID: "A2",
		CreatedAt:   time.Now(),
	}
	err := v.ValidateTransaction(tx)

	if err == nil {
		t.Fatal("expected error for invalid amount, got nil")
	}
}

func TestTransactionValidator_InvalidCurrencyFormat(t *testing.T) {
	v := NewTransactionValidator()
	tx := &domain.Transaction{
		ID:          "tx4",
		Amount:      50,
		Currency:    "US", // две буквы вместо трёх
		Type:        domain.TypeDeposit,
		ToAccountID: "A2",
		CreatedAt:   time.Now(),
	}
	err := v.ValidateTransaction(tx)
	if err == nil {
		t.Fatal("expected error for invalid currency format, got nil")
	}
}

func TestTransactionValidator_ExceedsLimit(t *testing.T) {
	v := NewTransactionValidator()
	tx := &domain.Transaction{
		ID:          "tx5",
		Amount:      2000000,
		Currency:    "USD",
		Type:        domain.TypeDeposit,
		ToAccountID: "A2",
		CreatedAt:   time.Now(),
	}
	err := v.ValidateAmount(tx.Amount, tx.Currency)
	if err == nil {
		t.Fatal("expected error for exceeding limit, got nil")
	}
}

func TestTransactionValidator_FutureTimestamp(t *testing.T) {
	v := NewTransactionValidator()
	tx := &domain.Transaction{
		ID:          "tx6",
		Amount:      10,
		Currency:    "USD",
		Type:        domain.TypeDeposit,
		ToAccountID: "A2",
		CreatedAt:   time.Now().Add(48 * time.Hour),
	}
	err := v.ValidateTransaction(tx)
	if err == nil {
		t.Fatal("expected error for future timestamp, got nil")
	}
}

func TestTransactionValidator_DuplicateTransaction(t *testing.T) {
	v := NewTransactionValidator()
	tx := &domain.Transaction{
		ID:          "dup1",
		Amount:      10,
		Currency:    "USD",
		Type:        domain.TypeDeposit,
		ToAccountID: "A2",
		CreatedAt:   time.Now(),
	}
	err := v.ValidateTransaction(tx)
	if err != nil {
		t.Fatalf("first validation should succeed, got %v", err)
	}
	err = v.ValidateTransaction(tx)
	if err == nil {
		t.Fatal("expected error for duplicate transaction, got nil")
	}
}
