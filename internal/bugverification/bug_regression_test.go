package bugverification

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"certlife/internal/model"
	"certlife/internal/store"
)

func TestBug002_ConcurrentPendingReadsAreIsolated(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	notifications := store.NewNotificationStore(db)
	for i := 0; i < 16; i++ {
		n := model.Notification{
			EventType: model.EventExpired,
			Channel:   "email",
			Recipient: filepath.Join("ops", string(rune('a'+i))) + "@example.com",
			Subject:   "expired",
			Body:      "expired",
		}
		if inserted, err := notifications.Insert(ctx, &n); err != nil || !inserted {
			t.Fatalf("inserted=%v err=%v", inserted, err)
		}
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			<-start
			for round := 0; round < 40; round++ {
				items, err := notifications.Pending(ctx, 1+(worker%16))
				if err != nil {
					errs <- err
					return
				}
				if len(items) == 0 {
					errs <- &pendingReadError{}
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

type pendingReadError struct{}

func (*pendingReadError) Error() string { return "pending result was unexpectedly empty" }
