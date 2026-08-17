// Package epoch detects and parses Unix timestamps.
//
// Supports seconds, milliseconds, microseconds, nanoseconds, and
// fractional seconds (e.g. 1710500000.123).
package epoch

import (
	"math/bits"
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

// maxInt64 is math.MaxInt64 and minInt64 is math.MinInt64, written out rather
// than imported from math, which would be one import for two constants. See
// parseInt and FromInt for what they are for.
//
// math/bits is imported, for digitCount, and that is a different thing: it is an
// algorithm and not a constant.
const (
	maxInt64 = 1<<63 - 1
	minInt64 = -1 << 63
)

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
	kind, ok := precisionFor(len(s) - start)
	if !ok {
		return nil
	}

	val, ok := parseInt(s[start:])
	if !ok {
		return nil
	}
	if neg {
		val = -val
	}

	t, ok := withinRange(split(val, kind))
	if !ok {
		return nil
	}
	return &Result{Time: t, Kind: kind}
}

// precisionFor returns the precision a decimal integer of digits digits names,
// and reports whether that digit count names one at all.
//
// This is the only copy of the table. It used to be a switch inside
// parseInteger, and FromInt needs the same answer for a value that arrives as an
// int64 rather than as digits: two copies of it would let the string and numeric
// paths drift apart in a band nobody would think to test, which is exactly the
// defect FromInt exists to close.
//
// Nine digits and fewer name nothing here, deliberately. "20240315" is a compact
// date and "2024" is a year, and reading either as a timestamp is the mistake
// maxSeconds was introduced to prevent. FromInt overrides that one arm, because a
// value the schema already typed as a number cannot be a date string; see the
// comment there.
func precisionFor(digits int) (Kind, bool) {
	switch {
	case digits == 13:
		return KindMilli, true
	case digits == 16:
		return KindMicro, true
	case digits == 19:
		return KindNano, true
	case digits >= 10 && digits <= 12:
		// 11 and 12 digits are still seconds when they land in range.
		return KindSec, true
	}
	return KindNone, false
}

// split converts a count at the given precision into seconds and nanoseconds.
//
// Go's division truncates toward zero, so a negative val gives a negative
// remainder, and time.Unix documents that it accepts an nsec outside
// [0, 999999999] and normalises it. The two agree: -1710500000123 is
// sec -1710500000 and nsec -123000000, which is the instant it names.
//
// Each arm used to flip that remainder positive when the input carried a
// minus, which moved every negative sub-second timestamp forward by twice
// its remainder. -1710500000123 came back as -1710499999877.
func split(val int64, kind Kind) (sec, nsec int64) {
	switch kind {
	case KindMilli:
		return val / 1e3, (val % 1e3) * 1e6
	case KindMicro:
		return val / 1e6, (val % 1e6) * 1e3
	case KindNano:
		return val / 1e9, val % 1e9
	}
	return val, 0
}

// pow10 holds the smallest magnitude written with n+1 decimal digits, so a
// magnitude's digit count is one more than the number of entries it is at or
// above. Nineteen is every width a uint64 magnitude of an int64 can have, and
// 1e19 fits a uint64 with room to spare.
var pow10 = [20]uint64{
	1, 10, 1e2, 1e3, 1e4, 1e5, 1e6, 1e7, 1e8, 1e9,
	1e10, 1e11, 1e12, 1e13, 1e14, 1e15, 1e16, 1e17, 1e18, 1e19,
}

