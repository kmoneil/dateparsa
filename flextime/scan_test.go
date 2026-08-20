package flextime

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strings"
	"sync"
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

// TestScanAllocates gates what a scanned row costs in a column of one format.
// A string arrives as a string, so there is nothing to copy and nothing to
// detect once scanParser holds the layout: zero. A []byte costs the conversion
// Go's string() requires and nothing else, and UnmarshalText the same.
//
// AllocsPerRun's warmup call primes the cache, so these measure the second row
// onward, which is the case the caches exist for. The first row of a column, and
// the first row after a format change, costs the Layout as well.
//
// Exact numbers rather than upper bounds, for the reason TestLayoutParseZeroAlloc
// uses them: an escape analysis change three packages away is how this regresses
// and a bound hides it.
func TestScanAllocates(t *testing.T) {
	const value = "2024-03-15T10:30:00Z"
	var ft FlexTime

	var str any = value
	var raw any = []byte(value)
	text := []byte(value)

	cases := []struct {
		name string
		want float64
		run  func()
	}{
		{"Scan string", 0, func() { _ = ft.Scan(str) }},
		{"Scan []byte", 1, func() { _ = ft.Scan(raw) }},
		{"UnmarshalText", 1, func() { _ = ft.UnmarshalText(text) }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := testing.AllocsPerRun(1000, tc.run); got != tc.want {
				t.Errorf("%s allocated %v times, want %v", tc.name, got, tc.want)
			}
		})
	}
}

// TestScanAcrossFormats runs values whose formats alternate through the caches
// Scan and UnmarshalText keep, and requires each to come back as
// dateparsa.Parse reads it on its own. The caches are package-level, so the
// sequence that primes them is not something one caller controls; the order here
// alternates rather than groups so that every value but the first meets a layout
// detected from a different format.
func TestScanAcrossFormats(t *testing.T) {
	values := []string{
		"2024-03-15T10:30:00Z",
		"03/15/2024",
		"2024-03-15",
		"March 15, 2024",
		"2024-03-15 10:30:00",
		"15 Mar 2024",
		"20240315",
		"2024-03-15",
		"10:30:45",
		"2024-03-15T10:30:00Z",
	}

	for i := 0; i < 3; i++ {
		for _, v := range values {
			want, err := dateparsa.Parse(v)
			if err != nil {
				t.Fatalf("dateparsa.Parse(%q): %v", v, err)
			}

			var fromString FlexTime
			if err := fromString.Scan(v); err != nil {
				t.Fatalf("Scan(%q): %v", v, err)
			}
			var fromBytes FlexTime
			if err := fromBytes.Scan([]byte(v)); err != nil {
				t.Fatalf("Scan([]byte(%q)): %v", v, err)
			}
			var fromText FlexTime
			if err := fromText.UnmarshalText([]byte(v)); err != nil {
				t.Fatalf("UnmarshalText(%q): %v", v, err)
			}

			for _, got := range []struct {
				path string
				ft   FlexTime
			}{
				{"Scan string", fromString},
				{"Scan []byte", fromBytes},
				{"UnmarshalText", fromText},
			} {
				if !got.ft.Time().Equal(want.Time) {
					t.Errorf("round %d, %s(%q) = %v, dateparsa.Parse = %v",
						i, got.path, v, got.ft.Time(), want.Time)
				}
				if got.ft.Ambiguous() != want.Ambiguous {
					t.Errorf("round %d, %s(%q) Ambiguous = %v, dateparsa.Parse says %v",
						i, got.path, v, got.ft.Ambiguous(), want.Ambiguous)
				}
			}
		}
	}
}

