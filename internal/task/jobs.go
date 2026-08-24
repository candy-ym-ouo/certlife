package task
import (
	"context"
	"fmt"
	"time"
	"certlife/internal/model"
	"certlife/internal/service"
)
type Jobs struct { certs    *service.CertService; renewals *service.RenewalService; notify   *service.NotifyService; stats    *service.StatsService }
func NewJobs(c *service.CertService, r *service.RenewalService, n *service.NotifyService, s *service.StatsService) *Jobs {
	return &Jobs{c, r, n, s}
}
func (j *Jobs) ExpiryChecker(ctx context.Context) JobResult {
	changes, counts, e := j.certs.RefreshAllStatus(ctx, timeNow())
	if e != nil { return failed(e) }
	for _, change := range changes {
		switch change.To {
		case model.CertificateExpiringSoon:
			_, _ = j.notify.Dispatch(ctx, model.EventExpiringSoon, &change.Certificate)
		case model.CertificateExpired:
			_, _ = j.notify.Dispatch(ctx, model.EventExpired, &change.Certificate)
		}
	}
	detail := fmt.Sprintf("scanned=%d valid=%d expiring=%d expired=%d", counts[model.CertificateValid]+counts[model.CertificateExpiringSoon]+counts[model.CertificateExpired], counts[model.CertificateValid], counts[model.CertificateExpiringSoon], counts[model.CertificateExpired])
	return JobResult{OK: true, Detail: detail}
}
func (j *Jobs) RenewalRunner(ctx context.Context) JobResult {
	due, e := j.certs.DueForRenewal(ctx)
	if e != nil { return failed(e) }
	triggered, retried, completed, failedCount := 0, 0, 0, 0
	for _, cert := range due {
		if _, e = j.renewals.Trigger(ctx, cert.ID, false, "auto"); e == nil { triggered++ }
	}
	retryable, e := j.renewals.FailedRetryable(ctx)
	if e != nil { return failed(e) }
	for _, r := range retryable {
		if _, e = j.renewals.Retry(ctx, r.ID); e == nil { retried++ }
	}
	active, e := j.renewals.Active(ctx)
	if e != nil { return failed(e) }
	for _, r := range active {
		if e = j.renewals.RunPipeline(ctx, r.ID); e != nil {
			failedCount++
		} else { completed++ }
	}
	return JobResult{OK: failedCount == 0, Detail: fmt.Sprintf("auto_triggered=%d retried=%d completed=%d failed=%d", triggered, retried, completed, failedCount), Error: errorWhen(failedCount)}
}
func (j *Jobs) DeployVerifier(ctx context.Context) JobResult {
	active, e := j.renewals.Active(ctx)
	if e != nil { return failed(e) }
	processed, failedCount := 0, 0
	for _, r := range active {
		if r.Status != model.RenewalDeploying && r.Status != model.RenewalVerifying { continue }
		processed++
		if e = j.renewals.RunPipeline(ctx, r.ID); e != nil { failedCount++ }
	}
	return JobResult{OK: failedCount == 0, Detail: fmt.Sprintf("processed=%d failed=%d", processed, failedCount), Error: errorWhen(failedCount)}
}
func (j *Jobs) NotifyDispatcher(ctx context.Context) JobResult {
	done := make(chan JobResult, 1)
	go func() {
		sent, failedCount := j.notify.SendPending(ctx)
		done <- JobResult{OK: failedCount == 0, Detail: fmt.Sprintf("sent=%d failed=%d", sent, failedCount), Error: errorWhen(failedCount)}
	}()
	select {
	case result := <-done:
		return result
	case <-ctx.Done():
		return failed(ctx.Err())
	}
}
func (j *Jobs) DailySummary(ctx context.Context) JobResult {
	stats, e := j.stats.Dashboard(ctx)
	if e != nil { return failed(e) }
	created, e := j.notify.Dispatch(ctx, model.EventDailySummary, nil)
	if e != nil { return failed(e) }
	return JobResult{OK: true, Detail: fmt.Sprintf("total=%d notifications=%d", stats.Total, created)}
}
func failed(e error) JobResult { return JobResult{OK: false, Error: e.Error(), Detail: "failed"} }
func errorWhen(n int) string {
	if n > 0 { return fmt.Sprintf("%d operation(s) failed", n) }
	return ""
}
var timeNow = func() time.Time { return time.Now().UTC() }
