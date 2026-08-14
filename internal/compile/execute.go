package compile

import (
	"fmt"
	"time"
)

// parse2Bounded extracts a 2-digit field at s[off:off+2] and validates it is within [lo, hi].
// Returns (value, true) on success, (0, false) on failure.
// Kept minimal for inlining — callers handle error construction.
func parse2Bounded(s string, off, slen, lo, hi int) (int, bool) {
	if off+2 > slen {
		return 0, false
	}
	v, ok := parse2Digits(s, off)
	if !ok || v < lo || v > hi {
		return 0, false
	}
	return v, true
}

// parse1or2Bounded extracts a 1-or-2-digit field at s[off:] and validates it is within [lo, hi].
// Returns (value, true) on success, (0, false) on failure.
func parse1or2Bounded(s string, off, slen, lo, hi int) (int, bool) {
	if off >= slen {
		return 0, false
	}
	d0 := s[off] - '0'
	if d0 > 9 {
		return 0, false
	}
	v := int(d0)
	if off+1 < slen {
		if d1 := s[off+1] - '0'; d1 <= 9 {
			v = v*10 + int(d1)
		}
	}
	if v < lo || v > hi {
		return 0, false
	}
	return v, true
}

// applyAMPM converts a 12-hour clock value to 24-hour using the AM/PM flag.
// ampm: 1=AM, -1=PM, 0=unset (no conversion).
func applyAMPM(hour int, ampm int8) int {
	if ampm == -1 && hour != 12 { // PM
		return hour + 12
	}
	if ampm == 1 && hour == 12 { // AM
		return 0
	}
	return hour
}

// NormalizeTwoDigitYear converts a 2-digit year to a 4-digit year.
// Pivot: years >= 69 map to 1900s, years < 69 map to 2000s.
func NormalizeTwoDigitYear(y int) int {
	if y >= 69 {
		return 1900 + y
	}
	return 2000 + y
}

// consumed1or2 returns how many digits parse1or2Bounded actually consumed
// at position off. Checks the second character to distinguish "04" (2) from "4-" (1).
func consumed1or2(s string, off, slen int) int {
	if off+1 < slen && s[off+1] >= '0' && s[off+1] <= '9' {
		return 2
	}
	return 1
}