// TestScanAmbiguityThroughTheCache is the half a wrong answer would hide.
// "01/02/2024" is a guess whichever way it is read, and a layout detected from
// an unambiguous value of the same shape must not make it look decided. The
// mechanism is in the root package, where Parser.Parse refuses to reuse an
// ambiguity-prone layout; this asserts the outcome at the boundary a caller
// touches.
func TestScanAmbiguityThroughTheCache(t *testing.T) {
	var primed FlexTime
	if err := primed.Scan("03/15/2024"); err != nil {
		t.Fatalf("priming Scan: %v", err)
	}
	if primed.Ambiguous() {
		t.Errorf(`"03/15/2024" reported Ambiguous, 15 cannot be a month`)
	}

	var ft FlexTime
	if err := ft.Scan("01/02/2024"); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	want, err := dateparsa.Parse("01/02/2024")
	if err != nil {
		t.Fatalf("dateparsa.Parse: %v", err)
	}
	if !ft.Ambiguous() {
		t.Error(`"01/02/2024" through a primed cache reported Ambiguous false, want true`)
	}
	if !ft.Time().Equal(want.Time) {
		t.Errorf("Scan = %v, dateparsa.Parse = %v", ft.Time(), want.Time)
	}
}

// TestScanConcurrentFormats is the shape the shared cache is exposed to: rows of
// different formats scanned by unrelated goroutines through one package-level
// Parser. Each goroutine checks its own value. Run under -race, which the suite
// is.
func TestScanConcurrentFormats(t *testing.T) {
	cases := []struct {
		value string
		want  time.Time
	}{
		{"2024-03-15T10:30:00Z", time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)},
		{"03/15/2024", time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
		{"2024-03-15", time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
		{"March 15, 2024", time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
		{"2024-03-15 10:30:00", time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)},
		{"20240315", time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
	}

	var wg sync.WaitGroup
	for _, tc := range cases {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				var ft FlexTime
				if err := ft.Scan(tc.value); err != nil {
					t.Errorf("Scan(%q): %v", tc.value, err)
					return
				}
				if !ft.Time().Equal(tc.want) {
					t.Errorf("Scan(%q) = %v, want %v", tc.value, ft.Time(), tc.want)
					return
				}
			}
		}()
	}
	wg.Wait()
}

// TestScanAndUnmarshalIgnorePadding is F9 reaching this package, which it does
// for free and which is worth a test anyway.
//
// Both entry points parse through dateparsa.Parser, so the trim is inherited
// rather than repeated here, and that is the property being pinned: a change
// that reimplemented parsing on either path would pass every other test in this
// file and fail this one.
//
// It is also the case flextime exists for. A driver hands back whatever the
// column holds, and a column loaded from a CSV holds the padding the CSV had.
func TestScanAndUnmarshalIgnorePadding(t *testing.T) {
	want := time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)

	for _, v := range []any{
		" 2024-03-15T10:30:00Z",
		"2024-03-15T10:30:00Z ",
		"\t2024-03-15T10:30:00Z\r\n",
		[]byte("  2024-03-15T10:30:00Z  "),
		[]byte("2024-03-15T10:30:00Z\n"),
	} {
		var ft FlexTime
		if err := ft.Scan(v); err != nil {
			t.Errorf("Scan(%q): %v", v, err)
			continue
		}
		if !ft.Time().Equal(want) {
			t.Errorf("Scan(%q) = %v, want %v", v, ft.Time(), want)
		}
	}

	for _, doc := range []string{
		`" 2024-03-15T10:30:00Z"`,
		`"2024-03-15T10:30:00Z "`,
		`"\t2024-03-15T10:30:00Z\r\n"`,
	} {
		var ft FlexTime
		if err := json.Unmarshal([]byte(doc), &ft); err != nil {
			t.Errorf("Unmarshal(%s): %v", doc, err)
			continue
		}
		if !ft.Time().Equal(want) {
			t.Errorf("Unmarshal(%s) = %v, want %v", doc, ft.Time(), want)
		}
	}

	// Whitespace alone is not a time, on either path.
	var ft FlexTime
	if err := ft.Scan("   "); err == nil {
		t.Error(`Scan("   ") = nil error, want one`)
	}
	if err := json.Unmarshal([]byte(`"   "`), &ft); err == nil {
		t.Error(`Unmarshal("   ") = nil error, want one`)
	}
}
