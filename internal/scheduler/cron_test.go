package scheduler

import (
	"testing"
	"time"
)

func TestParseCron_Wildcard(t *testing.T) {
	cs, err := parseCron("* * * * *")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should match any time
	for i := 0; i < 60; i++ {
		if !cs.minute[i] {
			t.Errorf("minute[%d] should be true", i)
		}
	}
}

func TestParseCron_Specific(t *testing.T) {
	cs, err := parseCron("30 3 15 6 2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cs.minute[30] {
		t.Error("minute[30] should be true")
	}
	if cs.minute[0] {
		t.Error("minute[0] should be false")
	}
	if !cs.hour[3] {
		t.Error("hour[3] should be true")
	}
	if !cs.dom[14] { // day 15 → index 14
		t.Error("dom[14] should be true")
	}
	if !cs.month[5] { // June → index 5
		t.Error("month[5] should be true")
	}
	if !cs.dow[2] { // Tuesday
		t.Error("dow[2] should be true")
	}
}

func TestParseCron_Range(t *testing.T) {
	cs, err := parseCron("0-5 * * * *")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i := 0; i <= 5; i++ {
		if !cs.minute[i] {
			t.Errorf("minute[%d] should be true", i)
		}
	}
	if cs.minute[6] {
		t.Error("minute[6] should be false")
	}
}

func TestParseCron_Step(t *testing.T) {
	cs, err := parseCron("*/15 * * * *")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[int]bool{0: true, 15: true, 30: true, 45: true}
	for i := 0; i < 60; i++ {
		if cs.minute[i] != want[i] {
			t.Errorf("minute[%d]: got %v, want %v", i, cs.minute[i], want[i])
		}
	}
}

func TestParseCron_List(t *testing.T) {
	cs, err := parseCron("0 1,12,23 * * *")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cs.hour[1] || !cs.hour[12] || !cs.hour[23] {
		t.Error("hours 1, 12, 23 should be true")
	}
	if cs.hour[0] || cs.hour[2] {
		t.Error("hours 0, 2 should be false")
	}
}

func TestParseCron_DOWSunday7(t *testing.T) {
	// 7 should be treated as Sunday (0) only
	cs, err := parseCron("0 0 * * 7")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cs.dow[0] {
		t.Error("dow[0] (Sunday via 7) should be true")
	}
	// Must NOT match other days
	for d := 1; d <= 6; d++ {
		if cs.dow[d] {
			t.Errorf("dow[%d] should be false for singleton 7", d)
		}
	}
}

func TestParseCron_Errors(t *testing.T) {
	cases := []string{
		"",
		"* * *",
		"* * * * * *",
		"60 * * * *",
		"-1 * * * *",
		"* 24 * * *",
		"* * 0 * *",
		"* * 32 * *",
		"* * * 0 *",
		"* * * 13 *",
		"abc * * * *",
		"*/0 * * * *",
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			if _, err := parseCron(c); err == nil {
				t.Errorf("expected error for %q", c)
			}
		})
	}
}

func TestCronSchedule_Matches(t *testing.T) {
	// "0 3 * * *" = every day at 03:00
	cs, err := parseCron("0 3 * * *")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	match := time.Date(2026, 3, 1, 3, 0, 0, 0, time.UTC)
	if !cs.matches(match) {
		t.Error("should match 2026-03-01 03:00")
	}

	noMatch := time.Date(2026, 3, 1, 3, 1, 0, 0, time.UTC)
	if cs.matches(noMatch) {
		t.Error("should not match 2026-03-01 03:01")
	}
}

func TestParseCron_RangeWithStep(t *testing.T) {
	cs, err := parseCron("1-30/10 * * * *")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[int]bool{1: true, 11: true, 21: true}
	for i := 0; i < 60; i++ {
		if cs.minute[i] != want[i] {
			t.Errorf("minute[%d]: got %v, want %v", i, cs.minute[i], want[i])
		}
	}
}

