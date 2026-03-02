package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/msutara/config-manager-core/plugin"
)

// JobRun records the outcome of a single job execution.
type JobRun struct {
	JobID     string     `json:"job_id"`
	Status    string     `json:"status"` // "running", "completed", "failed"
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
	Error     string     `json:"error,omitempty"`
	Duration  string     `json:"duration,omitempty"`
}

// Scheduler manages recurring jobs defined by plugins.
// It is not goroutine-safe for Start/Stop — call them from the main goroutine.
type Scheduler struct {
	mu       sync.RWMutex
	jobs     map[string]plugin.JobDefinition
	lastRuns map[string]*JobRun
	cancel   context.CancelFunc
	done     chan struct{}
}

// New creates a new Scheduler.
func New() *Scheduler {
	return &Scheduler{
		jobs:     make(map[string]plugin.JobDefinition),
		lastRuns: make(map[string]*JobRun),
	}
}

// RegisterJobs adds job definitions to the scheduler.
func (s *Scheduler) RegisterJobs(jobs []plugin.JobDefinition) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, j := range jobs {
		if j.ID == "" {
			slog.Warn("job registration skipped: empty ID")
			continue
		}
		if _, exists := s.jobs[j.ID]; exists {
			slog.Warn("duplicate job ID; registration skipped", "job_id", j.ID, "cron", j.Cron)
			continue
		}
		if j.Cron != "" {
			if _, err := parseCron(j.Cron); err != nil {
				slog.Warn("job registration skipped: invalid cron", "job_id", j.ID, "cron", j.Cron, "error", err)
				continue
			}
		}
		s.jobs[j.ID] = j
		slog.Info("job registered", "job_id", j.ID, "cron", j.Cron)
	}
}

// ListJobs returns all registered jobs.
func (s *Scheduler) ListJobs() []plugin.JobDefinition {
	s.mu.RLock()
	defer s.mu.RUnlock()

	jobs := make([]plugin.JobDefinition, 0, len(s.jobs))
	for _, j := range s.jobs {
		jobs = append(jobs, j)
	}
	return jobs
}

// TriggerJob runs a job by ID immediately and returns its error (if any).
// It does not track the run in lastRuns — use TriggerJobAsync for tracking.
func (s *Scheduler) TriggerJob(id string) error {
	s.mu.RLock()
	j, ok := s.jobs[id]
	s.mu.RUnlock()

	if !ok {
		return ErrJobNotFound
	}

	slog.Info("triggering job", "job_id", id)
	if j.Func == nil {
		return fmt.Errorf("job %q has no function defined", id)
	}
	return j.Func()
}

// TriggerJobAsync starts a job in a background goroutine and tracks its
// execution status. The caller can poll LatestRun to monitor progress.
func (s *Scheduler) TriggerJobAsync(id string) error {
	s.mu.RLock()
	j, ok := s.jobs[id]
	s.mu.RUnlock()

	if !ok {
		return ErrJobNotFound
	}
	if j.Func == nil {
		return fmt.Errorf("job %q has no function defined", id)
	}

	now := time.Now()
	run := &JobRun{
		JobID:     id,
		Status:    "running",
		StartedAt: now,
	}
	s.mu.Lock()
	s.lastRuns[id] = run
	s.mu.Unlock()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("job panicked", "job_id", id, "panic", r)
				end := time.Now()
				s.finalizeRun(run, "failed", fmt.Sprintf("panic: %v", r), end)
			}
		}()
		slog.Info("async job started", "job_id", id)
		err := j.Func()
		end := time.Now()
		if err != nil {
			s.finalizeRun(run, "failed", err.Error(), end)
			slog.Error("async job failed", "job_id", id, "error", err)
		} else {
			s.finalizeRun(run, "completed", "", end)
			slog.Info("async job completed", "job_id", id, "duration", run.Duration)
		}
	}()
	return nil
}

