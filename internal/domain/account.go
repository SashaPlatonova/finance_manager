package domain

import (
	"time"
)

type AccountStatus string

const (
	AccountActive    AccountStatus = "active"
	AccountSuspended AccountStatus = "suspended"
	AccountClosed    AccountStatus = "closed"
)

type Account struct {
	ID             string        `json:"id"`
	UserID         string        `json:"user_id"`
	Balance        float64       `json:"balance"`
	Currency       string        `json:"currency"`
	Status         AccountStatus `json:"status"`
	DailyLimit     float64       `json:"daily_limit"`
	MonthlyLimit   float64       `json:"monthly_limit"`
	CreatedAt      time.Time     `json:"created_at"`
	LastActivityAt time.Time     `json:"last_activity_at"`
	RiskCategory   string        `json:"risk_category"`
}

type BalanceUpdate struct {
	AccountID string
	Amount    float64
	Type      string
	Timestamp time.Time
}
