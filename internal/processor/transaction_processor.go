package processor

import (
	"context"
	"finance_manager/internal/domain"
	"finance_manager/internal/repository"
	"finance_manager/pkg/validator"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

type TransactionProcessor struct {
	txRepo        repository.TransactionRepository
	accountRepo   repository.AccountRepository
	ruleRepo      repository.RuleRepository
	fraudDetector *FraudDetector
	ruleEngine    *RuleEngine
	validator     *validator.TransactionValidator
	eventCh       chan domain.TransactionEvent
	workerPool    chan struct{}
	mu            sync.RWMutex
	metrics       map[string]int
	logger        *slog.Logger
}

func NewTransactionProcessor(
	txRepo repository.TransactionRepository,
	accountRepo repository.AccountRepository,
	ruleRepo repository.RuleRepository,
	maxWorkers int,
) *TransactionProcessor {
	return &TransactionProcessor{
		txRepo:        txRepo,
		accountRepo:   accountRepo,
		ruleRepo:      ruleRepo,
		fraudDetector: NewFraudDetector(),
		ruleEngine:    NewRuleEngine(ruleRepo, nil),
		validator:     validator.NewTransactionValidator(),
		eventCh:       make(chan domain.TransactionEvent, 1000),
		workerPool:    make(chan struct{}, maxWorkers),
		metrics:       make(map[string]int),
	}
}

func (p *TransactionProcessor) ProcessTransaction(ctx context.Context, tx *domain.Transaction) error {
	if err := p.validator.ValidateTransaction(tx); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	riskScore, flags := p.fraudDetector.AnalyzeTransaction(tx)
	tx.RiskScore = riskScore
	tx.FraudFlags = flags

	_, err := p.ruleEngine.EvaluateRules(ctx, tx)
	if err != nil {
		return fmt.Errorf("rule evaluation failed: %w", err)
	}

	if riskScore > 80 {
		tx.Status = domain.StatusSuspicious
		p.eventCh <- domain.TransactionEvent{
			TransactionID: tx.ID,
			Type:          "transaction_suspicious",
			Payload:       map[string]interface{}{"risk_score": riskScore, "flags": flags},
			Timestamp:     time.Now(),
		}
	} else if riskScore > 50 {
		tx.Status = domain.StatusPending
	} else {
		if err := p.executeTransaction(ctx, tx); err != nil {
			return err
		}
		tx.Status = domain.StatusCompleted
	}

	if err := p.txRepo.Save(ctx, tx); err != nil {
		return err
	}

	p.recordMetric("transactions_processed", 1)
	return nil
}

func (p *TransactionProcessor) GetTransaction(ctx context.Context, transactionID string) (*domain.Transaction, error) {
	return p.txRepo.GetByID(ctx, transactionID)
}

func (p *TransactionProcessor) executeTransaction(ctx context.Context, tx *domain.Transaction) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	switch tx.Type {
	case domain.TypeTransfer:
		return p.processTransfer(ctx, tx)
	case domain.TypeDeposit:
		return p.processDeposit(ctx, tx)
	case domain.TypeWithdrawal:
		return p.processWithdrawal(ctx, tx)
	default:
		return fmt.Errorf("unknown transaction type: %s", tx.Type)
	}
	return nil
}

func (p *TransactionProcessor) GetMetrics() map[string]int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.metrics
}

func (p *TransactionProcessor) recordMetric(key string, value int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.metrics[key] += value
}

func (p *TransactionProcessor) processTransfer(ctx context.Context, tx *domain.Transaction) error {
	p.logger.InfoContext(ctx, "Processing transfer",
		slog.String("transaction_id", tx.ID),
		slog.String("from_account", tx.FromAccountID),
		slog.String("to_account", tx.ToAccountID),
		slog.Float64("amount", tx.Amount))

	fromAccount, err := p.accountRepo.GetByID(ctx, tx.FromAccountID)
	if err != nil {
		return fmt.Errorf("failed to get from account: %w", err)
	}

	toAccount, err := p.accountRepo.GetByID(ctx, tx.ToAccountID)
	if err != nil {
		return fmt.Errorf("failed to get to account: %w", err)
	}

	if fromAccount.Currency != toAccount.Currency {
		return fmt.Errorf("currency mismatch: %s != %s", fromAccount.Currency, toAccount.Currency)
	}

	if fromAccount.Status != domain.AccountActive {
		return fmt.Errorf("from account is not active: %s", fromAccount.Status)
	}
	if toAccount.Status != domain.AccountActive {
		return fmt.Errorf("to account is not active: %s", toAccount.Status)
	}

	if fromAccount.Balance < tx.Amount {
		return repository.ErrInsufficientFunds
	}

	if err := p.checkAccountLimits(ctx, fromAccount, tx); err != nil {
		return err
	}

	fromAccount.Balance -= tx.Amount
	toAccount.Balance += tx.Amount

	now := time.Now()
	fromAccount.LastActivityAt = now
	toAccount.LastActivityAt = now

	if err := p.accountRepo.Update(ctx, fromAccount); err != nil {
		return fmt.Errorf("failed to update from account: %w", err)
	}
	if err := p.accountRepo.Update(ctx, toAccount); err != nil {
		fromAccount.Balance += tx.Amount
		p.accountRepo.Update(ctx, fromAccount)
		return fmt.Errorf("failed to update to account: %w", err)
	}

	p.logger.InfoContext(ctx, "Transfer completed successfully",
		slog.String("transaction_id", tx.ID))
	return nil
}

