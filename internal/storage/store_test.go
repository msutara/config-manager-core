package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func newTestStore(t *testing.T) (*JSONStore, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "job_history.json")
	store, err := NewJSONStore(path, 50)
	if err != nil {
		t.Fatalf("NewJSONStore: %v", err)
	}
	return store, path
}

func makeRun(jobID, status string, ago time.Duration) RunRecord {
	start := time.Now().Add(-ago)
	end := start.Add(1 * time.Second)
	return RunRecord{
		JobID:     jobID,
		Status:    status,
		StartedAt: start,
		EndedAt:   &end,
		Duration:  "1s",
	}
}

func TestSaveRunAndLatestRun(t *testing.T) {
	store, _ := newTestStore(t)

	rec := makeRun("test.job", "completed", 1*time.Minute)
	if err := store.SaveRun(rec); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}

	got, err := store.LatestRun("test.job")
	if err != nil {
		t.Fatalf("LatestRun: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil run")
	}
	if got.JobID != "test.job" {
		t.Errorf("JobID: got %q, want %q", got.JobID, "test.job")
	}
	if got.Status != "completed" {
		t.Errorf("Status: got %q, want %q", got.Status, "completed")
	}
}

func TestLatestRun_UnknownJob(t *testing.T) {
	store, _ := newTestStore(t)

	got, err := store.LatestRun("nonexistent")
	if err != nil {
		t.Fatalf("LatestRun: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for unknown job, got %+v", got)
	}
}

func TestListRuns_NewestFirst(t *testing.T) {
	store, _ := newTestStore(t)

	for i := 3; i >= 1; i-- {
		rec := makeRun("test.job", "completed", time.Duration(i)*time.Minute)
		if err := store.SaveRun(rec); err != nil {
			t.Fatalf("SaveRun: %v", err)
		}
	}

	runs, err := store.ListRuns("test.job", 10, 0)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 3 {
		t.Fatalf("got %d runs, want 3", len(runs))
	}

	// Newest first: last saved (1min ago) should be first.
	if runs[0].StartedAt.Before(runs[1].StartedAt) {
		t.Error("runs not in newest-first order")
	}
	if runs[1].StartedAt.Before(runs[2].StartedAt) {
		t.Error("runs not in newest-first order")
	}
}

func TestListRuns_Pagination(t *testing.T) {
	store, _ := newTestStore(t)

	for i := 0; i < 10; i++ {
		rec := makeRun("test.job", "completed", time.Duration(10-i)*time.Minute)
		if err := store.SaveRun(rec); err != nil {
			t.Fatalf("SaveRun: %v", err)
		}
	}

	// Limit=5 returns first 5 (newest).
	runs, err := store.ListRuns("test.job", 5, 0)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 5 {
		t.Fatalf("got %d runs, want 5", len(runs))
	}

	// Offset=5, limit=5 returns next 5.
	runs2, err := store.ListRuns("test.job", 5, 5)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs2) != 5 {
		t.Fatalf("got %d runs, want 5", len(runs2))
	}

	// Offset beyond count returns empty.
	runs3, err := store.ListRuns("test.job", 5, 20)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs3) != 0 {
		t.Fatalf("got %d runs, want 0", len(runs3))
	}
}

func TestListRuns_UnknownJobReturnsEmptySlice(t *testing.T) {
	store, _ := newTestStore(t)

	runs, err := store.ListRuns("nonexistent", 10, 0)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if runs == nil {
		t.Fatal("expected non-nil empty slice, got nil")
	}
	if len(runs) != 0 {
		t.Fatalf("got %d runs, want 0", len(runs))
	}
}

func TestListRuns_NegativeOffset(t *testing.T) {
	store, _ := newTestStore(t)

	_, err := store.ListRuns("test.job", 10, -1)
	if err == nil {
		t.Fatal("expected error for negative offset")
	}
}

func TestListRuns_NegativeLimit(t *testing.T) {
	store, _ := newTestStore(t)

	_, err := store.ListRuns("test.job", -1, 0)
	if err == nil {
		t.Fatal("expected error for negative limit")
	}
}

func TestPrune(t *testing.T) {
	store, _ := newTestStore(t)

	for i := 0; i < 10; i++ {
		rec := makeRun("test.job", "completed", time.Duration(10-i)*time.Minute)
		if err := store.SaveRun(rec); err != nil {
			t.Fatalf("SaveRun: %v", err)
		}
	}

	if err := store.Prune("test.job", 3); err != nil {
		t.Fatalf("Prune: %v", err)
	}

	runs, err := store.ListRuns("test.job", 100, 0)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 3 {
		t.Fatalf("got %d runs after prune, want 3", len(runs))
	}
}