func TestParseCron_DOWRange7(t *testing.T) {
	// "5-7" should match Fri(5), Sat(6), Sun(0)
	cs, err := parseCron("0 0 * * 5-7")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cs.dow[5] {
		t.Error("dow[5] (Friday) should be true")
	}
	if !cs.dow[6] {
		t.Error("dow[6] (Saturday) should be true")
	}
	if !cs.dow[0] {
		t.Error("dow[0] (Sunday via 7) should be true")
	}
}

func TestParseCron_DOWFullRange07(t *testing.T) {
	// "0-7" should match all days
	cs, err := parseCron("0 0 * * 0-7")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for d := 0; d <= 6; d++ {
		if !cs.dow[d] {
			t.Errorf("dow[%d] should be true for 0-7", d)
		}
	}
}

func TestParseCron_DOMDOWOrSemantics(t *testing.T) {
	// "0 0 1 * 1" = 1st of month OR every Monday
	cs, err := parseCron("0 0 1 * 1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Monday March 2, 2026 — not 1st but Monday → should match
	mon := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	if !cs.matches(mon) {
		t.Error("should match Monday (DOW=1) even though DOM!=1")
	}

	// Sunday March 1, 2026 — 1st but not Monday → should match
	first := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	if !cs.matches(first) {
		t.Error("should match 1st of month even though DOW!=1")
	}

	// Wednesday March 4, 2026 — neither 1st nor Monday → should NOT match
	wed := time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)
	if cs.matches(wed) {
		t.Error("should not match when neither DOM nor DOW matches")
	}
}

func TestParseCron_StepOverflow(t *testing.T) {
	// Step larger than field range should be rejected
	_, err := parseCron("1-59/9999999999 * * * *")
	if err == nil {
		t.Error("expected error for excessive step value")
	}
}

func TestParseCron_StepOnLiteral(t *testing.T) {
	// "5/10" in minutes = starting at 5, every 10 → {5, 15, 25, 35, 45, 55}
	cs, err := parseCron("5/10 * * * *")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := map[int]bool{5: true, 15: true, 25: true, 35: true, 45: true, 55: true}
	for m := 0; m < 60; m++ {
		if want[m] && !cs.minute[m] {
			t.Errorf("minute %d should be set", m)
		}
		if !want[m] && cs.minute[m] {
			t.Errorf("minute %d should not be set", m)
		}
	}
}

func TestParseCron_StepOnLiteralDOW(t *testing.T) {
	// "1/2" in DOW = starting at Mon(1), every 2 through 6 → {1, 3, 5}
	cs, err := parseCron("0 0 * * 1/2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should match Mon(1), Wed(3), Fri(5) — NOT Sunday
	want := [7]bool{false, true, false, true, false, true, false}
	for d := 0; d < 7; d++ {
		if cs.dow[d] != want[d] {
			t.Errorf("dow[%d]: got %v, want %v", d, cs.dow[d], want[d])
		}
	}
}

func TestParseCron_StepOnLiteralDOWSunday7(t *testing.T) {
	// "7/2" in DOW = Sunday(7→0), every 2 through 6 → {0, 2, 4, 6}
	cs, err := parseCron("0 0 * * 7/2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := [7]bool{true, false, true, false, true, false, true}
	for d := 0; d < 7; d++ {
		if cs.dow[d] != want[d] {
			t.Errorf("dow[%d]: got %v, want %v", d, cs.dow[d], want[d])
		}
	}
}

func TestParseCron_DOMUnrestrictedWithRestrictedDOW(t *testing.T) {
	// "0 0 1-31 * 1" — DOM syntactically restricted but covers all days.
	// Per cron semantics both fields are restricted → OR semantics apply.
	cs, err := parseCron("0 0 1-31 * 1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Tuesday March 3, 2026 — DOM=3 matches, DOW=2 doesn't → OR fires
	tue := time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC)
	if !cs.matches(tue) {
		t.Error("should match any day via DOM=1-31 OR semantics")
	}

	// Monday March 2, 2026 — both DOM and DOW match
	mon := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	if !cs.matches(mon) {
		t.Error("should match Monday as well")
	}
}
