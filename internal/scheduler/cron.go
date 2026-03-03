package scheduler

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// cronSchedule holds parsed cron fields (minute, hour, dom, month, dow).
type cronSchedule struct {
	minute  [60]bool
	hour    [24]bool
	dom     [31]bool // 0-indexed: dom[0] = day 1
	month   [12]bool // 0-indexed: month[0] = January
	dow     [7]bool  // 0=Sunday
	domStar bool     // true if DOM field was *
	dowStar bool     // true if DOW field was *
}

// matches returns true if t falls within this schedule.
// Standard cron semantics: when both DOM and DOW are restricted (not *),
// the job fires if EITHER field matches.
func (cs *cronSchedule) matches(t time.Time) bool {
	if !cs.minute[t.Minute()] || !cs.hour[t.Hour()] || !cs.month[t.Month()-1] {
		return false
	}
	domMatch := cs.dom[t.Day()-1]
	dowMatch := cs.dow[t.Weekday()]
	if cs.domStar || cs.dowStar {
		return domMatch && dowMatch
	}
	return domMatch || dowMatch
}

// predefined maps @-shortcuts to their 5-field equivalents.
var predefined = map[string]string{
	"@yearly":   "0 0 1 1 *",
	"@annually": "0 0 1 1 *",
	"@monthly":  "0 0 1 * *",
	"@weekly":   "0 0 * * 0",
	"@daily":    "0 0 * * *",
	"@midnight": "0 0 * * *",
	"@hourly":   "0 * * * *",
}

// dowNames maps 3-letter day abbreviations to numeric values (0=Sunday).
var dowNames = map[string]string{
	"SUN": "0", "MON": "1", "TUE": "2", "WED": "3",
	"THU": "4", "FRI": "5", "SAT": "6",
}

// monthNames maps 3-letter month abbreviations to numeric values (1=January).
var monthNames = map[string]string{
	"JAN": "1", "FEB": "2", "MAR": "3", "APR": "4",
	"MAY": "5", "JUN": "6", "JUL": "7", "AUG": "8",
	"SEP": "9", "OCT": "10", "NOV": "11", "DEC": "12",
}

// resolveNames replaces 3-letter name tokens (case-insensitive) with their
// numeric equivalents.  Safe because cron field delimiters (,  -  /) never
// collide with the 3-letter name keys.
//
// Limitation: this is substring replacement, not token-aware.  A malformed
// token like "JAN2" would be replaced to "12" (December) instead of being
// rejected outright.  In practice this is harmless — the resulting numeric
// value is still range-checked by parseField — but callers should be aware
// that typos may silently map to unexpected months/days rather than errors.
func resolveNames(field string, names map[string]string) string {
	upper := strings.ToUpper(field)
	for name, num := range names {
		upper = strings.ReplaceAll(upper, name, num)
	}
	return upper
}

// parseCron parses a standard 5-field cron expression.
// Fields: minute(0-59) hour(0-23) dom(1-31) month(1-12) dow(0-7, 0 and 7 = Sunday)
// Supports: * (wildcard), N (literal), N-M (range), N-M/S (stepped range),
// */S (stepped wildcard), N,M,... (list), @-shortcuts (@daily, @weekly, etc.),
// 3-letter day names (MON-FRI) and month names (JAN-DEC).
func parseCron(expr string) (*cronSchedule, error) {
	expr = strings.TrimSpace(expr)
	if replacement, ok := predefined[strings.ToLower(expr)]; ok {
		expr = replacement
	}

	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf(
			"expected 5 fields (minute hour dom month dow), got %d"+
				"; if your expression has a seconds field, remove it"+
				" (6/7-field cron is not supported)",
			len(fields))
	}

	cs := &cronSchedule{}

	if err := parseField(fields[0], cs.minute[:], 0, 59); err != nil {
		return nil, fmt.Errorf("minute: %w", err)
	}
	if err := parseField(fields[1], cs.hour[:], 0, 23); err != nil {
		return nil, fmt.Errorf("hour: %w", err)
	}
	cs.domStar = fields[2] == "*"
	if err := parseField(fields[2], cs.dom[:], 1, 31); err != nil {
		return nil, fmt.Errorf("dom: %w", err)
	}
	// Detect star-equivalent expressions like */1 that set all bits.
	if !cs.domStar {
		cs.domStar = allSet(cs.dom[:])
	}
	monthField := resolveNames(fields[3], monthNames)
	if err := parseField(monthField, cs.month[:], 1, 12); err != nil {
		return nil, fmt.Errorf("month: %w", err)
	}
	dowField := resolveNames(fields[4], dowNames)
	cs.dowStar = dowField == "*"
	if err := parseDOWField(dowField, cs.dow[:]); err != nil {
		return nil, fmt.Errorf("dow: %w", err)
	}
	// Detect star-equivalent expressions like */1 that set all bits.
	if !cs.dowStar {
		cs.dowStar = allSet(cs.dow[:])
	}

	return cs, nil
}