func (p *Program) executeInner(s string, slen int) (time.Time, error) {
	var (
		year = 0
		// yearSet distinguishes "the format has no year field" from "the year
		// field read 0". Testing year == 0 conflates them, which is how
		// Parse("0000-01-01") used to come back as the current year.
		yearSet    = false
		month      = time.January
		day        = 1
		hour       int
		minute     int
		second     int
		nsec       int
		loc        = p.Tz
		ampm       int8 // 0=unset, 1=AM, -1=PM
		isoWeek    int  // ISO week number (1-53), 0 = not set
		isoWeekDay int  // ISO weekday (1=Mon, 7=Sun), 0 = not set
		ordinalDay int  // Day of year (1-366), 0 = not set
	)

	if loc == nil {
		loc = time.UTC
	}

	// delta tracks the cumulative offset adjustment caused by variable-width
	// fields (Op*1or2) that consumed more bytes than their minimum Len.
	// For fixed-width programs (detection path), delta stays 0 — zero cost.
	var delta int

	// end is one past the last byte any instruction read, and covered is how
	// many bytes they read between them. Requiring both to equal the input
	// length is what makes a program describe the whole of it.
	//
	// end alone was the first version, and it only caught a tail: a byte
	// between two fields belonged to nothing and was never examined. That let
	// a layout built from "0000-001" accept "00000101", because ISO_ORDINAL
	// described its year and its day-of-year and nothing for the '-' between
	// them, so the compact date's "0101" was read as day-of-year 101. Every
	// detector now emits a field for every byte, so the sum is exact.
	//
	// Two counters rather than a coverage map because this runs per
	// instruction on the hot path. It assumes no two fields overlap, which
	// TestEveryInputByteIsDescribedExactlyOnce checks.
	end, covered := 0, 0

	for i := 0; i < p.N; i++ {
		inst := &p.Insts[i]
		off := int(inst.Offset) + delta
		w := int(inst.Len)

		switch inst.Op {

		// ── Date fields ──────────────────────────────────────────────

		case OpYear4:
			if off+4 > slen {
				return time.Time{}, fieldError("year", off, slen)
			}
			y, ok := parse4Digits(s, off)
			if !ok {
				return time.Time{}, fieldError("year", off, slen)
			}
			year = y
			yearSet = true
			w = 4

		case OpYear2:
			v, ok := parse2Bounded(s, off, slen, 0, 99)
			if !ok {
				return time.Time{}, fieldError("year", off, slen)
			}
			year = NormalizeTwoDigitYear(v)
			yearSet = true
			w = 2

		case OpMonth2:
			v, ok := parse2Bounded(s, off, slen, 1, 12)
			if !ok {
				return time.Time{}, fieldError("month", off, slen)
			}
			month = time.Month(v)
			w = 2

		case OpMonth1or2:
			v, ok := parse1or2Bounded(s, off, slen, 1, 12)
			if !ok {
				return time.Time{}, fieldError("month", off, slen)
			}
			month = time.Month(v)
			w = consumed1or2(s, off, slen)
			delta += w - int(inst.Len)

		case OpMonthName:
			// The month is carried in Aux, resolved when the format was
			// detected. Verify the input still names it, or a reused layout
			// answers with the month it was built from. See monthNameMatches.
			if !monthNameMatches(s, off, int(inst.Len), int(inst.Aux)) {
				return time.Time{}, fieldError("month name", off, slen)
			}
			month = time.Month(inst.Aux)

		case OpDay2:
			v, ok := parse2Bounded(s, off, slen, 1, 31)
			if !ok {
				return time.Time{}, fieldError("day", off, slen)
			}
			day = v
			w = 2

		case OpDay1or2:
			v, ok := parse1or2Bounded(s, off, slen, 1, 31)
			if !ok {
				return time.Time{}, fieldError("day", off, slen)
			}
			day = v
			w = consumed1or2(s, off, slen)
			delta += w - int(inst.Len)

		// ── Time fields ───────────────────────────────────────────��──

		case OpHour24:
			v, ok := parse2Bounded(s, off, slen, 0, 23)
			if !ok {
				return time.Time{}, fieldError("hour", off, slen)
			}
			hour = v
			w = 2

		case OpHour12:
			v, ok := parse2Bounded(s, off, slen, 1, 12)
			if !ok {
				return time.Time{}, fieldError("hour", off, slen)
			}
			hour = v
			w = 2

		case OpHour1or2:
			v, ok := parse1or2Bounded(s, off, slen, 0, 23)
			if !ok {
				return time.Time{}, fieldError("hour", off, slen)
			}
			hour = v
			w = consumed1or2(s, off, slen)
			delta += w - int(inst.Len)

		case OpMinute2:
			v, ok := parse2Bounded(s, off, slen, 0, 59)
			if !ok {
				return time.Time{}, fieldError("minute", off, slen)
			}
			minute = v
			w = 2

		case OpSecond2:
			v, ok := parse2Bounded(s, off, slen, 0, 60) // 60 for leap second
			if !ok {
				return time.Time{}, fieldError("second", off, slen)
			}
			second = v
			w = 2

		case OpFracSec:
			length := int(inst.Len)
			if off+length > slen {
				return time.Time{}, fieldError("fractional second", off, slen)
			}
			ns, ok := parseFracSec(s, off, length)
			if !ok {
				return time.Time{}, fieldError("fractional second", off, slen)
			}
			nsec = ns

		case OpAMPM:
			if off+2 > slen {
				return time.Time{}, fieldError("am/pm", off, slen)
			}
			c0 := s[off] | 0x20 // lowercase
			c1 := s[off+1] | 0x20
			if c0 == 'a' && c1 == 'm' {
				ampm = 1
			} else if c0 == 'p' && c1 == 'm' {
				ampm = -1
			} else {
				return time.Time{}, fieldError("am/pm", off, slen)
			}
			w = 2

		// ── Timezone fields ───────────────────────────────────────��──

		case OpTZZ:
			// Check the byte rather than assume it. Every producer of this
			// instruction emits it only where it saw a 'Z', but a layout is
			// reused against inputs no producer looked at, and setting UTC
			// without looking accepted "2024-03-15T10:30:00+" as UTC when the
			// value was a truncated offset.
			if off >= slen || s[off] != 'Z' {
				return time.Time{}, fieldError("timezone", off, slen)
			}
			loc = time.UTC
			w = 1

		case OpTZOffset:
			length := int(inst.Len)
			if off+length > slen {
				return time.Time{}, fieldError("timezone offset", off, slen)
			}
			tzLoc, ok := parseTZOffset(s, off, length)
			if !ok {
				return time.Time{}, fieldError("timezone offset", off, slen)
			}
			loc = tzLoc

		case OpTZName:
			length := int(inst.Len)
			if off+length > slen {
				return time.Time{}, fieldError("timezone name", off, slen)
			}
			name := s[off : off+length]
			tzLoc, ok := lookupTZAbbr(name)
			if !ok {
				return time.Time{}, fieldError("timezone name", off, slen)
			}
			loc = tzLoc

		case OpTZZOrOffset:
			if off >= slen {
				return time.Time{}, fieldError("timezone", off, slen)
			}
			if s[off] == 'Z' {
				loc = time.UTC
				w = 1
			} else {
				length := int(inst.Len)
				if off+length > slen {
					return time.Time{}, fieldError("timezone offset", off, slen)
				}
				tzLoc, ok := parseTZOffset(s, off, length)
				if !ok {
					return time.Time{}, fieldError("timezone offset", off, slen)
				}
				loc = tzLoc
			}

		// ── ISO week/ordinal fields ──────────────────────────────────

		case OpISOWeek:
			v, ok := parse2Bounded(s, off, slen, 1, 53)
			if !ok {
				return time.Time{}, fieldError("iso week", off, slen)
			}
			isoWeek = v
			w = 2

		case OpISOWeekDay:
			if off >= slen {
				return time.Time{}, fieldError("iso weekday", off, slen)
			}
			d := s[off] - '0'
			if d < 1 || d > 7 {
				return time.Time{}, fieldError("iso weekday", off, slen)
			}
			isoWeekDay = int(d)
			w = 1

		case OpOrdinalDay:
			length := int(inst.Len)
			if off+length > slen {
				return time.Time{}, fieldError("ordinal day", off, slen)
			}
			val := 0
			for j := off; j < off+length; j++ {
				d := s[j] - '0'
				if d > 9 {
					return time.Time{}, fieldError("ordinal day", off, slen)
				}
				val = val*10 + int(d)
			}
			if val < 1 || val > 366 {
				return time.Time{}, fieldError("ordinal day", off, slen)
			}
			ordinalDay = val

		// ── Structural fields ────────────────────────────────────────

		case OpLiteral:
			// When Aux is set (compiled layouts), validate the literal character.
			// Detection-path programs leave Aux=0 since the detector already validated.
			if inst.Aux != 0 {
				if off >= slen || s[off] != byte(inst.Aux) {
					return time.Time{}, fieldError("literal", off, slen)
				}
			}

		case OpSkip:
			// Nothing to extract — skip N bytes.

		case OpTail:
			// The remainder of the input is deliberately ignored. Emitted only
			// by GO_TIME_STRING, whose trailing zone name and monotonic clock
			// suffix ("m=+0.000000001") no fixed-width program can describe.
			// Everything that decides the instant has already been read.
			w = slen - off
		}

		covered += w
		if e := off + w; e > end {
			end = e
		}
	}

	// A program that read less than the whole input did not describe it.
	if end != slen || covered != slen {
		return time.Time{}, fmt.Errorf(
			"dateparsa: layout describes %d of %d bytes", covered, slen)
	}

	// A format with no year field at all takes the program's base year, so
	// that a Layout reproduces what the Parse which detected it returned.
	// Zero means leave it unset, which is what the public Compile passes and
	// what time.Parse does.
	if !yearSet && p.BaseYear != 0 {
		year = p.BaseYear
	}

	// Apply AM/PM conversion.
	hour = applyAMPM(hour, ampm)

	// ISO week date: convert year + week + weekday to calendar date.
	if isoWeek > 0 {
		wd := time.Monday
		if isoWeekDay > 0 {
			// ISO weekday: 1=Mon, 7=Sun. Go: time.Monday=1, time.Sunday=0.
			wd = time.Weekday(isoWeekDay % 7) // 1->1(Mon), 7->0(Sun)
		}
		t := isoWeekToDate(year, isoWeek, wd)
		return time.Date(t.Year(), t.Month(), t.Day(), hour, minute, second, nsec, loc), nil
	}

	// Ordinal day: convert year + day-of-year to calendar date.
	if ordinalDay > 0 {
		return time.Date(year, 1, ordinalDay, hour, minute, second, nsec, loc), nil
	}

	return time.Date(year, month, day, hour, minute, second, nsec, loc), nil
}

