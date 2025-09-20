package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
)

type Signer struct {
	secretKey []byte
	logger    *slog.Logger
}

func NewSigner(secretKey string, logger *slog.Logger) *Signer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Signer{
		secretKey: []byte(secretKey),
		logger:    logger,
	}
}

func (s *Signer) Sign(data []byte) string {
	mac := hmac.New(sha256.New, s.secretKey)
	mac.Write(data)
	signature := mac.Sum(nil)
	return hex.EncodeToString(signature)
}

func (s *Signer) Verify(data []byte, signature string) (bool, error) {
	expectedSignature := s.Sign(data)

	if !hmac.Equal([]byte(expectedSignature), []byte(signature)) {
		s.logger.Warn("Signature verification failed",
			slog.String("expected", expectedSignature),
			slog.String("received", signature))
		return false, fmt.Errorf("invalid signature")
	}

	return true, nil
}

func (s *Signer) SignTransaction(transactionID string, amount float64, currency string, timestamp int64) string {
	data := fmt.Sprintf("%s:%.2f:%s:%d", transactionID, amount, currency, timestamp)
	return s.Sign([]byte(data))
}

func (s *Signer) VerifyTransaction(transactionID string, amount float64, currency string, timestamp int64, signature string) (bool, error) {
	data := fmt.Sprintf("%s:%.2f:%s:%d", transactionID, amount, currency, timestamp)
	return s.Verify([]byte(data), signature)
}
