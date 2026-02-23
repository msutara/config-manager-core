package scheduler

import (
	"errors"
	"testing"

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
