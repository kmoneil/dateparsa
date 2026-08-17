package epoch

import (
	"math"
	"strconv"
	"testing"
	"time"
)

// TestFromIntAgreesWithDetect is the assertion C20 exists for: a value written
// as digits and the same value handed over as an int64 must name the same
// instant, or both must be refused.
//
// The divergence this does NOT assert is deliberate and is checked separately by
// TestFromIntAcceptsSmallValues: a decimal string of fewer than ten digits is not
// a timestamp, because "20240315" is a date, while a typed int64 carries no such
// ambiguity and is read as seconds.
func TestFromIntAgreesWithDetect(t *testing.T) {
	values := []int64{
		1710500000,          // 10 digits, seconds
		1710500000000,       // 13 digits, milliseconds
		1710500000000000,    // 16 digits, microseconds
		1710500000000000000, // 19 digits, nanoseconds
		99999999999,         // 11 digits, seconds
		999999999999,        // 12 digits, seconds
		9999999999999,       // 13 digits
		9999999999999999,    // 16 digits
		-1710500000,
		-1710500000000,
		-1710500000000000,
		-1710500000000000000,

		// Digit counts no precision claims. Refused as a string; must be
		// refused as an int64 too, or the two paths disagree in a band
		// narrow enough that nobody would find it.
		17105000000000,      // 14
		171050000000000,     // 15
		17105000000000000,   // 17
		171050000000000000,  // 18
		9223372036854775807, // 19, MaxInt64

		// Out of range in seconds.
		100000000000,  // 12 digits, 1e11, exactly maxSeconds
		999999999999,  // 12 digits, ~31690 years
		-100000000000, // -1e11
	}

	for _, v := range values {
		s := strconv.FormatInt(v, 10)
		want := Detect(s)
		got, ok := FromInt(v)

		switch {
		case want == nil && ok:
			t.Errorf("FromInt(%d) accepted %v, Detect(%q) refused", v, got.UTC(), s)
		case want != nil && !ok:
			t.Errorf("FromInt(%d) refused, Detect(%q) returned %v", v, s, want.Time.UTC())
		case want != nil && ok && !got.Equal(want.Time):
			t.Errorf("FromInt(%d) = %v, Detect(%q) = %v", v, got.UTC(), s, want.Time.UTC())
		}
	}
}

// TestFromIntAcceptsSmallValues pins the one place FromInt is deliberately more
// permissive than Detect. A string of fewer than ten digits is refused because it
// is far more likely to be a compact date or a bare year than a timestamp; an
// int64 from a driver column has already been typed as a number by the schema.
func TestFromIntAcceptsSmallValues(t *testing.T) {
	cases := []struct {
		v    int64
		want time.Time
	}{
		{0, time.Unix(0, 0).UTC()},
		{1, time.Unix(1, 0).UTC()},
		{86400, time.Date(1970, 1, 2, 0, 0, 0, 0, time.UTC)},
		{-86400, time.Date(1969, 12, 31, 0, 0, 0, 0, time.UTC)},
		{999999999, time.Unix(999999999, 0).UTC()}, // 9 digits
	}
	for _, c := range cases {
		got, ok := FromInt(c.v)
		if !ok {
			t.Errorf("FromInt(%d) refused, want %v", c.v, c.want)
			continue
		}
		if !got.Equal(c.want) {
			t.Errorf("FromInt(%d) = %v, want %v", c.v, got.UTC(), c.want)
		}
		if Detect(strconv.FormatInt(c.v, 10)) != nil {
			t.Errorf("Detect(%q) accepted a short numeric string; the comment on "+
				"FromInt about why it is more permissive is now wrong",
				strconv.FormatInt(c.v, 10))
		}
	}
}

