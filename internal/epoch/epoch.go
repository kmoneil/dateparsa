// Package epoch detects and parses Unix timestamps.
//
// Supports seconds, milliseconds, microseconds, nanoseconds, and
// fractional seconds (e.g. 1710500000.123).
package epoch

import (
	"time"
)

// Kind identifies the precision of a detected timestamp.
type Kind int

const (
	KindNone  Kind = iota
	KindSec        // 10-digit: seconds since epoch
	KindMilli      // 13-digit: milliseconds
	KindMicro      // 16-digit: microseconds
	KindNano       // 19-digit: nanoseconds
	KindFrac       // Fractional: 1710500000.123
)

// Result holds a detected epoch timestamp.
type Result struct {
	Time time.Time
	Kind Kind
}

// maxSeconds bounds what this package will call a timestamp, in seconds either
// side of the epoch. It is about 3168 years, which reaches 1173 BC and 5138 AD,
// and its real job is to keep a bare number like "20240315" from being read as
// a date in the year 643360.
const maxSeconds = 1e11

// maxInt64 is math.MaxInt64, written out so this package imports nothing but
// time. See parseInt for what it is for.
const maxInt64 = 1<<63 - 1

// Detect checks if the string is a Unix timestamp and parses it.
// Returns nil if the string is not a valid timestamp.
//
// Detection heuristics:
//   - All digits (optionally with one '.') and optional leading '-'
//   - Length determines precision: 10=sec, 13=milli, 16=micro, 19=nano
//   - Fractional part detected by presence of '.'
//   - Range check: the instant must land within maxSeconds of the epoch
//
// A value too large for an int64 is refused rather than wrapped, which costs
// exactly one input a caller might have meant: -9223372036854775808, the most
// negative int64, whose digits are one past what the positive side can hold.
func Detect(s string) *Result {
	if len(s) == 0 {
		return nil
	}

	neg := false
	start := 0
	if s[0] == '-' {
		neg = true
		start = 1
		if len(s) == 1 {
			return nil
		}
	}

	// Scan for digits and at most one dot.
	dotPos := -1
	for i := start; i < len(s); i++ {
		if s[i] == '.' {
			if dotPos >= 0 {
				return nil // two dots
			}
			dotPos = i
		} else if s[i] < '0' || s[i] > '9' {
			return nil // non-digit
		}
	}

	if dotPos >= 0 {
		return parseFractional(s, start, dotPos, neg)
	}

	return parseInteger(s, start, neg)
}

func parseInteger(s string, start int, neg bool) *Result {
	digitLen := len(s) - start

	// Determine precision by digit count.
	var kind Kind
	switch {
	case digitLen == 10:
		kind = KindSec
	case digitLen == 13:
		kind = KindMilli
	case digitLen == 16:
		kind = KindMicro
	case digitLen == 19:
		kind = KindNano
	default:
		// 11-12 digits could still be seconds if in range.
		if digitLen >= 10 && digitLen <= 12 {
			kind = KindSec
		} else {
			return nil
		}
	}

	val, ok := parseInt(s[start:])
	if !ok {
		return nil
	}
	if neg {
		val = -val
	}

	// Go's division truncates toward zero, so a negative val gives a negative
	// remainder, and time.Unix documents that it accepts an nsec outside
	// [0, 999999999] and normalises it. The two agree: -1710500000123 is
	// sec -1710500000 and nsec -123000000, which is the instant it names.
	//
	// Each arm used to flip that remainder positive when the input carried a
	// minus, which moved every negative sub-second timestamp forward by twice
	// its remainder. -1710500000123 came back as -1710499999877.
	var sec, nsec int64
	switch kind {
	case KindSec:
		sec = val
	case KindMilli:
		sec, nsec = val/1e3, (val%1e3)*1e6
	case KindMicro:
		sec, nsec = val/1e6, (val%1e6)*1e3
	case KindNano:
		sec, nsec = val/1e9, val%1e9
	}

	t, ok := withinRange(sec, nsec)
	if !ok {
		return nil
	}
	return &Result{Time: t, Kind: kind}
}

// withinRange builds the instant and reports whether it lands inside
// maxSeconds of the epoch.
//
// The check is on the instant rather than on the seconds handed in, because a
// negative nsec borrows a second: time.Unix(-1e11, -1e8) is 100000000001
// seconds before the epoch, one past the bound its arguments looked like they
// respected. A fuzzer found that on "-100000000000.1" within a minute of the
// target existing.
//
// One check for every precision rather than one arm's, which is what it used
// to be. It can only fire for the seconds and the fractional forms today,
// because 13, 16 and 19 digits cannot name more than about 1e10 seconds, but
// the range in this package's doc comment is meant to describe the function
// and not one of its cases.
func withinRange(sec, nsec int64) (time.Time, bool) {
	t := time.Unix(sec, nsec).UTC()
	if u := t.Unix(); u > maxSeconds || u < -maxSeconds {
		return time.Time{}, false
	}
	return t, true
}

func parseFractional(s string, start, dotPos int, neg bool) *Result {
	// Integer part must look like epoch seconds (roughly 10 digits).
	intDigits := dotPos - start
	if intDigits < 9 || intDigits > 12 {
		return nil
	}

	sec, ok := parseInt(s[start:dotPos])
	if !ok {
		return nil
	}

	fracStr := s[dotPos+1:]
	if len(fracStr) == 0 {
		return nil
	}

	// Cut to nine digits before parsing, not after. Nine is all a nanosecond
	// holds, and the digits past it used to be parsed anyway and then divided
	// back out, which meant a long enough fraction wrapped int64 before the
	// division could discard it: "1710500000.99999999999999999999999" came
	// back as .000002003 rather than .999999999.
	if len(fracStr) > 9 {
		fracStr = fracStr[:9]
	}
	nsec, ok := parseInt(fracStr)
	if !ok {
		return nil
	}
	for i := len(fracStr); i < 9; i++ {
		nsec *= 10
	}

	// The fraction carries the sign of the number it belongs to. Negating only
	// the integer part read "-1710500000.5" as half a second after
	// -1710500000 rather than half a second before it.
	if neg {
		sec, nsec = -sec, -nsec
	}

	t, ok := withinRange(sec, nsec)
	if !ok {
		return nil
	}
	return &Result{Time: t, Kind: KindFrac}
}

// parseInt parses the digits of s, which the caller has already checked are
// digits, and reports whether the value fits in an int64.
//
// It used to have neither the check nor the second return, on the reasoning
// that the caller bounds the digit count. The caller bounds it at 19, and 19
// digits reach 9999999999999999999 where int64 stops at 9223372036854775807,
// so Go wrapped and Detect("9999999999999999999") returned a date in 1702 with
// no error at all. Bounding a digit count does not bound a value.
func parseInt(s string) (int64, bool) {
	// Split into a comparison against a constant and one against the value,
	// which is what strconv.ParseInt does and for the same reason: the
	// direct form, val > (maxInt64-d)/10, divides once per digit, and a
	// nineteen-digit timestamp pays for all nineteen.
	const cutoff = maxInt64 / 10

	var val int64
	for i := 0; i < len(s); i++ {
		if val > cutoff {
			return 0, false
		}
		val *= 10
		d := int64(s[i] - '0')
		if val > maxInt64-d {
			return 0, false
		}
		val += d
	}
	return val, true
}
