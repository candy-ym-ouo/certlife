package bugverification

import (
	"context"
	"path/filepath"
	"testing"

	"certlife/internal/model"
	"certlife/internal/service"
	"certlife/internal/store"
)

func TestBug009_MissingDeploymentTargetReturnsError(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := service.NewDeployService(store.NewDeploymentStore(db), t.TempDir(), false)
	cert := model.Certificate{ID: 1, DeployTargetIDs: []int64{999999}}
	renewal := model.Renewal{ID: 1, CertificateID: cert.ID, NewSerial: "NEW"}
	var deployErr error
	panicked := false
	func() {
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()
		_, deployErr = svc.Deploy(context.Background(), renewal, cert)
	}()
	if panicked {
		t.Fatal("Deploy panicked for a missing target")
	}
	if deployErr == nil {
		t.Fatal("Deploy returned nil error for a missing target")
	}
}
