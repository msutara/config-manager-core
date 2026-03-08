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

	// ListRuns returns run records for the given job in reverse chronological
	// order (newest first by StartedAt). Pagination semantics:
	//   - offset: number of records to skip from the start of the ordered set.
	//   - limit:  maximum number of records to return after applying offset.
	//             If limit == 0, all remaining records are returned.
	//   - Both offset and limit must be non-negative; implementations should
	//     return an error for negative values.
	// Returns an empty (non-nil) slice when no records match.
	ListRuns(jobID string, limit, offset int) ([]RunRecord, error)

	Prune(jobID string, keepN int) error
	Close() error
}
