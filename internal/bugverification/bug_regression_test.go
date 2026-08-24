package bugverification

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"certlife/internal/model"
	"certlife/internal/service"
	"certlife/internal/store"
)

func TestBug006_StaticVerificationReleasesEachFile(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	targets := store.NewDeploymentStore(db)
	certStore := store.NewCertStore(db)
	cert := model.Certificate{Domain: "deploy.example", SerialNumber: "OLD", ValidFrom: time.Now().Add(-time.Hour), ValidUntil: time.Now().Add(time.Hour), Environment: "prod"}
	if err := certStore.Create(ctx, &cert); err != nil {
		t.Fatal(err)
	}
	renewal := model.Renewal{CertificateID: cert.ID, NewSerial: "NEW"}
	if err := store.NewRenewalStore(db).Create(ctx, &renewal); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	const targetCount = 256
	for i := 0; i < targetCount; i++ {
		target := model.DeploymentTarget{Name: fmt.Sprintf("static-%d", i), Host: "localhost", Method: "static", Enabled: true}
		if err := targets.CreateTarget(ctx, &target); err != nil {
			t.Fatal(err)
		}
		payload, err := json.Marshal(map[string]any{"certificate_id": cert.ID, "domain": cert.Domain, "sans": []string(nil), "serial_number": renewal.NewSerial})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("target-%d.json", target.ID)), payload, 0644); err != nil {
			t.Fatal(err)
		}
		deployment := model.Deployment{RenewalID: renewal.ID, CertificateID: cert.ID, TargetID: target.ID, Status: "deploying"}
		if err := targets.Create(ctx, &deployment); err != nil {
			t.Fatal(err)
		}
	}
	deploy := service.NewDeployService(targets, dir, false)
	var peak atomic.Int32
	result := make(chan struct {
		ok  bool
		err error
	}, 1)
	go func() {
		ok, err := deploy.Verify(ctx, renewal, cert)
		result <- struct {
			ok  bool
			err error
		}{ok: ok, err: err}
	}()
	var outcome struct {
		ok  bool
		err error
	}
	for {
		select {
		case outcome = <-result:
			goto verified
		default:
			entries, _ := filepath.Glob("/dev/fd/*")
			current := int32(len(entries))
			for {
				old := peak.Load()
				if current <= old || peak.CompareAndSwap(old, current) {
					break
				}
			}
			runtime.Gosched()
		}
	}
verified:
	ok, verifyErr := outcome.ok, outcome.err
	if verifyErr != nil || !ok {
		t.Fatalf("Verify failed: ok=%v err=%v", ok, verifyErr)
	}
	if got := peak.Load(); got > int32(targetCount/2) {
		t.Fatalf("peak open descriptors=%d, want below %d", got, targetCount/2)
	}
}
