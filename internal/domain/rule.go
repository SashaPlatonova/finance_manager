package domain

type RuleType string

const (
	RuleTypeFraud      RuleType = "fraud"
	RuleTypeCompliance RuleType = "compliance"
	RuleTypeBusiness   RuleType = "business"
)

type Rule struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Type        RuleType `json:"type"`
	Description string   `json:"description"`
	Condition   string   `json:"condition"`
	Action      string   `json:"action"`
	Priority    int      `json:"priority"`
	IsActive    bool     `json:"is_active"`
	Version     int      `json:"version"`
}