func (p *TransactionProcessor) processDeposit(ctx context.Context, tx *domain.Transaction) error {
	p.logger.InfoContext(ctx, "Processing deposit",
		slog.String("transaction_id", tx.ID),
		slog.String("to_account", tx.ToAccountID),
		slog.Float64("amount", tx.Amount))

	if tx.ToAccountID == "" {
		return fmt.Errorf("to account is required for deposit")
	}

	toAccount, err := p.accountRepo.GetByID(ctx, tx.ToAccountID)
	if err != nil {
		return fmt.Errorf("failed to get to account: %w", err)
	}

	if toAccount.Status != domain.AccountActive {
		return fmt.Errorf("account is not active: %s", toAccount.Status)
	}

	if err := p.checkDepositLimits(ctx, toAccount, tx); err != nil {
		return err
	}

	toAccount.Balance += tx.Amount
	toAccount.LastActivityAt = time.Now()

	if err := p.accountRepo.Update(ctx, toAccount); err != nil {
		return fmt.Errorf("failed to update account: %w", err)
	}

	p.logger.InfoContext(ctx, "Deposit completed successfully",
		slog.String("transaction_id", tx.ID))
	return nil
}

func (p *TransactionProcessor) processWithdrawal(ctx context.Context, tx *domain.Transaction) error {
	p.logger.InfoContext(ctx, "Processing withdrawal",
		slog.String("transaction_id", tx.ID),
		slog.String("from_account", tx.FromAccountID),
		slog.Float64("amount", tx.Amount))

	if tx.FromAccountID == "" {
		return fmt.Errorf("from account is required for withdrawal")
	}

	fromAccount, err := p.accountRepo.GetByID(ctx, tx.FromAccountID)
	if err != nil {
		return fmt.Errorf("failed to get from account: %w", err)
	}

	if fromAccount.Status != domain.AccountActive {
		return fmt.Errorf("account is not active: %s", fromAccount.Status)
	}

	if fromAccount.Balance < tx.Amount {
		return repository.ErrInsufficientFunds
	}

	if err := p.checkWithdrawalLimits(ctx, fromAccount, tx); err != nil {
		return err
	}

	fromAccount.Balance -= tx.Amount
	fromAccount.LastActivityAt = time.Now()

	if err := p.accountRepo.Update(ctx, fromAccount); err != nil {
		return fmt.Errorf("failed to update account: %w", err)
	}

	p.logger.InfoContext(ctx, "Withdrawal completed successfully",
		slog.String("transaction_id", tx.ID))
	return nil
}

func (p *TransactionProcessor) checkAccountLimits(ctx context.Context, account *domain.Account, tx *domain.Transaction) error {
	dailyVolume, err := p.txRepo.GetDailyVolume(ctx, account.ID, time.Now())
	if err != nil {
		return fmt.Errorf("failed to get daily volume: %w", err)
	}

	if account.DailyLimit > 0 && (dailyVolume+tx.Amount) > account.DailyLimit {
		return fmt.Errorf("daily limit exceeded: %.2f/%.2f", dailyVolume+tx.Amount, account.DailyLimit)
	}

	now := time.Now()
	monthlyVolume, err := p.txRepo.GetMonthlyVolume(ctx, account.ID, now.Year(), now.Month())
	if err != nil {
		return fmt.Errorf("failed to get monthly volume: %w", err)
	}

	if account.MonthlyLimit > 0 && (monthlyVolume+tx.Amount) > account.MonthlyLimit {
		return fmt.Errorf("monthly limit exceeded: %.2f/%.2f", monthlyVolume+tx.Amount, account.MonthlyLimit)
	}

	return nil
}

func (p *TransactionProcessor) checkDepositLimits(ctx context.Context, account *domain.Account, tx *domain.Transaction) error {
	const maxDepositAmount = 50000.0
	if tx.Amount > maxDepositAmount {
		return fmt.Errorf("deposit amount exceeds maximum limit: %.2f/%.2f", tx.Amount, maxDepositAmount)
	}

	return nil
}

func (p *TransactionProcessor) checkWithdrawalLimits(ctx context.Context, account *domain.Account, tx *domain.Transaction) error {
	dailyWithdrawal, err := p.getDailyWithdrawal(ctx, account.ID, time.Now())
	if err != nil {
		return fmt.Errorf("failed to get daily withdrawal: %w", err)
	}

	const dailyWithdrawalLimit = 5000.0
	if (dailyWithdrawal + tx.Amount) > dailyWithdrawalLimit {
		return fmt.Errorf("daily withdrawal limit exceeded: %.2f/%.2f", dailyWithdrawal+tx.Amount, dailyWithdrawalLimit)
	}

	return nil
}

func (p *TransactionProcessor) getDailyWithdrawal(ctx context.Context, accountID string, date time.Time) (float64, error) {
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	transactions, err := p.txRepo.GetByPeriod(ctx, startOfDay, endOfDay)
	if err != nil {
		return 0, err
	}

	var totalWithdrawal float64
	for _, tx := range transactions {
		if tx.FromAccountID == accountID && tx.Type == domain.TypeWithdrawal && tx.Status == domain.StatusCompleted {
			totalWithdrawal += tx.Amount
		}
	}

	return totalWithdrawal, nil
}
