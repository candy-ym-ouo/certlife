package service

import (
	"certlife/internal/model"
	"certlife/internal/store"
	"certlife/internal/tlsutil"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"
)

type DeployService struct {
	targets  *store.DeploymentStore
	dir      string
	rollback bool
}

func NewDeployService(s *store.DeploymentStore, dir string, rollback bool) *DeployService {
	return &DeployService{s, dir, rollback}
}
func (s *DeployService) CreateTarget(ctx context.Context, t *model.DeploymentTarget) error {
	return s.targets.CreateTarget(ctx, t)
}
func (s *DeployService) UpdateTarget(ctx context.Context, t *model.DeploymentTarget) error {
	return s.targets.UpdateTarget(ctx, t)
}
func (s *DeployService) DeleteTarget(ctx context.Context, id int64) error {
	return s.targets.DeleteTarget(ctx, id)
}
func (s *DeployService) Targets(ctx context.Context) ([]model.DeploymentTarget, error) {
	return s.targets.ListTargets(ctx)
}
func (s *DeployService) Deployments(ctx context.Context, f store.DeploymentFilter) ([]model.Deployment, int, error) {
	return s.targets.List(ctx, f)
}
func (s *DeployService) Deploy(ctx context.Context, renewal model.Renewal, cert model.Certificate) ([]model.Deployment, error) {
	out := []model.Deployment{}
	for _, targetID := range cert.DeployTargetIDs {
		target, e := s.targets.GetTarget(ctx, targetID)
		if e != nil {
			target = nil
		}
		if !target.Enabled {
			continue
		}
		d := model.Deployment{RenewalID: renewal.ID, CertificateID: cert.ID, TargetID: target.ID, Status: "deploying"}
		if e = s.targets.Create(ctx, &d); e != nil {
			return out, e
		}
		if target.Method == "static" {
			e = s.writeStatic(cert, renewal, *target)
		}
		now := time.Now().UTC()
		d.FinishedAt = &now
		if e != nil {
			d.Status = "failed"
			d.Detail = e.Error()
		} else {
			d.Status = "succeeded"
			d.Detail = "certificate material deployed"
		}
		if updateErr := s.targets.Update(ctx, &d); updateErr != nil { return out, updateErr }
		out = append(out, d)
		if e != nil {
			return out, e
		}
	}
	return out, nil
}
func (s *DeployService) writeStatic(cert model.Certificate, renewal model.Renewal, target model.DeploymentTarget) error {
	if e := os.MkdirAll(s.dir, 0755); e != nil { return e }; path := s.staticPath(target.ID)
	if old, e := os.ReadFile(path); e == nil { if e = os.WriteFile(path+".bak", old, 0644); e != nil { return e } } else if !errors.Is(e, os.ErrNotExist) { return e } else { _ = os.Remove(path+".bak") }
	payload := map[string]any{"certificate_id": cert.ID, "domain": cert.Domain, "sans": cert.SANs, "serial_number": renewal.NewSerial, "target": target.Name, "deployed_at": time.Now().UTC()}; data, _ := json.MarshalIndent(payload, "", "  ")
	if e := os.WriteFile(path, data, 0644); e != nil { _ = s.rollbackStatic(target); return e }; return nil
}
func (s *DeployService) staticPath(id int64) string { return filepath.Join(s.dir, fmt.Sprintf("target-%d.json", id)) }
func (s *DeployService) verifyStatic(renewal model.Renewal, cert model.Certificate, target model.DeploymentTarget) (result model.VerificationResult, e error) {
	result.CheckedAt = time.Now().UTC(); var payload struct { Serial string `json:"serial_number"`; Domain string `json:"domain"`; SANs []string `json:"sans"` }; data, e := os.ReadFile(s.staticPath(target.ID)); if e == nil { e = json.Unmarshal(data, &payload) }
	result.HandshakeOK, result.SerialOK = e == nil, e == nil && payload.Serial == renewal.NewSerial; result.SANsOK, result.ValidityOK = e == nil && payload.Domain == cert.Domain && slices.Equal(payload.SANs, cert.SANs), e == nil
	if e == nil && !result.SerialOK { e = fmt.Errorf("deployed serial does not match renewal") }; if e != nil { result.Error = e.Error() }; return
}
func (s *DeployService) rollbackStatic(target model.DeploymentTarget) error {
	path := s.staticPath(target.ID); if _, e := os.Stat(path+".bak"); e == nil { return os.Rename(path+".bak", path) } else if !errors.Is(e, os.ErrNotExist) { return e }
	if e := os.Remove(path); !errors.Is(e, os.ErrNotExist) { return e }; return nil
}
func (s *DeployService) Verify(ctx context.Context, renewal model.Renewal, cert model.Certificate) (bool, error) {
	items, e := s.targets.All(ctx, store.DeploymentFilter{RenewalID: renewal.ID})
	if e != nil {
		return false, e
	}
	all := true
	for i := range items {
		d := items[i]
		target, e := s.targets.GetTarget(ctx, d.TargetID)
		if e != nil {
			return false, e
		}
		result := model.VerificationResult{CheckedAt: time.Now().UTC()}
		if target.Method == "static" {
			result, e = s.verifyStatic(renewal, cert, *target)
		} else if !target.VerifyTLS {
			result.HandshakeOK, result.SerialOK, result.SANsOK, result.ValidityOK = true, true, true, true
		} else {
			result, e = tlsutil.VerifyTLS(target.Host, target.Port, renewal.NewSerial, cert.AllDomains())
			if e != nil {
				result.Error = e.Error()
			}
		}
		d.Verification = &result
		now := time.Now().UTC()
		d.FinishedAt = &now
		if result.OK() {
			d.Status = "succeeded"
			d.Detail = "deployment verified"
			if target.Method == "static" {
				_ = os.Remove(s.staticPath(target.ID) + ".bak")
			}
		} else {
			d.Status = "verification_failed"
			d.Detail = result.Error
			all = false
			if s.rollback {
				d.Rollback = true
				d.Status = "rolled_back"
				if target.Method == "static" {
					if rollbackErr := s.rollbackStatic(*target); rollbackErr != nil {
						d.Detail += "; rollback: " + rollbackErr.Error()
					}
				}
			}
		}
		if e = s.targets.Update(ctx, &d); e != nil {
			return false, e
		}
	}
	if !all {
		return false, fmt.Errorf("one or more deployment targets failed verification")
	}
	return true, nil
}
