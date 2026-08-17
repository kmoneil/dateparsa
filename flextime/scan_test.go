package flextime

import (
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kmoneil/dateparsa"
)

func TestScanTimeTime(t *testing.T) {
	now := time.Now()
	var ft FlexTime
	err := ft.Scan(now)
	if err != nil {
		t.Fatalf("Scan(time.Time) error: %v", err)
	}
	if !ft.Valid() {
		t.Error("expected Valid() == true")
	}
	if !ft.Time().Equal(now) {
		t.Errorf("Time() = %v, want %v", ft.Time(), now)
	}
}

func TestScanString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  time.Time
	}{
		{
			name:  "ISO8601 date",
			input: "2024-03-15",
			want:  time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:  "ISO8601 datetime",
			input: "2024-03-15T10:30:00Z",
			want:  time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC),
		},
		{
			name:  "RFC3339 with offset",
			input: "2024-03-15T10:30:00+05:30",
			want:  time.Date(2024, 3, 15, 10, 30, 0, 0, time.FixedZone("+05:30", 5*3600+30*60)),
		},
		{
			name:  "US date",
			input: "03/15/2024",
			want:  time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:  "MySQL datetime",
			input: "2024-03-15 10:30:00",
			want:  time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC),
		},
		{
			name:  "epoch string",
			input: "1710505800",
			want:  time.Unix(1710505800, 0),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ft FlexTime
			err := ft.Scan(tt.input)
			if err != nil {
				t.Fatalf("Scan(%q) error: %v", tt.input, err)
			}
			if !ft.Valid() {
				t.Error("expected Valid() == true")
			}
			if !ft.Time().Equal(tt.want) {
				t.Errorf("Time() = %v, want %v", ft.Time(), tt.want)
			}
		})
	}
}

func TestScanBytes(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  time.Time
	}{
		{
			name:  "ISO8601 bytes",
			input: []byte("2024-03-15T10:30:00Z"),
			want:  time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC),
		},
		{
			name:  "MySQL date bytes",
			input: []byte("2024-03-15"),
			want:  time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ft FlexTime
			err := ft.Scan(tt.input)
			if err != nil {
				t.Fatalf("Scan([]byte(%s)) error: %v", tt.input, err)
			}
			if !ft.Time().Equal(tt.want) {
				t.Errorf("Time() = %v, want %v", ft.Time(), tt.want)
			}
		})
	}
}

func TestScanInt64(t *testing.T) {
	tests := []struct {
		name  string
		input int64
		want  time.Time
	}{
		{"zero", 0, time.Unix(0, 0)},
		{"positive", 1710505800, time.Unix(1710505800, 0)},
		{"negative", -86400, time.Unix(-86400, 0)},
		{"far future", 4102444800, time.Unix(4102444800, 0)},
		{"year 1 approx", -62135596800, time.Unix(-62135596800, 0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ft FlexTime
			err := ft.Scan(tt.input)
			if err != nil {
				t.Fatalf("Scan(int64(%d)) error: %v", tt.input, err)
			}
			if !ft.Time().Equal(tt.want) {
				t.Errorf("Time() = %v, want %v", ft.Time(), tt.want)
			}
		})
	}
}

func TestScanFloat64(t *testing.T) {
	tests := []struct {
		name      string
		input     float64
		wantS     int64
		wantN     int64
		tolerance time.Duration // float64 can't represent all fractions exactly
	}{
		{"integer", 1710505800.0, 1710505800, 0, 0},
		{"with frac", 1710505800.123, 1710505800, 123000000, time.Microsecond},
		{"negative", -86400.5, -86401, 500000000, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ft FlexTime
			err := ft.Scan(tt.input)
			if err != nil {
				t.Fatalf("Scan(float64) error: %v", err)
			}
			want := time.Unix(tt.wantS, tt.wantN)
			diff := ft.Time().Sub(want)
			if diff < 0 {
				diff = -diff
			}
			if diff > tt.tolerance {
				t.Errorf("Time() = %v, want %v (diff %v > tolerance %v)",
					ft.Time(), want, diff, tt.tolerance)
			}
		})
	}
}

func TestScanNil(t *testing.T) {
	ft := New(time.Now())
	err := ft.Scan(nil)
	if err != nil {
		t.Fatalf("Scan(nil) error: %v", err)
	}
	if ft.Valid() {
		t.Error("expected Valid() == false after scanning NULL")
	}
	if !ft.IsZero() {
		t.Error("expected IsZero() == true after scanning NULL")
	}
}

func TestScanEmptyString(t *testing.T) {
	var ft FlexTime
	err := ft.Scan("")
	if err == nil {
		t.Error("expected error for empty string, got nil")
	}
}

func TestScanUnsupportedType(t *testing.T) {
	var ft FlexTime
	err := ft.Scan(struct{}{})
	if err == nil {
		t.Error("expected error for unsupported type, got nil")
	}
}

func TestValue(t *testing.T) {
	tests := []struct {
		name    string
		ft      FlexTime
		wantNil bool
	}{
		{"valid", New(time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)), false},
		{"invalid", FlexTime{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := tt.ft.Value()
			if err != nil {
				t.Fatalf("Value() error: %v", err)
			}
			if tt.wantNil {
				if v != nil {
					t.Errorf("Value() = %v, want nil", v)
				}
			} else {
				tv, ok := v.(time.Time)
				if !ok {
					t.Fatalf("Value() type = %T, want time.Time", v)
				}
				if !tv.Equal(tt.ft.Time()) {
					t.Errorf("Value() = %v, want %v", tv, tt.ft.Time())
				}
			}
		})
	}
}

// TestErrorMessagesDoNotRepeatTheInput covers every entry point that takes a
// string from outside and hands the failure back.
//
// Each of these wrapped a dateparsa error that already quoted the input with a
// second %q of the same string, so a megabyte in produced a 2,097,231-byte
// message where the library's own was 1,048,628. The library bounds its half
// now; a wrapper that quotes the input again puts the whole thing back.
func TestErrorMessagesDoNotRepeatTheInput(t *testing.T) {
	const big = 1 << 20
	input := strings.Repeat("x", big)

	var ft FlexTime
	sc := NewScanner()

	entries := []struct {
		name string
		err  func() error
	}{
		{"Scan", func() error { return ft.Scan(input) }},
		{"UnmarshalText", func() error { return ft.UnmarshalText([]byte(input)) }},
		{"UnmarshalJSON", func() error { return ft.UnmarshalJSON([]byte(`"` + input + `"`)) }},
		{"Scanner.Scan", func() error { return sc.Scan(&ft, input) }},
	}
	for _, e := range entries {
		t.Run(e.name, func(t *testing.T) {
			err := e.err()
			if err == nil {
				t.Fatal("expected an error for a megabyte of junk")
			}
			if n := len(err.Error()); n > 512 {
				t.Errorf("error is %d bytes for a %d-byte input, want <= 512", n, big)
			}
			if !errors.Is(err, dateparsa.ErrNoMatch) {
				t.Errorf("errors.Is(err, ErrNoMatch) = false; the wrapper stopped wrapping: %v", err)
			}
		})
	}
}

// Compile-time interface assertions.
var (
	_ driver.Valuer = FlexTime{}
	_ driver.Valuer = (*FlexTime)(nil)
)
