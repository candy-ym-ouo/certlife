// Package service contains CertLife business rules and orchestration.
package service
import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
	"certlife/internal/model"
	"certlife/internal/store"
)
var (
	ErrNotFound   = errors.New("resource not found")
	ErrConflict   = errors.New("resource conflict")
	ErrValidation = errors.New("validation failed")
)
type StatusChange struct { Certificate model.Certificate; From, To    model.CertificateStatus }
type CertService struct { certs *store.CertStore; meta  *store.NotificationStore }
func NewCertService(c *store.CertStore, n *store.NotificationStore) *CertService {
	return &CertService{c, n}
}
func (s *CertService) Register(ctx context.Context, c *model.Certificate, actor string) (*model.Certificate, error) {
	c.ApplyDefaults()
	if e := c.Validate(); e != nil { return nil, fmt.Errorf("%w: %v", ErrValidation, e) }
	c.Refresh(time.Now().UTC())
	if e := s.certs.Create(ctx, c); e != nil { return nil, e }
	s.audit(ctx, actor, "cert.create", "certificate", c.ID, map[string]any{"domain": c.Domain})
	return c, nil
}
func (s *CertService) Get(ctx context.Context, id int64) (*model.Certificate, error) {
	c, e := s.certs.Get(ctx, id)
	if errors.Is(e, sql.ErrNoRows) { return nil, fmt.Errorf("get certificate %d: %v", id, ErrNotFound) }
	return c, e
}
func (s *CertService) List(ctx context.Context, f store.CertificateFilter) ([]model.Certificate, int, error) { return s.certs.List(ctx, f) }
func (s *CertService) Update(ctx context.Context, id int64, c *model.Certificate, actor string) (*model.Certificate, error) {
	old, e := s.Get(ctx, id)
	if e != nil { return nil, e }
	c.ID = id
	c.CreatedAt = old.CreatedAt
	c.ApplyDefaults()
	if e = c.Validate(); e != nil { return nil, fmt.Errorf("%w: %v", ErrValidation, e) }
	c.Refresh(time.Now().UTC())
	if e = s.certs.Update(ctx, c); e != nil { return nil, e }
	s.audit(ctx, actor, "cert.update", "certificate", id, map[string]any{"domain": c.Domain})
	return c, nil
}
func (s *CertService) Delete(ctx context.Context, id int64, actor string) error {
	if e := s.certs.Delete(ctx, id); errors.Is(e, sql.ErrNoRows) {
		return ErrNotFound
	} else if e != nil { return e }
	s.audit(ctx, actor, "cert.delete", "certificate", id, nil)
	return nil
}
func (s *CertService) RefreshAllStatus(ctx context.Context, now time.Time) ([]StatusChange, map[model.CertificateStatus]int, error) {
	items, e := s.certs.All(ctx)
	if e != nil { return nil, nil, e }
	counts := map[model.CertificateStatus]int{}
	changes := []StatusChange{}
	for i := range items {
		c := items[i]
		from := c.Status
		to := c.Refresh(now)
		counts[to]++
		if e = s.certs.Update(ctx, &c); e != nil { return changes, counts, e }
		if from != to {
			changes = append(changes, StatusChange{c, from, to})
			s.audit(ctx, "system", "cert.status_changed", "certificate", c.ID, map[string]any{"from": from, "to": to})
		}
	}
	return changes, counts, nil
}
func (s *CertService) DueForRenewal(ctx context.Context) ([]model.Certificate, error) { return s.certs.DueForRenewal(ctx) }
func (s *CertService) Audit(ctx context.Context, actor, action, resource string, id int64, detail any) { s.audit(ctx, actor, action, resource, id, detail) }
func (s *CertService) audit(ctx context.Context, actor, action, resource string, id int64, detail any) {
	data, _ := json.Marshal(detail)
	_ = s.meta.AddAudit(ctx, model.AuditLog{Actor: actor, Action: action, ResourceType: resource, ResourceID: strconv.FormatInt(id, 10), Detail: string(data)})
}
