package detect

import (
	"testing"
)

func TestScan_BasicSignatures(t *testing.T) {
	tests := []struct {
		input string
		want  string // Signature as character class letters
	}{
		{"2024-03-15", "DDDDSDDSDD"},
		{"10:30:00", "DDCDDCDD"},
		{"10:30", "DDCDD"},
		{"15.03.2024", "DDSDDSDDDD"},
	}

	for _, tt := range tests {
		sig := Scan(tt.input)
		got := sigToString(&sig)
		if got != tt.want {
			t.Errorf("Scan(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestScan_ISODateTime(t *testing.T) {
	sig := Scan("2024-03-15T10:30:00Z")
	got := sigToString(&sig)
	want := "DDDDSDDSDDXDDCDDCDDX"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestScan_RFC3339(t *testing.T) {
	sig := Scan("2024-03-15T10:30:00+05:30")
	got := sigToString(&sig)
	want := "DDDDSDDSDDXDDCDDCDDXDDCDD"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDetect_ISO8601Date(t *testing.T) {
	cfg := Config{Timezone: nil}
	result, ok := Detect("2024-03-15", cfg)
	if !ok {
		t.Fatal("expected a result")
	}
	if result.Def.Name != "ISO8601_DATE" {
		t.Errorf("got %q, want ISO8601_DATE", result.Def.Name)
	}
}

func TestDetect_TextualMonth(t *testing.T) {
	cfg := Config{}
	result, ok := Detect("March 15, 2024", cfg)
	if !ok {
		t.Fatal("expected a result")
	}
	if result.Def == nil {
		t.Fatal("expected a FormatDef")
	}
}

func TestDetect_AmbiguousSlash(t *testing.T) {
	cfg := Config{}
	result, ok := Detect("01/02/2024", cfg)
	if !ok {
		t.Fatal("expected a result")
	}
	if !result.Ambig {
		t.Error("expected Ambig=true for 01/02/2024")
	}

	// With day-first preference.
	cfg.PreferDayFirst = true
	result2, ok2 := Detect("01/02/2024", cfg)
	if !ok2 {
		t.Fatal("expected a result")
	}
	if !result2.Ambig {
		t.Error("expected Ambig=true even with day-first preference")
	}
}

func TestDetect_UnambiguousSlash(t *testing.T) {
	cfg := Config{}
	result, ok := Detect("13/01/2024", cfg)
	if !ok {
		t.Fatal("expected a result")
	}
	if result.Ambig {
		t.Error("13/01/2024 should not be ambiguous (13 can't be month)")
	}
}

func TestFindMonthName(t *testing.T) {
	tests := []struct {
		input string
		month int
	}{
		{"march 15, 2024", 3},
		{"15 jan 2024", 1},
		{"december 31, 1999", 12},
		{"sep 1, 2020", 9},
		{"no month here", 0},
	}

	for _, tt := range tests {
		month, _, _ := findMonthNameCI(tt.input, nil)
		if month != tt.month {
			t.Errorf("findMonthName(%q) = %d, want %d", tt.input, month, tt.month)
		}
	}
}

// sigToString converts a Signature to a string of character class letters.
func sigToString(sig *Signature) string {
	chars := []byte("DLSWCX")
	out := make([]byte, sig.len)
	for i := 0; i < sig.len; i++ {
		out[i] = chars[sig.buf[i]]
	}
	return string(out)
}
