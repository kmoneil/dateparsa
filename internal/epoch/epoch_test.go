package epoch

import (
	"testing"
	"time"
)

func TestDetect_Seconds(t *testing.T) {
	// 2024-03-15T12:13:20Z
	r := Detect("1710504800")
	if r == nil {
		t.Fatal("expected result")
	}
	if r.Kind != KindSec {
		t.Errorf("kind = %v, want KindSec", r.Kind)
	}
	expected := time.Unix(1710504800, 0).UTC()
	if !r.Time.Equal(expected) {
		t.Errorf("got %v, want %v", r.Time, expected)
	}
}

func TestDetect_Milliseconds(t *testing.T) {
	r := Detect("1710504800000")
	if r == nil {
		t.Fatal("expected result")
	}
	if r.Kind != KindMilli {
		t.Errorf("kind = %v, want KindMilli", r.Kind)
	}
	expected := time.Unix(1710504800, 0).UTC()
	if !r.Time.Equal(expected) {
		t.Errorf("got %v, want %v", r.Time, expected)
	}
}

func TestDetect_Microseconds(t *testing.T) {
	r := Detect("1710504800000000")
	if r == nil {
		t.Fatal("expected result")
	}
	if r.Kind != KindMicro {
		t.Errorf("kind = %v, want KindMicro", r.Kind)
	}
}

func TestDetect_Nanoseconds(t *testing.T) {
	r := Detect("1710504800000000000")
	if r == nil {
		t.Fatal("expected result")
	}
	if r.Kind != KindNano {
		t.Errorf("kind = %v, want KindNano", r.Kind)
	}
}

func TestDetect_Fractional(t *testing.T) {
	r := Detect("1710504800.123")
	if r == nil {
		t.Fatal("expected result")
	}
	if r.Kind != KindFrac {
		t.Errorf("kind = %v, want KindFrac", r.Kind)
	}
	if r.Time.Nanosecond() != 123000000 {
		t.Errorf("nsec = %d, want 123000000", r.Time.Nanosecond())
	}
}

func TestDetect_Negative(t *testing.T) {
	// Before epoch.
	r := Detect("-1710504800")
	if r == nil {
		t.Fatal("expected result")
	}
	if r.Time.After(time.Unix(0, 0)) {
		t.Errorf("expected time before epoch, got %v", r.Time)
	}
}

func TestDetect_NotTimestamp(t *testing.T) {
	rejects := []string{
		"",
		"2024",     // Too short, not in timestamp range
		"20240315", // 8 digits — compact date, not timestamp
		"abc",
		"12:30:00",
		"2024-03-15",
		"1710504800.123.456", // Two dots
		"-",
	}
	for _, s := range rejects {
		r := Detect(s)
		if r != nil {
			t.Errorf("Detect(%q) = %v, want nil", s, r)
		}
	}
}

// TestDetect_RefusesWhatDoesNotFitInAnInt64 covers the overflow. parseInt had
// no check, on the reasoning in its doc comment that the caller bounds the
// digit count. The caller bounds it at 19, and 19 digits reach
// 9999999999999999999 where int64 stops at 9223372036854775807, so Go wrapped
// and each of these came back as a confident wrong date.
func TestDetect_RefusesWhatDoesNotFitInAnInt64(t *testing.T) {
	for _, tt := range []struct {
		in  string
		was string
	}{
		{"9999999999999999999", "1702-05-02"},
		{"9223372036854775808", "1677-09-21"},
		{"-9999999999999999999", "2237-09-01"}, // a negative epoch in the future
		{"-9223372036854775808", "1677-09-21"}, // the one int64 the positive side cannot hold
	} {
		if r := Detect(tt.in); r != nil {
			t.Errorf("Detect(%q) = %v, want nil (used to return %s)",
				tt.in, r.Time.UTC(), tt.was)
		}
	}

	// The largest value that does fit still parses, because refusing it would
	// be the same bug in the other direction.
	r := Detect("9223372036854775807")
	if r == nil {
		t.Fatal(`Detect("9223372036854775807") = nil, want the largest nanosecond timestamp`)
	}
	if r.Time.UnixNano() != 9223372036854775807 {
		t.Errorf("UnixNano = %d, want 9223372036854775807", r.Time.UnixNano())
	}
}

// TestDetect_LongFractionTruncates covers the other half of the overflow. The
// fraction was parsed whole and then divided back down to nine digits, so a
// long enough one wrapped before the division could discard anything.
func TestDetect_LongFractionTruncates(t *testing.T) {
	r := Detect("1710500000.99999999999999999999999")
	if r == nil {
		t.Fatal("expected a fractional timestamp")
	}
	if ns := r.Time.Nanosecond(); ns != 999999999 {
		t.Errorf("nanosecond = %d, want 999999999 (used to be 2003)", ns)
	}
}

// TestDetect_RangeIsOnTheInstant covers the boundary the range check used to
// miss. A negative fraction borrows a second from the integer part, so a value
// whose seconds sit exactly on the bound lands one past it.
func TestDetect_RangeIsOnTheInstant(t *testing.T) {
	for _, in := range []string{
		"-100000000000.1", // 100000000000.1 seconds before the epoch
		"-100000000001",   // one second past the bound outright
		"100000000001",
	} {
		if r := Detect(in); r != nil {
			t.Errorf("Detect(%q) = %v (%d seconds from the epoch), want nil",
				in, r.Time.UTC(), r.Time.Unix())
		}
	}

	// The bound itself is inside, on both sides and in both forms.
	for _, in := range []string{"100000000000", "-100000000000", "100000000000.1"} {
		if r := Detect(in); r == nil {
			t.Errorf("Detect(%q) = nil, want a timestamp", in)
		}
	}
}

// TestDetect_NegativeSubSecond covers the sign flip. Four code paths turned a
// negative remainder positive, which moved the instant forward by twice it.
// The standard library's own constructors are the reference.
func TestDetect_NegativeSubSecond(t *testing.T) {
	for _, tt := range []struct {
		in   string
		want time.Time
	}{
		{"-1710500000123", time.UnixMilli(-1710500000123)},
		{"-1710500000123456", time.UnixMicro(-1710500000123456)},
		{"-1710500000123456789", time.Unix(0, -1710500000123456789)},
		{"-1710500000.5", time.Unix(-1710500000, -500000000)},
		{"-1500000000000", time.UnixMilli(-1500000000000)}, // was right only because it divides evenly
	} {
		r := Detect(tt.in)
		if r == nil {
			t.Errorf("Detect(%q) = nil, want %v", tt.in, tt.want.UTC())
			continue
		}
		if !r.Time.Equal(tt.want) {
			t.Errorf("Detect(%q) = %v, want %v (off by %v)",
				tt.in, r.Time.UTC(), tt.want.UTC(), r.Time.Sub(tt.want))
		}
	}
}

func TestDetect_MillisWithFraction(t *testing.T) {
	// 1710504800123 = millis
	r := Detect("1710504800123")
	if r == nil {
		t.Fatal("expected result")
	}
	if r.Kind != KindMilli {
		t.Errorf("kind = %v, want KindMilli", r.Kind)
	}
	if r.Time.UnixMilli() != 1710504800123 {
		t.Errorf("got %d ms, want 1710504800123", r.Time.UnixMilli())
	}
}