// finalizeRun updates a run record under the scheduler lock. Using a
// dedicated helper with a deferred unlock ensures the lock is always
// released even if finalizeRun itself panics.
func (s *Scheduler) finalizeRun(run *JobRun, status, errMsg string, end time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run.Status = status
	run.EndedAt = &end
	run.Error = errMsg
	run.Duration = end.Sub(run.StartedAt).Round(time.Millisecond).String()
}

// LatestRun returns the most recent run record for a job, or nil if none.
func (s *Scheduler) LatestRun(id string) *JobRun {
	s.mu.RLock()
	defer s.mu.RUnlock()
	run, ok := s.lastRuns[id]
	if !ok {
		return nil
	}
	// Return a deep copy so callers don't race with in-flight updates.
	cp := *run
	// Deep-copy pointer fields so callers cannot mutate scheduler internals.
	if cp.EndedAt != nil {
		endedAtCopy := *cp.EndedAt
		cp.EndedAt = &endedAtCopy
	}
	return &cp
}

// JobExists returns true if a job with the given ID is registered.
func (s *Scheduler) JobExists(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.jobs[id]
	return ok
}

// Start begins the cron scheduler. Jobs are evaluated every minute.
// Call from the main goroutine only; calling Start twice without Stop is a no-op.
func (s *Scheduler) Start() {
	if s.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.done = make(chan struct{})

	go s.run(ctx)
	slog.Info("scheduler started")
}

// Stop gracefully stops the scheduler and waits for the run loop to exit.
func (s *Scheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
		<-s.done
		s.cancel = nil
		slog.Info("scheduler stopped")
	}
}

// Reschedule updates a job's cron expression. Pass an empty string to disable.
func (s *Scheduler) Reschedule(id, cron string) error {
	if cron != "" {
		if _, err := parseCron(cron); err != nil {
			return fmt.Errorf("invalid cron %q: %w", cron, err)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	j, ok := s.jobs[id]
	if !ok {
		return ErrJobNotFound
	}
	j.Cron = cron
	s.jobs[id] = j
	slog.Info("job rescheduled", "job_id", id, "cron", cron)
	return nil
}

func (s *Scheduler) run(ctx context.Context) {
	defer close(s.done)

	// Align to the start of the next minute for predictable firing.
	now := time.Now()
	next := now.Truncate(time.Minute).Add(time.Minute)
	alignTimer := time.NewTimer(time.Until(next))
	defer alignTimer.Stop()

	select {
	case <-ctx.Done():
		return
	case <-alignTimer.C:
	}

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	s.tick(time.Now())
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			s.tick(t)
		}
	}
}

// tick fires all jobs whose cron expression matches the given time.
// TODO: add per-job overlap protection (skip if previous instance still running).
func (s *Scheduler) tick(t time.Time) {
	s.mu.RLock()
	snapshot := make([]plugin.JobDefinition, 0)
	for _, j := range s.jobs {
		if j.Cron == "" || j.Func == nil {
			continue
		}
		sched, err := parseCron(j.Cron)
		if err != nil {
			slog.Warn("bad cron expression", "job_id", j.ID, "cron", j.Cron, "error", err)
			continue
		}
		if sched.matches(t) {
			snapshot = append(snapshot, j)
		}
	}
	s.mu.RUnlock()

	for _, job := range snapshot {
		j := job // capture for goroutine
		now := time.Now()
		run := &JobRun{
			JobID:     j.ID,
			Status:    "running",
			StartedAt: now,
		}
		s.mu.Lock()
		s.lastRuns[j.ID] = run
		s.mu.Unlock()

		go func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("job panicked", "job_id", j.ID, "panic", r)
					end := time.Now()
					s.finalizeRun(run, "failed", fmt.Sprintf("panic: %v", r), end)
				}
			}()
			slog.Info("cron firing job", "job_id", j.ID)
			err := j.Func()
			end := time.Now()
			if err != nil {
				s.finalizeRun(run, "failed", err.Error(), end)
				slog.Error("job failed", "job_id", j.ID, "error", err)
			} else {
				s.finalizeRun(run, "completed", "", end)
			}
		}()
	}
}

// ErrJobNotFound is returned when a job ID is not in the registry.
var ErrJobNotFound = errors.New("job not found")
