package memory

import (
	"context"
	_ "errors"
	"finance_manager/internal/domain"
	"finance_manager/internal/repository"
	"fmt"
	"sort"
	"sync"
	"time"
)

type TransactionRepository struct {
	mu           sync.RWMutex
	transactions map[string]*domain.Transaction
	index        map[string][]string
}

func NewTransactionRepository() *TransactionRepository {
	return &TransactionRepository{
		transactions: make(map[string]*domain.Transaction),
		index:        make(map[string][]string),
	}
}

func (r *TransactionRepository) Save(ctx context.Context, tx *domain.Transaction) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.transactions[tx.ID]; exists {
		return fmt.Errorf("%w: transaction %s", repository.ErrDuplicate, tx.ID)
	}

	tx.UpdatedAt = time.Now()
	r.transactions[tx.ID] = tx

	if tx.FromAccountID != "" {
		r.index[tx.FromAccountID] = append(r.index[tx.FromAccountID], tx.ID)
	}
	if tx.ToAccountID != "" {
		r.index[tx.ToAccountID] = append(r.index[tx.ToAccountID], tx.ID)
	}

	return nil
}

func (r *TransactionRepository) GetByID(ctx context.Context, id string) (*domain.Transaction, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tx, exists := r.transactions[id]
	if !exists {
		return nil, fmt.Errorf("%w: transaction %s", repository.ErrNotFound, id)
	}
	return tx, nil
}

func (r *TransactionRepository) GetByAccountID(ctx context.Context, accountID string, limit, offset int) ([]*domain.Transaction, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	transactionIDs, exists := r.index[accountID]
	if !exists {
		return nil, fmt.Errorf("%w: account %s", repository.ErrNotFound, accountID)
	}

	sort.Slice(transactionIDs, func(i, j int) bool {
		return r.transactions[transactionIDs[i]].CreatedAt.After(r.transactions[transactionIDs[j]].CreatedAt)
	})

	start := offset
	end := offset + limit
	if end > len(transactionIDs) {
		end = len(transactionIDs)
	}
	if start >= len(transactionIDs) {
		return []*domain.Transaction{}, nil
	}

	var result []*domain.Transaction
	for _, id := range transactionIDs[start:end] {
		result = append(result, r.transactions[id])
	}

	return result, nil
}

func (r *TransactionRepository) GetByStatus(ctx context.Context, status domain.TransactionStatus) ([]*domain.Transaction, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*domain.Transaction
	for _, tx := range r.transactions {
		if tx.Status == status {
			result = append(result, tx)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})

	return result, nil
}

func (r *TransactionRepository) GetByPeriod(ctx context.Context, from, to time.Time) ([]*domain.Transaction, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*domain.Transaction
	for _, tx := range r.transactions {
		if !tx.CreatedAt.Before(from) && !tx.CreatedAt.After(to) {
			result = append(result, tx)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})

	return result, nil
}

func (r *TransactionRepository) UpdateStatus(ctx context.Context, id string, status domain.TransactionStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	tx, exists := r.transactions[id]
	if !exists {
		return fmt.Errorf("%w: transaction %s", repository.ErrNotFound, id)
	}

	tx.Status = status
	tx.UpdatedAt = time.Now()
	r.transactions[id] = tx

	return nil
}

func (r *TransactionRepository) GetDailyVolume(ctx context.Context, accountID string, date time.Time) (float64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	var total float64
	for _, tx := range r.transactions {
		if (tx.FromAccountID == accountID || tx.ToAccountID == accountID) &&
			!tx.CreatedAt.Before(startOfDay) && tx.CreatedAt.Before(endOfDay) &&
			tx.Status == domain.StatusCompleted {
			total += tx.Amount
		}
	}

	return total, nil
}

func (r *TransactionRepository) GetMonthlyVolume(ctx context.Context, accountID string, year int, month time.Month) (float64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	startOfMonth := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	endOfMonth := startOfMonth.AddDate(0, 1, 0)

	var total float64
	for _, tx := range r.transactions {
		if (tx.FromAccountID == accountID || tx.ToAccountID == accountID) &&
			!tx.CreatedAt.Before(startOfMonth) && tx.CreatedAt.Before(endOfMonth) &&
			tx.Status == domain.StatusCompleted {
			total += tx.Amount
		}
	}

	return total, nil
}