// TestFromIntRefusesMinInt64 records a deliberate loss, the same one C3 took for
// the string path: the most negative int64 has one more digit than the positive
// side can hold, so parseInt refuses it and FromInt refuses it to match.
func TestFromIntRefusesMinInt64(t *testing.T) {
	if _, ok := FromInt(math.MinInt64); ok {
		t.Error("FromInt(MinInt64) was accepted; Detect refuses the same digits")
	}
}

func TestFromSecondsRefusesWhatIsNotANumber(t *testing.T) {
	for _, f := range []float64{
		math.NaN(),
		math.Inf(1),
		math.Inf(-1),
		math.MaxFloat64,
		-math.MaxFloat64,
		1e300,
		-1e300,
		1e15,  // in float range, out of epoch range
		-1e15, //
	} {
		if got, ok := FromSeconds(f); ok {
			t.Errorf("FromSeconds(%v) accepted %v, want refused", f, got.UTC())
		}
	}
}

func TestFromSecondsAcceptsWhatItShould(t *testing.T) {
	cases := []struct {
		f    float64
		want time.Time
	}{
		{0, time.Unix(0, 0).UTC()},
		{1710500000, time.Unix(1710500000, 0).UTC()},
		{1710500000.5, time.Unix(1710500000, 500000000).UTC()},
		{-1710500000.5, time.Unix(-1710500000, -500000000).UTC()},
	}
	for _, c := range cases {
		got, ok := FromSeconds(c.f)
		if !ok {
			t.Errorf("FromSeconds(%v) refused, want %v", c.f, c.want)
			continue
		}
		if !got.Equal(c.want) {
			t.Errorf("FromSeconds(%v) = %v, want %v", c.f, got.UTC(), c.want)
		}
	}
}

// TestFromSecondsAgreesWithFractionalDetect holds the float path to the string
// path wherever the string path has an answer, the same property
// TestFromIntAgreesWithDetect holds for integers.
func TestFromSecondsAgreesWithFractionalDetect(t *testing.T) {
	for _, s := range []string{
		"1710500000.123",
		"1710500000.5",
		"-1710500000.5",
		"999999999999.5",
		"100000000000.5",
	} {
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			t.Fatalf("bad test input %q: %v", s, err)
		}
		want := Detect(s)
		got, ok := FromSeconds(f)

		switch {
		case want == nil && ok:
			t.Errorf("FromSeconds(%v) accepted %v, Detect(%q) refused", f, got.UTC(), s)
		case want != nil && !ok:
			t.Errorf("FromSeconds(%v) refused, Detect(%q) returned %v", f, s, want.Time.UTC())
		case want != nil && ok && !got.Equal(want.Time):
			// A float64 cannot hold every decimal fraction exactly, so allow a
			// microsecond of slack rather than requiring bit equality.
			if d := got.Sub(want.Time); d > time.Microsecond || d < -time.Microsecond {
				t.Errorf("FromSeconds(%v) = %v, Detect(%q) = %v, %v apart",
					f, got.UTC(), s, want.Time.UTC(), d)
			}
		}
	}
}

// TestDigitCountMatchesStrconv holds the binary search in digitCount to what
// formatting the value actually produces, for every width and both signs, and for
// the two values the obvious divide-until-zero version gets wrong.
func TestDigitCountMatchesStrconv(t *testing.T) {
	values := []int64{
		0, 1, 9, 10, 99, 100, 999,
		-1, -9, -10, -99, -100,
		999999999, 1000000000,
		1710500000, 1710500000000, 1710500000000000, 1710500000000000000,
		math.MaxInt64, math.MinInt64,
	}
	// Every decimal width, both signs.
	for p := int64(1); ; p *= 10 {
		values = append(values, p, p-1, -p, -(p - 1))
		if p > math.MaxInt64/10 {
			break
		}
	}

	for _, v := range values {
		want := len(strconv.FormatInt(v, 10))
		if v < 0 {
			want-- // the sign is not a digit
		}
		if got := digitCount(v); got != want {
			t.Errorf("digitCount(%d) = %d, want %d", v, got, want)
		}
	}
}
