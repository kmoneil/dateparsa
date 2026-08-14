package epoch

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"
)

// This package had no fuzz target at all, and it is the one package in the
// tree that does unchecked integer arithmetic on untrusted input. The two
// root-package targets reach internal/detect and internal/compile on every
// execution, but epoch sits behind a failed structured detection and is
// reached only by an input that is entirely digits, so it got almost none of
// that budget. Two independent bugs lived in 183 lines until a code sweep
// found them by reading.
//
// make fuzz discovers packages rather than naming them, so these are swept
// from the moment they exist.

// FuzzEpochRoundTrips is the assertion that catches a wrong instant: an
// integer this package accepts names an instant, and it has to be that one.
//
// The oracle is the standard library's own constructor for each precision, so
// it shares no arithmetic with the code under test. That is what makes it able
// to see the sign bug: three arms flipped the remainder of a negative value
// positive, and every negative sub-second timestamp came back later than it
// should by twice its remainder.
func FuzzEpochRoundTrips(f *testing.F) {
	seeds := []int64{
		0, 1, -1,
		1710500000, -1710500000,
		1710500000123, -1710500000123,
		1710500000123456, -1710500000123456,
		1710500000123456789, -1710500000123456789,
		-1500000000000,
		9223372036854775807, -9223372036854775807,
	}
	for _, n := range seeds {
		f.Add(n)
	}

	f.Fuzz(func(t *testing.T, n int64) {
		s := strconv.FormatInt(n, 10)
		r := Detect(s)
		if r == nil {
			return // refusing is always allowed
		}

		var want time.Time
		switch r.Kind {
		case KindSec:
			want = time.Unix(n, 0)
		case KindMilli:
			want = time.UnixMilli(n)
		case KindMicro:
			want = time.UnixMicro(n)
		case KindNano:
			want = time.Unix(0, n)
		default:
			t.Fatalf("Detect(%q) returned Kind %d for an input with no dot", s, r.Kind)
		}

		if !r.Time.Equal(want) {
			t.Errorf("Detect(%q) [kind %d] = %v, want %v (off by %v)",
				s, r.Kind, r.Time.UTC(), want.UTC(), r.Time.Sub(want))
		}
	})
}

// FuzzEpochFractionalRoundTrips is the same property for the decimal form,
// which reaches different code and had its own copy of the sign bug: only the
// integer part was negated, so "-1710500000.5" was read as half a second after
// -1710500000 rather than half a second before it.
//
// The string is built rather than fuzzed so that every execution is a shape
// this package accepts. What varies is the value, which is what the property
// is about.
func FuzzEpochFractionalRoundTrips(f *testing.F) {
	seeds := []struct {
		sec   int64
		nanos uint32
	}{
		{1710500000, 123000000},
		{-1710500000, 123000000},
		{-1710500000, 500000000},
		{1710500000, 999999999},
		{-1710500000, 999999999},
		{1710500000, 0},
		{-1710500000, 1},
	}
	for _, s := range seeds {
		f.Add(s.sec, s.nanos)
	}

	f.Fuzz(func(t *testing.T, sec int64, nanos uint32) {
		if nanos > 999999999 {
			return
		}
		abs := sec
		if abs < 0 {
			abs = -abs
		}
		// Nine to twelve integer digits is the shape parseFractional accepts,
		// and maxSeconds is the range it accepts within that.
		if abs < 100000000 || abs > maxSeconds {
			return
		}

		s := fmt.Sprintf("%d.%09d", sec, nanos)
		r := Detect(s)
		if r == nil {
			t.Fatalf("Detect(%q) = nil, want a fractional timestamp", s)
		}
		if r.Kind != KindFrac {
			t.Errorf("Detect(%q) Kind = %d, want KindFrac", s, r.Kind)
		}

		n := int64(nanos)
		if sec < 0 {
			n = -n
		}
		want := time.Unix(sec, n)
		if !r.Time.Equal(want) {
			t.Errorf("Detect(%q) = %v, want %v (off by %v)",
				s, r.Time.UTC(), want.UTC(), r.Time.Sub(want))
		}
	})
}

// FuzzEpochAcceptsNothingOutOfRange runs Detect on arbitrary bytes, which is
// the target that would have caught the overflow: parseInt multiplied and
// added with no check on the reasoning that the caller bounds the digit count.
// The caller bounds it at 19, and 19 digits reach 9999999999999999999 where
// int64 stops at 9223372036854775807.
//
// strconv.ParseInt is the oracle. A number it refuses as out of range is a
// number this package must refuse too, and the wrapped values it used to
// return landed inside the declared range, so a range assertion alone would
// have watched Detect("9999999999999999999") return a date in 1702 and said
// nothing.
func FuzzEpochAcceptsNothingOutOfRange(f *testing.F) {
	seeds := []string{
		"", "-", ".", "-.", "0", "1710500000",
		"9999999999999999999",
		"9223372036854775808",
		"9223372036854775807",
		"-9223372036854775808",
		"-9999999999999999999",
		"-1710500000123",
		"1710500000.99999999999999999999999",
		"1710500000.123",
		"-1710500000.5",
		"20240315",
		"1710500000.123.456",

		// Found by this target a minute after it existed. The range check was
		// on the seconds handed to time.Unix rather than on the instant, and a
		// negative fraction borrows a second: -1e11 seconds with -1e8
		// nanoseconds lands 100000000001 seconds before the epoch, one past
		// the bound its arguments looked like they respected.
		"-100000000000.1",
		"100000000000.1",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		r := Detect(s)
		if r == nil {
			return
		}

		// Whatever it accepted, it has to be a number the standard library can
		// hold, in both halves.
		intPart, frac, hasDot := strings.Cut(s, ".")
		if _, err := strconv.ParseInt(intPart, 10, 64); err != nil {
			t.Errorf("Detect(%q) = %v, but %q is not an int64: %v",
				s, r.Time.UTC(), intPart, err)
		}
		if hasDot {
			if r.Kind != KindFrac {
				t.Errorf("Detect(%q) Kind = %d, want KindFrac", s, r.Kind)
			}
			if ns := r.Time.Nanosecond(); ns < 0 || ns > 999999999 {
				t.Errorf("Detect(%q) nanosecond = %d, out of range", s, ns)
			}
			if len(frac) > 0 && len(frac) <= 9 {
				// Short enough to compare digit for digit: the nanosecond must
				// be the fraction scaled up, with the sign of the number.
				want, err := strconv.ParseInt(frac, 10, 64)
				if err != nil {
					t.Fatalf("Detect(%q) accepted a fraction %q that is not digits", s, frac)
				}
				for i := len(frac); i < 9; i++ {
					want *= 10
				}
				got := int64(r.Time.Nanosecond())
				if r.Time.Unix() < 0 && got != 0 {
					got = 1e9 - got
				}
				if got != want {
					t.Errorf("Detect(%q) fraction = %d ns, want %d ns", s, got, want)
				}
			}
		} else if r.Kind == KindFrac {
			t.Errorf("Detect(%q) Kind = KindFrac for an input with no dot", s)
		}

		if sec := r.Time.Unix(); sec > maxSeconds || sec < -maxSeconds {
			t.Errorf("Detect(%q) = %v, %d seconds from the epoch, over the declared %d",
				s, r.Time.UTC(), sec, int64(maxSeconds))
		}
	})
}
