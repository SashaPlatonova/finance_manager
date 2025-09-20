package memory

import (
	"context"
	"finance_manager/internal/domain"
	"finance_manager/internal/repository"
	"fmt"
	"sort"
	"sync"
)

type RuleRepository struct {
	mu    sync.RWMutex
	rules map[string]*domain.Rule
}

func NewRuleRepository() *RuleRepository {
	return &RuleRepository{
		rules: make(map[string]*domain.Rule),
	}
}

func (r *RuleRepository) Save(ctx context.Context, rule *domain.Rule) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.rules[rule.ID]; exists {
		return fmt.Errorf("%w: rule %s", repository.ErrDuplicate, rule.ID)
	}

	rule.Version = 1
	r.rules[rule.ID] = rule

	return nil
}

func (r *RuleRepository) GetByID(ctx context.Context, id string) (*domain.Rule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rule, exists := r.rules[id]
	if !exists {
		return nil, fmt.Errorf("%w: rule %s", repository.ErrNotFound, id)
	}
	return rule, nil
}

func (r *RuleRepository) GetAll(ctx context.Context) ([]*domain.Rule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*domain.Rule
	for _, rule := range r.rules {
		result = append(result, rule)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Priority > result[j].Priority
	})

	return result, nil
}

func (r *RuleRepository) GetByType(ctx context.Context, ruleType domain.RuleType) ([]*domain.Rule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*domain.Rule
	for _, rule := range r.rules {
		if rule.Type == ruleType {
			result = append(result, rule)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Priority > result[j].Priority
	})

	return result, nil
}

func (r *RuleRepository) GetActiveRules(ctx context.Context) ([]*domain.Rule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*domain.Rule
	for _, rule := range r.rules {
		if rule.IsActive {
			result = append(result, rule)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Priority > result[j].Priority
	})

	return result, nil
}

func (r *RuleRepository) Update(ctx context.Context, rule *domain.Rule) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, exists := r.rules[rule.ID]
	if !exists {
		return fmt.Errorf("%w: rule %s", repository.ErrNotFound, rule.ID)
	}

	rule.Version = existing.Version + 1
	r.rules[rule.ID] = rule

	return nil
}

func (r *RuleRepository) Deactivate(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rule, exists := r.rules[id]
	if !exists {
		return fmt.Errorf("%w: rule %s", repository.ErrNotFound, id)
	}

	rule.IsActive = false
	r.rules[id] = rule

	return nil
}

func (r *RuleRepository) GetByPriority(ctx context.Context, minPriority, maxPriority int) ([]*domain.Rule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*domain.Rule
	for _, rule := range r.rules {
		if rule.Priority >= minPriority && rule.Priority <= maxPriority && rule.IsActive {
			result = append(result, rule)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Priority > result[j].Priority
	})

	return result, nil
}
