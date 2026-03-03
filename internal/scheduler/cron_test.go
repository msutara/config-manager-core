package scheduler

import (
	"strings"
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
	// "0 0 1-31 * 1" — DOM 1-31 covers all days, so it's star-equivalent.
	// Star-equivalent fields use AND semantics: fires only on Mondays.
	cs, err := parseCron("0 0 1-31 * 1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cs.domStar {
		t.Error("domStar should be true for 1-31 (star-equivalent)")
	}

	// Monday March 2, 2026 — DOM matches (any day) AND DOW matches (Mon) → fires
	mon := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	if !cs.matches(mon) {
		t.Error("should match Monday")
	}

	// Tuesday March 3, 2026 — DOM matches but DOW doesn't → AND fails
	tue := time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC)
	if cs.matches(tue) {
		t.Error("should NOT match Tuesday — 1-31 is star-equivalent, AND semantics apply")
	}
}

func TestParseCron_StarEquivalence(t *testing.T) {
	// "*/1" in DOM should be treated as star (AND semantics with DOW).
	// "0 0 */1 * 1" means "every Monday" — NOT "every day OR Monday".
	cs, err := parseCron("0 0 */1 * 1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cs.domStar {
		t.Error("domStar should be true for */1 (star-equivalent)")
	}

	// Monday March 2, 2026 — both DOM (any) AND DOW (Mon) match → fires
	mon := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	if !cs.matches(mon) {
		t.Error("should match Monday")
	}

	// Wednesday March 4, 2026 — DOM matches (any day) but DOW doesn't (not Mon)
	// With AND semantics: should NOT match (DOM=star → AND → DOW must match)
	wed := time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)
	if cs.matches(wed) {
		t.Error("should NOT match Wednesday — */1 is star-equivalent, AND semantics apply")
	}
}

func TestParseCron_DOWStarEquivalence(t *testing.T) {
	// "0 0 15 * */1" — DOW */1 is star-equivalent → AND semantics
	// Should only fire on 15th, not every day.
	cs, err := parseCron("0 0 15 * */1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cs.dowStar {
		t.Error("dowStar should be true for */1 (star-equivalent)")
	}

	// March 15 (Sunday) — DOM=15 matches → fires
	day15 := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	if !cs.matches(day15) {
		t.Error("should match 15th of month")
	}

	// March 4 (Wednesday) — DOM=4 doesn't match 15 → should NOT fire
	day4 := time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)
	if cs.matches(day4) {
		t.Error("should NOT match day 4 — DOM restricted to 15, DOW star-equivalent")
	}
}

func TestParseCron_NamedDOW(t *testing.T) {
	cs, err := parseCron("0 2 * * MON")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cs.dow[1] {
		t.Error("dow[1] (Monday) should be true")
	}
	for d := 2; d <= 6; d++ {
		if cs.dow[d] {
			t.Errorf("dow[%d] should be false", d)
		}
	}
	if cs.dow[0] {
		t.Error("dow[0] (Sunday) should be false")
	}
}

func TestParseCron_NamedDOWRange(t *testing.T) {
	cs, err := parseCron("0 9 * * MON-FRI")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for d := 1; d <= 5; d++ {
		if !cs.dow[d] {
			t.Errorf("dow[%d] should be true for MON-FRI", d)
		}
	}
	if cs.dow[0] {
		t.Error("dow[0] (Sunday) should be false")
	}
	if cs.dow[6] {
		t.Error("dow[6] (Saturday) should be false")
	}
}

func TestParseCron_NamedDOWList(t *testing.T) {
	cs, err := parseCron("0 9 * * MON,WED,FRI")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := [7]bool{false, true, false, true, false, true, false}
	for d := 0; d < 7; d++ {
		if cs.dow[d] != want[d] {
			t.Errorf("dow[%d]: got %v, want %v", d, cs.dow[d], want[d])
		}
	}
}

func TestParseCron_NamedDOWCaseInsensitive(t *testing.T) {
	cs, err := parseCron("0 2 * * mon")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cs.dow[1] {
		t.Error("dow[1] (Monday) should be true for lowercase 'mon'")
	}
}

func TestParseCron_NamedMonths(t *testing.T) {
	cs, err := parseCron("0 0 1 JAN *")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cs.month[0] {
		t.Error("month[0] (January) should be true")
	}
	for m := 1; m < 12; m++ {
		if cs.month[m] {
			t.Errorf("month[%d] should be false", m)
		}
	}
}

