package processor

import (
	"finance_manager/internal/domain"
	"time"
)

type FraudDetector struct {
	patterns []FraudPattern
}

type FraudPattern struct {
	Name        string
	Description string
	Detect      func(*domain.Transaction) (bool, string)
	Weight      int
}

func NewFraudDetector() *FraudDetector {
	fd := &FraudDetector{}
	fd.patterns = []FraudPattern{
		{
			Name:        "large_amount",
			Description: "Transaction amount exceeds threshold",
			Detect: func(tx *domain.Transaction) (bool, string) {
				return tx.Amount > 10000, "large_amount"
			},
			Weight: 30,
		},
		{
			Name:        "frequent_transactions",
			Description: "Too many transactions in short time",
			Detect:      fd.detectFrequentTransactions,
			Weight:      25,
		},
		{
			Name:        "geographical_anomaly",
			Description: "Transaction from unusual location",
			Detect:      fd.detectGeographicalAnomaly,
			Weight:      35,
		},
		{
			Name:        "velocity_check",
			Description: "Unusual transaction velocity",
			Detect:      fd.detectVelocityAnomaly,
			Weight:      40,
		},
	}
	return fd
}

func (fd *FraudDetector) AnalyzeTransaction(tx *domain.Transaction) (int, []string) {
	var riskScore int
	var flags []string

	for _, pattern := range fd.patterns {
		if detected, flag := pattern.Detect(tx); detected {
			riskScore += pattern.Weight
			flags = append(flags, flag)
		}
	}

	if riskScore > 0 {
		riskScore = fd.applyTimeBasedModifiers(tx, riskScore)
	}

	return min(riskScore, 100), flags
}

func (fd *FraudDetector) detectFrequentTransactions(tx *domain.Transaction) (bool, string) {
	return tx.Amount > 5000 && time.Now().Hour() < 6, "frequent_transactions"
}

func (fd *FraudDetector) detectGeographicalAnomaly(tx *domain.Transaction) (bool, string) {
	if location, exists := tx.Metadata["location"]; exists {
		return location == "high_risk_country", "geographical_anomaly"
	}
	return false, ""
}

func (fd *FraudDetector) detectVelocityAnomaly(tx *domain.Transaction) (bool, string) {
	if velocity, exists := tx.Metadata["velocity"]; exists && velocity == "high" {
		return true, "velocity_anomaly"
	}
	return false, ""
}

func (fd *FraudDetector) applyTimeBasedModifiers(tx *domain.Transaction, baseScore int) int {
	hour := time.Now().Hour()
	if hour >= 23 || hour <= 5 {
		return baseScore + 15
	}
	return baseScore
}
