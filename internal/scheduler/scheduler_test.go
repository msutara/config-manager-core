package scheduler

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/msutara/config-manager-core/plugin"
)

func TestRegisterAndTrigger(t *testing.T) {
	s := New()

	called := false
	s.RegisterJobs([]plugin.JobDefinition{
		{ID: "test.job", Cron: "* * * * *", Func: func() error {
			called = true
			return nil
		}},
	})

	if err := s.TriggerJob("test.job"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("job function was not called")
	}
}

func TestTriggerJobNotFound(t *testing.T) {
	s := New()

	err := s.TriggerJob("missing")
	if !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("got %v, want ErrJobNotFound", err)
	}
}

func TestRegisterJobsEmptyID(t *testing.T) {
	s := New()

	s.RegisterJobs([]plugin.JobDefinition{
		{ID: "", Cron: "* * * * *"},
	})

	if len(s.ListJobs()) != 0 {
		t.Fatal("empty ID job should not be registered")
	}
}

func TestRegisterJobsDuplicate(t *testing.T) {
	s := New()

	s.RegisterJobs([]plugin.JobDefinition{
		{ID: "dup.job", Cron: "* * * * *"},
		{ID: "dup.job", Cron: "0 * * * *"},
	})

	jobs := s.ListJobs()
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
}

func TestTriggerJobNilFunc(t *testing.T) {
	s := New()

	s.RegisterJobs([]plugin.JobDefinition{
		{ID: "nil.func", Cron: "* * * * *", Func: nil},
	})

	err := s.TriggerJob("nil.func")
	if err == nil {
		t.Fatal("expected error for nil Func")
	}
}

func TestTriggerJobReturnsError(t *testing.T) {
	s := New()

	s.RegisterJobs([]plugin.JobDefinition{
		{ID: "fail.job", Func: func() error {
			return errors.New("oops")
		}},
	})

	err := s.TriggerJob("fail.job")
	if err == nil || err.Error() != "oops" {
		t.Fatalf("got %v, want 'oops'", err)
	}
}

func TestListJobs(t *testing.T) {
	s := New()

	s.RegisterJobs([]plugin.JobDefinition{
		{ID: "a.job"},
		{ID: "b.job"},
	})

	jobs := s.ListJobs()
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2", len(jobs))
	}
}

func TestJobExists(t *testing.T) {
	s := New()

	s.RegisterJobs([]plugin.JobDefinition{
		{ID: "exists.job"},
	})

	if !s.JobExists("exists.job") {
		t.Fatal("expected JobExists to return true for registered job")
	}
	if s.JobExists("missing.job") {
		t.Fatal("expected JobExists to return false for unregistered job")
	}
	if s.JobExists("") {
		t.Fatal("expected JobExists to return false for empty ID")
	}
}

func TestStartStop(t *testing.T) {
	s := New()
	s.Start()
	s.Stop()
	// Double stop should not panic
	s.Stop()
}

func TestTickFiresJob(t *testing.T) {
	s := New()

	var wg sync.WaitGroup
	wg.Add(1)
	s.RegisterJobs([]plugin.JobDefinition{
		{
			ID:   "test.tick",
			Cron: "* * * * *",
			Func: func() error {
				wg.Done()
				return nil
			},
		},
	})

	s.tick(time.Now())

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for job to fire")
	}
}

func TestTickSkipsNonMatching(t *testing.T) {
	s := New()

	fired := make(chan struct{}, 1)
	s.RegisterJobs([]plugin.JobDefinition{
		{
			ID:   "test.nomatch",
			Cron: "0 3 * * *",
			Func: func() error {
				fired <- struct{}{}
				return nil
			},
		},
	})

	// 12:30 doesn't match "0 3 * * *"
	noon := time.Date(2026, 3, 1, 12, 30, 0, 0, time.UTC)
	s.tick(noon)

	select {
	case <-fired:
		t.Error("job should not have fired for non-matching time")
	case <-time.After(50 * time.Millisecond):
		// expected: no fire
	}
}

func TestTickSkipsNilFunc(t *testing.T) {
	s := New()
	s.RegisterJobs([]plugin.JobDefinition{
		{ID: "test.nilfunc2", Cron: "* * * * *", Func: nil},
	})
	// Should not panic
	s.tick(time.Now())
}

