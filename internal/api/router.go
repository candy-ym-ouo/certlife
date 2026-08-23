// Package api exposes the REST API and embedded browser interface.
package api
import (
	"crypto/subtle"
	"embed"
	"io/fs"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
	"certlife/internal/service"
	"certlife/internal/store"
	"certlife/internal/task"
)
//go:embed web/*
var webFiles embed.FS
type Server struct {
	apiKey    string
	db        *store.DB
	certs     *service.CertService
	renewals  *service.RenewalService
	deploy    *service.DeployService
	notify    *service.NotifyService
	stats     *service.StatsService
	scheduler *task.Scheduler
}
func New(apiKey string, db *store.DB, c *service.CertService, r *service.RenewalService, d *service.DeployService, n *service.NotifyService, s *service.StatsService, scheduler *task.Scheduler) http.Handler {
	server := &Server{apiKey, db, c, r, d, n, s, scheduler}
	mux := http.NewServeMux()
	server.routes(mux)
	return server.recover(server.accessLog(server.auth(mux)))
}
func (s *Server) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/health", s.health)
	mux.HandleFunc("GET /api/v1/stats", s.dashboard)
	mux.HandleFunc("POST /api/v1/certificates", s.createCertificate)
	mux.HandleFunc("GET /api/v1/certificates", s.listCertificates)
	mux.HandleFunc("GET /api/v1/certificates/{id}", s.getCertificate)
	mux.HandleFunc("PUT /api/v1/certificates/{id}", s.updateCertificate)
	mux.HandleFunc("DELETE /api/v1/certificates/{id}", s.deleteCertificate)
	mux.HandleFunc("POST /api/v1/certificates/{id}/renew", s.triggerRenewal)
	mux.HandleFunc("GET /api/v1/certificates/{id}/renewals", s.certificateRenewals)
	mux.HandleFunc("GET /api/v1/certificates/{id}/deployments", s.certificateDeployments)
	mux.HandleFunc("GET /api/v1/certificates/{id}/notifications", s.certificateNotifications)
	mux.HandleFunc("GET /api/v1/renewals", s.listRenewals)
	mux.HandleFunc("POST /api/v1/renewals/{id}/retry", s.retryRenewal)
	mux.HandleFunc("GET /api/v1/deployments", s.listDeployments)
	mux.HandleFunc("GET /api/v1/targets", s.listTargets)
	mux.HandleFunc("POST /api/v1/targets", s.createTarget)
	mux.HandleFunc("PUT /api/v1/targets/{id}", s.updateTarget)
	mux.HandleFunc("DELETE /api/v1/targets/{id}", s.deleteTarget)
	mux.HandleFunc("GET /api/v1/notifications", s.listNotifications)
	mux.HandleFunc("GET /api/v1/notification-rules", s.listRules)
	mux.HandleFunc("POST /api/v1/notification-rules", s.createRule)
	mux.HandleFunc("PUT /api/v1/notification-rules/{id}", s.updateRule)
	mux.HandleFunc("DELETE /api/v1/notification-rules/{id}", s.deleteRule)
	mux.HandleFunc("GET /api/v1/audit-logs", s.listAudit)
	mux.HandleFunc("GET /api/v1/tasks", s.listTasks)
	mux.HandleFunc("POST /api/v1/tasks/run", s.runTask)
	sub, _ := fs.Sub(webFiles, "web")
	mux.Handle("GET /web/", http.StripPrefix("/web/", http.FileServer(http.FS(sub))))
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/web/", http.StatusTemporaryRedirect)
	})
}
func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/health" || r.URL.Path == "/" || len(r.URL.Path) >= 5 && r.URL.Path[:5] == "/web/" { next.ServeHTTP(w, r); return }
		got := []byte(r.Header.Get("X-API-Key"))
		want := []byte(s.apiKey)
		if len(got) != len(want) || subtle.ConstantTimeCompare(got, want) != 1 { writeError(w, http.StatusUnauthorized, 40100, "invalid API key"); return }
		next.ServeHTTP(w, r)
	})
}
func (s *Server) recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if value := recover(); value != nil { slog.Error("request panic", "error", value, "stack", string(debug.Stack())); writeError(w, 500, 50000, "internal server error") }
		}()
		next.ServeHTTP(w, r)
	})
}
func (s *Server) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		rec := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(rec, r)
		slog.Info("http request", "method", r.Method, "path", r.URL.Path, "status", rec.status, "duration", time.Since(started))
	})
}
type statusWriter struct { http.ResponseWriter; status int }
func (w *statusWriter) WriteHeader(code int) { w.status = code; w.ResponseWriter.WriteHeader(code) }
