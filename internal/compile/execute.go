package compile

import (
	"fmt"
	"time"
)

// parse2Bounded extracts a 2-digit field at s[off:off+2] and validates it is within [lo, hi].
// Returns (value, true) on success, (0, false) on failure.
// Kept minimal for inlining — callers handle error construction.
//
// The length comes from s and is not passed in. It used to be a slen parameter,
// which every caller filled with len(s) and the compiler could not know that:
// an opaque int bounds nothing, so `off+2 > slen` returning false proved
// nothing about s and the two index expressions below both kept their bounds
// checks. Asking s directly is what lets the prove pass discharge them.
func parse2Bounded(s string, off, lo, hi int) (int, bool) {
	if off+2 > len(s) {
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
func parse1or2Bounded(s string, off, lo, hi int) (int, bool) {
	if off >= len(s) {
		return 0, false
	}
	d0 := s[off] - '0'
	if d0 > 9 {
		return 0, false
	}
	v := int(d0)
	if off+1 < len(s) {
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
func consumed1or2(s string, off int) int {
	if off+1 < len(s) && s[off+1] >= '0' && s[off+1] <= '9' {
		return 2
	}
	return 1
}

// executeInner takes the length from s rather than as an argument. It used to
// take a slen, and both callers passed len(s), and the compiler had no way to
// know that: an opaque int cannot bound an index, so every read in this function
// and in the extractors it inlines kept a bounds check that the surrounding test
// had already made redundant. The local below is the same value by construction,
// which is the point.
// numericW returns the total width a fixed-width numeric instruction accounts
// for: its own w, plus one for a separator fused onto it, and reports whether
// that separator matched. See the Aux convention in instructions.go.
//
// Kept minimal for inlining, like the extractors above. sepNone is the common
// case and returns without looking at the input at all, so an instruction with
// nothing fused onto it pays one compare against a register.
func numericW(s string, off, w int, aux uint16) (int, bool) {
	if aux == sepNone {
		return w, true
	}
	at := off + w
	if at >= len(s) {
		return 0, false
	}
	// What the byte has to be is Aux's three readings, and litAccepts answers
	// all three. The class reading is the one that carries a trie format's
	// separator: TIME_HMS declares a colon at 2 and at 5, and asking only that
	// those bytes were not digits let it read "12/25/24" as 12:25:24.
	if !litAccepts(aux, s[at]) {
		return 0, false
	}
	return w + 1, true
}

func (p *Program) executeInner(s string) (time.Time, error) {
	// A program whose fields all sit at fixed offsets does not need
	// interpreting, and interpreting it was costing more than the extractions.
	// See planFast.
	if p.isFast() {
		return p.executeFast(s)
	}

	slen := len(s)
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

	// Slice once rather than index p.Insts per iteration. N is a plain int field
	// with no proven relation to the array's length, so the compiler kept a
	// bounds check on every instruction of every parse, and reloaded N through
	// the pointer with it.
	//
	// The clamp is what makes the slice expression safe. Compile refuses a def
	// over the limit before it fills anything, so N is in range for every Program
	// this package builds; but Program is a plain struct with exported fields,
	// and a slice expression panics where an index expression would have read a
	// zero Inst. Truncating instead leaves the program describing fewer bytes
	// than the input, which the coverage check below turns into an error. "Parse
	// never panics on any input" is an invariant, and this is cheaper than
	// arguing that no caller can ever build such a Program.
	n := p.N
	if n > MaxInstructions {
		n = MaxInstructions
	}
	insts := p.Insts[:n]

	for i := range insts {
		inst := &insts[i]
		off := int(inst.Offset) + delta
		if off < 0 {
			return time.Time{}, fieldError("instruction offset", off, slen)
		}
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
			w, ok = numericW(s, off, 4, inst.Aux)
			if !ok {
				return time.Time{}, fieldError("separator after year", off+4, slen)
			}

		case OpYear2:
			v, ok := parse2Bounded(s, off, 0, 99)
			if !ok {
				return time.Time{}, fieldError("year", off, slen)
			}
			year = NormalizeTwoDigitYear(v)
			yearSet = true
			w, ok = numericW(s, off, 2, inst.Aux)
			if !ok {
				return time.Time{}, fieldError("separator after year", off+2, slen)
			}

		case OpMonth2:
			v, ok := parse2Bounded(s, off, 1, 12)
			if !ok {
				return time.Time{}, fieldError("month", off, slen)
			}
			month = time.Month(v)
			w, ok = numericW(s, off, 2, inst.Aux)
			if !ok {
				return time.Time{}, fieldError("separator after month", off+2, slen)
			}

		case OpMonth1or2:
			v, ok := parse1or2Bounded(s, off, 1, 12)
			if !ok {
				return time.Time{}, fieldError("month", off, slen)
			}
			month = time.Month(v)
			w = consumed1or2(s, off)
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
			v, ok := parse2Bounded(s, off, 1, 31)
			if !ok {
				return time.Time{}, fieldError("day", off, slen)
			}
			day = v
			w, ok = numericW(s, off, 2, inst.Aux)
			if !ok {
				return time.Time{}, fieldError("separator after day", off+2, slen)
			}

		case OpDay1or2:
			v, ok := parse1or2Bounded(s, off, 1, 31)
			if !ok {
				return time.Time{}, fieldError("day", off, slen)
			}
			day = v
			w = consumed1or2(s, off)
			delta += w - int(inst.Len)

		case OpDaySpacePad:
			// Go's "_2" token: exactly two bytes, either a space and a digit
			// or two digits. It mapped to OpDay1or2, which computes s[off]-'0'
			// on the leading space, reads 251, and refuses. Only the two-digit
			// half of what the token advertises was ever implemented.
			//
			// This is its own op rather than a widening of parse1or2Bounded,
			// which is on the hot path for NUMERIC_MDY, NUMERIC_DMY and
			// CJK_DATE, and none of those can carry a leading space.
			if off+2 > slen {
				return time.Time{}, fieldError("day", off, slen)
			}
			var v int
			if s[off] == ' ' {
				d := s[off+1] - '0'
				if d > 9 {
					return time.Time{}, fieldError("day", off, slen)
				}
				v = int(d)
			} else {
				pv, ok := parse2Digits(s, off)
				if !ok {
					return time.Time{}, fieldError("day", off, slen)
				}
				v = pv
			}
			if v < 1 || v > 31 {
				return time.Time{}, fieldError("day", off, slen)
			}
			day = v
			sw, sok := numericW(s, off, 2, inst.Aux)
			if !sok {
				return time.Time{}, fieldError("separator after day", off+2, slen)
			}
			w = sw

		// ── Time fields ───────────────────────────────────────────��──

		case OpHour24:
			v, ok := parse2Bounded(s, off, 0, 23)
			if !ok {
				return time.Time{}, fieldError("hour", off, slen)
			}
			hour = v
			w, ok = numericW(s, off, 2, inst.Aux)
			if !ok {
				return time.Time{}, fieldError("separator after hour", off+2, slen)
			}

		case OpHour12:
			v, ok := parse2Bounded(s, off, 1, 12)
			if !ok {
				return time.Time{}, fieldError("hour", off, slen)
			}
			hour = v
			w, ok = numericW(s, off, 2, inst.Aux)
			if !ok {
				return time.Time{}, fieldError("separator after hour", off+2, slen)
			}

		case OpHour1or2:
			v, ok := parse1or2Bounded(s, off, 0, 23)
			if !ok {
				return time.Time{}, fieldError("hour", off, slen)
			}
			hour = v
			w = consumed1or2(s, off)
			delta += w - int(inst.Len)

		case OpMinute2:
			v, ok := parse2Bounded(s, off, 0, 59)
			if !ok {
				return time.Time{}, fieldError("minute", off, slen)
			}
			minute = v
			w, ok = numericW(s, off, 2, inst.Aux)
			if !ok {
				return time.Time{}, fieldError("separator after minute", off+2, slen)
			}

		case OpSecond2:
			v, ok := parse2Bounded(s, off, 0, 60) // 60 for leap second
			if !ok {
				return time.Time{}, fieldError("second", off, slen)
			}
			second = v
			w, ok = numericW(s, off, 2, inst.Aux)
			if !ok {
				return time.Time{}, fieldError("separator after second", off+2, slen)
			}

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
				// This was the one variable-width op that did not report its
				// width back, so a field after it read from wherever the
				// offset form would have ended. The Op*1or2 arms have always
				// done this. Only the 'Z' branch is narrower than declared;
				// the offset branch consumes exactly inst.Len and adds zero,
				// so it does not pay for the arithmetic.
				delta += 1 - int(inst.Len)
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
			v, ok := parse2Bounded(s, off, 1, 53)
			if !ok {
				return time.Time{}, fieldError("iso week", off, slen)
			}
			isoWeek = v
			w, ok = numericW(s, off, 2, inst.Aux)
			if !ok {
				return time.Time{}, fieldError("separator after iso week", off+2, slen)
			}

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
			// Aux says what the byte has to be, under the three readings in
			// instructions.go. ParseGoLayout and lit() name the exact byte,
			// because both read it off an input they had in front of them.
			//
			// A trie entry cannot name a byte, because it matches on a
			// signature of character classes and one entry serves every byte in
			// the class: ISO8601_DATE reads "2024-03-15", "2024/03/15" and
			// "2024.03.15", all three CSep at offsets 4 and 7. What it can name
			// is that class, and buildTrie stamps it here from the signature
			// position the literal sits at.
			//
			// The class is what separates two formats of the same shape.
			// NUMERIC_MDY and TIME_HMS are both DD?DD?DD, so while this asked
			// only that the byte was not a digit, a layout detected from
			// "20-1-00" read "10:01:00" as the tenth of January 2000.
			//
			// The loop covers the whole declared width. Only buildDatePartFields
			// emits a literal wider than one byte, one spanning the run between
			// two date parts, and the bound refuses nothing the coverage check
			// below would have allowed: a literal running past the end leaves
			// end > slen.
			if off+w > slen {
				return time.Time{}, fieldError("literal", off, slen)
			}
			for j := off; j < off+w; j++ {
				if !litAccepts(inst.Aux, s[j]) {
					return time.Time{}, fieldError("literal", j, slen)
				}
			}

		case OpSkip:
			// Nothing to extract, but the run is not unconstrained either: it
			// may not contain a digit.
			//
			// A skip covers bytes the detector scanned past — a weekday name,
			// punctuation, an ordinal suffix. What made them skippable is that
			// they held no value, and what told the detector they held no value
			// is that they were not digits. Every textual detector dispatches
			// on the numeric tokens it finds: how many there are, how wide they
			// are, and whether each is over 31. A digit inside a skip is a
			// token detection would have seen and this program does not model,
			// so the two read the same input differently.
			//
			// Parse("MAY A1") builds a month name, a skip of width 2 over " A",
			// and a 1-or-2 digit day. Against "MAY 15" the skip swallows " 1"
			// and the day reads "5", for May 5th; detection reads May 15th.
			// Against "MAY1010" the skip swallows "10", the day widens to "10",
			// and 3 + 2 + 2 still equals 7, so the coverage check in this
			// function cannot see it. Both now fail here instead.
			//
			// The bound is needed because this reads the bytes. It refuses
			// nothing the coverage check below would have allowed: a skip
			// running past the end leaves end > slen.
			if off+w > slen {
				return time.Time{}, fieldError("skipped run", off, slen)
			}
			for j := off; j < off+w; j++ {
				if s[j] >= '0' && s[j] <= '9' {
					return time.Time{}, fieldError("skipped run", j, slen)
				}
			}

		case OpTail:
			// The remainder of the input is deliberately ignored. Emitted only
			// by GO_TIME_STRING, whose trailing zone name and monotonic clock
			// suffix ("m=+0.000000001") no fixed-width program can describe.
			// Everything that decides the instant has already been read.
			w = slen - off

		case OpNop:
			// An unused slot in a fast program. It reads nothing and covers
			// nothing, which is what lets the interpreter run a program in slot
			// form and reach the same answer. TestFastAgreesWithInterpreter
			// depends on that and is the cross-check on the whole fast path.
			w = 0
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
		year = int(p.BaseYear)
	}

	// Apply AM/PM conversion.
	hour = applyAMPM(hour, ampm)

	// ISO week date: convert year + week + weekday to calendar date.
	//
	// The instruction range-checks the week at 1..53, which is a constant, and
	// most years have 52. time.Date normalises rather than refusing, so week 53
	// of a 52-week year used to roll into the next one: "2023-W53" came back as
	// 2024-01-01 and "2024-W53" as 2024-12-30, which is really 2025-W01-1.
	//
	// The check has to be here rather than in the detector, because this is the
	// one place that has the week and the year together.
	if isoWeek > 0 {
		week1Monday, weeks := isoWeek1(year)
		if isoWeek > weeks {
			return time.Time{}, fmt.Errorf(
				"dateparsa: ISO week %d does not exist in %d, which has %d weeks",
				isoWeek, year, weeks)
		}
		wd := time.Monday
		if isoWeekDay > 0 {
			// ISO weekday: 1=Mon, 7=Sun. Go: time.Monday=1, time.Sunday=0.
			wd = time.Weekday(isoWeekDay % 7) // 1->1(Mon), 7->0(Sun)
		}
		t := isoWeekToDate(week1Monday, isoWeek, wd)
		return makeTime(t.Year(), t.Month(), t.Day(), hour, minute, second, nsec, loc), nil
	}

	// Ordinal day: convert year + day-of-year to calendar date.
	//
	// Same defect, same shape: the instruction checks 1..366 against a constant,
	// so "2023-366" returned 2024-01-01 with a nil error where time.Parse
	// refuses it by name. "1900-366" returned 1901-01-01, which is the case
	// that needs the full leap rule and not just a test for divisibility by 4.
	if ordinalDay > 0 {
		if ordinalDay > daysInYear(year) {
			return time.Time{}, fmt.Errorf(
				"dateparsa: day-of-year %d does not exist in %d, which has %d days",
				ordinalDay, year, daysInYear(year))
		}
		return makeTime(year, 1, ordinalDay, hour, minute, second, nsec, loc), nil
	}

	return makeTime(year, month, day, hour, minute, second, nsec, loc), nil
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

// MaxFracDigits is how many fractional digits a nanosecond holds, and therefore
// the widest OpFracSec field this package will execute.
//
// Exported because detect has to honour it: a detector emitting a wider field
// produces a parse error rather than a wrong instant, which is safe but is not the
// error anybody wants. One constant read from both places, rather than two held
// together by a test that somebody has to remember to write.
const MaxFracDigits = 9

// parseFracSec parses fractional seconds and returns nanoseconds.
//
// Every one of the length bytes has to be a digit, and only the first nine are
// read. Those are two separate jobs and conflating them was the bug. The digit
// check covers the whole declared field because the executor's coverage
// accounting says the field is that wide, so a non-digit at byte twelve of a
// twelve-digit fraction still has to refuse; the value has to stop at nine
// because that is all a nanosecond holds.
//
// It used to accumulate the whole run and then scale with `for i := length; i < 9`,
// which does nothing at all once length is over nine. So the digits past the ninth
// were added and never divided back out: ten digits moved the answer by up to nine
// seconds and twenty wrapped int64 and moved it by years, with a nil error.
// "2024-03-15 10:30:00.99999999999999999999999" came back as 2030-07-21 where
// time.Parse returns 2024-03-15.
//
// This is C3 over again, in the file C3 did not touch. internal/epoch's
// parseFractional cut its fraction to nine digits before parsing on 2026-08-13, for
// the same reason and with the same wrapped-int64 symptom, and nobody checked
// whether the other fractional parser in the tree had the same defect. It did.
// A field declared wider than a nanosecond holds is refused here, and that
// refusal is the whole fix. It is one compare in front of the loop this function
// has always run, so every format that carries a fraction pays nothing.
//
// Truncating instead was tried first, because it is what time.Parse does with the
// same input, and it was measurably worse for the formats that are not broken.
// Reading nine digits and validating the rest needs work inside this function
// however it is arranged, and this function is inlined into both executors, so
// adding any costs the inline. Three versions, measured on linux/arm64:
//
//	a counter tested per digit   FracSec3 +33%   FracSec9 +23%   Layout.Parse +8.8%
//	two loops in one function    FracSec3 +37%   FracSec9 +10%
//	the long case split out      FracSec3 +25%   FracSec9 +11%   inline lost in both
//	                                                             executors
//	this                         flat            flat            inline kept
//
// So the producers bound the field instead, and an over-long fraction is refused
// at detection rather than truncated at execution. That is stricter than the
// stdlib, and CLAUDE.md says that is the decision: "staying stricter than the
// stdlib is the decision. Do not try to enumerate the stdlib's leniency."
//
// The check stays here as well as in the producers because Program is a plain
// struct with exported fields, and Compile is not the only way to fill one. It is
// the same argument as the MaxInstructions clamp in executeInner: cheaper than
// proving no caller can ever build such a Program, and the failure it prevents is
// a wrong instant rather than a panic.
func parseFracSec(s string, off, length int) (int, bool) {
	if length > MaxFracDigits {
		return 0, false
	}
	val := 0
	for i := range length {
		d := s[off+i] - '0'
		if d > 9 {
			return 0, false
		}
		val = val*10 + int(d)
	}
	// Scale to nanoseconds: three digits are millis and multiply by 1e6, nine are
	// already nanoseconds.
	for i := length; i < MaxFracDigits; i++ { //nolint:rangeint
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
//
// Every abbreviation this library accepts is a fixed offset, taken as written.
// The four European names carried a second meaning until now, because they are
// tzdata zone names as well as abbreviations and fell through to
// time.LoadLocation, which applies the zone's daylight rules:
//
//	"2024-07-15 10:30:00 CET"   was +0200 CEST, an hour off what it says
//	"2024-07-15 10:30:00 EST"   was -0500 EST, taken as written
//
// Two rules for the same shape of input, and the one that read the calendar
// silently overrode the abbreviation the caller wrote. They are fixed offsets
// here for the same reason EST is: an abbreviation names an offset, and a
// caller who means the summer one writes CEST.
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
	tzHST = time.FixedZone("HST", -10*3600)
	tzCET = time.FixedZone("CET", 1*3600)
	tzEET = time.FixedZone("EET", 2*3600)
	tzMET = time.FixedZone("MET", 1*3600)
	tzWET = time.FixedZone("WET", 0)
)

// lookupTZAbbr resolves a timezone abbreviation to a *time.Location. Every
// answer is a pre-built fixed offset, so this allocates nothing and reads
// nothing.
//
// It used to fall through to time.LoadLocation for anything not listed, which
// reads the zone file on every call and does not cache: Layout.Parse allocated
// 24 times on "2024-03-15 10:30:00 CET" while README promised zero. It was also
// the library's only filesystem access, documented as such in SECURITY.md, and
// it is gone.
//
// The fallback reached exactly nine names beyond this list, enumerated rather
// than guessed by asking LoadLocation about all 17576 three-letter strings.
// Four are the European abbreviations above and two, HST and UCT, are listed
// here now. The other three were PRC, ROC and ROK, which are tzdata aliases for
// countries rather than timezone abbreviations, and nobody writes them in a
// timestamp. They are refused.
func lookupTZAbbr(name string) (*time.Location, bool) {
	switch name {
	case "UTC", "UCT":
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
	case "HST":
		return tzHST, true
	case "CET":
		return tzCET, true
	case "EET":
		return tzEET, true
	case "MET":
		return tzMET, true
	case "WET":
		return tzWET, true
	}
	return nil, false
}

// isLeap is the full Gregorian rule. Testing only for divisibility by 4 gets
// 1900 and 2100 wrong, and 1900 is inside the range this library parses.
func isLeap(y int) bool {
	return y%4 == 0 && (y%100 != 0 || y%400 == 0)
}

func daysInYear(y int) int {
	if isLeap(y) {
		return 366
	}
	return 365
}

// isoWeek1 returns the Monday of ISO week 1 of isoYear, and how many ISO weeks
// that year has.
//
// Both answers come from the weekday of 4 January, which is why they are
// computed together: 4 January is always in ISO week 1, and 1 January is three
// days before it. Building a second time.Date to ask about 1 January measured
// +10.67% on Parse_ISOWeekDate for an answer already in hand.
//
// A year has 53 weeks when 1 January is a Thursday, or when it is a leap year
// and 1 January is a Wednesday. Those are the two cases where the year holds
// 53 Thursdays, and ISO week 1 is the week containing the first Thursday.
// Every other year has 52. Being a leap year is not on its own enough, which
// is why "2024-W53" used to come back as 2024-12-30.
func isoWeek1(isoYear int) (time.Time, int) {
	jan4 := time.Date(isoYear, 1, 4, 0, 0, 0, 0, time.UTC)
	jan4wd := jan4.Weekday()

	offset := int(time.Monday - jan4wd)
	if offset > 0 {
		offset -= 7
	}

	jan1wd := time.Weekday((int(jan4wd) + 4) % 7) // three days earlier
	weeks := 52
	if jan1wd == time.Thursday || (isLeap(isoYear) && jan1wd == time.Wednesday) {
		weeks = 53
	}

	return jan4.AddDate(0, 0, offset), weeks
}

// isoWeekToDate advances from the Monday of ISO week 1 to the requested week
// and weekday.
func isoWeekToDate(week1Monday time.Time, isoWeek int, weekday time.Weekday) time.Time {
	daysFromMonday := int(weekday - time.Monday)
	if daysFromMonday < 0 {
		daysFromMonday += 7
	}
	return week1Monday.AddDate(0, 0, (isoWeek-1)*7+daysFromMonday)
}

func fieldError(field string, off, slen int) error {
	return fmt.Errorf("dateparsa: invalid %s at offset %d (input length %d)", field, off, slen)
}

// lengthError is what a fast program returns in place of the interpreter's
// coverage check. It reports the same thing in the same shape: the program
// describes a number of bytes and the input has a different number.
func lengthError(slen, want int) error {
	return fmt.Errorf("dateparsa: layout describes %d of %d bytes", want, slen)
}

// makeTime builds the result, and is time.Date with a shortcut for UTC.
//
// time.Date costs 8.3 ns of a parse that costs 22 to 29, which makes it the
// largest single line in this package by some way. Most of that is work this
// caller does not need: it normalises all six fields, then asks the location
// what offset applies at the instant it just computed, and for a zone with no
// transitions the answer never depended on the instant.
//
// For UTC the offset is zero by definition, so the whole of it reduces to a
// day count and some arithmetic: 5.9 ns against 8.3, measured on linux/arm64.
// Anything else keeps time.Date, which is the only thing that reads a zone's
// transitions correctly, and getting that wrong would move an instant by an
// hour rather than fail.
//
// The out-of-range values this still has to handle are real. A day is checked
// against 1..31 without reference to the month, so "2024-02-31" reaches here,
// and a second is checked against 0..60 for leap seconds. Both normalise here
// exactly as time.Date normalises them, because both are a count added to a
// running total rather than a field written into a slot.
// TestMakeTimeMatchesTimeDate and FuzzMakeTimeMatchesTimeDate hold that.
func makeTime(year int, month time.Month, day, hour, min, sec, nsec int, loc *time.Location) time.Time {
	if loc != time.UTC {
		return time.Date(year, month, day, hour, min, sec, nsec, loc)
	}
	unix := daysFromCivil(year, int(month), day)*secondsPerDay +
		int64(hour)*3600 + int64(min)*60 + int64(sec)
	return time.Unix(unix, int64(nsec)).UTC()
}

const secondsPerDay = 24 * 60 * 60

// daysFromCivil returns the number of days from 1970-01-01 to the proleptic
// Gregorian date year-month-day, which may be out of range in any field.
//
// This is Howard Hinnant's days_from_civil, shifting the year to start in March
// so that the leap day lands at the end of it and the month-length pattern
// becomes the linear (153*m+2)/5. It is exact for every year time.Date accepts
// and has no table and no branch on leap years.
//
// The month is normalised first, so a month of 13 or 0 means the next or the
// previous year, as time.Date reads them. The day is not normalised and does not
// need to be: it is added to a day count, so the 31st of February is two days
// after the 29th in a leap year and three in a common one, which is what
// time.Date answers.
func daysFromCivil(year, month, day int) int64 {
	// Fold a month outside 1..12 into the year, the way time.Date's norm does.
	if month < 1 || month > 12 {
		m := month - 1
		yAdj := m / 12
		m %= 12
		if m < 0 {
			m += 12
			yAdj--
		}
		year, month = year+yAdj, m+1
	}

	y := year
	if month <= 2 {
		y-- // March-based year: January and February belong to the one before
	}
	era := y
	if y < 0 {
		era = y - 399 // floor division towards negative infinity
	}
	era /= 400
	yoe := y - era*400            // year of era, 0..399
	mp := (month + 9) % 12        // March is 0
	doy := (153*mp+2)/5 + day - 1 // day of the March-based year
	doe := yoe*365 + yoe/4 - yoe/100 + doy
	return int64(era)*146097 + int64(doe) - 719468
}
