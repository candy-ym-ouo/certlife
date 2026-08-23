package api
import (
	"net/http"
	"strconv"
	"certlife/internal/store"
)
func (s *Server) triggerRenewal(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil { writeError(w, 400, 40001, "invalid id"); return }
	body := struct {
		Force bool `json:"force"`
	}{}
	if e = decodeOptional(r, &body); e != nil { writeError(w, 400, 40001, e.Error()); return }
	renewal, e := s.renewals.Trigger(r.Context(), id, body.Force, "api")
	if e != nil { handleError(w, e); return }
	writeOK(w, 202, renewal)
}
func (s *Server) certificateRenewals(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil { writeError(w, 400, 40001, "invalid id"); return }
	page, size := pageSize(r)
	items, total, e := s.renewals.List(r.Context(), store.RenewalFilter{CertificateID: id, Page: page, Size: size})
	if e != nil { handleError(w, e); return }
	writeList(w, items, total, page, size)
}
func (s *Server) listRenewals(w http.ResponseWriter, r *http.Request) {
	page, size := pageSize(r)
	certID, _ := strconv.ParseInt(r.URL.Query().Get("certificate_id"), 10, 64)
	items, total, e := s.renewals.List(r.Context(), store.RenewalFilter{Status: r.URL.Query().Get("status"), CertificateID: certID, Page: page, Size: size})
	if e != nil { handleError(w, e); return }
	writeList(w, items, total, page, size)
}
func (s *Server) retryRenewal(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil { writeError(w, 400, 40001, "invalid id"); return }
	renewal, e := s.renewals.Retry(r.Context(), id)
	if e != nil { handleError(w, e); return }
	writeOK(w, 202, renewal)
}
