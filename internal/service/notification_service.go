package service

import (
	"context"
	"finance_manager/internal/domain"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

type NotificationType string

const (
	NotificationEmail NotificationType = "email"
	NotificationSMS   NotificationType = "sms"
	NotificationPush  NotificationType = "push"
	NotificationSlack NotificationType = "slack"
)

type NotificationService struct {
	emailService EmailService
	smsService   SMSService
	pushService  PushService
	slackService SlackService
	messageQueue chan NotificationMessage
	workers      int
	shutdownChan chan struct{}
	wg           sync.WaitGroup
	logger       *slog.Logger
}

type NotificationMessage struct {
	Type      NotificationType
	Recipient string
	Subject   string
	Message   string
	Priority  int
	Metadata  map[string]string
	CreatedAt time.Time
}

type EmailService interface {
	SendEmail(to, subject, body string) error
}

type SMSService interface {
	SendSMS(to, message string) error
}

type PushService interface {
	SendPush(deviceID, title, message string) error
}

type SlackService interface {
	SendMessage(channel, message string) error
}

func NewNotificationService(
	emailService EmailService,
	smsService SMSService,
	pushService PushService,
	slackService SlackService,
	workers int,
	logger *slog.Logger,
) *NotificationService {
	if logger == nil {
		logger = slog.Default()
	}

	service := &NotificationService{
		emailService: emailService,
		smsService:   smsService,
		pushService:  pushService,
		slackService: slackService,
		messageQueue: make(chan NotificationMessage, 1000),
		workers:      workers,
		shutdownChan: make(chan struct{}),
		logger:       logger,
	}

	service.startWorkers()

	return service
}

func (s *NotificationService) SendTransactionNotification(
	ctx context.Context,
	tx *domain.Transaction,
	recipient string,
	notificationType NotificationType,
) error {
	var subject, message string

	switch tx.Status {
	case domain.StatusCompleted:
		subject = "Transaction Completed"
		message = fmt.Sprintf("Your transaction of %.2f %s has been completed successfully.", tx.Amount, tx.Currency)
	case domain.StatusFailed:
		subject = "Transaction Failed"
		message = fmt.Sprintf("Your transaction of %.2f %s has failed. Reason: %s", tx.Amount, tx.Currency, tx.Metadata["failure_reason"])
	case domain.StatusSuspicious:
		subject = "Suspicious Transaction Detected"
		message = fmt.Sprintf("A suspicious transaction of %.2f %s has been detected and is under review.", tx.Amount, tx.Currency)
	default:
		subject = "Transaction Update"
		message = fmt.Sprintf("Your transaction of %.2f %s is now %s.", tx.Amount, tx.Currency, tx.Status)
	}

	notification := NotificationMessage{
		Type:      notificationType,
		Recipient: recipient,
		Subject:   subject,
		Message:   message,
		Priority:  5,
		Metadata: map[string]string{
			"transaction_id":   tx.ID,
			"transaction_type": string(tx.Type),
			"risk_score":       fmt.Sprintf("%d", tx.RiskScore),
		},
		CreatedAt: time.Now(),
	}

	select {
	case s.messageQueue <- notification:
		s.logger.Info("Notification queued",
			slog.String("type", string(notificationType)),
			slog.String("recipient", recipient),
			slog.String("transaction_id", tx.ID))
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *NotificationService) SendFraudAlert(
	ctx context.Context,
	tx *domain.Transaction,
	reason string,
	severity string,
) error {
	message := fmt.Sprintf(
		"🚨 Fraud Alert!\nTransaction ID: %s\nAmount: %.2f %s\nRisk Score: %d\nFlags: %v\nReason: %s",
		tx.ID, tx.Amount, tx.Currency, tx.RiskScore, tx.FraudFlags, reason,
	)

	notifications := []NotificationMessage{
		{
			Type:      NotificationSlack,
			Recipient: "#fraud-alerts",
			Subject:   fmt.Sprintf("Fraud Alert - %s", severity),
			Message:   message,
			Priority:  10,
			Metadata: map[string]string{
				"transaction_id": tx.ID,
				"severity":       severity,
				"risk_score":     fmt.Sprintf("%d", tx.RiskScore),
			},
			CreatedAt: time.Now(),
		},
		{
			Type:      NotificationEmail,
			Recipient: "security@example.com",
			Subject:   fmt.Sprintf("Fraud Alert: %s - %s", severity, tx.ID),
			Message:   message,
			Priority:  10,
			Metadata: map[string]string{
				"transaction_id": tx.ID,
				"severity":       severity,
			},
			CreatedAt: time.Now(),
		},
	}

	for _, notification := range notifications {
		select {
		case s.messageQueue <- notification:
			s.logger.Warn("Fraud alert notification queued",
				slog.String("type", string(notification.Type)),
				slog.String("transaction_id", tx.ID),
				slog.String("severity", severity))
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

func (s *NotificationService) startWorkers() {
	for i := 0; i < s.workers; i++ {
		s.wg.Add(1)
		go s.worker(i)
	}
}

func (s *NotificationService) worker(id int) {
	defer s.wg.Done()

	s.logger.Info("Notification worker started", slog.Int("worker_id", id))

	for {
		select {
		case msg := <-s.messageQueue:
			s.processNotification(msg, id)
		case <-s.shutdownChan:
			s.logger.Info("Notification worker stopping", slog.Int("worker_id", id))
			return
		}
	}
}

func (s *NotificationService) processNotification(msg NotificationMessage, workerID int) {
	startTime := time.Now()
	var err error

	switch msg.Type {
	case NotificationEmail:
		err = s.emailService.SendEmail(msg.Recipient, msg.Subject, msg.Message)
	case NotificationSMS:
		err = s.smsService.SendSMS(msg.Recipient, msg.Message)
	case NotificationPush:
		err = s.pushService.SendPush(msg.Recipient, msg.Subject, msg.Message)
	case NotificationSlack:
		err = s.slackService.SendMessage(msg.Recipient, msg.Message)
	default:
		err = fmt.Errorf("unknown notification type: %s", msg.Type)
	}

	duration := time.Since(startTime)

	if err != nil {
		s.logger.Error("Failed to send notification",
			slog.String("type", string(msg.Type)),
			slog.String("recipient", msg.Recipient),
			slog.String("error", err.Error()),
			slog.Int("worker_id", workerID),
			slog.Duration("duration", duration))
	} else {
		s.logger.Info("Notification sent successfully",
			slog.String("type", string(msg.Type)),
			slog.String("recipient", msg.Recipient),
			slog.Int("worker_id", workerID),
			slog.Duration("duration", duration))
	}
}

func (s *NotificationService) Shutdown(ctx context.Context) error {
	close(s.shutdownChan)

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.logger.Info("Notification service shutdown complete")
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type MockEmailService struct {
	SentEmails []struct {
		To      string
		Subject string
		Body    string
	}
}

func (m *MockEmailService) SendEmail(to, subject, body string) error {
	m.SentEmails = append(m.SentEmails, struct {
		To      string
		Subject string
		Body    string
	}{to, subject, body})
	return nil
}

type MockSMSService struct {
	SentSMS []struct {
		To      string
		Message string
	}
}

func (m *MockSMSService) SendSMS(to, message string) error {
	m.SentSMS = append(m.SentSMS, struct {
		To      string
		Message string
	}{to, message})
	return nil
}
