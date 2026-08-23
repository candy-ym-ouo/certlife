package store_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"certlife/internal/model"
	"certlife/internal/store"
)

func openDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func testCertificate(i int) model.Certificate {
	now := time.Now().UTC()
	return model.Certificate{Name: fmt.Sprintf("cert-%d", i), Domain: fmt.Sprintf("cert-%d.example", i), Issuer: "Test CA", SerialNumber: fmt.Sprintf("%04d", i), Algorithm: "ECDSA", KeyBits: 256, ValidFrom: now.Add(-time.Hour), ValidUntil: now.Add(30 * 24 * time.Hour), Environment: "prod", Status: model.CertificateValid}
}

func TestAllCertificatesIsNotTruncated(t *testing.T) {
	ctx := context.Background()
	certs := store.NewCertStore(openDB(t))
	for i := 0; i < 120; i++ {
		c := testCertificate(i)
		if err := certs.Create(ctx, &c); err != nil {
			t.Fatal(err)
		}
	}
	items, err := certs.All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 120 {
		t.Fatalf("All returned %d certificates, want 120", len(items))
	}
}

func TestDeleteTargetUsesExactJSONID(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	targets := store.NewDeploymentStore(db)
	var target10 model.DeploymentTarget
	for i := 2; i <= 10; i++ {
		target := model.DeploymentTarget{Name: fmt.Sprintf("target-%d", i), Host: "localhost", Method: "static", Enabled: true}
		if err := targets.CreateTarget(ctx, &target); err != nil {
			t.Fatal(err)
		}
		if i == 10 {
			target10 = target
		}
	}
	cert := testCertificate(1)
	cert.DeployTargetIDs = []int64{target10.ID}
	if err := store.NewCertStore(db).Create(ctx, &cert); err != nil {
		t.Fatal(err)
	}
	if err := targets.DeleteTarget(ctx, 1); err != nil {
		t.Fatalf("ID 1 was falsely matched inside ID %d: %v", target10.ID, err)
	}
	if err := targets.DeleteTarget(ctx, target10.ID); !errors.Is(err, store.ErrReferenced) {
		t.Fatalf("referenced target deletion error = %v", err)
	}
}

func TestOnlyOneActiveRenewalPerCertificate(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	cert := testCertificate(1)
	if err := store.NewCertStore(db).Create(ctx, &cert); err != nil {
		t.Fatal(err)
	}
	renewals := store.NewRenewalStore(db)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			errs <- renewals.Create(ctx, &model.Renewal{CertificateID: cert.ID, TriggeredBy: "test", Status: model.RenewalPending})
		}()
	}
	successes := 0
	for i := 0; i < 2; i++ {
		if err := <-errs; err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful active renewal inserts = %d, want 1", successes)
	}
}

func TestNotificationRetryBackoff(t *testing.T) {
	ctx := context.Background()
	notifications := store.NewNotificationStore(openDB(t))
	n := model.Notification{EventType: model.EventRenewalFailed, Channel: "email", Recipient: "ops@example.com", Subject: "test", Body: "test"}
	inserted, err := notifications.Insert(ctx, &n)
	if err != nil || !inserted {
		t.Fatalf("inserted=%v err=%v", inserted, err)
	}
	n.Status, n.RetryCount = "failed", 1
	now := time.Now().UTC()
	n.LastAttemptAt = &now
	if err := notifications.Update(ctx, &n); err != nil {
		t.Fatal(err)
	}
	items, err := notifications.Pending(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("notification retried before 1 minute: %d item(s)", len(items))
	}
	past := now.Add(-2 * time.Minute)
	n.LastAttemptAt = &past
	if err := notifications.Update(ctx, &n); err != nil {
		t.Fatal(err)
	}
	items, err = notifications.Pending(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != n.ID {
		t.Fatalf("eligible notification not returned: %#v", items)
	}
}

func TestMemoryDatabasesAreIsolated(t *testing.T) {
	ctx := context.Background()
	first, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	cert := testCertificate(99)
	if err = store.NewCertStore(first).Create(ctx, &cert); err != nil {
		t.Fatal(err)
	}
	items, err := store.NewCertStore(second).All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("second in-memory database saw %d certificate(s) from first", len(items))
	}
}

func TestNotificationDedupeUsesRolling24HoursAndSkipsDailySummary(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	notifications := store.NewNotificationStore(db)
	certID := int64(42)
	first := model.Notification{CertificateID: &certID, EventType: model.EventExpired, Channel: "email", Recipient: "ops@example.com", Subject: "test", Body: "test"}
	if inserted, err := notifications.Insert(ctx, &first); err != nil || !inserted {
		t.Fatalf("first inserted=%v err=%v", inserted, err)
	}
	old := time.Now().UTC().Add(-23*time.Hour - 59*time.Minute)
	if _, err := db.Exec(`UPDATE notifications SET created_at=? WHERE id=?`, old.Format(time.RFC3339Nano), first.ID); err != nil {
		t.Fatal(err)
	}
	second := first
	second.ID = 0
	if inserted, err := notifications.Insert(ctx, &second); err != nil || inserted {
		t.Fatalf("rolling duplicate inserted=%v err=%v", inserted, err)
	}
	if _, err := db.Exec(`UPDATE notifications SET created_at=? WHERE id=?`, time.Now().UTC().Add(-25*time.Hour).Format(time.RFC3339Nano), first.ID); err != nil {
		t.Fatal(err)
	}
	if inserted, err := notifications.Insert(ctx, &second); err != nil || !inserted {
		t.Fatalf("notification after 24h inserted=%v err=%v", inserted, err)
	}
	for i := 0; i < 2; i++ {
		daily := model.Notification{EventType: model.EventDailySummary, Channel: "email", Recipient: "ops@example.com", Subject: "daily", Body: "daily"}
		if inserted, err := notifications.Insert(ctx, &daily); err != nil || !inserted {
			t.Fatalf("daily %d inserted=%v err=%v", i, inserted, err)
		}
	}
}

func TestConcurrentNotificationDedupe(t *testing.T) {
	ctx := context.Background()
	notifications := store.NewNotificationStore(openDB(t))
	errs := make(chan error, 10)
	inserted := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			n := model.Notification{EventType: model.EventExpired, Channel: "email", Recipient: "ops@example.com", Subject: "test", Body: "test"}
			ok, err := notifications.Insert(ctx, &n)
			inserted <- ok
			errs <- err
		}()
	}
	count := 0
	for i := 0; i < 10; i++ {
		if <-inserted {
			count++
		}
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if count != 1 {
		t.Fatalf("inserted %d duplicates, want 1", count)
	}
}
