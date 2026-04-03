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
		"2024",       // Too short, not in timestamp range
		"20240315",   // 8 digits — compact date, not timestamp
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
