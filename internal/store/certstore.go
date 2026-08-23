package store

import (
	"certlife/internal/model"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type CertificateFilter struct {
	Query, Status, Environment, Tag string
	Page, Size                      int
}
type CertStore struct {
	db    *DB
	cache []model.Certificate
}

func NewCertStore(db *DB) *CertStore { return &CertStore{db: db} }
func (s *CertStore) Create(ctx context.Context, c *model.Certificate) error {
	now := time.Now().UTC()
	c.CreatedAt, c.UpdatedAt = now, now
	result, err := s.db.ExecContext(ctx, `INSERT INTO certificates(name,domain,sans,issuer,serial_number,algorithm,key_bits,valid_from,valid_until,owner,environment,tags,contacts,notify_threshold_days,auto_renew,renew_threshold_days,deploy_target_ids,status,days_to_expire,last_checked_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, certArgs(c)...)
	if err != nil {
		return err
	}
	c.ID, err = result.LastInsertId()
	return err
}
func (s *CertStore) Update(ctx context.Context, c *model.Certificate) error {
	c.UpdatedAt = time.Now().UTC()
	args := certArgs(c)
	args = append(args, c.ID)
	result, err := s.db.ExecContext(ctx, `UPDATE certificates SET name=?,domain=?,sans=?,issuer=?,serial_number=?,algorithm=?,key_bits=?,valid_from=?,valid_until=?,owner=?,environment=?,tags=?,contacts=?,notify_threshold_days=?,auto_renew=?,renew_threshold_days=?,deploy_target_ids=?,status=?,days_to_expire=?,last_checked_at=?,created_at=?,updated_at=? WHERE id=? AND deleted_at IS NULL`, args...)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
func certArgs(c *model.Certificate) []any {
	return []any{c.Name, c.Domain, toJSON(c.SANs), c.Issuer, c.SerialNumber, c.Algorithm, c.KeyBits, formatTime(c.ValidFrom), formatTime(c.ValidUntil), c.Owner, c.Environment, toJSON(c.Tags), toJSON(c.Contacts), c.NotifyThresholdDays, boolInt(c.AutoRenew), c.RenewThresholdDays, toJSON(c.DeployTargetIDs), c.Status, c.DaysToExpire, formatTimePtr(c.LastCheckedAt), formatTime(c.CreatedAt), formatTime(c.UpdatedAt)}
}
func (s *CertStore) Get(ctx context.Context, id int64) (*model.Certificate, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id,name,domain,sans,issuer,serial_number,algorithm,key_bits,valid_from,valid_until,owner,environment,tags,contacts,notify_threshold_days,auto_renew,renew_threshold_days,deploy_target_ids,status,days_to_expire,COALESCE(last_checked_at,''),created_at,updated_at,COALESCE(deleted_at,'') FROM certificates WHERE id=? AND deleted_at IS NULL`, id)
	return scanCertificate(row)
}
func (s *CertStore) List(ctx context.Context, f CertificateFilter) ([]model.Certificate, int, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.Size < 1 {
		f.Size = 20
	}
	if f.Size > 100 {
		f.Size = 100
	}
	where := []string{"deleted_at IS NULL"}
	args := []any{}
	if f.Query != "" {
		where = append(where, "(name LIKE ? OR domain LIKE ? OR issuer LIKE ?)")
		q := "%" + f.Query + "%"
		args = append(args, q, q, q)
	}
	if f.Status != "" {
		where = append(where, "status=?")
		args = append(args, f.Status)
	}
	if f.Environment != "" {
		where = append(where, "environment=?")
		args = append(args, f.Environment)
	}
	if f.Tag != "" {
		where = append(where, "tags LIKE ?")
		args = append(args, "%\""+f.Tag+"\"%")
	}
	clause := strings.Join(where, " AND ")
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM certificates WHERE "+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	query := "SELECT id,name,domain,sans,issuer,serial_number,algorithm,key_bits,valid_from,valid_until,owner,environment,tags,contacts,notify_threshold_days,auto_renew,renew_threshold_days,deploy_target_ids,status,days_to_expire,COALESCE(last_checked_at,''),created_at,updated_at,COALESCE(deleted_at,'') FROM certificates WHERE " + clause + " ORDER BY id DESC LIMIT ? OFFSET ?"
	args = append(args, f.Size, (f.Page-1)*f.Size)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []model.Certificate{}
	for rows.Next() {
		c, err := scanCertificate(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *c)
	}
	s.cache = append(s.cache[:0], out...)
	return s.cache, total, rows.Err()
}
func (s *CertStore) All(ctx context.Context) ([]model.Certificate, error) {
	out := []model.Certificate{}
	for page := 1; ; page++ {
		items, total, e := s.List(ctx, CertificateFilter{Page: page, Size: 100})
		if e != nil {
			return nil, e
		}
		out = append(out, items...)
		if len(out) >= total {
			return out, nil
		}
	}
}
func (s *CertStore) Delete(ctx context.Context, id int64) error {
	now := formatTime(time.Now().UTC())
	r, e := s.db.ExecContext(ctx, `UPDATE certificates SET deleted_at=?,updated_at=? WHERE id=? AND deleted_at IS NULL`, now, now, id)
	if e != nil {
		return e
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
func (s *CertStore) DueForRenewal(ctx context.Context) ([]model.Certificate, error) {
	rows, e := s.db.QueryContext(ctx, `SELECT id,name,domain,sans,issuer,serial_number,algorithm,key_bits,valid_from,valid_until,owner,environment,tags,contacts,notify_threshold_days,auto_renew,renew_threshold_days,deploy_target_ids,status,days_to_expire,COALESCE(last_checked_at,''),created_at,updated_at,COALESCE(deleted_at,'') FROM certificates c WHERE deleted_at IS NULL AND auto_renew=1 AND days_to_expire<=renew_threshold_days AND NOT EXISTS(SELECT 1 FROM renewals r WHERE r.certificate_id=c.id AND r.status IN ('pending','issuing','issued','deploying','verifying'))`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []model.Certificate
	for rows.Next() {
		c, e := scanCertificate(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

type scanner interface{ Scan(...any) error }

func scanCertificate(row scanner) (*model.Certificate, error) {
	var c model.Certificate
	var sans, tags, contacts, targets, from, until, last, created, updated, deleted string
	var auto int
	err := row.Scan(&c.ID, &c.Name, &c.Domain, &sans, &c.Issuer, &c.SerialNumber, &c.Algorithm, &c.KeyBits, &from, &until, &c.Owner, &c.Environment, &tags, &contacts, &c.NotifyThresholdDays, &auto, &c.RenewThresholdDays, &targets, &c.Status, &c.DaysToExpire, &last, &created, &updated, &deleted)
	if err != nil {
		return nil, err
	}
	c.AutoRenew = auto == 1
	parseJSON(sans, &c.SANs)
	parseJSON(tags, &c.Tags)
	parseJSON(contacts, &c.Contacts)
	parseJSON(targets, &c.DeployTargetIDs)
	c.ValidFrom = parseTime(from)
	c.ValidUntil = parseTime(until)
	c.CreatedAt = parseTime(created)
	c.UpdatedAt = parseTime(updated)
	c.LastCheckedAt = parseTimePtr(last)
	c.DeletedAt = parseTimePtr(deleted)
	return &c, nil
}
func toJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
func parseJSON(s string, v any) {
	if s != "" {
		_ = json.Unmarshal([]byte(s), v)
	}
}
func formatTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }
func formatTimePtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return formatTime(*t)
}
func parseTime(v string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, v)
	if t.IsZero() {
		t, _ = time.Parse("2006-01-02 15:04:05", v)
	}
	return t.UTC()
}
func parseTimePtr(v string) *time.Time {
	if v == "" {
		return nil
	}
	t := parseTime(v)
	return &t
}
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

var _ = errors.Is
var _ = fmt.Sprintf
