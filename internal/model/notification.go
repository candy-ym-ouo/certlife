package model

import (
	"errors"
	"strings"
	"time"
)

type EventType string

const (
	EventExpiringSoon       EventType = "expiring_soon"
	EventExpired            EventType = "expired"
	EventRenewalFailed      EventType = "renewal_failed"
	EventVerificationFailed EventType = "verification_failed"
	EventDeployFailed       EventType = "deploy_failed"
	EventRenewalSucceeded   EventType = "renewal_succeeded"
	EventDailySummary       EventType = "daily_summary"
)

type Notification struct {
	ID            int64      `json:"id"`
	CertificateID *int64     `json:"certificate_id,omitempty"`
	EventType     EventType  `json:"event_type"`
	Channel       string     `json:"channel"`
	Recipient     string     `json:"recipient"`
	Subject       string     `json:"subject"`
	Body          string     `json:"body"`
	Status        string     `json:"status"`
	RetryCount    int        `json:"retry_count"`
	LastError     string     `json:"last_error,omitempty"`
	SentAt        *time.Time `json:"sent_at,omitempty"`
	LastAttemptAt *time.Time `json:"last_attempt_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}
type NotificationRule struct {
	ID           int64       `json:"id"`
	Name         string      `json:"name"`
	EventTypes   []EventType `json:"event_types"`
	Environments []string    `json:"environments"`
	Channels     []string    `json:"channels"`
	Recipients   []string    `json:"recipients"`
	WebhookURL   string      `json:"webhook_url,omitempty"`
	Enabled      bool        `json:"enabled"`
	CreatedAt    time.Time   `json:"created_at"`
}

func (r NotificationRule) Validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return errors.New("rule name is required")
	}
	if len(r.Channels) == 0 {
		return errors.New("at least one channel is required")
	}
	for _, channel := range r.Channels {
		switch channel {
		case "email":
			if len(r.Recipients) == 0 {
				return errors.New("email recipients are required")
			}
		case "webhook":
			if strings.TrimSpace(r.WebhookURL) == "" {
				return errors.New("webhook_url is required")
			}
		default:
			return errors.New("unsupported notification channel")
		}
	}
	return nil
}
func (r NotificationRule) Matches(event EventType, environment string) bool {
	if !r.Enabled {
		return false
	}
	if len(r.EventTypes) > 0 && !containsEvent(r.EventTypes, event) {
		return false
	}
	if len(r.Environments) > 0 && !containsString(r.Environments, environment) {
		return false
	}
	return true
}
func containsEvent(values []EventType, want EventType) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

type AuditLog struct {
	ID           int64     `json:"id"`
	Actor        string    `json:"actor"`
	Action       string    `json:"action"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id,omitempty"`
	Detail       string    `json:"detail,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}
