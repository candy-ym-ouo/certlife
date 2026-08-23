// Package task provides the small in-process scheduler used by CertLife.
package task
import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)
var ErrTaskNotFound = errors.New("task not found")
var ErrTaskRunning = errors.New("task already running")
type JobResult struct { OK     bool   `json:"ok"`; Detail string `json:"detail"`; Error  string `json:"error,omitempty"` }
type Job struct { Name     string; Interval time.Duration; Run      func(context.Context) JobResult }
type RunInfo struct {
	Name       string     `json:"name"`
	Interval   string     `json:"interval"`
	LastRunAt  *time.Time `json:"last_run_at,omitempty"`
	LastResult string     `json:"last_result"`
	LastError  string     `json:"last_error"`
	Runs       int        `json:"runs"`
	Running    bool       `json:"running"`
}
type Scheduler struct { mu      sync.Mutex; jobs    map[string]Job; last    map[string]RunInfo; running map[string]bool; cancel  context.CancelFunc; wg      sync.WaitGroup; started bool }
func NewScheduler() *Scheduler {
	return &Scheduler{jobs: map[string]Job{}, last: map[string]RunInfo{}, running: map[string]bool{}}
}
func (s *Scheduler) Register(job Job) error {
	if job.Name == "" || job.Interval <= 0 || job.Run == nil { return fmt.Errorf("invalid job") }
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.jobs[job.Name]; ok { return fmt.Errorf("duplicate job %s", job.Name) }
	s.jobs[job.Name] = job
	s.last[job.Name] = RunInfo{Name: job.Name, Interval: job.Interval.String()}
	return nil
}
func (s *Scheduler) Start(parent context.Context) {
	s.mu.Lock()
	if s.started { s.mu.Unlock(); return }
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	s.started = true
	jobs := make([]Job, 0, len(s.jobs))
	for _, job := range s.jobs { jobs = append(jobs, job) }
	s.mu.Unlock()
	for _, job := range jobs { s.wg.Add(1); go s.loop(ctx, job) }
}
func (s *Scheduler) loop(ctx context.Context, job Job) {
	defer s.wg.Done()
	s.run(ctx, job, false)
	ticker := time.NewTicker(job.Interval)
	defer ticker.Stop()
	for {
		select { case <-ctx.Done():; return; case <-ticker.C:; go s.run(ctx, job, false) }
	}
}
func (s *Scheduler) TriggerNow(ctx context.Context, name string) (JobResult, error) {
	s.mu.Lock()
	job, ok := s.jobs[name]
	if !ok {
		s.mu.Unlock()
		return JobResult{}, ErrTaskNotFound
	}
	if s.running[name] {
		s.mu.Unlock()
		return JobResult{}, ErrTaskRunning
	}
	s.mu.Unlock()
	return s.run(ctx, job, true), nil
}
func (s *Scheduler) run(ctx context.Context, job Job, manual bool) JobResult {
	s.mu.Lock()
	if s.running[job.Name] {
		s.mu.Unlock()
		return JobResult{OK: false, Detail: "skipped", Error: ErrTaskRunning.Error()}
	}
	s.running[job.Name] = true
	info := s.last[job.Name]
	info.Running = true
	s.last[job.Name] = info
	s.mu.Unlock()
	result := JobResult{}
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				result = JobResult{OK: false, Error: fmt.Sprint(recovered), Detail: "panic recovered"}
			}
		}()
		result = job.Run(ctx)
	}()
	now := time.Now().UTC()
	s.mu.Lock()
	info = s.last[job.Name]
	info.Runs++
	info.LastRunAt = &now
	info.Running = false
	if result.OK {
		info.LastResult = "ok"
		info.LastError = ""
	} else { info.LastResult = "failed"; info.LastError = result.Error }
	s.running[job.Name] = false
	s.last[job.Name] = info
	s.mu.Unlock()
	_ = manual
	return result
}
func (s *Scheduler) List() []RunInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]RunInfo, 0, len(s.last))
	for _, info := range s.last { info.Running = s.running[info.Name]; out = append(out, info) }
	return out
}
func (s *Scheduler) RunningCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, running := range s.running {
		if running { n++ }
	}
	return n
}
func (s *Scheduler) Stop(ctx context.Context) error {
	s.mu.Lock()
	if s.cancel != nil { s.cancel() }
	s.mu.Unlock()
	done := make(chan struct{})
	go func() { s.wg.Wait(); close(done) }()
	select { case <-done:; return nil; case <-ctx.Done():; return ctx.Err() }
}