// digitCount returns how many decimal digits v is written with, ignoring its
// sign.
//
// The bit length gives an estimate of the decimal width, because 1233/4096 is
// log10(2) to four decimal places, and one comparison against pow10 corrects it.
// Two branches whatever the value.
//
// Both obvious versions are slower, and this was worth measuring rather than
// assuming, because a numeric Scan is short enough that the digit count is most of
// it. Dividing by ten until the value is gone costs up to nineteen 64-bit
// divisions. A binary search over pow10 costs five data-dependent branches, and
// mispredicting all five is worse than the divisions on this machine. Measured on
// linux/arm64 against a ten-digit value, in a scratch benchmark deleted with the
// measurement:
//
//	binary search over pow10   6.5 ns
//	this                       0.58 ns
//
// FromInt as a whole went 8.7ns to 6.4ns with no other change, and
// BenchmarkScanInt64 10.3ns to 7.3ns.
//
// The magnitude is taken in uint64 rather than by negating, because negating
// math.MinInt64 overflows and the result would be counted as one digit.
//
// gosec reports the conversion below as G115, an int64 to uint64 overflow. The
// reinterpretation is the point: Go defines the conversion as the two's-complement
// bit pattern, and negating in uint64 then gives the exact magnitude for every
// input including MinInt64, which is the one value the obvious version gets wrong.
// Do not "fix" it by negating first. TestDigitCountMatchesStrconv is what holds
// both of those claims, for every decimal width and both signs.
func digitCount(v int64) int {
	u := uint64(v)
	if v < 0 {
		u = -u
	}
	if u == 0 {
		// bits.Len64(0) is 0, and the correction below would take the estimate
		// to -1 and index pow10 out of range.
		return 1
	}
	d := (bits.Len64(u) * 1233) >> 12
	if u < pow10[d] {
		d--
	}
	return d + 1
}

// FromInt reads v as a Unix timestamp and reports whether it names an instant
// this package will accept.
//
// The precision comes from how many decimal digits v is written with, the same
// table Detect applies to a string, so that a timestamp handed over as an int64
// and the same timestamp written as digits name the same instant. Before this
// existed, flextime read every numeric driver value and JSON number as seconds:
// 1710500000000 is 2024-03-15 as a string and was year 56173 as a number, and a
// millisecond epoch is the most common timestamp on the wire.
//
// It is more permissive than Detect in one place. A decimal string of fewer than
// ten digits names no precision, because "20240315" is a date and "2024" is a
// year; an int64 has already been typed as a number by whatever produced it, so
// there is no date reading to protect and a small value is read as seconds. That
// is the one input class where FromInt and Detect disagree, and it is checked by
// TestFromIntAcceptsSmallValues.
//
// math.MinInt64 is refused, matching Detect: its nineteen digits are one past
// what the positive side holds, so parseInt refuses the string and this refuses
// the value. A real loss, and the same one C3 took.
func FromInt(v int64) (time.Time, bool) {
	if v == minInt64 {
		return time.Time{}, false
	}
	digits := digitCount(v)
	kind, ok := precisionFor(digits)
	if !ok {
		// Short values are seconds. Anything else precisionFor refuses is a
		// digit count that names no precision, and guessing one for it would
		// disagree with the string path.
		if digits >= 10 {
			return time.Time{}, false
		}
		kind = KindSec
	}
	return withinRange(split(v, kind))
}

// FromSeconds reads f as a count of seconds since the epoch, fraction included,
// and reports whether it names an instant this package will accept.
//
// Unlike FromInt this does not read a precision off the magnitude. A float64
// holds 53 bits of mantissa, so it cannot carry a nanosecond timestamp anyway,
// and the documented contract for a float driver value has always been seconds.
//
// NaN and the infinities are refused rather than converted. Go leaves the result
// of an out-of-range float-to-int conversion implementation-defined: on arm64 it
// saturates, on amd64 it yields the most negative int64, so the same value
// produced two different instants on two different machines. int64(NaN) was 0 on
// arm64, which read NaN as the epoch itself.
//
// The magnitude is checked against the float, before any conversion, so no
// out-of-range conversion happens at all rather than happening and being caught
// afterwards. f != f is the NaN test, and it has to come first because every
// comparison against NaN is false, so the bounds below would let it through.
//
// That check is deliberately not maxSeconds. It exists only to make int64(f)
// defined; withinRange is the bound, and it is applied to the instant rather than
// to the argument for the reason C3 recorded. Checking maxSeconds here instead
// refused "100000000000.5", which Detect accepts, because the half second is past
// the bound while the instant it belongs to is not.
func FromSeconds(f float64) (time.Time, bool) {
	// Comfortably inside int64 and far above anything withinRange will accept.
	const convertible = 1 << 62
	if f != f || f > convertible || f < -convertible {
		return time.Time{}, false
	}
	// int64(f) truncates toward zero and is in range by the test above.
	sec := int64(f)
	nsf := (f - float64(sec)) * 1e9
	// Round half away from zero, which is what math.Round does, without the
	// import. The fraction carries the sign of the value it belongs to.
	if nsf >= 0 {
		nsf += 0.5
	} else {
		nsf -= 0.5
	}
	return withinRange(sec, int64(nsf))
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