// parse2Digits parses two ASCII decimal digits at s[off:off+2].
func parse2Digits(s string, off int) (int, bool) {
	d0 := s[off] - '0'
	d1 := s[off+1] - '0'
	if d0 > 9 || d1 > 9 {
		return 0, false
	}
	return int(d0)*10 + int(d1), true
}

// parse4Digits parses four ASCII decimal digits at s[off:off+4].
func parse4Digits(s string, off int) (int, bool) {
	d0 := s[off] - '0'
	d1 := s[off+1] - '0'
	d2 := s[off+2] - '0'
	d3 := s[off+3] - '0'
	if d0 > 9 || d1 > 9 || d2 > 9 || d3 > 9 {
		return 0, false
	}
	return int(d0)*1000 + int(d1)*100 + int(d2)*10 + int(d3), true
}

// parseFracSec parses fractional seconds and returns nanoseconds.
func parseFracSec(s string, off, length int) (int, bool) {
	val := 0
	for i := range length {
		d := s[off+i] - '0'
		if d > 9 {
			return 0, false
		}
		val = val*10 + int(d)
	}
	// Scale to nanoseconds: if length=3 (millis), multiply by 1e6; length=6 (micros), 1e3; etc.
	for i := length; i < 9; i++ { //nolint:rangeint
		val *= 10
	}
	return val, true
}

