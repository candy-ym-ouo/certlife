package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"certlife/internal/config"
	"certlife/internal/model"
	"certlife/internal/store"
)

type countingIssuer struct{ calls atomic.Int32 }

func (i *countingIssuer) Issue(context.Context, model.Certificate) (string, error) {
	i.calls.Add(1)
	time.Sleep(25 * time.Millisecond)
	return "NEW-SERIAL", nil
}

func renewalFixture(t *testing.T, issuer Issuer) (*RenewalService, *model.Certificate) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	certStore := store.NewCertStore(db)
	notificationStore := store.NewNotificationStore(db)
	certService := NewCertService(certStore, notificationStore)
	deployService := NewDeployService(store.NewDeploymentStore(db), t.TempDir(), false)
	notifyService := NewNotifyService(notificationStore, config.NotifyConfig{})
	service := NewRenewalService(store.NewRenewalStore(db), certService, deployService, notifyService, issuer)
	now := time.Now().UTC()
	cert, err := certService.Register(context.Background(), &model.Certificate{Name: "test", Domain: "test.example", Issuer: "Test CA", SerialNumber: "OLD", Algorithm: "ECDSA", KeyBits: 256, ValidFrom: now.Add(-time.Hour), ValidUntil: now.Add(30 * 24 * time.Hour), Environment: "prod"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	return service, cert
}

func TestConcurrentTriggerReturnsConflict(t *testing.T) {
	svc, cert := renewalFixture(t, MockIssuer{})
	ctx := context.Background()
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { _, err := svc.Trigger(ctx, cert.ID, true, "test"); errs <- err }()
	}
	var success, conflicts int
	for i := 0; i < 2; i++ {
		err := <-errs
		if err == nil {
			success++
		} else if errors.Is(err, ErrConflict) {
			conflicts++
		} else {
			t.Fatalf("unexpected trigger error: %v", err)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatalf("success=%d conflicts=%d, want 1/1", success, conflicts)
	}
}

func TestConcurrentPipelineIssuesOnce(t *testing.T) {
	issuer := &countingIssuer{}
	svc, cert := renewalFixture(t, issuer)
	renewal, err := svc.Trigger(context.Background(), cert.ID, true, "test")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- svc.RunPipeline(context.Background(), renewal.ID) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := issuer.calls.Load(); got != 1 {
		t.Fatalf("issuer calls = %d, want 1", got)
	}
}

func TestExpiredCertificateStaticRenewalCompletes(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	targets := store.NewDeploymentStore(db)
	target := model.DeploymentTarget{Name: "static", Host: "localhost", Method: "static", Enabled: true}
	if err = targets.CreateTarget(ctx, &target); err != nil {
		t.Fatal(err)
	}
	certStore, notificationStore := store.NewCertStore(db), store.NewNotificationStore(db)
	certService := NewCertService(certStore, notificationStore)
	dir := t.TempDir()
	svc := NewRenewalService(store.NewRenewalStore(db), certService, NewDeployService(targets, dir, true), NewNotifyService(notificationStore, config.NotifyConfig{}), MockIssuer{})
	now := time.Now().UTC()
	cert, err := certService.Register(ctx, &model.Certificate{Name: "expired", Domain: "expired.example", Issuer: "Test CA", SerialNumber: "OLD", Algorithm: "ECDSA", KeyBits: 256, ValidFrom: now.Add(-48 * time.Hour), ValidUntil: now.Add(-24 * time.Hour), Environment: "prod", DeployTargetIDs: []int64{target.ID}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	r, err := svc.Trigger(ctx, cert.ID, true, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.RunPipeline(ctx, r.ID); err != nil {
		t.Fatalf("expired certificate renewal failed: %v", err)
	}
	r, err = svc.Get(ctx, r.ID)
	if err != nil || r.Status != model.RenewalCompleted {
		t.Fatalf("renewal=%#v err=%v", r, err)
	}
}

func TestSendPendingCountsPersistenceFailure(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer server.Close()
	notifications := store.NewNotificationStore(db)
	n := model.Notification{EventType: model.EventRenewalFailed, Channel: "webhook", Recipient: server.URL, Subject: "test", Body: "test"}
	if inserted, err := notifications.Insert(ctx, &n); err != nil || !inserted {
		t.Fatalf("inserted=%v err=%v", inserted, err)
	}
	if _, err = db.Exec(`CREATE TRIGGER reject_notification_update BEFORE UPDATE ON notifications BEGIN SELECT RAISE(ABORT,'blocked'); END`); err != nil {
		t.Fatal(err)
	}
	sent, failed := NewNotifyService(notifications, config.NotifyConfig{}).SendPending(ctx)
	if sent != 0 || failed != 1 {
		t.Fatalf("sent=%d failed=%d, want 0/1", sent, failed)
	}
}

func TestPipelineRecoversFromDeployingState(t *testing.T) {
	svc, cert := renewalFixture(t, MockIssuer{})
	r, err := svc.Trigger(context.Background(), cert.ID, true, "test")
	if err != nil {
		t.Fatal(err)
	}
	r.Status, r.NewSerial = model.RenewalDeploying, "RECOVERED"
	if err = svc.renewals.Update(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if err = svc.RunPipeline(context.Background(), r.ID); err != nil {
		t.Fatal(err)
	}
	r, err = svc.Get(context.Background(), r.ID)
	if err != nil || r.Status != model.RenewalCompleted {
		t.Fatalf("renewal=%#v err=%v", r, err)
	}
}

func TestStaticVerificationFailureRestoresPreviousFile(t *testing.T) {
	svc, cert := renewalFixture(t, MockIssuer{})
	svc.deploy.rollback = true
	ctx := context.Background()
	cert.DeployTargetIDs = []int64{1}
	if _, err := svc.certs.Update(ctx, cert.ID, cert, "test"); err != nil {
		t.Fatal(err)
	}
	r, err := svc.Trigger(ctx, cert.ID, true, "test")
	if err != nil {
		t.Fatal(err)
	}
	r.NewSerial = "NEW"
	path := filepath.Join(svc.deploy.dir, "target-1.json")
	old := []byte(`{"serial_number":"OLD"}`)
	if err = os.WriteFile(path, old, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.deploy.Deploy(ctx, *r, *cert); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte(`{"serial_number":"WRONG"}`), 0644); err != nil {
		t.Fatal(err)
	}
	ok, err := svc.deploy.Verify(ctx, *r, *cert)
	if ok || err == nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(old) {
		t.Fatalf("restored=%q err=%v", got, err)
	}
}

func TestCertificateUpdateFailureDoesNotCompleteRenewal(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	certStore, notificationStore := store.NewCertStore(db), store.NewNotificationStore(db)
	certService := NewCertService(certStore, notificationStore)
	svc := NewRenewalService(store.NewRenewalStore(db), certService, NewDeployService(store.NewDeploymentStore(db), t.TempDir(), false), NewNotifyService(notificationStore, config.NotifyConfig{}), MockIssuer{})
	now := time.Now().UTC()
	cert, err := certService.Register(ctx, &model.Certificate{Name: "test", Domain: "test.example", Issuer: "Test CA", SerialNumber: "OLD", Algorithm: "ECDSA", KeyBits: 256, ValidFrom: now.Add(-time.Hour), ValidUntil: now.Add(24 * time.Hour), Environment: "prod"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	r, err := svc.Trigger(ctx, cert.ID, true, "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TRIGGER reject_certificate_update BEFORE UPDATE ON certificates BEGIN SELECT RAISE(ABORT,'blocked'); END`); err != nil {
		t.Fatal(err)
	}
	if err = svc.RunPipeline(ctx, r.ID); err == nil {
		t.Fatal("pipeline unexpectedly succeeded")
	}
	r, err = svc.Get(ctx, r.ID)
	if err != nil || r.Status != model.RenewalVerifying {
		t.Fatalf("renewal=%#v err=%v", r, err)
	}
	if _, err = db.Exec(`DROP TRIGGER reject_certificate_update`); err != nil {
		t.Fatal(err)
	}
	if err = svc.RunPipeline(ctx, r.ID); err != nil {
		t.Fatal(err)
	}
	r, err = svc.Get(ctx, r.ID)
	if err != nil || r.Status != model.RenewalCompleted {
		t.Fatalf("renewal=%#v err=%v", r, err)
	}
}
