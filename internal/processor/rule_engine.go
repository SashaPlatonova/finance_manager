package processor

import (
	"context"
	"encoding/json"
	"finance_manager/internal/domain"
	"finance_manager/internal/repository"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
)

type RuleEngine struct {
	ruleRepo repository.RuleRepository
	logger   *slog.Logger
	cache    map[string][]*domain.Rule
}

type Condition struct {
	Field    string      `json:"field"`
	Operator string      `json:"operator"`
	Value    interface{} `json:"value"`
}

type RuleAction struct {
	Type    string                 `json:"type"`
	Params  map[string]interface{} `json:"params"`
	Message string                 `json:"message"`
}

type RuleResult struct {
	RuleID      string
	RuleName    string
	Triggered   bool
	Action      RuleAction
	Description string
}

func NewRuleEngine(ruleRepo repository.RuleRepository, logger *slog.Logger) *RuleEngine {
	if logger == nil {
		logger = slog.Default()
	}

	return &RuleEngine{
		ruleRepo: ruleRepo,
		logger:   logger,
		cache:    make(map[string][]*domain.Rule),
	}
}

func (e *RuleEngine) EvaluateRules(ctx context.Context, tx *domain.Transaction) ([]RuleResult, error) {
	rules, err := e.getActiveRules(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get active rules: %w", err)
	}

	var results []RuleResult

	for _, rule := range rules {
		result, err := e.evaluateRule(ctx, rule, tx)
		if err != nil {
			e.logger.ErrorContext(ctx, "Failed to evaluate rule",
				slog.String("rule_id", rule.ID),
				slog.String("error", err.Error()))
			continue
		}

		if result.Triggered {
			results = append(results, result)
			e.logger.InfoContext(ctx, "Rule triggered",
				slog.String("rule_id", rule.ID),
				slog.String("rule_name", rule.Name),
				slog.String("transaction_id", tx.ID))
		}
	}

	slices.SortFunc(results, func(a, b RuleResult) int {
		return e.getRulePriority(a.RuleID) - e.getRulePriority(b.RuleID)
	})

	return results, nil
}

func (e *RuleEngine) evaluateRule(ctx context.Context, rule *domain.Rule, tx *domain.Transaction) (RuleResult, error) {
	result := RuleResult{
		RuleID:      rule.ID,
		RuleName:    rule.Name,
		Description: rule.Description,
	}

	condition, err := e.parseCondition(rule.Condition)
	if err != nil {
		return result, fmt.Errorf("failed to parse condition: %w", err)
	}

	triggered, err := e.checkCondition(condition, tx)
	if err != nil {
		return result, fmt.Errorf("failed to check condition: %w", err)
	}

	result.Triggered = triggered

	if triggered {
		action, err := e.parseAction(rule.Action)
		if err != nil {
			return result, fmt.Errorf("failed to parse action: %w", err)
		}
		result.Action = action
	}

	return result, nil
}

func (e *RuleEngine) parseCondition(conditionStr string) (Condition, error) {
	var condition Condition
	if err := json.Unmarshal([]byte(conditionStr), &condition); err != nil {
		return Condition{}, fmt.Errorf("invalid condition JSON: %w", err)
	}
	return condition, nil
}

func (e *RuleEngine) parseAction(actionStr string) (RuleAction, error) {
	var action RuleAction
	if err := json.Unmarshal([]byte(actionStr), &action); err != nil {
		return RuleAction{}, fmt.Errorf("invalid action JSON: %w", err)
	}
	return action, nil
}

func (e *RuleEngine) checkCondition(condition Condition, tx *domain.Transaction) (bool, error) {
	switch condition.Field {
	case "amount":
		return e.checkAmountCondition(condition, tx.Amount)
	case "currency":
		return e.checkStringCondition(condition, tx.Currency)
	case "type":
		return e.checkStringCondition(condition, string(tx.Type))
	case "risk_score":
		return e.checkNumericCondition(condition, float64(tx.RiskScore))
	case "metadata":
		return e.checkMetadataCondition(condition, tx.Metadata)
	default:
		return false, fmt.Errorf("unknown field: %s", condition.Field)
	}
}

func (e *RuleEngine) checkAmountCondition(condition Condition, amount float64) (bool, error) {
	targetValue, ok := condition.Value.(float64)
	if !ok {
		return false, fmt.Errorf("invalid value type for amount: %v", condition.Value)
	}

	switch condition.Operator {
	case ">":
		return amount > targetValue, nil
	case ">=":
		return amount >= targetValue, nil
	case "<":
		return amount < targetValue, nil
	case "<=":
		return amount <= targetValue, nil
	case "==":
		return amount == targetValue, nil
	case "!=":
		return amount != targetValue, nil
	default:
		return false, fmt.Errorf("unknown operator: %s", condition.Operator)
	}
}

func (e *RuleEngine) checkNumericCondition(condition Condition, value float64) (bool, error) {
	targetValue, ok := condition.Value.(float64)
	if !ok {
		return false, fmt.Errorf("invalid value type for numeric field: %v", condition.Value)
	}

	switch condition.Operator {
	case ">":
		return value > targetValue, nil
	case ">=":
		return value >= targetValue, nil
	case "<":
		return value < targetValue, nil
	case "<=":
		return value <= targetValue, nil
	case "==":
		return value == targetValue, nil
	case "!=":
		return value != targetValue, nil
	default:
		return false, fmt.Errorf("unknown operator: %s", condition.Operator)
	}
}

