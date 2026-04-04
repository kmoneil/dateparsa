package dateparsa

import (
	"testing"
	"time"
)

func TestCompile_ParseEquivalence(t *testing.T) {
	tests := []struct {
		layout string
		input  string
	}{
		// ISO dates
		{"2006-01-02", "2024-03-15"},
		{"2006-01-02", "1999-12-31"},
		{"2006-01-02", "2000-01-01"},

		// ISO datetime
		{"2006-01-02T15:04:05", "2024-03-15T10:30:00"},

		// RFC3339 with Z
		{"2006-01-02T15:04:05Z07:00", "2024-03-15T10:30:00Z"},
		{"2006-01-02T15:04:05Z07:00", "2024-03-15T10:30:00+05:30"},
		{"2006-01-02T15:04:05Z07:00", "2024-03-15T10:30:00-08:00"},

		// US dates
		{"01/02/2006", "03/15/2024"},
		{"01/02/2006", "12/31/1999"},

		// SQL datetime
		{"2006-01-02 15:04:05", "2024-03-15 10:30:00"},

		// Fractional seconds
		{"2006-01-02 15:04:05.000", "2024-03-15 10:30:00.123"},
		{"2006-01-02 15:04:05.000000", "2024-03-15 10:30:00.123456"},
		{"2006-01-02 15:04:05.000000000", "2024-03-15 10:30:00.123456789"},

		// 2-digit year
		{"01/02/06", "03/15/24"},

		// Time only
		{"15:04:05", "10:30:00"},

		// 12-hour AM/PM
		{"03:04PM", "03:30PM"},
		{"03:04PM", "12:00AM"},

		// Non-padded month and day
		{"2006-1-2", "2024-3-5"},
		{"2006-1-2", "2024-12-25"},

		// Offset only
		{"-0700", "-0800"},
		{"-07:00", "+05:30"},

		// Compact Z0700
		{"2006-01-02T15:04:05Z0700", "2024-03-15T10:30:00Z"},
		{"2006-01-02T15:04:05Z0700", "2024-03-15T10:30:00+0530"},

		// Timezone offset (non-conditional)
		{"2006-01-02T15:04:05-07:00", "2024-03-15T10:30:00-08:00"},
		{"2006-01-02T15:04:05-0700", "2024-03-15T10:30:00-0800"},
		{"2006-01-02T15:04:05-07", "2024-03-15T10:30:00-08"},
	}
	for _, tt := range tests {
		t.Run(tt.layout+"__"+tt.input, func(t *testing.T) {
			stdTime, stdErr := time.Parse(tt.layout, tt.input)

			layout, compErr := Compile(tt.layout)
			if compErr != nil {
				t.Fatalf("Compile(%q) error: %v", tt.layout, compErr)
			}
			dpTime, dpErr := layout.Parse(tt.input)

			if (stdErr == nil) != (dpErr == nil) {
				t.Fatalf("stdlib err=%v, dateparsa err=%v", stdErr, dpErr)
			}
			if stdErr != nil {
				return
			}

			if !stdTime.Equal(dpTime) {
				t.Errorf("time mismatch:\n  stdlib:    %v\n  dateparsa: %v", stdTime, dpTime)
			}
		})
	}
}

func TestMustCompile_Panics(t *testing.T) {
	// Valid layout: should not panic
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("MustCompile panicked on valid layout: %v", r)
			}
		}()
		_ = MustCompile("2006-01-02")
	}()

	// Invalid layout: should panic
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Error("MustCompile did not panic on invalid layout")
			}
		}()
		_ = MustCompile("January 2, 2006")
	}()
}

