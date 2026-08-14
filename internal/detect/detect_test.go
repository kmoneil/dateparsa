package detect

import (
	"strings"
	"testing"

	"github.com/kmoneil/dateparsa/internal/compile"
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

// TestEveryFieldDeclaresTheWidthItsOpReads asserts the invariant C9 violated:
// a Field's Len is the number of bytes the instruction for its Kind will read.
//
// buildDatePartFields gave a three-character part an FDay2 with Len 3. OpDay2
// reads exactly two, so detection validated the twentieth and the program
// returned the second, and Parse("020/01/2024") came back as 2024-01-02 with no
// error. Comparing Parse against Layout.Parse could not catch it: both run the
// same program, so both were wrong identically and agreed.
//
// The corpus is deliberately full of shapes nobody writes on purpose. A
// well-formed input cannot produce the mismatch, which is why the round-trip
// generator never did.
func TestEveryFieldDeclaresTheWidthItsOpReads(t *testing.T) {
	corpus := []string{
		// Well-formed, one per family.
		"2024-03-15", "2024-03-15T10:30:00Z", "2024-03-15T10:30:00+05:30",
		"2024-03-15 10:30:00", "2024-03-15 10:30:00 UTC", "20240315",
		"March 15, 2024", "15 Mar 2024", "sept. 1, 2020", "March 2024",
		"3/15/2024", "15.03.2024", "01/02/2024", "10:30", "10:30:00", "10:30 PM",
		"2024-W11-5", "2024-074", "2014年04月08日", "2020-07-20+08:00",
		"2015-02-08 03:02:00 +0300 MSK", "Fri Jul 03 2015 18:04:07 GMT+0100",
		"December 23rd", "September 17, 2012 at 10:09am",

		// Zero-padded and over-wide parts, which is where it went wrong.
		"020/01/2024", "3/015/2024", "020/1/0000", "0020/1/2024",
		"1/2/003", "013/1/2", "003/1/2", "1/0002/3", "0001/02/03",
		"17/11/0000", "1/1/1", "12/12/12", "123/1/2024", "1/234/2024",
	}

	for _, in := range corpus {
		r, ok := Detect(in, Config{})
		if !ok || r.Def == nil {
			continue // refusing is always allowed
		}
		for i, f := range r.Def.Fields {
			want, fixed := compile.FixedWidth(f.Kind)
			if !fixed {
				continue
			}
			if f.Len != want {
				t.Errorf("Detect(%q) field %d: Kind %d declares Len %d, its op reads %d",
					in, i, f.Kind, f.Len, want)
			}
		}
	}
}

// TestMaybeLetterCoversScan checks the direction maybeLetter has to hold in:
// every byte Scan can classify as CLetter must answer true, so that a letter
// past the signature buffer is never missed. The reverse is allowed to fail,
// and does for 'Z', which Scan makes CSpecial at the end of a string.
func TestMaybeLetterCoversScan(t *testing.T) {
	for c := 0; c < 256; c++ {
		b := byte(c)
		// Put the byte between two spaces so no context rule can fire, which
		// is the classification maybeLetter is answering about.
		sig := Scan(" " + string(b) + " ")
		if sig.HasLetter && !maybeLetter(b) {
			t.Errorf("Scan classifies %q as a letter, maybeLetter says no", b)
		}
	}

	// The bytes that must not count, or a numeric date behind a long separator
	// run would start reaching the textual detector.
	for _, b := range []byte("0123456789-/. \t:+,") {
		if maybeLetter(b) {
			t.Errorf("maybeLetter(%q) = true, want false", b)
		}
	}
}

// TestLetterPastTheSignatureIsFound covers C13 at the detection layer. A date
// behind 64 bytes of anything but letters was never offered to the textual
// detector, because the condition was computed from a pass that stops at
// maxSigLen.
func TestLetterPastTheSignatureIsFound(t *testing.T) {
	for _, pad := range []int{0, maxSigLen - 1, maxSigLen, maxSigLen + 1, 100} {
		in := strings.Repeat(" ", pad) + "March 15, 2024"
		r, ok := Detect(in, Config{})
		if !ok || r.Def == nil {
			t.Errorf("Detect with %d bytes of padding found no format", pad)
			continue
		}
		if r.Def.Name != "MONTH_DAY_YEAR" {
			t.Errorf("Detect with %d bytes of padding = %s, want MONTH_DAY_YEAR", pad, r.Def.Name)
		}
	}

	// hasLetterPastSignature must not look at bytes Scan already covered, or it
	// is doing the work twice on every input that misses the trie.
	if hasLetterPastSignature("March 15, 2024") {
		t.Error("hasLetterPastSignature looked inside the signature buffer")
	}
	if !hasLetterPastSignature(strings.Repeat(" ", maxSigLen) + "M") {
		t.Error("hasLetterPastSignature missed the first byte past the buffer")
	}
}

// TestNoUnreadRunCoversADigit is the detection half of the rule OpSkip and
// OpLiteral enforce at run time: a byte the program does not read may not be a
// digit.
//
// The executor's checks are what stop a reused layout hiding a numeric token
// where it declared punctuation. This one says detection never asks it to: a
// format that emitted an unread run over a digit would be refusing the very
// input it was detected from, which is a detector bug and not a reuse question.
//
// An unread run is a skip, or a literal that names no byte. A literal that does
// name one is checked against that byte instead, so a digit there would be
// consistent and is not this test's business.
//
// The numeric corpus is here because C11 named it as the family to check rather
// than assume. buildDatePartFields separates its parts with literals rather
// than skips, and both are covered here.
func TestNoUnreadRunCoversADigit(t *testing.T) {
	corpus := []string{
		// Textual, which is where the skips are: weekday names, punctuation,
		// ordinal suffixes, " at ", and the name half of "GMT+0100".
		"March 15, 2024", "15 Mar 2024", "sept. 1, 2020", "March 2024",
		"December 23rd", "September 17, 2012 at 10:09am", "March 1st, 2024",
		"Fri Jul 03 2015 18:04:07 GMT+0100", "Thu, 4 Jan 2018 17:53:36 +0000",
		"Mon Jan 2 15:04:05 2006", "Sat, 03 Feb 2024 11:45:00 GMT",
		"MAY A1", "MAY B2", "1MAY",

		// Numeric, where the unread runs are the separators between parts.
		"2024-03-15", "2024/03/15", "20240315", "3/15/2024", "15.03.2024",
		"01/02/2024", "2024-03-15T10:30:00Z", "2024-03-15 10:30:00 UTC",
		"2015-02-08 03:02:00 +0300 MSK", "2024-W11-5", "2024-074",
		"2014年04月08日", "10:30:00.123", "10:30 PM", "10:30", "00:00:00",
		"2024:03:15", "2024-03-15T10:30:00.123456+05:30", "20240315T103000Z",
	}

	for _, in := range corpus {
		r, ok := Detect(in, Config{})
		if !ok || r.Def == nil {
			continue // refusing is always allowed
		}
		for _, f := range r.Def.Fields {
			unread := f.Kind == compile.FSkip ||
				(f.Kind == compile.FLiteral && f.Aux == 0)
			if !unread {
				continue
			}
			for i := f.Offset; i < f.Offset+f.Len && i < len(in); i++ {
				if in[i] >= '0' && in[i] <= '9' {
					t.Errorf("Detect(%q) [%s]: %s at %d len %d covers the digit %q at %d",
						in, r.Def.Name, kindName(f.Kind), f.Offset, f.Len, in[i:i+1], i)
					break
				}
			}
		}
	}
}

func kindName(k compile.FieldKind) string {
	if k == compile.FSkip {
		return "skip"
	}
	return "literal"
}

// TestEveryInputByteIsDescribedExactlyOnce asserts what the executor's coverage
// check assumes: every byte of an input a format claims belongs to exactly one
// field, no byte to none and no byte to two.
//
// The executor counts bytes rather than marking them, because it runs per
// instruction on the hot path, and a sum only equals the length if there are no
// gaps AND no overlaps. This test is the half that a sum cannot check.
//
// It exists because a gap was a wrong answer, not just untidiness: ISO_ORDINAL
// described its year and its day-of-year and nothing for the '-' between them,
// so a layout built from "0000-001" accepted the compact date "00000101" and
// read its "0101" as day-of-year 101.
func TestEveryInputByteIsDescribedExactlyOnce(t *testing.T) {
	corpus := []string{
		"2024-03-15", "2024/03/15", "2024.03.15",
		"2024-03-15T10:30:00Z", "2024-03-15T10:30:00+05:30",
		"2024-03-15T10:30:00.123456+05:30", "2024-03-15T10:30:00.123Z",
		"2024-03-15 10:30:00", "2024-03-15 10:30:00 UTC", "2024-03-15 10:30:00.123456",
		"2015-02-08 03:02:00 +0300 MSK", "2012-08-03 18:31:59.257000000 +0000 UTC",
		"2024-03-15 10:30:00 m=+0.000000001",
		"0000-001", "2024-074", "2024-W11-5", "2024-W11",
		"2014年04月08日", "2020-07-20+08:00", "20240315", "20240315T103000Z",
		"March 15, 2024", "September 17, 2012 at 10:09am", "15 Mar 2024",
		"December 23rd", "March 2024", "sept. 1, 2020",
		"Fri Jul 03 2015 18:04:07 GMT+0100", "Thu, 4 Jan 2018 17:53:36 +0000",
		"3/15/2024", "3/15/2024 10:30:00 AM", "3/15/2024 10:30:00",
		"15.03.2024", "01/02/2024", "10:30", "10:30:00", "10:30 PM", "10:30:00.123",
	}

	for _, in := range corpus {
		r, ok := Detect(in, Config{})
		if !ok || r.Def == nil {
			continue // refusing is always allowed
		}
		count := make([]int, len(in))
		for _, f := range r.Def.Fields {
			w := f.Len
			if fw, fixed := compile.FixedWidth(f.Kind); fixed {
				w = fw
			}
			if f.Kind == compile.FTail {
				w = len(in) - f.Offset
			}
			for i := f.Offset; i < f.Offset+w && i < len(in); i++ {
				count[i]++
			}
		}
		for i, c := range count {
			if c != 1 {
				t.Errorf("Detect(%q) [%s]: byte %d (%q) described %d times, want 1",
					in, r.Def.Name, i, in[i:i+1], c)
				break
			}
		}
		if n := len(r.Def.Fields); n > compile.MaxInstructions {
			t.Errorf("Detect(%q) [%s]: %d fields, over MaxInstructions %d",
				in, r.Def.Name, n, compile.MaxInstructions)
		}
	}
}
