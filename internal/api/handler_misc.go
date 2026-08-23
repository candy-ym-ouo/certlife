package api
import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"certlife/internal/model"
	"certlife/internal/service"
	"certlife/internal/store"
	"certlife/internal/task"
)
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	dbStatus := "ok"
	if e := s.db.PingContext(r.Context()); e != nil { dbStatus = "error" }
	writeOK(w, 200, map[string]any{"status": "up", "db": dbStatus, "tasks_running": s.scheduler.RunningCount()})
}
func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	stats, e := s.stats.Dashboard(r.Context())
	if e != nil { handleError(w, e); return }
	writeOK(w, 200, stats)
}
func (s *Server) certificateNotifications(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil { writeError(w, 400, 40001, "invalid id"); return }
	page, size := pageSize(r)
	items, total, e := s.notify.Notifications(r.Context(), store.NotificationFilter{CertificateID: id, Page: page, Size: size})
	if e != nil { handleError(w, e); return }
	writeList(w, items, total, page, size)
}
func (s *Server) listNotifications(w http.ResponseWriter, r *http.Request) {
	page, size := pageSize(r)
	certID, _ := strconv.ParseInt(r.URL.Query().Get("certificate_id"), 10, 64)
	items, total, e := s.notify.Notifications(r.Context(), store.NotificationFilter{Status: r.URL.Query().Get("status"), EventType: r.URL.Query().Get("event_type"), CertificateID: certID, Page: page, Size: size})
	if e != nil { handleError(w, e); return }
	writeList(w, items, total, page, size)
}
func (s *Server) listRules(w http.ResponseWriter, r *http.Request) {
	rules, e := s.notify.Rules(r.Context())
	if e != nil { handleError(w, e); return }
	writeOK(w, 200, rules)
}
func (s *Server) createRule(w http.ResponseWriter, r *http.Request) {
	rule := model.NotificationRule{Enabled: true}
	if e := decode(r, &rule); e != nil { writeError(w, 400, 40001, e.Error()); return }
	if e := s.notify.CreateRule(r.Context(), &rule); e != nil { writeError(w, 400, 40001, e.Error()); return }
	s.certs.Audit(r.Context(), "api", "rule.create", "rule", rule.ID, nil)
	writeOK(w, 201, rule)
}
func (s *Server) updateRule(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil { writeError(w, 400, 40001, "invalid id"); return }
	rule := model.NotificationRule{Enabled: true}
	if e = decode(r, &rule); e != nil { writeError(w, 400, 40001, e.Error()); return }
	rule.ID = id
	if e = s.notify.UpdateRule(r.Context(), &rule); e != nil {
		if errors.Is(e, sql.ErrNoRows) { e = service.ErrNotFound }
		handleError(w, e)
		return
	}
	s.certs.Audit(r.Context(), "api", "rule.update", "rule", id, nil)
	writeOK(w, 200, rule)
}
func (s *Server) deleteRule(w http.ResponseWriter, r *http.Request) {
	id, e := pathID(r)
	if e != nil { writeError(w, 400, 40001, "invalid id"); return }
	if e = s.notify.DeleteRule(r.Context(), id); e != nil {
		if errors.Is(e, sql.ErrNoRows) { e = service.ErrNotFound }
		handleError(w, e)
		return
	}
	s.certs.Audit(r.Context(), "api", "rule.delete", "rule", id, nil)
	w.WriteHeader(204)
}
func (s *Server) listAudit(w http.ResponseWriter, r *http.Request) {
	page, size := pageSize(r)
	q := r.URL.Query()
	items, total, e := s.notify.Audit(r.Context(), store.AuditFilter{Action: q.Get("action"), ResourceType: q.Get("resource_type"), ResourceID: q.Get("resource_id"), Page: page, Size: size})
	if e != nil { handleError(w, e); return }
	writeList(w, items, total, page, size)
}
func (s *Server) listTasks(w http.ResponseWriter, r *http.Request) { writeOK(w, 200, s.scheduler.List()) }
func (s *Server) runTask(w http.ResponseWriter, r *http.Request) {
	var body struct { Name string `json:"name"` }
	if e := decode(r, &body); e != nil { writeError(w, 400, 40001, e.Error()); return }
	result, e := s.scheduler.TriggerNow(r.Context(), body.Name)
	if errors.Is(e, task.ErrTaskNotFound) { writeError(w, 404, 40401, e.Error()); return }
	if errors.Is(e, task.ErrTaskRunning) { writeError(w, 409, 40903, e.Error()); return }
	if e != nil { handleError(w, e); return }
	s.certs.Audit(r.Context(), "api", "task.run", "task", 0, map[string]any{"name": body.Name, "ok": result.OK})
	writeOK(w, 200, map[string]any{"name": body.Name, "result": map[bool]string{true: "ok", false: "failed"}[result.OK], "detail": result.Detail, "error": result.Error})
}