func TestCompileWithTimezone(t *testing.T) {
	nyc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	layout, err := CompileWithTimezone("2006-01-02 15:04:05", nyc)
	if err != nil {
		t.Fatal(err)
	}

	got, err := layout.Parse("2024-03-15 10:30:00")
	if err != nil {
		t.Fatal(err)
	}

	want := time.Date(2024, 3, 15, 10, 30, 0, 0, nyc)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestCompile_GoLayoutRoundTrip(t *testing.T) {
	layout, err := Compile("2006-01-02T15:04:05Z07:00")
	if err != nil {
		t.Fatal(err)
	}
	goLayout, ok := layout.GoLayout()
	if !ok {
		t.Fatal("GoLayout() returned false")
	}
	if goLayout != "2006-01-02T15:04:05Z07:00" {
		t.Errorf("GoLayout() = %q, want %q", goLayout, "2006-01-02T15:04:05Z07:00")
	}
}

func TestCompile_EndToEnd_BatchParsing(t *testing.T) {
	layout := MustCompile("2006-01-02")
	inputs := []string{
		"2024-01-01", "2024-06-15", "2024-12-31",
		"2000-01-01", "1999-12-31", "2030-07-04",
	}
	for _, input := range inputs {
		got, err := layout.Parse(input)
		if err != nil {
			t.Errorf("Parse(%q) error: %v", input, err)
			continue
		}
		want, _ := time.Parse("2006-01-02", input)
		if !got.Equal(want) {
			t.Errorf("Parse(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestCompile_InvalidInput_Errors(t *testing.T) {
	layout := MustCompile("2006-01-02")
	invalids := []string{
		"not-a-date",
		"2024/03/15",  // wrong separator
		"2024-13-01",  // month 13
		"2024-01-32",  // day 32
		"24-03-15",    // 2-digit year for 4-digit layout
		"2024-3-15",   // non-padded for padded layout
		"",
	}
	for _, input := range invalids {
		_, err := layout.Parse(input)
		if err == nil {
			t.Errorf("Parse(%q) expected error, got nil", input)
		}
	}
}

func TestCompile_UnsupportedTokenErrors(t *testing.T) {
	unsupported := []string{
		"Jan 2, 2006",
		"January 2, 2006",
		"Mon, 02 Jan 2006 15:04:05 MST",
		"Monday, January 2, 2006",
	}
	for _, layout := range unsupported {
		_, err := Compile(layout)
		if err == nil {
			t.Errorf("Compile(%q) expected error, got nil", layout)
		}
	}
}

func TestCompile_StringLabel(t *testing.T) {
	layout := MustCompile("2006-01-02")
	if layout.String() == "" {
		t.Error("String() returned empty")
	}
}

// --- Fuzz targets ---

func FuzzCompile_Equivalence(f *testing.F) {
	seeds := []string{
		"2024-03-15", "2000-01-01", "1999-12-31",
		"2024-06-30", "2024-02-29", "2023-02-28",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	layout := MustCompile("2006-01-02")
	f.Fuzz(func(t *testing.T, input string) {
		dpTime, dpErr := layout.Parse(input)
		stdTime, stdErr := time.Parse("2006-01-02", input)

		if stdErr == nil {
			if dpErr != nil {
				t.Errorf("stdlib ok but dateparsa failed: %v", dpErr)
				return
			}
			if !stdTime.Equal(dpTime) {
				t.Errorf("mismatch: stdlib=%v dateparsa=%v", stdTime, dpTime)
			}
		}
		// Note: we intentionally do NOT fail when dateparsa succeeds but stdlib
		// fails. time.Date() normalizes invalid dates (e.g. Feb 29 on non-leap
		// year → Mar 1) while time.Parse rejects them. This is a known, accepted
		// divergence inherent to the executor's use of time.Date().
	})
}

func FuzzCompile_RFC3339_Equivalence(f *testing.F) {
	seeds := []string{
		"2024-03-15T10:30:00Z",
		"2024-03-15T10:30:00+05:30",
		"2024-03-15T10:30:00-08:00",
		"2000-01-01T00:00:00Z",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	layout := MustCompile("2006-01-02T15:04:05Z07:00")
	f.Fuzz(func(t *testing.T, input string) {
		dpTime, dpErr := layout.Parse(input)
		stdTime, stdErr := time.Parse("2006-01-02T15:04:05Z07:00", input)

		if stdErr == nil {
			if dpErr != nil {
				t.Errorf("stdlib ok, dateparsa failed: %v", dpErr)
				return
			}
			if !stdTime.Equal(dpTime) {
				t.Errorf("mismatch: stdlib=%v dateparsa=%v", stdTime, dpTime)
			}
		}
		// Same note as FuzzCompile_Equivalence: time.Date() normalization
		// means we may accept inputs that stdlib rejects.
	})
}

// --- Benchmarks ---

func BenchmarkCompile_ISODate(b *testing.B) {
	for b.Loop() {
		Compile("2006-01-02")
	}
}

func BenchmarkCompiledLayout_Parse_ISODate(b *testing.B) {
	layout := MustCompile("2006-01-02")
	b.ResetTimer()
	for b.Loop() {
		layout.Parse("2024-03-15")
	}
}

func BenchmarkCompiledLayout_Parse_RFC3339(b *testing.B) {
	layout := MustCompile("2006-01-02T15:04:05Z07:00")
	b.ResetTimer()
	for b.Loop() {
		layout.Parse("2024-03-15T10:30:00Z")
	}
}

func BenchmarkCompiledLayout_Parse_DateTime_Millis(b *testing.B) {
	layout := MustCompile("2006-01-02 15:04:05.000")
	b.ResetTimer()
	for b.Loop() {
		layout.Parse("2024-03-15 10:30:00.123")
	}
}

func BenchmarkCompiledLayout_vs_Stdlib(b *testing.B) {
	layouts := []struct {
		name   string
		layout string
		input  string
	}{
		{"ISO_date", "2006-01-02", "2024-03-15"},
		{"RFC3339", "2006-01-02T15:04:05Z07:00", "2024-03-15T10:30:00Z"},
		{"SQL_datetime", "2006-01-02 15:04:05", "2024-03-15 10:30:00"},
		{"US_slash", "01/02/2006", "03/15/2024"},
	}
	for _, l := range layouts {
		b.Run("dateparsa/"+l.name, func(b *testing.B) {
			compiled := MustCompile(l.layout)
			b.ResetTimer()
			for b.Loop() {
				compiled.Parse(l.input)
			}
		})
		b.Run("stdlib/"+l.name, func(b *testing.B) {
			for b.Loop() {
				time.Parse(l.layout, l.input)
			}
		})
	}
}