// ValidateCron checks whether expr is a valid cron expression.
// It performs full parsing including @-shortcuts and named days/months.
// External packages (e.g., plugins) can call this at config time to reject
// invalid schedules before they reach the scheduler.
func ValidateCron(expr string) error {
	_, err := parseCron(expr)
	return err
}

// parseField parses a single cron field into a boolean slice.
// offset adjusts for 1-based fields (dom, month): value - offset = slice index.
func parseField(field string, bits []bool, min, max int) error {
	offset := min
	for _, part := range strings.Split(field, ",") {
		if err := parsePart(part, bits, min, max, offset); err != nil {
			return err
		}
	}
	return nil
}

func parsePart(part string, bits []bool, min, max, offset int) error {
	// Handle step: "X/S"
	step := 1
	if i := strings.Index(part, "/"); i >= 0 {
		s, err := strconv.Atoi(part[i+1:])
		if err != nil || s <= 0 {
			return fmt.Errorf("invalid step %q", part[i+1:])
		}
		if s > max-min+1 {
			return fmt.Errorf("step %d exceeds field range %d-%d", s, min, max)
		}
		step = s
		part = part[:i]
	}

	var lo, hi int

	switch {
	case part == "*":
		lo, hi = min, max
	case strings.HasPrefix(part, "-"):
		return fmt.Errorf("invalid value %q", part)
	case strings.Contains(part, "-"):
		rng := strings.SplitN(part, "-", 2)
		var err error
		lo, err = strconv.Atoi(rng[0])
		if err != nil {
			return fmt.Errorf("invalid range start %q", rng[0])
		}
		hi, err = strconv.Atoi(rng[1])
		if err != nil {
			return fmt.Errorf("invalid range end %q", rng[1])
		}
	default:
		v, err := strconv.Atoi(part)
		if err != nil {
			return fmt.Errorf("invalid value %q", part)
		}
		lo = v
		if step > 1 {
			hi = max // "N/S" means "N-max/S"
		} else {
			hi = v
		}
	}

	if lo < min || hi > max || lo > hi {
		return fmt.Errorf("value %d-%d out of range %d-%d", lo, hi, min, max)
	}

	for v := lo; v <= hi; v += step {
		bits[v-offset] = true
	}
	return nil
}

// parseDOWField handles the day-of-week field, normalizing 7 to 0 (Sunday).
func parseDOWField(field string, bits []bool) error {
	for _, part := range strings.Split(field, ",") {
		if err := parseDOWPart(part, bits); err != nil {
			return err
		}
	}
	return nil
}

func parseDOWPart(part string, bits []bool) error {
	step := 1
	if i := strings.Index(part, "/"); i >= 0 {
		s, err := strconv.Atoi(part[i+1:])
		if err != nil || s <= 0 {
			return fmt.Errorf("invalid step %q", part[i+1:])
		}
		if s > 7 {
			return fmt.Errorf("step %d exceeds dow range", s)
		}
		step = s
		part = part[:i]
	}

	var lo, hi int

	switch {
	case part == "*":
		lo, hi = 0, 6
	case strings.Contains(part, "-"):
		rng := strings.SplitN(part, "-", 2)
		var err error
		lo, err = strconv.Atoi(rng[0])
		if err != nil {
			return fmt.Errorf("invalid range start %q", rng[0])
		}
		hi, err = strconv.Atoi(rng[1])
		if err != nil {
			return fmt.Errorf("invalid range end %q", rng[1])
		}
	default:
		v, err := strconv.Atoi(part)
		if err != nil {
			return fmt.Errorf("invalid value %q", part)
		}
		if v == 7 {
			v = 0 // normalize Sunday alias before range expansion
		}
		lo = v
		if step > 1 {
			hi = 6 // "N/S" means "N-6/S" (DOW logical range is 0-6)
		} else {
			hi = v
		}
	}

	// Allow 0-7 (both 0 and 7 represent Sunday); validate bounds.
	if lo < 0 || hi > 7 || lo > hi {
		return fmt.Errorf("value %d-%d out of range 0-7", lo, hi)
	}

	// Iterate with 7→0 normalization inside the loop so stepped ranges
	// only include Sunday when the step actually lands on 7.
	for v := lo; v <= hi; v += step {
		if v == 7 {
			bits[0] = true
		} else {
			bits[v] = true
		}
	}
	return nil
}

// allSet returns true if every element in bits is true.
func allSet(bits []bool) bool {
	for _, b := range bits {
		if !b {
			return false
		}
	}
	return true
}