func TestPrune_PreservesOtherJobs(t *testing.T) {
	store, _ := newTestStore(t)

	for i := 0; i < 5; i++ {
		if err := store.SaveRun(makeRun("job.a", "completed", time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
		if err := store.SaveRun(makeRun("job.b", "completed", time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}

	if err := store.Prune("job.a", 2); err != nil {
		t.Fatal(err)
	}

	runsA, _ := store.ListRuns("job.a", 100, 0)
	runsB, _ := store.ListRuns("job.b", 100, 0)

	if len(runsA) != 2 {
		t.Errorf("job.a: got %d runs, want 2", len(runsA))
	}
	if len(runsB) != 5 {
		t.Errorf("job.b: got %d runs, want 5 (should be untouched)", len(runsB))
	}
}

func TestPrune_NegativeKeepN(t *testing.T) {
	store, _ := newTestStore(t)

	for i := 0; i < 3; i++ {
		if err := store.SaveRun(makeRun("test.job", "completed", time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}

	err := store.Prune("test.job", -1)
	if err == nil {
		t.Fatal("expected error for negative keepN")
	}

	// Records should be untouched.
	runs, _ := store.ListRuns("test.job", 100, 0)
	if len(runs) != 3 {
		t.Errorf("got %d runs after rejected Prune, want 3 (should be intact)", len(runs))
	}
}

func TestSaveRun_AutoPrune(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "job_history.json")
	store, err := NewJSONStore(path, 3) // max 3 per job
	if err != nil {
		t.Fatalf("NewJSONStore: %v", err)
	}

	for i := 0; i < 5; i++ {
		rec := makeRun("test.job", "completed", time.Duration(5-i)*time.Minute)
		if err := store.SaveRun(rec); err != nil {
			t.Fatalf("SaveRun: %v", err)
		}
	}

	runs, _ := store.ListRuns("test.job", 100, 0)
	if len(runs) != 3 {
		t.Fatalf("got %d runs, want 3 (auto-prune)", len(runs))
	}
}

func TestConcurrentSaveRun(t *testing.T) {
	store, _ := newTestStore(t)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			rec := makeRun("test.job", "completed", time.Duration(n)*time.Second)
			if err := store.SaveRun(rec); err != nil {
				t.Errorf("concurrent SaveRun: %v", err)
			}
		}(i)
	}
	wg.Wait()

	runs, err := store.ListRuns("test.job", 100, 0)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 20 {
		t.Fatalf("got %d runs, want 20", len(runs))
	}
}

func TestMissingFileBootstraps(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent", "job_history.json")
	store, err := NewJSONStore(path, 0)
	if err != nil {
		t.Fatalf("NewJSONStore: %v", err)
	}

	rec := makeRun("test.job", "completed", 1*time.Minute)
	if err := store.SaveRun(rec); err != nil {
		t.Fatalf("SaveRun on missing dir: %v", err)
	}

	got, err := store.LatestRun("test.job")
	if err != nil {
		t.Fatalf("LatestRun: %v", err)
	}
	if got == nil {
		t.Fatal("expected run after bootstrap")
	}
}

func TestCorruptFileRecovery(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "job_history.json")

	// Write corrupt data before creating the store.
	if err := os.WriteFile(path, []byte("not valid json{{{"), 0o644); err != nil {
		t.Fatal(err)
	}

	// NewJSONStore should recover (start fresh) on corrupt JSON.
	store, err := NewJSONStore(path, 50)
	if err != nil {
		t.Fatalf("NewJSONStore with corrupt file: %v", err)
	}

	// SaveRun should succeed on the recovered store.
	rec := makeRun("test.job", "completed", 1*time.Minute)
	if err := store.SaveRun(rec); err != nil {
		t.Fatalf("SaveRun after corrupt file: %v", err)
	}

	got, _ := store.LatestRun("test.job")
	if got == nil {
		t.Fatal("expected run after recovery")
	}
}

func TestSaveRun_WithError(t *testing.T) {
	store, _ := newTestStore(t)

	rec := RunRecord{
		JobID:     "test.fail",
		Status:    "failed",
		StartedAt: time.Now().Add(-1 * time.Minute),
		Error:     "something went wrong",
	}
	if err := store.SaveRun(rec); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}

	got, _ := store.LatestRun("test.fail")
	if got == nil {
		t.Fatal("expected run")
	}
	if got.Error != "something went wrong" {
		t.Errorf("Error: got %q, want %q", got.Error, "something went wrong")
	}
}

func TestLatestRun_ReturnsNewest(t *testing.T) {
	store, _ := newTestStore(t)

	old := makeRun("test.job", "completed", 5*time.Minute)
	newer := makeRun("test.job", "failed", 1*time.Minute)

	if err := store.SaveRun(old); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRun(newer); err != nil {
		t.Fatal(err)
	}

	got, _ := store.LatestRun("test.job")
	if got == nil {
		t.Fatal("expected run")
	}
	if got.Status != "failed" {
		t.Errorf("expected newest run (failed), got %q", got.Status)
	}
}

func TestListRuns_FiltersByJobID(t *testing.T) {
	store, _ := newTestStore(t)

	if err := store.SaveRun(makeRun("job.a", "completed", 2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRun(makeRun("job.b", "completed", 1*time.Minute)); err != nil {
		t.Fatal(err)
	}

	runsA, _ := store.ListRuns("job.a", 10, 0)
	if len(runsA) != 1 {
		t.Fatalf("job.a: got %d runs, want 1", len(runsA))
	}
	if runsA[0].JobID != "job.a" {
		t.Errorf("expected job.a, got %q", runsA[0].JobID)
	}
}

func TestFileRoundTrip(t *testing.T) {
	store, path := newTestStore(t)

	rec := makeRun("test.job", "completed", 1*time.Minute)
	if err := store.SaveRun(rec); err != nil {
		t.Fatal(err)
	}

	// Create a new store from the same file.
	store2, err := NewJSONStore(path, 50)
	if err != nil {
		t.Fatalf("NewJSONStore: %v", err)
	}
	got, err := store2.LatestRun("test.job")
	if err != nil {
		t.Fatalf("LatestRun from new store: %v", err)
	}
	if got == nil {
		t.Fatal("expected persisted run from new store instance")
	}

	// Verify JSON on disk is valid.
	data, _ := os.ReadFile(path)
	var records []RunRecord
	if err := json.Unmarshal(data, &records); err != nil {
		t.Fatalf("on-disk JSON invalid: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record on disk, got %d", len(records))
	}
}

func TestClose(t *testing.T) {
	store, _ := newTestStore(t)
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Double close should be safe.
	if err := store.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestNewJSONStore_FileTooLarge(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "job_history.json")

	// Create a file that exceeds maxHistoryFileSize.
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	// Write slightly over the limit. Use Truncate for speed.
	if err := f.Truncate(maxHistoryFileSize + 1); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	// Oversized file is treated as corrupt: store starts fresh, no error.
	store, err := NewJSONStore(path, 50)
	if err != nil {
		t.Fatalf("expected no error (start fresh), got: %v", err)
	}
	runs, _ := store.ListRuns("any", 10, 0)
	if len(runs) != 0 {
		t.Errorf("expected empty store, got %d runs", len(runs))
	}
}

func TestNewJSONStore_ReadError(t *testing.T) {
	// Point at a directory instead of a file to force a read error.
	dir := t.TempDir()
	dirPath := filepath.Join(dir, "job_history.json")
	if err := os.Mkdir(dirPath, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := NewJSONStore(dirPath, 50)
	if err == nil {
		t.Fatal("expected error when path is a directory")
	}
}

func TestNewJSONStore_NegativeMaxPerJob(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "job_history.json")

	_, err := NewJSONStore(path, -1)
	if err == nil {
		t.Fatal("expected error for negative maxPerJob")
	}
}

func TestSaveRun_CacheRollbackOnWriteFailure(t *testing.T) {
	// Create a store with an initial record.
	store, _ := newTestStore(t)
	initial := makeRun("job.a", "completed", 2*time.Hour)
	if err := store.SaveRun(initial); err != nil {
		t.Fatalf("initial SaveRun: %v", err)
	}

	// Point path through a file (not a dir) to guarantee write failure.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	store.path = filepath.Join(blocker, "impossible", "job_history.json")

	// Attempt to save a second record — should fail.
	second := makeRun("job.a", "failed", 1*time.Hour)
	err := store.SaveRun(second)
	if err == nil {
		t.Fatal("expected SaveRun to fail with invalid path")
	}

	// Cache should still have only the initial record.
	latest, err := store.LatestRun("job.a")
	if err != nil {
		t.Fatalf("LatestRun: %v", err)
	}
	if latest == nil {
		t.Fatal("expected initial record in cache")
	}
	if latest.Status != "completed" {
		t.Errorf("cache has status %q, want %q (rollback failed)", latest.Status, "completed")
	}

	runs, err := store.ListRuns("job.a", 10, 0)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 1 {
		t.Errorf("cache has %d runs, want 1 (rollback failed)", len(runs))
	}
}

func TestPrune_KeepZero(t *testing.T) {
	store, _ := newTestStore(t)

	for i := 0; i < 5; i++ {
		rec := makeRun("test.job", "completed", time.Duration(i)*time.Minute)
		if err := store.SaveRun(rec); err != nil {
			t.Fatalf("SaveRun: %v", err)
		}
	}

	// Prune with keepN=0 should clear all runs for the job.
	if err := store.Prune("test.job", 0); err != nil {
		t.Fatalf("Prune(0): %v", err)
	}

	runs, err := store.ListRuns("test.job", 100, 0)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 0 {
		t.Errorf("got %d runs after Prune(0), want 0", len(runs))
	}
}

func TestSaveRun_UnlimitedMaxPerJob(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "job_history.json")

	// maxPerJob=0 means unlimited — no auto-pruning.
	store, err := NewJSONStore(path, 0)
	if err != nil {
		t.Fatalf("NewJSONStore: %v", err)
	}

	for i := 0; i < 100; i++ {
		rec := makeRun("test.job", "completed", time.Duration(i)*time.Minute)
		if err := store.SaveRun(rec); err != nil {
			t.Fatalf("SaveRun[%d]: %v", i, err)
		}
	}

	runs, err := store.ListRuns("test.job", 200, 0)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 100 {
		t.Errorf("got %d runs, want 100 (unlimited should keep all)", len(runs))
	}
}
