package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/msutara/config-manager-core/internal/storage"
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
	scheds   map[string]*cronSchedule // cached parsed cron expressions
	lastRuns map[string]*JobRun
	store    storage.JobStore
	cancel   context.CancelFunc
	done     chan struct{}
}

// New creates a new Scheduler. If store is non-nil, job execution history
// is persisted across restarts.
func New(store storage.JobStore) *Scheduler {
	return &Scheduler{
		jobs:     make(map[string]plugin.JobDefinition),
		scheds:   make(map[string]*cronSchedule),
		lastRuns: make(map[string]*JobRun),
		store:    store,
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
			sched, err := parseCron(j.Cron)
			if err != nil {
				slog.Warn("job registration skipped: invalid cron", "job_id", j.ID, "cron", j.Cron, "error", err)
				continue
			}
			s.scheds[j.ID] = sched
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

	if s.store != nil {
		rec := runToRecord(run)
		if err := s.store.SaveRun(rec); err != nil {
			slog.Error("failed to persist job run", "job_id", run.JobID, "error", err)
		}
	}
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
	var sched *cronSchedule
	if cron != "" {
		var err error
		sched, err = parseCron(cron)
		if err != nil {
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
	if sched != nil {
		s.scheds[id] = sched
	} else {
		delete(s.scheds, id)
	}
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

// tick fires all jobs whose cron schedule matches the given time.
// Jobs already running from a previous tick are skipped (overlap protection).
func (s *Scheduler) tick(t time.Time) {
	s.mu.RLock()
	snapshot := make([]plugin.JobDefinition, 0)
	for _, j := range s.jobs {
		if j.Cron == "" || j.Func == nil {
			continue
		}
		sched, ok := s.scheds[j.ID]
		if !ok {
			slog.Warn("cron schedule missing for job; skipping", "job_id", j.ID, "cron", j.Cron)
			continue
		}
		// Skip if previous invocation is still running.
		if run, exists := s.lastRuns[j.ID]; exists && run.Status == "running" {
			slog.Debug("skipping overlapping job", "job_id", j.ID)
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

// ListRuns returns historical run records for a job from the persistent store.
func (s *Scheduler) ListRuns(id string, limit, offset int) ([]storage.RunRecord, error) {
	if s.store == nil {
		return []storage.RunRecord{}, nil
	}
	return s.store.ListRuns(id, limit, offset)
}

// LoadHistory restores the latest run for each registered job from the
// persistent store into the in-memory lastRuns map. Call after RegisterJobs
// so that LatestRun works immediately after a restart.
func (s *Scheduler) LoadHistory() {
	if s.store == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	for id := range s.jobs {
		rec, err := s.store.LatestRun(id)
		if err != nil {
			slog.Error("failed to load history for job", "job_id", id, "error", err)
			continue
		}
		if rec != nil {
			s.lastRuns[id] = recordToRun(rec)
		}
	}
	slog.Info("job history loaded from store")
}

// runToRecord converts an in-memory JobRun to a persistent RunRecord.
func runToRecord(run *JobRun) storage.RunRecord {
	rec := storage.RunRecord{
		JobID:     run.JobID,
		Status:    run.Status,
		StartedAt: run.StartedAt,
		EndedAt:   run.EndedAt,
		Error:     run.Error,
		Duration:  run.Duration,
	}
	return rec
}

// recordToRun converts a persistent RunRecord to an in-memory JobRun.
func recordToRun(rec *storage.RunRecord) *JobRun {
	return &JobRun{
		JobID:     rec.JobID,
		Status:    rec.Status,
		StartedAt: rec.StartedAt,
		EndedAt:   rec.EndedAt,
		Error:     rec.Error,
		Duration:  rec.Duration,
	}
}
