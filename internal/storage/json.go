package storage

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

// maxHistoryFileSize is the maximum allowed job history file size (10 MB).
// Prevents OOM on ARM devices with limited RAM if the file is corrupt or huge.
const maxHistoryFileSize = 10 * 1024 * 1024

func init() {
	Register("json", func(dataDir string, maxRuns int) (JobStore, error) {
		path := filepath.Join(dataDir, "job_history.json")
		return NewJSONStore(path, maxRuns)
	})
}

// JSONStore is a file-backed JobStore that persists run records as JSON.
// Records are cached in memory; only SaveRun and Prune write to disk.
type JSONStore struct {
	mu      sync.RWMutex
	path    string
	maxN    int                    // max records per job (0 = unlimited)
	records map[string][]RunRecord // in-memory cache keyed by jobID
}

// NewJSONStore creates a JSONStore that reads and writes to the given path.
// maxPerJob controls automatic pruning on SaveRun (0 = no pruning).
// Returns an error if the history file exists but cannot be read (permissions,
// I/O errors, non-regular file). Corrupt or oversized JSON files are recovered
// by logging a warning and starting fresh.
func NewJSONStore(path string, maxPerJob int) (*JSONStore, error) {
	if maxPerJob < 0 {
		return nil, fmt.Errorf("maxPerJob must be >= 0, got %d", maxPerJob)
	}
	s := &JSONStore{
		path:    path,
		maxN:    maxPerJob,
		records: make(map[string][]RunRecord),
	}
	if err := s.loadLocked(); err != nil {
		return nil, err
	}
	return s, nil
}

// Close is a no-op for JSONStore (no resources to release). It satisfies
// the JobStore interface for backends like SQLite that need cleanup.
func (s *JSONStore) Close() error { return nil }

// SaveRun appends a run record to the store. If maxN > 0, records for the
// same job are pruned to keep only the most recent maxN entries.
// The in-memory cache is only updated after a successful disk write.
func (s *JSONStore) SaveRun(rec RunRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Build candidate state without mutating cache.
	existing := s.records[rec.JobID]
	candidate := make([]RunRecord, len(existing)+1)
	copy(candidate, existing)
	candidate[len(existing)] = rec

	if s.maxN > 0 && len(candidate) > s.maxN {
		candidate = candidate[len(candidate)-s.maxN:]
	}

	// Build full map for write.
	all := make(map[string][]RunRecord, len(s.records))
	for k, v := range s.records {
		all[k] = v
	}
	all[rec.JobID] = candidate

	// Persist — only update cache on success.
	if err := s.writeLocked(all); err != nil {
		return err
	}
	s.records[rec.JobID] = candidate
	return nil
}

// LatestRun returns the most recent run record for the given job ID.
// Returns (nil, nil) when no records exist for that job.
func (s *JSONStore) LatestRun(jobID string) (*RunRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	recs := s.records[jobID]
	if len(recs) == 0 {
		return nil, nil
	}
	r := recs[len(recs)-1]
	if r.EndedAt != nil {
		endCopy := *r.EndedAt
		r.EndedAt = &endCopy
	}
	return &r, nil
}

// ListRuns returns run records for a job, newest-first, with pagination.
// Returns an empty (non-nil) slice when no records match.
func (s *JSONStore) ListRuns(jobID string, limit, offset int) ([]RunRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if offset < 0 {
		return nil, fmt.Errorf("offset must be non-negative, got %d", offset)
	}
	if limit < 0 {
		return nil, fmt.Errorf("limit must be non-negative, got %d", limit)
	}

	recs := s.records[jobID]
	if len(recs) == 0 {
		return []RunRecord{}, nil
	}

	// Build newest-first slice with deep-copied pointer fields so callers
	// cannot mutate internal cache state via shared pointers.
	filtered := make([]RunRecord, len(recs))
	for i, r := range recs {
		if r.EndedAt != nil {
			endCopy := *r.EndedAt
			r.EndedAt = &endCopy
		}
		filtered[len(recs)-1-i] = r
	}

	// Apply offset.
	if offset >= len(filtered) {
		return []RunRecord{}, nil
	}
	filtered = filtered[offset:]

	// Apply limit.
	if limit > 0 && limit < len(filtered) {
		filtered = filtered[:limit]
	}

	return filtered, nil
}

