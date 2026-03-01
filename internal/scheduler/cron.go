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

// parseCron parses a standard 5-field cron expression.
// Fields: minute(0-59) hour(0-23) dom(1-31) month(1-12) dow(0-7, 0 and 7 = Sunday)
// Supports: * (wildcard), N (literal), N-M (range), N-M/S (stepped range), */S (stepped wildcard), N,M,... (list)
func parseCron(expr string) (*cronSchedule, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("expected 5 fields, got %d", len(fields))
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
	if err := parseField(fields[3], cs.month[:], 1, 12); err != nil {
		return nil, fmt.Errorf("month: %w", err)
	}
	cs.dowStar = fields[4] == "*"
	if err := parseDOWField(fields[4], cs.dow[:]); err != nil {
		return nil, fmt.Errorf("dow: %w", err)
	}

	return cs, nil
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
