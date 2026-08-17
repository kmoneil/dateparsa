package detect

import (
	"strings"
	"testing"

	"github.com/kmoneil/dateparsa/internal/compile"
	"github.com/kmoneil/dateparsa/internal/locale"
	_ "github.com/kmoneil/dateparsa/internal/locale/data"
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
			if int(f.Len) != want {
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
			for i := int(f.Offset); i < int(f.Offset)+int(f.Len) && i < len(in); i++ {
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
			off, w := int(f.Offset), int(f.Len)
			if fw, fixed := compile.FixedWidth(f.Kind); fixed {
				w = fw
			}
			if f.Kind == compile.FTail {
				w = len(in) - off
			}
			for i := off; i < off+w && i < len(in); i++ {
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

// TestWordMatcherAgreesWithScanning is the safety argument for prefiltering the
// month-name search by word span instead of scanning the input once per
// spelling. The two have to give the same answer for every spelling this
// package will ever ask about, over inputs that exercise the boundaries the old
// scan cared about: punctuation, digits, accents, repeats, and a word that
// contains a month name without being one.
//
// It also covers the fallback. An input with more words than monthWordCap
// leaves wordMatcher.words nil and find delegates to matchWordCI, so the
// property has to hold on both sides of that cap.
func TestWordMatcherAgreesWithScanning(t *testing.T) {
	spellings := append([]monthSpelling(nil), defaultMonths...)
	for _, tag := range locale.Tags() {
		d := locale.Lookup(tag)
		if d == nil {
			continue
		}
		spellings = append(spellings, getLocaleMonths(d).spellings...)
	}

	inputs := []string{
		"", "x", "march", "March", "MARCH", "march.", ".march", "1march2",
		"March 15, 2024", "15 mars 2024", "15 März 2024", "15 марта 2024",
		"mar 1 september 2024", "sept. 1, 2020", "sep 15", "sept 15",
		"marching band", "amarch", "marcha", "may may may",
		"MAY70", "MAY10", "december 25th at 5pm", "Fri, 15 Mar 2024 10:30:00 +0000",
		strings.Repeat("word ", monthWordCap-1) + "march",
		strings.Repeat("word ", monthWordCap) + "march",
		strings.Repeat("word ", monthWordCap+40) + "march",
		strings.Repeat(" ", 200) + "March 15, 2024",
		"\xff\xfe march \xff", "1é2é3",

		// The two ways a dotted spelling answered from the word list can
		// disagree with the scan, both found by running one against the other.
		// The first needs the byte after the dot checked, and the second needs
		// the search to continue past a word that is spelled right and has no
		// dot after it.
		"x sept.y", "sept sept.", "sept.", "sept.. 2020", "sept.1",
		"1 sept. 2020", "15 janv. 2024", "janv.2024", "SEPT. 1 2020",
		"dic. dic dic.", "x dic.y dic.",
	}

	sawFast, sawSlow := false, false
	sawList, sawDotted, sawScan := false, false, false
	sawFiltered := false
	for _, in := range inputs {
		var buf [monthWordCap]wordSpan
		m := newWordMatcher(in, buf[:])
		if m.words == nil {
			sawSlow = true
		} else {
			sawFast = true
		}
		for i := range spellings {
			sp := &spellings[i]
			switch sp.how {
			case lookupWordList:
				sawList = true
			case lookupDotted:
				sawDotted = true
			case lookupScan:
				sawScan = true
			}
			ws, we, wok := matchWordCI(in, sp.name)

			// The pair findMonthNameCI uses is the prefilter and the lookup,
			// so the pair is what has to agree with the scan. A filtered
			// spelling has to be one the scan does not find, or the filter is
			// deciding an answer instead of dismissing work.
			if sp.lenBit&m.lenMask == 0 {
				sawFiltered = true
				if wok {
					t.Errorf("lenMask dismissed %q (how=%d), but matchWordCI finds it in %q at (%d,%d)",
						sp.name, sp.how, in, ws, we)
				}
				continue
			}
			gs, ge, gok := m.findSpelling(sp)
			if gs != ws || ge != we || gok != wok {
				t.Errorf("findSpelling(%q, how=%d) in %q = (%d,%d,%v), matchWordCI = (%d,%d,%v)",
					sp.name, sp.how, in, gs, ge, gok, ws, we, wok)
			}
		}
	}
	if !sawFast || !sawSlow {
		t.Errorf("corpus did not exercise both paths: fast=%v slow=%v", sawFast, sawSlow)
	}
	if !sawList || !sawDotted || !sawScan {
		t.Errorf("corpus did not exercise every lookup: list=%v dotted=%v scan=%v",
			sawList, sawDotted, sawScan)
	}
	if !sawFiltered {
		t.Error("corpus never had a spelling dismissed by lenMask")
	}
}

// TestLenMaskNeverHidesAMatch is the prefilter's own property, stated without
// the lookup in the way: a spelling the mask dismisses cannot occur in the
// input as a whole word.
//
// The mask is a length filter over the input's word runs, and the bit a
// spelling carries is the length its lookup matches on, which is one shorter
// than the name for a dotted abbreviation. Getting that off by one would
// dismiss every dotted spelling in an input that holds the word, and every test
// above would still pass, because a dismissed spelling and an absent one look
// the same from outside.
func TestLenMaskNeverHidesAMatch(t *testing.T) {
	spellings := append([]monthSpelling(nil), defaultMonths...)
	for _, tag := range locale.Tags() {
		d := locale.Lookup(tag)
		if d == nil {
			continue
		}
		spellings = append(spellings, getLocaleMonths(d).spellings...)
	}

	// Inputs built from the spellings themselves, so a match is present far
	// more often than in a corpus of hand-written dates.
	var inputs []string
	for i := range spellings {
		n := spellings[i].name
		inputs = append(inputs, n, n+" 1, 2024", "1 "+n+" 2024", "x "+n+" y",
			strings.ToUpper(n), n+".", "."+n, n+n)
	}
	inputs = append(inputs, "", " ", ".", "N/A", "2024-03-15", "1月1日",
		// Over the word cap, where there is no word list and every spelling
		// falls back to scanning. The mask has to let all of them through.
		strings.Repeat("word ", monthWordCap+2)+"march",
		strings.Repeat("word ", monthWordCap+2)+"sept. 1",
	)

	dismissed, present := 0, 0
	for _, in := range inputs {
		var buf [monthWordCap]wordSpan
		m := newWordMatcher(in, buf[:])
		for i := range spellings {
			sp := &spellings[i]
			if _, _, ok := matchWordCI(in, sp.name); ok {
				present++
				if sp.lenBit&m.lenMask == 0 {
					t.Fatalf("lenMask dismissed %q (how=%d, lenBit=%#x) which occurs in %q (lenMask=%#x)",
						sp.name, sp.how, sp.lenBit, in, m.lenMask)
				}
				continue
			}
			if sp.lenBit&m.lenMask == 0 {
				dismissed++
			}
		}
	}
	if present == 0 || dismissed == 0 {
		t.Errorf("corpus proved nothing: %d present, %d dismissed", present, dismissed)
	}
	t.Logf("%d occurrences kept, %d spellings dismissed", present, dismissed)
}

// TestClassifySpelling pins the three lookups to the shapes they are for. A
// spelling classified as lookupWordList that is not word characters throughout
// stops matching entirely, which no format test would necessarily notice: the
// input just fails to detect.
func TestClassifySpelling(t *testing.T) {
	tests := []struct {
		name string
		want spellingLookup
	}{
		{"march", lookupWordList},
		{"mars", lookupWordList},
		{"März", lookupWordList},  // non-ASCII bytes are word characters
		{"марта", lookupWordList}, // so Cyrillic takes the word list too
		{"sept.", lookupDotted},
		{"janv.", lookupDotted},
		{"1月", lookupScan}, // a digit is not a word character
		{"1월", lookupScan},
		{"s.p.", lookupScan}, // a dot that is not only trailing
		{".", lookupScan},
		{"", lookupWordList}, // vacuously; findKnown dismisses it on length
	}
	for _, tt := range tests {
		if got := classifySpelling(tt.name); got != tt.want {
			t.Errorf("classifySpelling(%q) = %d, want %d", tt.name, got, tt.want)
		}
	}
}

// TestLocaleMonthsMatchesTheLocaleData checks the prepared table against the
// data it is built from: same spellings, same order, same month numbers as the
// wide, abbreviated, dot-stripped sequence findMonthNameCI documents.
func TestLocaleMonthsMatchesTheLocaleData(t *testing.T) {
	for _, tag := range locale.Tags() {
		d := locale.Lookup(tag)
		if d == nil {
			continue
		}
		var want []monthSpelling
		for i := range 12 {
			if n := d.MonthsWide[i]; n != "" {
				want = append(want, monthSpelling{name: n, num: i + 1})
			}
			if n := d.MonthsAbbr[i]; n != "" {
				want = append(want, monthSpelling{name: n, num: i + 1})
				if c := strings.TrimRight(n, "."); c != n {
					want = append(want, monthSpelling{name: c, num: i + 1})
				}
			}
		}
		got := getLocaleMonths(d).spellings
		if len(got) != len(want) {
			t.Errorf("%s: prepared %d spellings, the data has %d", tag, len(got), len(want))
			continue
		}
		for i := range want {
			if got[i].name != want[i].name || got[i].num != want[i].num {
				t.Errorf("%s: spelling %d is (%q, %d), want (%q, %d)",
					tag, i, got[i].name, got[i].num, want[i].name, want[i].num)
			}
		}
	}
}

// TestTrieLiteralsCarryTheirClass holds the invariant stampLiteralClasses
// depends on, over the trie Detect actually walks rather than a rebuilt copy.
//
// Every literal in a trie entry is one byte, and it sits at a signature
// position whose class is not CDigit. Both halves matter. A wider literal would
// need a class per byte and Aux holds one, so it is left at "not a digit" and
// nobody notices. A literal at a digit position would be stamped ClassAny,
// which is the one class that excludes digits, and the format would refuse the
// input it matched on.
func TestTrieLiteralsCarryTheirClass(t *testing.T) {
	var walk func(n *trieNode)
	seen := 0
	walk = func(n *trieNode) {
		if e := n.entry; e != nil {
			for _, f := range e.fields {
				if f.Kind != compile.FLiteral {
					continue
				}
				seen++
				if f.Len != 1 {
					t.Errorf("%s: literal at offset %d is %d bytes wide, and a class covers one",
						e.name, f.Offset, f.Len)
					continue
				}
				if int(f.Offset) >= len(e.sig) {
					t.Errorf("%s: literal at offset %d is past its %d-class signature",
						e.name, f.Offset, len(e.sig))
					continue
				}
				cc := e.sig[f.Offset]
				if cc == CDigit {
					t.Errorf("%s: literal at offset %d sits at a CDigit position", e.name, f.Offset)
					continue
				}
				if want := compile.AuxFor(litClassOf[cc]); f.Aux != want {
					t.Errorf("%s: literal at offset %d has Aux %d, want %d for class %d",
						e.name, f.Offset, f.Aux, want, cc)
				}
			}
		}
		for _, c := range n.children {
			if c != nil {
				walk(c)
			}
		}
	}
	walk(&globalTrie.root)

	// A stamping pass that silently stopped matching anything would pass every
	// assertion above.
	if seen < 100 {
		t.Errorf("walked %d trie literals, want the ~105 the formats declare", seen)
	}
}

// TestLitClassSupersetsScan holds the direction that decides whether the class
// check can refuse an input detection accepted.
//
// A trie entry matches because Scan gave every byte the class the signature
// names. The literal at one of those positions now asks compile whether the
// byte is in that class, and compile answers from a context-free table while
// Scan reads three of its classes from context: 'T' between digits, 'Z' at the
// end or before a sign, '-' after a time. If the table were narrower than Scan
// anywhere, a format would refuse its own input.
//
// Refusing is the only failure worth testing for. The table being wider than
// Scan costs over-acceptance, which is what the class was narrowing in the
// first place, and the cross-format sweep in the root package is what measures
// that.
func TestLitClassSupersetsScan(t *testing.T) {
	// Contexts that between them reach every arm of Scan's switch.
	contexts := []struct {
		before, after string
	}{
		{"", ""},
		{"1", "1"},
		{"1", ""},
		{"", "1"},
		{"10:30:00", ""},
		{"10:30:00", "05:00"},
		{"2024-03-15", "10:30:00"},
	}

	for b := 0; b < 256; b++ {
		c := byte(b)
		for _, ctx := range contexts {
			s := ctx.before + string(c) + ctx.after
			sig := Scan(s)
			at := len(ctx.before)
			if at >= sig.Len() {
				continue
			}
			cc := sig.At(at)
			if cc == CDigit {
				continue // never a literal position
			}
			if !compile.AuxAccepts(compile.AuxFor(litClassOf[cc]), c) {
				t.Errorf("Scan(%q) gives byte %#x class %d, which compile refuses",
					s, c, cc)
			}
		}
	}
}

// TestLitClassOfIsComplete checks the map has an entry for every class a
// literal can sit at. A class added to the scanner with no entry here maps to
// the zero value, which is ClassAny, and the format goes back to accepting any
// byte that is not a digit without anything failing.
func TestLitClassOfIsComplete(t *testing.T) {
	want := map[CharClass]compile.LitClass{
		CLetter:  compile.ClassLetter,
		CSep:     compile.ClassSep,
		CSpace:   compile.ClassSpace,
		CColon:   compile.ClassColon,
		CSpecial: compile.ClassSpecial,
	}
	for cc := CharClass(0); cc < numClasses; cc++ {
		if cc == CDigit {
			continue
		}
		if litClassOf[cc] != want[cc] {
			t.Errorf("litClassOf[%d] = %d, want %d", cc, litClassOf[cc], want[cc])
		}
	}
}

// TestAmbiguousResultCarriesItsReadings is the invariant at the layer that sets
// the flag: a detector that says this input needed a guess has to say which
// guess, and that kind has to be able to produce both readings.
//
// Ambig without a kind is what strict mode cannot do anything with. It used to
// answer every ambiguity by detecting again with PreferDayFirst flipped, which
// is a preference the textual detector does not read and the dot heuristic
// overrules, so two of the shapes below came back as two copies of one reading.
func TestAmbiguousResultCarriesItsReadings(t *testing.T) {
	inputs := []string{
		"01/02/2024", "01/02/03", "01-02-03", "03.02.2024", "1/2/2024",
		"01/02/2024 10:30:00",
		"March 15", "MAY 15", "MAY15", "15 March", "December 25",
		"Mar 15 10:30:00", "Sept 09",
	}
	for _, in := range inputs {
		r, ok := Detect(in, Config{})
		if !ok {
			t.Errorf("Detect(%q) found no format", in)
			continue
		}
		if !r.Ambig {
			t.Errorf("test premise gone: Detect(%q) is no longer ambiguous", in)
			continue
		}
		if r.AmbigKind == AmbigNone {
			t.Errorf("Detect(%q): Ambig with no AmbigKind", in)
			continue
		}
		readings := r.Readings()
		if len(readings) < 2 {
			t.Errorf("Detect(%q): AmbigKind %d offers %d reading(s); an input that needed a "+
				"guess has at least two", in, r.AmbigKind, len(readings))
			continue
		}
		chosen := readings[0]
		seen := map[string]bool{}
		for _, alt := range readings {
			if alt.Def == nil {
				t.Errorf("Detect(%q): reading %q has no format", in, alt.Label)
				continue
			}
			if seen[alt.Label] {
				t.Errorf("Detect(%q): two readings are labelled %q", in, alt.Label)
			}
			seen[alt.Label] = true
			if len(chosen.Def.Fields) != len(alt.Def.Fields) {
				t.Errorf("Detect(%q): the readings describe %d and %d fields; an alternative "+
					"reads the same bytes differently and cannot read different bytes",
					in, len(chosen.Def.Fields), len(alt.Def.Fields))
				continue
			}
			differ := 0
			for i := range chosen.Def.Fields {
				c, a := chosen.Def.Fields[i], alt.Def.Fields[i]
				if c.Offset != a.Offset || c.Len != a.Len {
					t.Errorf("Detect(%q): field %d moved between the readings, %d+%d to %d+%d",
						in, i, c.Offset, c.Len, a.Offset, a.Len)
				}
				if c.Kind != a.Kind {
					differ++
				}
			}
			if differ == 0 && alt.Label != chosen.Label {
				t.Errorf("Detect(%q): readings %q and %q are the same program",
					in, chosen.Label, alt.Label)
			}
		}
	}
}

// TestUnambiguousResultHasNoReadings is the other side of it. An input that
// needed no guess has nothing to choose between, and Readings must not invent a
// second answer for one.
func TestUnambiguousResultHasNoReadings(t *testing.T) {
	inputs := []string{
		"2024-03-15", "13/01/2024", "March 15, 2024", "March 2024", "March 32",
		"MAY70", "March 5", "March 15th", "15th March", "2024-03-15T10:30:00Z",
	}
	for _, in := range inputs {
		r, ok := Detect(in, Config{})
		if !ok {
			t.Errorf("Detect(%q) found no format", in)
			continue
		}
		if r.Ambig || r.AmbigKind != AmbigNone {
			t.Errorf("Detect(%q): Ambig=%v AmbigKind=%d, want neither", in, r.Ambig, r.AmbigKind)
		}
		if got := r.Readings(); got != nil {
			t.Errorf("Detect(%q): offers %d readings of an input that needed no guess", in, len(got))
		}
	}
}

// TestOrdinalSuffixEndsTheDayOrYearQuestion pins the shape that has one reading
// only. "March 15th" is the fifteenth: no year is written "15th", so the number
// is not a two-digit year and the format cannot take one either, which is why
// AmbigProne goes with the flag rather than staying behind.
func TestOrdinalSuffixEndsTheDayOrYearQuestion(t *testing.T) {
	for _, in := range []string{"March 15th", "MAY 15th", "15th March", "December 25th", "March 1st"} {
		r, ok := Detect(in, Config{})
		if !ok {
			t.Errorf("Detect(%q) found no format", in)
			continue
		}
		if r.Ambig || r.AmbigProne {
			t.Errorf("Detect(%q): Ambig=%v AmbigProne=%v, want neither: an ordinal suffix "+
				"is a day and every input this layout accepts carries one", in, r.Ambig, r.AmbigProne)
		}
	}
	// Without the suffix the same shapes are a guess, so the test above is not
	// passing because the inputs stopped being detected.
	for _, in := range []string{"March 15", "MAY 15", "15 March", "December 25"} {
		r, ok := Detect(in, Config{})
		if !ok || !r.Ambig {
			t.Errorf("test premise gone: Detect(%q) ok=%v Ambig=%v", in, ok, r.Ambig)
		}
	}
}