// Prune keeps only the N most recent records per job.
// The in-memory cache is only updated after a successful disk write.
func (s *JSONStore) Prune(jobID string, keepN int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if keepN < 0 {
		return fmt.Errorf("storage: keepN must be non-negative, got %d", keepN)
	}

	recs := s.records[jobID]
	var candidate []RunRecord
	switch {
	case keepN == 0:
		candidate = []RunRecord{}
	case len(recs) > keepN:
		candidate = recs[len(recs)-keepN:]
	default:
		candidate = recs
	}

	// Build full map for write.
	all := make(map[string][]RunRecord, len(s.records))
	for k, v := range s.records {
		all[k] = v
	}
	all[jobID] = candidate

	// Persist — only update cache on success.
	if err := s.writeLocked(all); err != nil {
		return err
	}
	s.records[jobID] = candidate
	return nil
}

// loadLocked reads records from disk into the in-memory cache. Called once
// during NewJSONStore construction (before the store is shared) and may also be
// reused internally. Callers must hold mu whenever the store may be accessed
// concurrently.
//
// Uses fd-based stat to avoid TOCTOU races and rejects non-regular file targets
// (directories, devices, pipes). Note: os.Open follows symlinks; the IsRegular
// check catches non-regular targets but not symlinks to regular files.
//
// Error handling:
//   - File does not exist → start fresh (no error)
//   - Not a regular file → return error
//   - File too large → log warning, start fresh (no error)
//   - Read/stat I/O error → return error
//   - Corrupt JSON → log warning, start fresh (no error)
func (s *JSONStore) loadLocked() error {
	f, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("open history file: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat history file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("history file %q (mode %v) is not a regular file", s.path, info.Mode())
	}
	if info.Size() > maxHistoryFileSize {
		slog.Warn("job history file too large, starting fresh",
			"path", s.path, "size", info.Size(), "limit", maxHistoryFileSize)
		return nil
	}

	data, err := io.ReadAll(io.LimitReader(f, maxHistoryFileSize+1))
	if err != nil {
		return fmt.Errorf("read history file: %w", err)
	}

	var flat []RunRecord
	if err := json.Unmarshal(data, &flat); err != nil {
		slog.Warn("job history file corrupt, starting fresh", "path", s.path, "error", err)
		return nil
	}

	for _, r := range flat {
		s.records[r.JobID] = append(s.records[r.JobID], r)
	}
	return nil
}

// writeLocked atomically writes the given records to disk using an
// unpredictable temp file (os.CreateTemp). Must be called while holding
// mu for write.
func (s *JSONStore) writeLocked(records map[string][]RunRecord) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	// Flatten to a single slice for on-disk format.
	var flat []RunRecord
	for _, recs := range records {
		flat = append(flat, recs...)
	}

	data, err := json.MarshalIndent(flat, "", "  ")
	if err != nil {
		return err
	}

	if len(data) > maxHistoryFileSize {
		slog.Warn("job history file exceeds size limit; refusing to write",
			"path", s.path,
			"size_bytes", len(data),
			"max_bytes", maxHistoryFileSize,
		)
		return fmt.Errorf("job history file size %d exceeds limit %d", len(data), maxHistoryFileSize)
	}

	f, err := os.CreateTemp(dir, ".job-history-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()

	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		os.Remove(tmp)
		return err
	}
	// Sync the directory to ensure the rename is durable on crash.
	// Best-effort: directory fsync failures are non-fatal since the file
	// data and metadata are already durable from f.Sync() above.
	d, dErr := os.Open(dir)
	if dErr == nil {
		if syncErr := d.Sync(); syncErr != nil {
			slog.Warn("failed to sync directory", "dir", dir, "error", syncErr)
		}
		d.Close()
	}
	return nil
}
