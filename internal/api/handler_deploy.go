package api
import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"certlife/internal/model"
	"certlife/internal/service"
	"certlife/internal/store"
)
func (s *Server) certificateDeployments(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil { writeError(w, 400, 40001, "invalid id"); return }
	page, size := pageSize(r)
	items, total, e := s.deploy.Deployments(r.Context(), store.DeploymentFilter{CertificateID: id, Page: page, Size: size})
	if e != nil { handleError(w, e); return }
	writeList(w, items, total, page, size)
}
func (s *Server) listDeployments(w http.ResponseWriter, r *http.Request) {
	page, size := pageSize(r)
	renewalID, _ := strconv.ParseInt(r.URL.Query().Get("renewal_id"), 10, 64)
	items, total, e := s.deploy.Deployments(r.Context(), store.DeploymentFilter{Status: r.URL.Query().Get("status"), RenewalID: renewalID, Page: page, Size: size})
	if e != nil { handleError(w, e); return }
	writeList(w, items, total, page, size)
}
func (s *Server) listTargets(w http.ResponseWriter, r *http.Request) {
	items, e := s.deploy.Targets(r.Context())
	if e != nil { handleError(w, e); return }
	writeOK(w, 200, items)
}
func (s *Server) createTarget(w http.ResponseWriter, r *http.Request) {
	target := model.DeploymentTarget{VerifyTLS: true, Enabled: true}
	if e := decode(r, &target); e != nil { writeError(w, 400, 40001, e.Error()); return }
	if e := s.deploy.CreateTarget(r.Context(), &target); e != nil { writeError(w, 400, 40001, e.Error()); return }
	s.certs.Audit(r.Context(), "api", "target.create", "target", target.ID, nil)
	writeOK(w, 201, target)
}
func (s *Server) updateTarget(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil { writeError(w, 400, 40001, "invalid id"); return }
	target := model.DeploymentTarget{VerifyTLS: true, Enabled: true}
	if e = decode(r, &target); e != nil { writeError(w, 400, 40001, e.Error()); return }
	target.ID = id
	if e = s.deploy.UpdateTarget(r.Context(), &target); e != nil {
		if errors.Is(e, sql.ErrNoRows) { e = service.ErrNotFound }
		handleError(w, e)
		return
	}
	s.certs.Audit(r.Context(), "api", "target.update", "target", id, nil)
	writeOK(w, 200, target)
}
func (s *Server) deleteTarget(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil { writeError(w, 400, 40001, "invalid id"); return }
	if e = s.deploy.DeleteTarget(r.Context(), id); e != nil {
		if errors.Is(e, sql.ErrNoRows) { e = service.ErrNotFound }
		handleError(w, e)
		return
	}
	s.certs.Audit(r.Context(), "api", "target.delete", "target", id, nil)
	w.WriteHeader(204)
}