// tzOffsetTable is a pre-built lookup table of *time.Location for common UTC
// offsets at 15-minute granularity. Eliminates time.FixedZone allocations on
// the hot path. Covers -12:00 to +14:00 (105 entries at 15-min increments).
// Index = (offsetMinutes / 15) + 48, where offsetMinutes ranges from -720 to +840.
const (
	tzTableMinOffset = -720 // -12:00 in minutes
	tzTableMaxOffset = 840  // +14:00 in minutes
	tzTableStep      = 15   // 15-minute granularity
	tzTableSize      = (tzTableMaxOffset-tzTableMinOffset)/tzTableStep + 1
)

var tzOffsetTable [tzTableSize]*time.Location

func init() {
	for i := range tzTableSize {
		minutes := tzTableMinOffset + i*tzTableStep
		seconds := minutes * 60
		sign := "+"
		absMin := minutes
		if minutes < 0 {
			sign = "-"
			absMin = -minutes
		}
		h := absMin / 60
		m := absMin % 60
		var name string
		if m == 0 {
			name = fmt.Sprintf("%s%02d:00", sign, h)
		} else {
			name = fmt.Sprintf("%s%02d:%02d", sign, h, m)
		}
		tzOffsetTable[i] = time.FixedZone(name, seconds)
	}
}

// lookupTZByOffset returns a pre-built *time.Location for the given offset
// in seconds. Returns nil if the offset is not in the lookup table (i.e., not
// at a 15-minute boundary or out of range).
func lookupTZByOffset(totalSeconds int) *time.Location {
	if totalSeconds == 0 {
		return time.UTC
	}
	minutes := totalSeconds / 60
	if minutes*60 != totalSeconds {
		return nil // not an exact minute boundary
	}
	if minutes%tzTableStep != 0 {
		return nil // not at 15-minute granularity
	}
	idx := (minutes - tzTableMinOffset) / tzTableStep
	if idx < 0 || idx >= tzTableSize {
		return nil
	}
	return tzOffsetTable[idx]
}

