package memory

import (
	"context"
	"finance_manager/internal/domain"
	"finance_manager/internal/repository"
	"fmt"
	"sync"
	"time"
)

type AccountRepository struct {
	mu        sync.RWMutex
	accounts  map[string]*domain.Account
	userIndex map[string][]string
}

func NewAccountRepository() *AccountRepository {
	return &AccountRepository{
		accounts:  make(map[string]*domain.Account),
		userIndex: make(map[string][]string),
	}
}

func (r *AccountRepository) Save(ctx context.Context, account *domain.Account) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.accounts[account.ID]; exists {
		return fmt.Errorf("%w: account %s", repository.ErrDuplicate, account.ID)
	}

	account.CreatedAt = time.Now()
	account.LastActivityAt = time.Now()
	r.accounts[account.ID] = account

	r.userIndex[account.UserID] = append(r.userIndex[account.UserID], account.ID)

	return nil
}

func (r *AccountRepository) GetByID(ctx context.Context, id string) (*domain.Account, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	account, exists := r.accounts[id]
	if !exists {
		return nil, fmt.Errorf("%w: account %s", repository.ErrNotFound, id)
	}
	return account, nil
}

func (r *AccountRepository) GetByUserID(ctx context.Context, userID string) ([]*domain.Account, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	accountIDs, exists := r.userIndex[userID]
	if !exists {
		return nil, fmt.Errorf("%w: user %s", repository.ErrNotFound, userID)
	}

	var result []*domain.Account
	for _, id := range accountIDs {
		if account, exists := r.accounts[id]; exists {
			result = append(result, account)
		}
	}

	return result, nil
}

func (r *AccountRepository) Update(ctx context.Context, account *domain.Account) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.accounts[account.ID]; !exists {
		return fmt.Errorf("%w: account %s", repository.ErrNotFound, account.ID)
	}

	account.LastActivityAt = time.Now()
	r.accounts[account.ID] = account

	return nil
}

func (r *AccountRepository) UpdateBalance(ctx context.Context, id string, amount float64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	account, exists := r.accounts[id]
	if !exists {
		return fmt.Errorf("%w: account %s", repository.ErrNotFound, id)
	}

	account.Balance += amount
	account.LastActivityAt = time.Now()
	r.accounts[id] = account

	return nil
}

func (r *AccountRepository) UpdateStatus(ctx context.Context, id string, status domain.AccountStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	account, exists := r.accounts[id]
	if !exists {
		return fmt.Errorf("%w: account %s", repository.ErrNotFound, id)
	}

	account.Status = status
	account.LastActivityAt = time.Now()
	r.accounts[id] = account

	return nil
}

func (r *AccountRepository) GetAllActive(ctx context.Context) ([]*domain.Account, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*domain.Account
	for _, account := range r.accounts {
		if account.Status == domain.AccountActive {
			result = append(result, account)
		}
	}

	return result, nil
}

func (r *AccountRepository) GetByRiskCategory(ctx context.Context, category string) ([]*domain.Account, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*domain.Account
	for _, account := range r.accounts {
		if account.RiskCategory == category {
			result = append(result, account)
		}
	}

	return result, nil
}