func TestTickSkipsEmptyCron(t *testing.T) {
	s := New()

	fired := make(chan struct{}, 1)
	s.RegisterJobs([]plugin.JobDefinition{
		{
			ID:   "test.nocron",
			Cron: "",
			Func: func() error {
				fired <- struct{}{}
				return nil
			},
		},
	})

	s.tick(time.Now())

	select {
	case <-fired:
		t.Error("job should not have fired with empty cron")
	case <-time.After(50 * time.Millisecond):
		// expected: no fire
	}
}

func TestTickJobError(t *testing.T) {
	s := New()

	var wg sync.WaitGroup
	wg.Add(1)
	s.RegisterJobs([]plugin.JobDefinition{
		{
			ID:   "test.err",
			Cron: "* * * * *",
			Func: func() error {
				defer wg.Done()
				return errors.New("boom")
			},
		},
	})

	s.tick(time.Now())

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()

	select {
	case <-done:
		// Error was logged; job completed without panic
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for error job")
	}
}

func TestTickInvalidCron(t *testing.T) {
	s := New()

	// Bypass validation by directly inserting an invalid cron
	s.mu.Lock()
	s.jobs["test.badcron"] = plugin.JobDefinition{
		ID:   "test.badcron",
		Cron: "not valid",
		Func: func() error { return nil },
	}
	s.mu.Unlock()

	// Should not panic
	s.tick(time.Now())
}

func TestDoubleStartIsNoop(t *testing.T) {
	s := New()
	s.Start()
	s.Start() // should be no-op
	s.Stop()
	// If double-start leaked, this would hang
}

func TestReschedule(t *testing.T) {
	s := New()
	s.RegisterJobs([]plugin.JobDefinition{
		{ID: "test.resched", Cron: "0 3 * * *", Func: func() error { return nil }},
	})

	if err := s.Reschedule("test.resched", "* * * * *"); err != nil {
		t.Fatalf("Reschedule failed: %v", err)
	}

	s.mu.RLock()
	j := s.jobs["test.resched"]
	s.mu.RUnlock()
	if j.Cron != "* * * * *" {
		t.Errorf("cron: got %q, want %q", j.Cron, "* * * * *")
	}
}

func TestReschedule_NotFound(t *testing.T) {
	s := New()
	err := s.Reschedule("nonexistent", "* * * * *")
	if !errors.Is(err, ErrJobNotFound) {
		t.Errorf("expected ErrJobNotFound, got %v", err)
	}
}

func TestReschedule_InvalidCron(t *testing.T) {
	s := New()
	s.RegisterJobs([]plugin.JobDefinition{
		{ID: "test.bad", Cron: "0 3 * * *", Func: func() error { return nil }},
	})

	err := s.Reschedule("test.bad", "not a cron")
	if err == nil {
		t.Fatal("expected error for invalid cron")
	}
}

func TestReschedule_EmptyDisables(t *testing.T) {
	s := New()
	s.RegisterJobs([]plugin.JobDefinition{
		{ID: "test.disable", Cron: "0 3 * * *", Func: func() error { return nil }},
	})

	if err := s.Reschedule("test.disable", ""); err != nil {
		t.Fatalf("Reschedule to empty should succeed: %v", err)
	}

	s.mu.RLock()
	j := s.jobs["test.disable"]
	s.mu.RUnlock()
	if j.Cron != "" {
		t.Errorf("cron: got %q, want empty", j.Cron)
	}
}

func TestTickJobPanicRecovery(t *testing.T) {
	s := New()

	done := make(chan struct{})
	s.RegisterJobs([]plugin.JobDefinition{
		{
			ID:   "test.panic",
			Cron: "* * * * *",
			Func: func() error {
				defer func() { close(done) }()
				panic("boom")
			},
		},
	})

	// Should not crash the process
	s.tick(time.Now())

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for panicking job")
	}
}

func TestRegisterJobsInvalidCron(t *testing.T) {
	s := New()

	s.RegisterJobs([]plugin.JobDefinition{
		{ID: "test.badcron", Cron: "not valid", Func: func() error { return nil }},
	})

	if s.JobExists("test.badcron") {
		t.Error("job with invalid cron should not be registered")
	}
}

