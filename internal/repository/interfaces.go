package repository

import (
	"context"
	"errors"
	"finance_manager/internal/domain"
	"time"
)

type TransactionRepository interface {
	Save(ctx context.Context, transaction *domain.Transaction) error
	GetByID(ctx context.Context, id string) (*domain.Transaction, error)
	GetByAccountID(ctx context.Context, accountID string, limit, offset int) ([]*domain.Transaction, error)
	GetByStatus(ctx context.Context, status domain.TransactionStatus) ([]*domain.Transaction, error)
	GetByPeriod(ctx context.Context, from, to time.Time) ([]*domain.Transaction, error)
	UpdateStatus(ctx context.Context, id string, status domain.TransactionStatus) error
	GetDailyVolume(ctx context.Context, accountID string, date time.Time) (float64, error)
	GetMonthlyVolume(ctx context.Context, accountID string, year int, month time.Month) (float64, error)
}

type AccountRepository interface {
	Save(ctx context.Context, account *domain.Account) error
	GetByID(ctx context.Context, id string) (*domain.Account, error)
	GetByUserID(ctx context.Context, userID string) ([]*domain.Account, error)
	Update(ctx context.Context, account *domain.Account) error
	UpdateBalance(ctx context.Context, id string, amount float64) error
	UpdateStatus(ctx context.Context, id string, status domain.AccountStatus) error
	GetAllActive(ctx context.Context) ([]*domain.Account, error)
	GetByRiskCategory(ctx context.Context, category string) ([]*domain.Account, error)
}

type RuleRepository interface {
	Save(ctx context.Context, rule *domain.Rule) error
	GetByID(ctx context.Context, id string) (*domain.Rule, error)
	GetAll(ctx context.Context) ([]*domain.Rule, error)
	GetByType(ctx context.Context, ruleType domain.RuleType) ([]*domain.Rule, error)
	GetActiveRules(ctx context.Context) ([]*domain.Rule, error)
	Update(ctx context.Context, rule *domain.Rule) error
	Deactivate(ctx context.Context, id string) error
	GetByPriority(ctx context.Context, minPriority, maxPriority int) ([]*domain.Rule, error)
}

var (
	ErrNotFound            = errors.New("not found")
	ErrDuplicate           = errors.New("duplicate entry")
	ErrInsufficientFunds   = errors.New("insufficient funds")
	ErrAccountSuspended    = errors.New("account suspended")
	ErrTransactionConflict = errors.New("transaction conflict")
)
