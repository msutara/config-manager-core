package scheduler

import (
	"log/slog"
	"sync"

	"github.com/msutara/config-manager-core/internal/plugin"
)

// Scheduler manages recurring jobs defined by plugins.
type Scheduler struct {
	mu   sync.RWMutex
	jobs map[string]plugin.JobDefinition
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
	return j.Func()
}

// Start begins the cron scheduler. Placeholder for Phase 2.
func (s *Scheduler) Start() {
	slog.Info("scheduler started (cron not yet implemented)")
}

// Stop gracefully stops the scheduler.
func (s *Scheduler) Stop() {
	slog.Info("scheduler stopped")
}

// ErrJobNotFound is returned when a job ID is not in the registry.
var ErrJobNotFound = &jobNotFoundError{}

type jobNotFoundError struct{}

func (e *jobNotFoundError) Error() string { return "job not found" }