func TestTriggerJobAsync_Success(t *testing.T) {
	s := New()
	barrier := make(chan struct{}) // blocks job until test checks "running"
	done := make(chan struct{})

	s.RegisterJobs([]plugin.JobDefinition{
		{ID: "test.async", Cron: "* * * * *", Func: func() error {
			<-barrier // wait for test to assert "running"
			close(done)
			return nil
		}},
	})

	if err := s.TriggerJobAsync("test.async"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be running immediately (job is blocked on barrier)
	run := s.LatestRun("test.async")
	if run == nil {
		t.Fatal("expected a run record")
	}
	if run.Status != "running" {
		t.Errorf("status: got %q, want running", run.Status)
	}

	close(barrier) // unblock the job

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for async job")
	}

	waitForRunStatus(t, s, "test.async", "completed", 2*time.Second)

	run = s.LatestRun("test.async")
	if run.Status != "completed" {
		t.Errorf("status after completion: got %q, want completed", run.Status)
	}
	if run.EndedAt == nil {
		t.Error("ended_at should be set after completion")
	}
	if run.Duration == "" {
		t.Error("duration should be set after completion")
	}
}

func TestTriggerJobAsync_Failure(t *testing.T) {
	s := New()
	done := make(chan struct{})

	s.RegisterJobs([]plugin.JobDefinition{
		{ID: "test.fail", Func: func() error {
			defer close(done)
			return errors.New("boom")
		}},
	})

	if err := s.TriggerJobAsync("test.fail"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out")
	}

	waitForRunStatus(t, s, "test.fail", "failed", 2*time.Second)

	run := s.LatestRun("test.fail")
	if run.Status != "failed" {
		t.Errorf("status: got %q, want failed", run.Status)
	}
	if run.Error != "boom" {
		t.Errorf("error: got %q, want boom", run.Error)
	}
}

func TestTriggerJobAsync_Panic(t *testing.T) {
	s := New()
	done := make(chan struct{})

	s.RegisterJobs([]plugin.JobDefinition{
		{ID: "test.panic2", Func: func() error {
			defer close(done)
			panic("kaboom")
		}},
	})

	if err := s.TriggerJobAsync("test.panic2"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out")
	}

	waitForRunStatus(t, s, "test.panic2", "failed", 2*time.Second)

	run := s.LatestRun("test.panic2")
	if run.Status != "failed" {
		t.Errorf("status: got %q, want failed", run.Status)
	}
	if run.Error == "" {
		t.Error("error should describe the panic")
	}
}

func TestTriggerJobAsync_NotFound(t *testing.T) {
	s := New()
	err := s.TriggerJobAsync("missing")
	if !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("got %v, want ErrJobNotFound", err)
	}
}

func TestTriggerJobAsync_NilFunc(t *testing.T) {
	s := New()
	s.RegisterJobs([]plugin.JobDefinition{
		{ID: "test.nilfunc", Func: nil},
	})
	err := s.TriggerJobAsync("test.nilfunc")
	if err == nil {
		t.Fatal("expected error for nil func")
	}
	if !strings.Contains(err.Error(), "no function defined") {
		t.Errorf("error should mention 'no function defined', got: %v", err)
	}
}

func TestLatestRun_NoRuns(t *testing.T) {
	s := New()
	s.RegisterJobs([]plugin.JobDefinition{
		{ID: "test.noruns"},
	})
	run := s.LatestRun("test.noruns")
	if run != nil {
		t.Errorf("expected nil, got %+v", run)
	}
}

func TestTickTracksRuns(t *testing.T) {
	s := New()
	done := make(chan struct{})

	s.RegisterJobs([]plugin.JobDefinition{
		{
			ID:   "test.tickrun",
			Cron: "* * * * *",
			Func: func() error {
				close(done)
				return nil
			},
		},
	})

	s.tick(time.Now())

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out")
	}

	waitForRunStatus(t, s, "test.tickrun", "completed", 2*time.Second)

	run := s.LatestRun("test.tickrun")
	if run == nil {
		t.Fatal("expected tick to record a run")
	}
	if run.Status != "completed" {
		t.Errorf("status: got %q, want completed", run.Status)
	}
}

// waitForRunStatus polls LatestRun until the run reaches the expected status
// or the timeout expires. This avoids timing-sensitive sleeps.
func waitForRunStatus(t *testing.T, s *Scheduler, jobID, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		run := s.LatestRun(jobID)
		if run != nil && run.Status == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for job %q to reach status %q", jobID, want)
}