func TestParseCron_NamedMonthRange(t *testing.T) {
	cs, err := parseCron("0 0 1 JAN-MAR *")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for m := 0; m < 3; m++ {
		if !cs.month[m] {
			t.Errorf("month[%d] should be true for JAN-MAR", m)
		}
	}
	for m := 3; m < 12; m++ {
		if cs.month[m] {
			t.Errorf("month[%d] should be false for JAN-MAR", m)
		}
	}
}

func TestParseCron_Predefined(t *testing.T) {
	cases := map[string]string{
		"@daily":    "0 0 * * *",
		"@midnight": "0 0 * * *",
		"@hourly":   "0 * * * *",
		"@weekly":   "0 0 * * 0",
		"@monthly":  "0 0 1 * *",
		"@yearly":   "0 0 1 1 *",
		"@annually": "0 0 1 1 *",
	}
	for shortcut, equivalent := range cases {
		t.Run(shortcut, func(t *testing.T) {
			got, err := parseCron(shortcut)
			if err != nil {
				t.Fatalf("unexpected error for %s: %v", shortcut, err)
			}
			want, err := parseCron(equivalent)
			if err != nil {
				t.Fatalf("unexpected error for %s: %v", equivalent, err)
			}
			if got.minute != want.minute || got.hour != want.hour ||
				got.dom != want.dom || got.month != want.month ||
				got.dow != want.dow {
				t.Errorf("%s should be equivalent to %s", shortcut, equivalent)
			}
		})
	}
}

func TestParseCron_SixFieldError(t *testing.T) {
	_, err := parseCron("0 0 * * * *")
	if err == nil {
		t.Fatal("expected error for 6-field cron")
	}
	if !strings.Contains(err.Error(), "seconds field") {
		t.Errorf("error should mention seconds field, got: %v", err)
	}
}

func TestParseCron_NamedDOWStepRange(t *testing.T) {
	// MON-FRI/2 = Mon(1), Wed(3), Fri(5)
	cs, err := parseCron("0 0 * * MON-FRI/2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := [7]bool{false, true, false, true, false, true, false}
	for d := 0; d < 7; d++ {
		if cs.dow[d] != want[d] {
			t.Errorf("dow[%d]: got %v, want %v", d, cs.dow[d], want[d])
		}
	}
}

func TestValidateCron_Valid(t *testing.T) {
	for _, expr := range []string{"0 3 * * *", "@daily", "@weekly", "0 2 * * MON", "0 0 1 JAN *"} {
		if err := ValidateCron(expr); err != nil {
			t.Errorf("ValidateCron(%q) = %v, want nil", expr, err)
		}
	}
}

func TestValidateCron_Invalid(t *testing.T) {
	for _, expr := range []string{"0 2 * * * MON", "not cron", ""} {
		if err := ValidateCron(expr); err == nil {
			t.Errorf("ValidateCron(%q) = nil, want error", expr)
		}
	}
}

func TestParseCron_NamedMonthStepRange(t *testing.T) {
	// JAN-MAR/2 = Jan(0), Mar(2) — skip Feb(1)
	cs, err := parseCron("0 0 1 JAN-MAR/2 *")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := [12]bool{true, false, true}
	for m := 0; m < 12; m++ {
		if cs.month[m] != want[m] {
			t.Errorf("month[%d]: got %v, want %v", m, cs.month[m], want[m])
		}
	}
}

func TestParseCron_DOWRange6to7(t *testing.T) {
	// 6-7 = Sat(6) and Sun(0 via 7-normalization)
	cs, err := parseCron("0 0 * * 6-7")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cs.dow[6] {
		t.Error("dow[6] (Saturday) should be true")
	}
	if !cs.dow[0] {
		t.Error("dow[0] (Sunday via 7) should be true")
	}
	for d := 1; d <= 5; d++ {
		if cs.dow[d] {
			t.Errorf("dow[%d] should be false", d)
		}
	}
}

func TestParseCron_NamedDOWMixedList(t *testing.T) {
	// MON,3,FRI = 1,3,5
	cs, err := parseCron("0 0 * * MON,3,FRI")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := [7]bool{false, true, false, true, false, true, false}
	for d := 0; d < 7; d++ {
		if cs.dow[d] != want[d] {
			t.Errorf("dow[%d]: got %v, want %v", d, cs.dow[d], want[d])
		}
	}
}

func TestParseCron_NamedMonthInvalidAfterReplace(t *testing.T) {
	// "JANX" resolves to "1X" which should fail during numeric parsing.
	_, err := parseCron("0 0 1 JANX *")
	if err == nil {
		t.Fatal("expected error for invalid month token after name resolution")
	}
}
