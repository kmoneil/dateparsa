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

func TestDetectTextualMonth_AllPatterns(t *testing.T) {
	cfg := Config{}
	tests := []struct {
		input   string
		wantOK  bool
		wantDef string // expected DefName prefix or "" if not checked
		desc    string
	}{
		// Pattern: "Month Day, Year" (beforeNums=0, afterNums>=2)
		{"March 15, 2024", true, "MONTH_DAY_YEAR", "month day comma year"},
		{"Mar 15, 2024", true, "MONTH_DAY_YEAR", "abbreviated month day comma year"},
		{"January 1, 2000", true, "MONTH_DAY_YEAR", "month day year with 1"},

		// Pattern: "Day Month Year" (beforeNums>=1, afterNums>=1)
		{"15 March 2024", true, "DAY_MONTH_YEAR", "day month year"},
		{"15 Mar 2024", true, "DAY_MONTH_YEAR", "day abbr month year"},
		{"1 January 2020", true, "DAY_MONTH_YEAR", "single digit day month year"},

		// Pattern: "Month Year" (beforeNums=0, afterNums=1, value>31)
		{"March 2024", true, "MONTH_YEAR", "month year only"},

		// Pattern: "Month Day" (beforeNums=0, afterNums=1, value<=31)
		{"March 15", true, "MONTH_DAY", "month day only"},
		{"December 25", true, "MONTH_DAY", "month day christmas"},

		// Pattern: "Day Month" (beforeNums=1, afterNums=0)
		{"15 March", true, "DAY_MONTH", "day month only"},
		{"1 January", true, "DAY_MONTH", "single day month only"},

		// RFC 2822 style: "Fri, 15 Mar 2024 10:30:00 +0000"
		{"Fri, 15 Mar 2024 10:30:00 +0000", true, "DAY_MONTH_YEAR", "rfc2822 with weekday"},

		// With time component
		{"Mar 15, 2024 10:30:00", true, "", "textual with time"},

		// Should NOT match (NL expressions with "at" and no year)
		{"december 25th at 5pm", false, "", "NL expression should bail"},

		// No match — no month name
		{"2024-03-15", false, "", "ISO date has no textual month"},
	}

	for _, tt := range tests {
		result, ok := detectTextualMonth(tt.input, cfg)
		if ok != tt.wantOK {
			t.Errorf("%s: detectTextualMonth(%q) ok=%v, want %v", tt.desc, tt.input, ok, tt.wantOK)
			continue
		}
		if ok && tt.wantDef != "" && result.Def.Name != tt.wantDef {
			t.Errorf("%s: detectTextualMonth(%q) name=%q, want %q", tt.desc, tt.input, result.Def.Name, tt.wantDef)
		}
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

func TestScan_SpecialCharClassification(t *testing.T) {
	tests := []struct {
		input string
		want  string
		desc  string
	}{
		// T classification: special between digits, letter otherwise
		{"2024-03-15T10:30:00", "DDDDSDDSDDXDDCDDCDD", "T between digits is CSpecial"},
		{"T12345", "LDDDDD", "T at start is CLetter"},
		{"abcT", "LLLL", "T after letters is CLetter"},
		{"1T2", "DXD", "T between single digits is CSpecial"},

		// Z classification: special at end or before +/-, letter otherwise
		{"2024-03-15T10:30:00Z", "DDDDSDDSDDXDDCDDCDDX", "Z at end is CSpecial"},
		{"Zone", "LLLL", "Z followed by letter is CLetter"},
		{"Z", "X", "bare Z is CSpecial (end of string)"},
		{"Z+05:00", "XXDDCDD", "Z before + is CSpecial"},
		{"Z-05:00", "XXDDCDD", "Z before - is CSpecial"},
		{"Z1234", "LDDDD", "Z before digit is CLetter"},

		// + is always CSpecial
		{"+05:30", "XDDCDD", "+ is CSpecial"},

		// - classification: separator normally, CSpecial at TZ position
		{"2024-03-15", "DDDDSDDSDD", "- as date separator is CSep"},
		{"10:30:00-05:00", "DDCDDCDDXDDCDD", "- after time pattern is CSpecial"},

		// Comma is CSpecial
		{"Fri, 15", "LLLXWDD", "comma is CSpecial"},

		// Dot is CSep
		{"15.03.2024", "DDSDDSDDDD", "dot is CSep"},

		// Space is CSpace
		{"2024 03", "DDDDWDD", "space is CSpace"},

		// Colon is CColon
		{"10:30", "DDCDD", "colon is CColon"},

		// HasLetter flag
		{"2024-03-15", "", "no letters => HasLetter=false"},
		{"March", "", "letters => HasLetter=true"},
	}

	for _, tt := range tests {
		sig := Scan(tt.input)

		// Check HasLetter for the last two test cases
		if tt.desc == "no letters => HasLetter=false" {
			if sig.HasLetter {
				t.Errorf("%s: HasLetter should be false for %q", tt.desc, tt.input)
			}
			continue
		}
		if tt.desc == "letters => HasLetter=true" {
			if !sig.HasLetter {
				t.Errorf("%s: HasLetter should be true for %q", tt.desc, tt.input)
			}
			continue
		}

		got := sigToString(&sig)
		if got != tt.want {
			t.Errorf("%s: Scan(%q) = %q, want %q", tt.desc, tt.input, got, tt.want)
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
