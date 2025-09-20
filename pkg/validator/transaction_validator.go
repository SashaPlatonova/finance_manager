package validator

import (
	"errors"
	"finance_manager/internal/domain"
	"fmt"
	"regexp"
	"time"
)

var (
	ErrInvalidAmount        = errors.New("invalid transaction amount")
	ErrInvalidCurrency      = errors.New("invalid currency")
	ErrInvalidAccount       = errors.New("invalid account")
	ErrDuplicateTransaction = errors.New("duplicate transaction")
)

type TransactionValidator struct {
	currencyRegex *regexp.Regexp
	seen          map[string]struct{}
}

func NewTransactionValidator() *TransactionValidator {
	return &TransactionValidator{
		currencyRegex: regexp.MustCompile(`^[A-Z]{3}$`),
		seen:          make(map[string]struct{}),
	}
}

func (v *TransactionValidator) ValidateTransaction(tx *domain.Transaction) error {
	var errs []error

	if tx.Amount <= 0 {
		errs = append(errs, ErrInvalidAmount)
	}

	if !v.currencyRegex.MatchString(tx.Currency) {
		errs = append(errs, ErrInvalidCurrency)
	}

	if tx.Type == domain.TypeTransfer {
		if tx.FromAccountID == "" || tx.ToAccountID == "" {
			errs = append(errs, ErrInvalidAccount)
		}
		if tx.FromAccountID == tx.ToAccountID {
			errs = append(errs, errors.New("cannot transfer to same account"))
		}
	}

	if tx.CreatedAt.After(time.Now().Add(5 * time.Minute)) {
		errs = append(errs, errors.New("transaction date cannot be in the future"))
	}

	if _, ok := v.seen[tx.ID]; ok {
		return ErrDuplicateTransaction
	}
	v.seen[tx.ID] = struct{}{}

	if len(errs) > 0 {
		return fmt.Errorf("validation errors: %v", errs)
	}

	return nil
}

func (v *TransactionValidator) ValidateAmount(amount float64, currency string) error {
	if amount <= 0 {
		return ErrInvalidAmount
	}

	limits := map[string]float64{
		"USD": 1000000,
		"EUR": 900000,
		"GBP": 800000,
	}

	if max, exists := limits[currency]; exists && amount > max {
		return fmt.Errorf("amount exceeds maximum limit for %s: %f", currency, max)
	}

	return nil
}
