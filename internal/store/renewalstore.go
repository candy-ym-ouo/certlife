package store

import (
	"certlife/internal/model"
	"context"
	"database/sql"
	"strings"
	"time"
)

type RenewalFilter struct {
	Status        string
	CertificateID int64
	Page, Size    int
}
type RenewalStore struct{ db *DB }

func NewRenewalStore(db *DB) *RenewalStore { return &RenewalStore{db} }
func (s *RenewalStore) Create(ctx context.Context, r *model.Renewal) error {
	if r.Attempt == 0 {
		r.Attempt = 1
	}
	if r.Status == "" {
		r.Status = model.RenewalPending
	}
	r.CreatedAt = time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `INSERT INTO renewals(certificate_id,triggered_by,status,old_serial,new_serial,attempt,error,started_at,finished_at,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, r.CertificateID, r.TriggeredBy, r.Status, r.OldSerial, r.NewSerial, r.Attempt, r.Error, formatTimePtr(r.StartedAt), formatTimePtr(r.FinishedAt), formatTime(r.CreatedAt))
	if err != nil {
		return err
	}
	r.ID, err = result.LastInsertId()
	return err
}
func (s *RenewalStore) Get(ctx context.Context, id int64) (*model.Renewal, error) {
	return scanRenewal(s.db.QueryRowContext(ctx, `SELECT id,certificate_id,triggered_by,status,COALESCE(old_serial,''),COALESCE(new_serial,''),attempt,COALESCE(error,''),COALESCE(started_at,''),COALESCE(finished_at,''),created_at FROM renewals WHERE id=?`, id))
}
func (s *RenewalStore) ActiveForCert(ctx context.Context, certID int64) (*model.Renewal, error) {
	r, e := scanRenewal(s.db.QueryRowContext(ctx, `SELECT id,certificate_id,triggered_by,status,COALESCE(old_serial,''),COALESCE(new_serial,''),attempt,COALESCE(error,''),COALESCE(started_at,''),COALESCE(finished_at,''),created_at FROM renewals WHERE certificate_id=? AND status IN ('pending','issuing','issued','deploying','verifying') ORDER BY id DESC LIMIT 1`, certID))
	if e == sql.ErrNoRows {
		return nil, nil
	}
	return r, e
}
func (s *RenewalStore) Update(ctx context.Context, r *model.Renewal) error {
	res, e := s.db.ExecContext(ctx, `UPDATE renewals SET status=?,new_serial=?,attempt=?,error=?,started_at=?,finished_at=? WHERE id=?`, r.Status, r.NewSerial, r.Attempt, r.Error, formatTimePtr(r.StartedAt), formatTimePtr(r.FinishedAt), r.ID)
	if e != nil {
		return e
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
func (s *RenewalStore) List(ctx context.Context, f RenewalFilter) ([]model.Renewal, int, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.Size < 1 {
		f.Size = 20
	}
	if f.Size > 100 {
		f.Size = 100
	}
	where := []string{"1=1"}
	args := []any{}
	if f.Status != "" {
		where = append(where, "status=?")
		args = append(args, f.Status)
	}
	if f.CertificateID > 0 {
		where = append(where, "certificate_id=?")
		args = append(args, f.CertificateID)
	}
	clause := strings.Join(where, " AND ")
	var total int
	if e := s.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM renewals WHERE "+clause, args...).Scan(&total); e != nil {
		return nil, 0, e
	}
	args = append(args, f.Size, (f.Page-1)*f.Size)
	rows, e := s.db.QueryContext(ctx, "SELECT id,certificate_id,triggered_by,status,COALESCE(old_serial,''),COALESCE(new_serial,''),attempt,COALESCE(error,''),COALESCE(started_at,''),COALESCE(finished_at,''),created_at FROM renewals WHERE "+clause+" ORDER BY id DESC LIMIT ? OFFSET ?", args...)
	if e != nil {
		return nil, 0, e
	}
	defer rows.Close()
	out := []model.Renewal{}
	for rows.Next() {
		r, e := scanRenewal(rows)
		if e != nil {
			return nil, 0, e
		}
		out = append(out, *r)
	}
	return out, total, rows.Err()
}
func (s *RenewalStore) All(ctx context.Context, f RenewalFilter) ([]model.Renewal, error) {
	out := []model.Renewal{}
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
func (s *RenewalStore) Active(ctx context.Context) ([]model.Renewal, error) {
	items, e := s.All(ctx, RenewalFilter{})
	if e != nil {
		return nil, e
	}
	out := items[:0]
	for _, r := range items {
		if r.Active() {
			out = append(out, r)
		}
	}
	return out, nil
}
func (s *RenewalStore) FailedRetryable(ctx context.Context) ([]model.Renewal, error) {
	items, e := s.All(ctx, RenewalFilter{Status: string(model.RenewalFailed)})
	if e != nil {
		return nil, e
	}
	out := items[:0]
	for _, r := range items {
		if r.Attempt < 3 {
			out = append(out, r)
		}
	}
	return out, nil
}
func scanRenewal(row scanner) (*model.Renewal, error) {
	var r model.Renewal
	var started, finished, created string
	if e := row.Scan(&r.ID, &r.CertificateID, &r.TriggeredBy, &r.Status, &r.OldSerial, &r.NewSerial, &r.Attempt, &r.Error, &started, &finished, &created); e != nil {
		return nil, e
	}
	r.StartedAt = parseTimePtr(started)
	r.FinishedAt = parseTimePtr(finished)
	r.CreatedAt = parseTime(created)
	return &r, nil
}
