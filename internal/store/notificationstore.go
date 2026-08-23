package store

import (
	"certlife/internal/model"
	"context"
	"database/sql"
	"strings"
	"time"
)

type NotificationFilter struct {
	Status, EventType string
	CertificateID     int64
	Page, Size        int
}
type AuditFilter struct {
	Action, ResourceType, ResourceID string
	Page, Size                       int
}
type NotificationStore struct{ db *DB }

func NewNotificationStore(db *DB) *NotificationStore { return &NotificationStore{db} }
func (s *NotificationStore) Insert(ctx context.Context, n *model.Notification) (bool, error) {
	if n.Status == "" {
		n.Status = "pending"
	}
	n.CreatedAt = time.Now().UTC()
	r, e := s.db.ExecContext(ctx, `INSERT INTO notifications(certificate_id,event_type,channel,recipient,subject,body,status,retry_count,last_error,sent_at,last_attempt_at,created_at)VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, n.CertificateID, n.EventType, n.Channel, n.Recipient, n.Subject, n.Body, n.Status, n.RetryCount, n.LastError, formatTimePtr(n.SentAt), formatTimePtr(n.LastAttemptAt), formatTime(n.CreatedAt))
	if e != nil {
		return false, e
	}
	count, _ := r.RowsAffected()
	if count > 0 {
		n.ID, _ = r.LastInsertId()
	}
	return count > 0, nil
}
func (s *NotificationStore) Update(ctx context.Context, n *model.Notification) error {
	_, e := s.db.ExecContext(ctx, `UPDATE notifications SET status=?,retry_count=?,last_error=?,sent_at=?,last_attempt_at=? WHERE id=?`, n.Status, n.RetryCount, n.LastError, formatTimePtr(n.SentAt), formatTimePtr(n.LastAttemptAt), n.ID)
	return e
}
func (s *NotificationStore) Pending(ctx context.Context, limit int) ([]model.Notification, error) {
	if limit < 1 {
		limit = 50
	}
	rows, e := s.db.QueryContext(ctx, `SELECT id,certificate_id,event_type,channel,recipient,subject,body,status,retry_count,COALESCE(last_error,''),COALESCE(sent_at,''),COALESCE(last_attempt_at,''),created_at FROM notifications WHERE status='pending' OR (status='failed' AND retry_count<=3 AND datetime(COALESCE(last_attempt_at,created_at))<=CASE retry_count WHEN 1 THEN datetime('now','-1 minute') WHEN 2 THEN datetime('now','-5 minutes') ELSE datetime('now','-30 minutes') END) ORDER BY created_at LIMIT ?`, limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []model.Notification
	for rows.Next() {
		n, e := scanNotification(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, *n)
	}
	return out, rows.Err()
}
func (s *NotificationStore) List(ctx context.Context, f NotificationFilter) ([]model.Notification, int, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.Size < 1 {
		f.Size = 20
	}
	where := []string{"1=1"}
	args := []any{}
	if f.Status != "" {
		where = append(where, "status=?")
		args = append(args, f.Status)
	}
	if f.EventType != "" {
		where = append(where, "event_type=?")
		args = append(args, f.EventType)
	}
	if f.CertificateID > 0 {
		where = append(where, "certificate_id=?")
		args = append(args, f.CertificateID)
	}
	clause := strings.Join(where, " AND ")
	var total int
	if e := s.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM notifications WHERE "+clause, args...).Scan(&total); e != nil {
		return nil, 0, e
	}
	args = append(args, f.Size, (f.Page-1)*f.Size)
	rows, e := s.db.QueryContext(ctx, "SELECT id,certificate_id,event_type,channel,recipient,subject,body,status,retry_count,COALESCE(last_error,''),COALESCE(sent_at,''),COALESCE(last_attempt_at,''),created_at FROM notifications WHERE "+clause+" ORDER BY id DESC LIMIT ? OFFSET ?", args...)
	if e != nil {
		return nil, 0, e
	}
	defer rows.Close()
	out := []model.Notification{}
	for rows.Next() {
		n, e := scanNotification(rows)
		if e != nil {
			return nil, 0, e
		}
		out = append(out, *n)
	}
	return out, total, rows.Err()
}
func (s *NotificationStore) All(ctx context.Context, f NotificationFilter) ([]model.Notification, error) {
	out := []model.Notification{}
	for page := 1; ; page++ {
		f.Page, f.Size = page, 100
		items, total, e := s.List(ctx, f)
		if e != nil {
			return nil, e
		}
		out = append(out, items...)
		if len(out) >= total {
			return out, nil
		}
	}
}
func scanNotification(row scanner) (*model.Notification, error) {
	var n model.Notification
	var cert sql.NullInt64
	var sent, attempted, created string
	if e := row.Scan(&n.ID, &cert, &n.EventType, &n.Channel, &n.Recipient, &n.Subject, &n.Body, &n.Status, &n.RetryCount, &n.LastError, &sent, &attempted, &created); e != nil {
		return nil, e
	}
	if cert.Valid {
		n.CertificateID = &cert.Int64
	}
	n.SentAt = parseTimePtr(sent)
	n.LastAttemptAt = parseTimePtr(attempted)
	n.CreatedAt = parseTime(created)
	return &n, nil
}
func (s *NotificationStore) CreateRule(ctx context.Context, r *model.NotificationRule) error {
	if e := r.Validate(); e != nil {
		return e
	}
	r.CreatedAt = time.Now().UTC()
	result, e := s.db.ExecContext(ctx, `INSERT INTO notification_rules(name,event_types,environments,channels,recipients,webhook_url,enabled,created_at)VALUES(?,?,?,?,?,?,?,?)`, r.Name, toJSON(r.EventTypes), toJSON(r.Environments), toJSON(r.Channels), toJSON(r.Recipients), r.WebhookURL, boolInt(r.Enabled), formatTime(r.CreatedAt))
	if e != nil {
		return e
	}
	r.ID, e = result.LastInsertId()
	return e
}
func (s *NotificationStore) UpdateRule(ctx context.Context, r *model.NotificationRule) error {
	if e := r.Validate(); e != nil {
		return e
	}
	result, e := s.db.ExecContext(ctx, `UPDATE notification_rules SET name=?,event_types=?,environments=?,channels=?,recipients=?,webhook_url=?,enabled=? WHERE id=?`, r.Name, toJSON(r.EventTypes), toJSON(r.Environments), toJSON(r.Channels), toJSON(r.Recipients), r.WebhookURL, boolInt(r.Enabled), r.ID)
	if e != nil {
		return e
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
func (s *NotificationStore) DeleteRule(ctx context.Context, id int64) error {
	r, e := s.db.ExecContext(ctx, `DELETE FROM notification_rules WHERE id=?`, id)
	if e != nil {
		return e
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
func (s *NotificationStore) ListRules(ctx context.Context) ([]model.NotificationRule, error) {
	rows, e := s.db.QueryContext(ctx, `SELECT id,name,event_types,environments,channels,recipients,COALESCE(webhook_url,''),enabled,created_at FROM notification_rules ORDER BY id`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []model.NotificationRule
	for rows.Next() {
		var r model.NotificationRule
		var events, environments, channels, recipients, created string
		var enabled int
		if e := rows.Scan(&r.ID, &r.Name, &events, &environments, &channels, &recipients, &r.WebhookURL, &enabled, &created); e != nil {
			return nil, e
		}
		parseJSON(events, &r.EventTypes)
		parseJSON(environments, &r.Environments)
		parseJSON(channels, &r.Channels)
		parseJSON(recipients, &r.Recipients)
		r.Enabled = enabled == 1
		r.CreatedAt = parseTime(created)
		out = append(out, r)
	}
	return out, rows.Err()
}
func (s *NotificationStore) AddAudit(ctx context.Context, a model.AuditLog) error {
	if a.Actor == "" {
		a.Actor = "system"
	}
	a.CreatedAt = time.Now().UTC()
	_, e := s.db.ExecContext(ctx, `INSERT INTO audit_logs(actor,action,resource_type,resource_id,detail,created_at)VALUES(?,?,?,?,?,?)`, a.Actor, a.Action, a.ResourceType, a.ResourceID, a.Detail, formatTime(a.CreatedAt))
	return e
}
func (s *NotificationStore) ListAudit(ctx context.Context, f AuditFilter) ([]model.AuditLog, int, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.Size < 1 {
		f.Size = 20
	}
	where := []string{"1=1"}
	args := []any{}
	if f.Action != "" {
		where = append(where, "action=?")
		args = append(args, f.Action)
	}
	if f.ResourceType != "" {
		where = append(where, "resource_type=?")
		args = append(args, f.ResourceType)
	}
	if f.ResourceID != "" {
		where = append(where, "resource_id=?")
		args = append(args, f.ResourceID)
	}
	clause := strings.Join(where, " AND ")
	var total int
	if e := s.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM audit_logs WHERE "+clause, args...).Scan(&total); e != nil {
		return nil, 0, e
	}
	args = append(args, f.Size, (f.Page-1)*f.Size)
	rows, e := s.db.QueryContext(ctx, "SELECT id,actor,action,resource_type,COALESCE(resource_id,''),COALESCE(detail,''),created_at FROM audit_logs WHERE "+clause+" ORDER BY id DESC LIMIT ? OFFSET ?", args...)
	if e != nil {
		return nil, 0, e
	}
	defer rows.Close()
	var out []model.AuditLog
	for rows.Next() {
		var a model.AuditLog
		var created string
		if e := rows.Scan(&a.ID, &a.Actor, &a.Action, &a.ResourceType, &a.ResourceID, &a.Detail, &created); e != nil {
			return nil, 0, e
		}
		a.CreatedAt = parseTime(created)
		out = append(out, a)
	}
	return out, total, rows.Err()
}
