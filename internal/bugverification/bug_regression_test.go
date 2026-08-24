package bugverification

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"certlife/internal/service"
	"certlife/internal/store"
)

func TestBug004_NotFoundSurvivesServiceBoundary(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	certs := service.NewCertService(store.NewCertStore(db), store.NewNotificationStore(db))
	_, err = certs.Get(context.Background(), 999999)
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("err=%v, want errors.Is(err, service.ErrNotFound)", err)
	}
}
