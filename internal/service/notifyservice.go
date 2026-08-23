package service

import (
	"bytes"
	"certlife/internal/config"
	"certlife/internal/model"
	"certlife/internal/store"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"strings"
	"time"
)

type NotifyService struct {
	store             *store.NotificationStore
	cfg               config.NotifyConfig
	client            *http.Client
	dispatchRecipients []string
}

func NewNotifyService(s *store.NotificationStore, c config.NotifyConfig) *NotifyService {
	return &NotifyService{store: s, cfg: c, client: &http.Client{Timeout: 10 * time.Second}}
}
func (s *NotifyService) Rules(ctx context.Context) ([]model.NotificationRule, error) {
	return s.store.ListRules(ctx)
}
func (s *NotifyService) CreateRule(ctx context.Context, r *model.NotificationRule) error {
	return s.store.CreateRule(ctx, r)
}
func (s *NotifyService) UpdateRule(ctx context.Context, r *model.NotificationRule) error {
	return s.store.UpdateRule(ctx, r)
}
func (s *NotifyService) DeleteRule(ctx context.Context, id int64) error {
	return s.store.DeleteRule(ctx, id)
}
func (s *NotifyService) Notifications(ctx context.Context, f store.NotificationFilter) ([]model.Notification, int, error) {
	return s.store.List(ctx, f)
}
func (s *NotifyService) Audit(ctx context.Context, f store.AuditFilter) ([]model.AuditLog, int, error) {
	return s.store.ListAudit(ctx, f)
}
func (s *NotifyService) Dispatch(ctx context.Context, event model.EventType, cert *model.Certificate) (int, error) {
	rules, e := s.store.ListRules(ctx)
	if e != nil {
		return 0, e
	}
	created := 0
	for _, rule := range rules {
		environment := ""
		if cert != nil {
			environment = cert.Environment
		}
		if !rule.Matches(event, environment) {
			continue
		}
		subject, body := Render(event, cert)
		for _, channel := range rule.Channels {
			s.dispatchRecipients = rule.Recipients
			recipients := s.dispatchRecipients
			if channel == "webhook" {
				s.dispatchRecipients = []string{rule.WebhookURL}
				recipients = s.dispatchRecipients
			}
			for _, recipient := range recipients {
				n := model.Notification{EventType: event, Channel: channel, Recipient: recipient, Subject: subject, Body: body}
				if cert != nil {
					id := cert.ID
					n.CertificateID = &id
				}
				inserted, e := s.store.Insert(ctx, &n)
				if e != nil {
					return created, e
				}
				if inserted {
					created++
				}
			}
		}
	}
	return created, nil
}
func Render(event model.EventType, cert *model.Certificate) (string, string) {
	name := "CertLife"
	domain := "all certificates"
	days := 0
	if cert != nil {
		name = cert.Name
		domain = cert.Domain
		days = cert.DaysToExpire
	}
	subject := fmt.Sprintf("[CertLife] %s: %s", event, name)
	body := fmt.Sprintf("event=%s\ncertificate=%s\ndomain=%s\ndays_to_expire=%d\ntime=%s\n", event, name, domain, days, time.Now().UTC().Format(time.RFC3339))
	return subject, body
}
func (s *NotifyService) SendPending(ctx context.Context) (sent, failed int) {
	items, e := s.store.Pending(ctx, 50)
	if e != nil {
		return 0, 1
	}
	for i := range items {
		n := &items[i]
		e = s.send(ctx, *n)
		now := time.Now().UTC()
		n.LastAttemptAt = &now
		if e == nil {
			n.Status = "sent"
			n.SentAt = &now
			n.LastError = ""
		} else {
			n.Status = "failed"
			n.RetryCount++
			n.LastError = e.Error()
		}
		if updateErr := s.store.Update(ctx, n); updateErr != nil {
			failed++
		} else if e == nil {
			sent++
		} else {
			failed++
		}
	}
	return
}
func (s *NotifyService) send(ctx context.Context, n model.Notification) error {
	switch n.Channel {
	case "webhook":
		return s.webhook(ctx, n)
	case "email":
		return s.email(n)
	default:
		return fmt.Errorf("unsupported channel %q", n.Channel)
	}
}
func (s *NotifyService) webhook(ctx context.Context, n model.Notification) error {
	body, _ := json.Marshal(map[string]any{"event_type": n.EventType, "subject": n.Subject, "body": n.Body, "certificate_id": n.CertificateID})
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, n.Recipient, bytes.NewReader(body))
	if e != nil {
		return e
	}
	req.Header.Set("Content-Type", "application/json")
	mac := hmac.New(sha256.New, []byte(s.cfg.WebhookSecret))
	mac.Write(body)
	req.Header.Set("X-CertLife-Sign", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	resp, e := s.client.Do(req)
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned %s", resp.Status)
	}
	return nil
}
func (s *NotifyService) email(n model.Notification) error {
	if strings.TrimSpace(s.cfg.SMTPHost) == "" {
		return nil
	}
	addr := fmt.Sprintf("%s:%d", s.cfg.SMTPHost, s.cfg.SMTPPort)
	var auth smtp.Auth
	if s.cfg.SMTPUsername != "" {
		auth = smtp.PlainAuth("", s.cfg.SMTPUsername, s.cfg.SMTPPassword, s.cfg.SMTPHost)
	}
	msg := []byte("To: " + n.Recipient + "\r\nSubject: " + n.Subject + "\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" + n.Body)
	return smtp.SendMail(addr, auth, s.cfg.SMTPFrom, []string{n.Recipient}, msg)
}
