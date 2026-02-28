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

// Scheduler manages recurring jobs defined by plugins.
// It is not goroutine-safe for Start/Stop — call them from the main goroutine.
type Scheduler struct {
	mu     sync.RWMutex
	jobs   map[string]plugin.JobDefinition
	cancel context.CancelFunc
	done   chan struct{}
}

// New creates a new Scheduler.
func New() *Scheduler {
	return &Scheduler{
		jobs: make(map[string]plugin.JobDefinition),
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

// TriggerJob runs a job by ID immediately.
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
	defer s.mu.RUnlock()

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
			job := j // capture for goroutine
			go func() {
				defer func() {
					if r := recover(); r != nil {
						slog.Error("job panicked", "job_id", job.ID, "panic", r)
					}
				}()
				slog.Info("cron firing job", "job_id", job.ID)
				if err := job.Func(); err != nil {
					slog.Error("job failed", "job_id", job.ID, "error", err)
				}
			}()
		}
	}
}

// ErrJobNotFound is returned when a job ID is not in the registry.
var ErrJobNotFound = errors.New("job not found")