func (e *RuleEngine) checkStringCondition(condition Condition, value string) (bool, error) {
	targetValue, ok := condition.Value.(string)
	if !ok {
		return false, fmt.Errorf("invalid value type for string field: %v", condition.Value)
	}

	switch condition.Operator {
	case "==":
		return value == targetValue, nil
	case "!=":
		return value != targetValue, nil
	case "contains":
		return regexp.MustCompile(targetValue).MatchString(value), nil
	case "in":
		if arr, ok := condition.Value.([]string); ok {
			return slices.Contains(arr, value), nil
		}
		return false, fmt.Errorf("invalid value for 'in' operator")
	default:
		return false, fmt.Errorf("unknown operator: %s", condition.Operator)
	}
}

func (e *RuleEngine) checkMetadataCondition(condition Condition, metadata map[string]string) (bool, error) {
	conditions, ok := condition.Value.(map[string]interface{})
	if !ok {
		return false, fmt.Errorf("invalid value type for metadata condition")
	}

	for key, expectedValue := range conditions {
		actualValue, exists := metadata[key]
		if !exists {
			return false, nil
		}

		if fmt.Sprintf("%v", actualValue) != fmt.Sprintf("%v", expectedValue) {
			return false, nil
		}
	}

	return true, nil
}

func (e *RuleEngine) getActiveRules(ctx context.Context) ([]*domain.Rule, error) {
	if cached, exists := e.cache["active"]; exists {
		return cached, nil
	}

	rules, err := e.ruleRepo.GetActiveRules(ctx)
	if err != nil {
		return nil, err
	}

	e.cache["active"] = rules

	return rules, nil
}

func (e *RuleEngine) getRulePriority(ruleID string) int {
	if e == nil {
		return 0
	}
	if e.cache != nil {
		for _, rules := range e.cache {
			for _, rule := range rules {
				if rule.ID == ruleID {
					return rule.Priority
				}
			}
		}
	}
	ctx := context.Background()
	rule, err := e.ruleRepo.GetByID(ctx, ruleID)
	if err == nil && rule != nil {
		return rule.Priority
	}
	return 0
}

func (e *RuleEngine) InvalidateCache() {
	e.cache = make(map[string][]*domain.Rule)
}

func (e *RuleEngine) ExecuteAction(ctx context.Context, action RuleAction, tx *domain.Transaction) error {
	switch action.Type {
	case "flag_transaction":
		return e.handleFlagAction(ctx, action, tx)
	case "block_transaction":
		return e.handleBlockAction(ctx, action, tx)
	case "require_approval":
		return e.handleApprovalAction(ctx, action, tx)
	case "notify":
		return e.handleNotifyAction(ctx, action, tx)
	case "adjust_risk_score":
		return e.handleRiskAdjustAction(ctx, action, tx)
	default:
		return fmt.Errorf("unknown action type: %s", action.Type)
	}
}

func (e *RuleEngine) handleFlagAction(ctx context.Context, action RuleAction, tx *domain.Transaction) error {
	reason, _ := action.Params["reason"].(string)
	e.logger.WarnContext(ctx, "Transaction flagged",
		slog.String("transaction_id", tx.ID),
		slog.String("reason", reason))

	if tx.Metadata == nil {
		tx.Metadata = make(map[string]string)
	}
	tx.Metadata["flagged_reason"] = reason

	return nil
}

func (e *RuleEngine) handleBlockAction(ctx context.Context, action RuleAction, tx *domain.Transaction) error {
	reason, _ := action.Params["reason"].(string)
	e.logger.ErrorContext(ctx, "Transaction blocked",
		slog.String("transaction_id", tx.ID),
		slog.String("reason", reason))

	tx.Status = domain.StatusFailed
	tx.Metadata["block_reason"] = reason

	return nil
}

func (e *RuleEngine) handleApprovalAction(ctx context.Context, action RuleAction, tx *domain.Transaction) error {
	e.logger.InfoContext(ctx, "Transaction requires approval",
		slog.String("transaction_id", tx.ID))

	tx.Status = domain.StatusPending
	tx.Metadata["requires_approval"] = "true"

	return nil
}

func (e *RuleEngine) handleNotifyAction(ctx context.Context, action RuleAction, tx *domain.Transaction) error {
	channel, _ := action.Params["channel"].(string)
	message, _ := action.Params["message"].(string)

	e.logger.InfoContext(ctx, "Notification sent",
		slog.String("channel", channel),
		slog.String("transaction_id", tx.ID),
		slog.String("message", message))

	return nil
}

func (e *RuleEngine) handleRiskAdjustAction(ctx context.Context, action RuleAction, tx *domain.Transaction) error {
	adjustment, _ := action.Params["adjustment"].(float64)

	tx.RiskScore += int(adjustment)
	if tx.RiskScore > 100 {
		tx.RiskScore = 100
	}
	if tx.RiskScore < 0 {
		tx.RiskScore = 0
	}

	e.logger.InfoContext(ctx, "Risk score adjusted",
		slog.String("transaction_id", tx.ID),
		slog.Int("new_risk_score", tx.RiskScore),
		slog.Int("adjustment", int(adjustment)))

	return nil
}
