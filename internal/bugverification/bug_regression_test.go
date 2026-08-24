package bugverification

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"certlife/internal/model"
	"certlife/internal/service"
	"certlife/internal/store"
)

func TestBug005_ConcurrentCertificateListsAreIsolated(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	certStore := store.NewCertStore(db)
	for i := 0; i < 40; i++ {
		now := time.Now().UTC()
		cert := model.Certificate{
			Name:         fmt.Sprintf("cert-%d", i),
			Domain:       fmt.Sprintf("cert-%d.example", i),
			Issuer:       "Test CA",
			SerialNumber: fmt.Sprintf("%04d", i),
			Algorithm:    "ECDSA",
			KeyBits:      256,
			ValidFrom:    now.Add(-time.Hour),
			ValidUntil:   now.Add(24 * time.Hour),
			Environment:  "prod",
			Status:       model.CertificateValid,
		}
		if err := certStore.Create(ctx, &cert); err != nil {
			t.Fatal(err)
		}
	}
	certs := service.NewCertService(certStore, store.NewNotificationStore(db))
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, 24)
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			<-start
			for round := 0; round < 30; round++ {
				items, total, err := certs.List(ctx, store.CertificateFilter{Page: 1 + worker%4, Size: 5})
				if err != nil {
					errs <- err
					return
				}
				if total != 40 || len(items) == 0 || len(items) > 5 {
					errs <- fmt.Errorf("total=%d items=%d", total, len(items))
					return
				}
			}
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}
