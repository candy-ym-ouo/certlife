package bugverification

import (
	"context"
	"testing"
	"time"

	"certlife/internal/task"
)

func TestBug001_TriggerNowPreservesCancelledContext(t *testing.T) {
	scheduler := task.NewScheduler()
	if err := scheduler.Register(task.Job{
		Name:     "cancel-aware",
		Interval: time.Hour,
		Run: func(ctx context.Context) task.JobResult {
			if err := ctx.Err(); err != nil {
				return task.JobResult{Error: err.Error(), Detail: "cancelled"}
			}
			return task.JobResult{OK: true, Detail: "completed"}
		},
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := scheduler.TriggerNow(ctx, "cancel-aware")
	if err != nil {
		t.Fatal(err)
	}
	if result.OK || result.Error != context.Canceled.Error() {
		t.Fatalf("result=%+v, want cancelled job result", result)
	}
}
