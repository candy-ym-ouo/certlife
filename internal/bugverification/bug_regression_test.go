package bugverification

import (
	"path/filepath"
	"testing"

	"certlife/internal/model"
	"certlife/internal/service"
	"certlife/internal/store"
)

func TestBug003_RememberVerificationDoesNotCrash(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	deploy := service.NewDeployService(store.NewDeploymentStore(db), t.TempDir(), false)
	panicked := false
	func() {
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()
		deploy.RememberVerification(7, model.VerificationResult{SerialOK: true})
	}()
	if panicked {
		t.Fatal("RememberVerification panicked")
	}
}
