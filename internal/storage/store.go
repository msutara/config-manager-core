package storage

import "time"

// RunRecord represents a single job execution.
type RunRecord struct {
	JobID     string     `json:"job_id"`
	Status    string     `json:"status"` // "running", "completed", "failed"
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
	Error     string     `json:"error,omitempty"`
	Duration  string     `json:"duration,omitempty"`
}

// JobStore persists job execution history.
type JobStore interface {
	SaveRun(rec RunRecord) error
	LatestRun(jobID string) (*RunRecord, error)
	ListRuns(jobID string, limit, offset int) ([]RunRecord, error)
	Prune(jobID string, keepN int) error
	Close() error
}
