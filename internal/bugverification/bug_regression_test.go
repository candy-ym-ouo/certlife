package bugverification

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"testing"
	"time"

	"certlife/internal/model"
	"certlife/internal/service"
	"certlife/internal/store"
)

func TestBug007_VerifyStopsWhenContextIsCancelled(t *testing.T) {
	listener, accepted, stop := blackholeListener(t)
	defer stop()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	targets := store.NewDeploymentStore(db)
	target := model.DeploymentTarget{Name: "tls", Host: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port, Method: "tls", VerifyTLS: true, Enabled: true}
	if err := targets.CreateTarget(ctx, &target); err != nil {
		t.Fatal(err)
	}
	cert := model.Certificate{Domain: "deploy.example", SerialNumber: "OLD", ValidFrom: time.Now().Add(-time.Hour), ValidUntil: time.Now().Add(time.Hour), Environment: "prod"}
	if err := store.NewCertStore(db).Create(ctx, &cert); err != nil {
		t.Fatal(err)
	}
	renewal := model.Renewal{CertificateID: cert.ID, NewSerial: "NEW"}
	if err := store.NewRenewalStore(db).Create(ctx, &renewal); err != nil {
		t.Fatal(err)
	}
	deployment := model.Deployment{RenewalID: renewal.ID, CertificateID: cert.ID, TargetID: target.ID, Status: "deploying"}
	if err := targets.Create(ctx, &deployment); err != nil {
		t.Fatal(err)
	}
	svc := service.NewDeployService(targets, t.TempDir(), false)
	verifyCtx, cancel := context.WithCancel(ctx)
	result := make(chan error, 1)
	go func() {
		_, err := svc.Verify(verifyCtx, renewal, cert)
		result <- err
	}()
	select {
	case <-accepted:
	case <-time.After(time.Second):
		t.Fatal("TLS verifier did not connect")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err=%v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Verify did not stop after context cancellation")
	}
}

func blackholeListener(t *testing.T) (net.Listener, <-chan struct{}, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	accepted := make(chan struct{})
	stop := make(chan struct{})
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		close(accepted)
		<-stop
		_ = conn.Close()
	}()
	return listener, accepted, func() {
		close(stop)
		_ = listener.Close()
	}
}
