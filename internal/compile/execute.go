package compile

import (
	"fmt"
	"time"
)

func (p *Program) executeInner(s string, slen int) (time.Time, error) {
	var (
		year       = 0
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

	for i := 0; i < p.N; i++ {
		inst := &p.Insts[i]
		off := int(inst.Offset)

		switch inst.Op {
		case OpYear4:
			if off+4 > slen {
				return time.Time{}, fieldError("year", off, slen)
			}
			y, ok := parse4Digits(s, off)
			if !ok {
				return time.Time{}, fieldError("year", off, slen)
			}
			year = y

		case OpYear2:
			if off+2 > slen {
				return time.Time{}, fieldError("year", off, slen)
			}
			y, ok := parse2Digits(s, off)
			if !ok {
				return time.Time{}, fieldError("year", off, slen)
			}
			if y >= 69 {
				year = 1900 + y
			} else {
				year = 2000 + y
			}

		case OpMonth2:
			if off+2 > slen {
				return time.Time{}, fieldError("month", off, slen)
			}
			m, ok := parse2Digits(s, off)
			if !ok || m < 1 || m > 12 {
				return time.Time{}, fieldError("month", off, slen)
			}
			month = time.Month(m)

		case OpMonth1or2:
			m, n, ok := parse1or2Digits(s, off, slen)
			if !ok || m < 1 || m > 12 {
				return time.Time{}, fieldError("month", off, slen)
			}
			month = time.Month(m)
			_ = n // consumed bytes — variable-width handled by compiler

		case OpMonthName:
			month = time.Month(inst.Aux)

		case OpDay2:
			if off+2 > slen {
				return time.Time{}, fieldError("day", off, slen)
			}
			d, ok := parse2Digits(s, off)
			if !ok || d < 1 || d > 31 {
				return time.Time{}, fieldError("day", off, slen)
			}
			day = d

		case OpDay1or2:
			d, n, ok := parse1or2Digits(s, off, slen)
			if !ok || d < 1 || d > 31 {
				return time.Time{}, fieldError("day", off, slen)
			}
			day = d
			_ = n

		case OpHour24:
			if off+2 > slen {
				return time.Time{}, fieldError("hour", off, slen)
			}
			h, ok := parse2Digits(s, off)
			if !ok || h > 23 {
				return time.Time{}, fieldError("hour", off, slen)
			}
			hour = h

		case OpHour12:
			if off+2 > slen {
				return time.Time{}, fieldError("hour", off, slen)
			}
			h, ok := parse2Digits(s, off)
			if !ok || h < 1 || h > 12 {
				return time.Time{}, fieldError("hour", off, slen)
			}
			hour = h

		case OpHour1or2:
			h, _, ok := parse1or2Digits(s, off, slen)
			if !ok || h > 23 {
				return time.Time{}, fieldError("hour", off, slen)
			}
			hour = h

		case OpMinute2:
			if off+2 > slen {
				return time.Time{}, fieldError("minute", off, slen)
			}
			m, ok := parse2Digits(s, off)
			if !ok || m > 59 {
				return time.Time{}, fieldError("minute", off, slen)
			}
			minute = m

		case OpSecond2:
			if off+2 > slen {
				return time.Time{}, fieldError("second", off, slen)
			}
			sec, ok := parse2Digits(s, off)
			if !ok || sec > 60 { // 60 for leap second
				return time.Time{}, fieldError("second", off, slen)
			}
			second = sec

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

		case OpTZZ:
			loc = time.UTC

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

		case OpISOWeek:
			if off+2 > slen {
				return time.Time{}, fieldError("iso week", off, slen)
			}
			w, ok := parse2Digits(s, off)
			if !ok || w < 1 || w > 53 {
				return time.Time{}, fieldError("iso week", off, slen)
			}
			isoWeek = w

		case OpISOWeekDay:
			if off >= slen {
				return time.Time{}, fieldError("iso weekday", off, slen)
			}
			d := s[off] - '0'
			if d < 1 || d > 7 {
				return time.Time{}, fieldError("iso weekday", off, slen)
			}
			isoWeekDay = int(d)

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

		case OpLiteral, OpSkip:
			// Nothing to extract — these exist for offset tracking.
		}
	}

	// Apply AM/PM conversion.
	if ampm == -1 { // PM
		if hour != 12 {
			hour += 12
		}
	} else if ampm == 1 { // AM
		if hour == 12 {
			hour = 0
		}
	}

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
		t := time.Date(year, 1, ordinalDay, hour, minute, second, nsec, loc)
		return t, nil
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

// parse1or2Digits parses one or two ASCII decimal digits starting at s[off].
// Returns the value, number of bytes consumed, and success.
func parse1or2Digits(s string, off int, slen int) (int, int, bool) {
	if off >= slen {
		return 0, 0, false
	}
	d0 := s[off] - '0'
	if d0 > 9 {
		return 0, 0, false
	}
	if off+1 < slen {
		d1 := s[off+1] - '0'
		if d1 <= 9 {
			return int(d0)*10 + int(d1), 2, true
		}
	}
	return int(d0), 1, true
}

// parseFracSec parses fractional seconds and returns nanoseconds.
func parseFracSec(s string, off, length int) (int, bool) {
	val := 0
	for i := 0; i < length; i++ {
		d := s[off+i] - '0'
		if d > 9 {
			return 0, false
		}
		val = val*10 + int(d)
	}
	// Scale to nanoseconds: if length=3 (millis), multiply by 1e6; length=6 (micros), 1e3; etc.
	for i := length; i < 9; i++ {
		val *= 10
	}
	return val, true
}

// parseTZOffset parses a timezone offset like +05:30, -0800, +00:00, +00.
func parseTZOffset(s string, off, length int) (*time.Location, bool) {
	if length < 3 {
		return nil, false
	}

	sign := 1
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
	name := s[off : off+length]
	return time.FixedZone(name, totalSeconds), true
}

// lookupTZAbbr resolves a timezone abbreviation to a *time.Location.
func lookupTZAbbr(name string) (*time.Location, bool) {
	switch name {
	case "UTC":
		return time.UTC, true
	case "GMT":
		return time.FixedZone("GMT", 0), true
	case "EST":
		return time.FixedZone("EST", -5*3600), true
	case "EDT":
		return time.FixedZone("EDT", -4*3600), true
	case "CST":
		return time.FixedZone("CST", -6*3600), true
	case "CDT":
		return time.FixedZone("CDT", -5*3600), true
	case "MST":
		return time.FixedZone("MST", -7*3600), true
	case "MDT":
		return time.FixedZone("MDT", -6*3600), true
	case "PST":
		return time.FixedZone("PST", -8*3600), true
	case "PDT":
		return time.FixedZone("PDT", -7*3600), true
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
