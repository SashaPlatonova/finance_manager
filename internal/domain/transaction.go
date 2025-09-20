package domain

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

type TransactionType string
type TransactionStatus string

const (
	TypeDeposit    TransactionType = "deposit"
	TypeWithdrawal TransactionType = "withdrawal"
	TypeTransfer   TransactionType = "transfer"

	StatusPending    TransactionStatus = "pending"
	StatusProcessing TransactionStatus = "processing"
	StatusCompleted  TransactionStatus = "completed"
	StatusFailed     TransactionStatus = "failed"
	StatusSuspicious TransactionStatus = "suspicious"
)

type Transaction struct {
	ID            string            `json:"id"`
	Type          TransactionType   `json:"type"`
	Amount        float64           `json:"amount"`
	Currency      string            `json:"currency"`
	FromAccountID string            `json:"from_account_id,omitempty"`
	ToAccountID   string            `json:"to_account_id,omitempty"`
	Description   string            `json:"description"`
	Status        TransactionStatus `json:"status"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	RiskScore     int               `json:"risk_score"`
	FraudFlags    []string          `json:"fraud_flags,omitempty"`
}

type TransactionEvent struct {
	TransactionID string
	Type          string
	Payload       interface{}
	Timestamp     time.Time
}

func NewTransaction(t TransactionType, amount float64, currency string) *Transaction {
	return &Transaction{
		ID:        generateTransactionID(),
		Type:      t,
		Amount:    amount,
		Currency:  currency,
		CreatedAt: time.Now(),
		Status:    StatusPending,
		Metadata:  make(map[string]string),
	}
}

func (tx *Transaction) WithDescription(desc string) *Transaction {
	tx.Description = desc
	return tx
}

func (tx *Transaction) WithAccounts(fromID, toID string) *Transaction {
	tx.FromAccountID = fromID
	tx.ToAccountID = toID
	return tx
}

func (tx *Transaction) AddMetadata(key, value string) {
	if tx.Metadata == nil {
		tx.Metadata = make(map[string]string)
	}
	tx.Metadata[key] = value
}

func generateTransactionID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
