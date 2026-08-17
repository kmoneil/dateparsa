package dateparsa

import (
	"errors"
	"math"
	"testing"
	"time"
)

// TestBaseYearOutsideInt32IsRefused covers the second half of C20.
//
// Program.BaseYear is an int32 for a documented size reason, and time.Time reaches
// year 292277026596, so the conversion truncated: ParseWith("10:30:00") with such a
// base came back as 219250468-01-01 with a nil error. The chain that made it
// reachable was flextime's numeric path building the base time out of a JSON
// number, which the other half of C20 closes; the guard stays because the threat
// model says the options are not chosen by anybody hostile, not that the
// conversion is safe.
func TestBaseYearOutsideInt32IsRefused(t *testing.T) {
	// The largest instant time.Time represents, reached the way a caller would:
	// a nanosecond count at the top of int64 scaled to seconds.
	far := time.Unix(math.MaxInt64, 0).UTC()
	if far.Year() <= math.MaxInt32 {
		t.Fatalf("test premise gone: time.Unix(MaxInt64, 0).Year() is %d, which fits int32", far.Year())
	}

	// A format with no year field is the only one that reads BaseYear.
	for _, in := range []string{"10:30:00", "10:30", "March 15"} {
		_, err := ParseWith(in, WithBaseTime(far))
		if err == nil {
			t.Errorf("ParseWith(%q, WithBaseTime(year %d)) accepted; want an error",
				in, far.Year())
			continue
		}
		if !errors.Is(err, ErrNoMatch) {
			t.Errorf("ParseWith(%q): error does not unwrap to ErrNoMatch: %v", in, err)
		}
	}

	if _, err := Detect("10:30:00", WithBaseTime(far)); err == nil {
		t.Error("Detect accepted a base year outside int32; want an error")
	}
}

// TestBaseYearInsideInt32StillWorks is the other direction, so the guard cannot be
// satisfied by refusing everything.
func TestBaseYearInsideInt32StillWorks(t *testing.T) {
	base := time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC)
	r, err := ParseWith("10:30:00", WithBaseTime(base))
	if err != nil {
		t.Fatalf("ParseWith: %v", err)
	}
	if r.Time.Year() != 2026 {
		t.Errorf("year = %d, want 2026", r.Time.Year())
	}

	// A format that carries its own year never reads BaseYear, so it must not be
	// affected by the guard at all.
	far := time.Unix(math.MaxInt64, 0).UTC()
	r2, err := ParseWith("2024-03-15", WithBaseTime(far))
	if err != nil {
		t.Fatalf("a format with its own year must not consult the base year: %v", err)
	}
	if r2.Time.Year() != 2024 {
		t.Errorf("year = %d, want 2024", r2.Time.Year())
	}
}

// TestBaseYearGuardReachesTheStrictModePath covers the two conversions inside
// buildAmbiguousError, which no test would otherwise touch: strict mode plus an
// ambiguous input plus a format with no year field.
func TestBaseYearGuardReachesTheStrictModePath(t *testing.T) {
	far := time.Unix(math.MaxInt64, 0).UTC()
	_, err := ParseWith("01/02/03", WithBaseTime(far), WithStrictMode(true))
	if err == nil {
		t.Fatal("want an error from the strict-mode path")
	}
	// Either error is acceptable -- the input is ambiguous and the base year is
	// unrepresentable -- but it must not be a nil error and must not panic.
	if !errors.Is(err, ErrNoMatch) && !errors.Is(err, ErrAmbiguous) {
		t.Errorf("error unwraps to neither sentinel: %v", err)
	}
}