// parseTZOffset parses a timezone offset like +05:30, -0800, +00:00, +00.
// Uses a pre-built lookup table for common offsets to avoid allocation.
func parseTZOffset(s string, off, length int) (*time.Location, bool) {
	if length < 3 {
		return nil, false
	}

	var sign int
	switch s[off] {
	case '+':
		sign = 1
	case '-':
		sign = -1
	default:
		return nil, false
	}

	h, ok := parse2Digits(s, off+1)
	if !ok || h > 23 {
		return nil, false
	}

	var m int
	if length == 3 {
		// +HH (short form, e.g. PostgreSQL)
		m = 0
	} else if length == 5 {
		// +HHMM
		m2, ok2 := parse2Digits(s, off+3)
		if !ok2 || m2 > 59 {
			return nil, false
		}
		m = m2
	} else if length == 6 && s[off+3] == ':' {
		// +HH:MM
		m2, ok2 := parse2Digits(s, off+4)
		if !ok2 || m2 > 59 {
			return nil, false
		}
		m = m2
	} else {
		return nil, false
	}

	totalSeconds := sign * (h*3600 + m*60)

	// Fast path: look up pre-built Location from table.
	if loc := lookupTZByOffset(totalSeconds); loc != nil {
		return loc, true
	}

	// Slow path: uncommon offset, allocate via FixedZone.
	name := s[off : off+length]
	return time.FixedZone(name, totalSeconds), true
}

// Pre-built timezone abbreviation Locations — allocated once at init.
var (
	tzGMT = time.FixedZone("GMT", 0)
	tzEST = time.FixedZone("EST", -5*3600)
	tzEDT = time.FixedZone("EDT", -4*3600)
	tzCST = time.FixedZone("CST", -6*3600)
	tzCDT = time.FixedZone("CDT", -5*3600)
	tzMST = time.FixedZone("MST", -7*3600)
	tzMDT = time.FixedZone("MDT", -6*3600)
	tzPST = time.FixedZone("PST", -8*3600)
	tzPDT = time.FixedZone("PDT", -7*3600)
)

// lookupTZAbbr resolves a timezone abbreviation to a *time.Location.
// Uses pre-built Locations for common abbreviations to avoid allocation.
func lookupTZAbbr(name string) (*time.Location, bool) {
	switch name {
	case "UTC":
		return time.UTC, true
	case "GMT":
		return tzGMT, true
	case "EST":
		return tzEST, true
	case "EDT":
		return tzEDT, true
	case "CST":
		return tzCST, true
	case "CDT":
		return tzCDT, true
	case "MST":
		return tzMST, true
	case "MDT":
		return tzMDT, true
	case "PST":
		return tzPST, true
	case "PDT":
		return tzPDT, true
	default:
		// Try Go's timezone database.
		loc, err := time.LoadLocation(name)
		if err != nil {
			return nil, false
		}
		return loc, true
	}
}

// isoWeekToDate converts an ISO year, week number, and weekday to a calendar date.
func isoWeekToDate(isoYear, isoWeek int, weekday time.Weekday) time.Time {
	// Jan 4 is always in ISO week 1.
	jan4 := time.Date(isoYear, 1, 4, 0, 0, 0, 0, time.UTC)
	// Find the Monday of ISO week 1.
	offset := int(time.Monday - jan4.Weekday())
	if offset > 0 {
		offset -= 7
	}
	week1Monday := jan4.AddDate(0, 0, offset)
	// Advance to the target week and weekday.
	daysFromMonday := int(weekday - time.Monday)
	if daysFromMonday < 0 {
		daysFromMonday += 7
	}
	return week1Monday.AddDate(0, 0, (isoWeek-1)*7+daysFromMonday)
}

func fieldError(field string, off, slen int) error {
	return fmt.Errorf("dateparsa: invalid %s at offset %d (input length %d)", field, off, slen)
}
