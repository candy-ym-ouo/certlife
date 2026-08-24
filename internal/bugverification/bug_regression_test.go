package bugverification

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"certlife/internal/model"
	"certlife/internal/service"
	"certlife/internal/store"
)

func TestBug010_ConcurrentDashboardRequestsAreIsolated(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	certs := store.NewCertStore(db)
	now := time.Now().UTC()
	for i := 0; i < 32; i++ {
		cert := model.Certificate{
			Name:         "certificate-" + string(rune('a'+i)),
			Domain:       "example-" + string(rune('a'+i)) + ".test",
			Issuer:       "Test CA",
			SerialNumber: "serial-" + string(rune('a'+i)),
			Algorithm:    "RSA",
			KeyBits:      2048,
			ValidFrom:    now.Add(-time.Hour),
			ValidUntil:   now.Add(365 * 24 * time.Hour),
			Environment:  "prod",
			Status:       model.CertificateValid,
			DaysToExpire: 365,
		}
		if err := certs.Create(context.Background(), &cert); err != nil {
			t.Fatal(err)
		}
	}

	svc := service.NewStatsService(certs, store.NewRenewalStore(db), store.NewNotificationStore(db))
	errs := make(chan error, 1)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 30; j++ {
				stats, err := svc.Dashboard(context.Background())
				if err != nil {
					select {
					case errs <- err:
					default:
					}
					return
				}
				if stats.Total != 32 || stats.ByStatus[model.CertificateValid] != 32 {
					select {
					case errs <- &statsIsolationError{total: stats.Total, valid: stats.ByStatus[model.CertificateValid]}:
					default:
					}
					return
				}
			}
		}()
	}
	wg.Wait()
	select {
	case err := <-errs:
		t.Fatal(err)
	default:
	}
}

type statsIsolationError struct {
	total int
	valid int
}

func (e *statsIsolationError) Error() string {
	return "concurrent dashboard responses shared aggregate state"
}
