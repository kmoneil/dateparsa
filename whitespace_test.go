package dateparsa

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// paddingBytes is the set isPadding accepts, named once so a test that adds a
// byte to the loop without adding it here fails on the count below.
var paddingBytes = []struct{ name, b string }{
	{"space", " "},
	{"tab", "\t"},
	{"newline", "\n"},
	{"carriage return", "\r"},
	{"vertical tab", "\v"},
	{"form feed", "\f"},
}

// TestSurroundingWhitespaceIsPadding is F9.
//
// The library refused a padded numeric date and accepted a padded textual one,
// and the tolerance was not a decision anybody made: textual formats locate
// their fields by scanning and coverGaps absorbs whatever is left over, and
// natural language skips space and tab while tokenising. The trie matches a
// signature over the whole input and whitespace is not in the signature, so
// " 2024-03-15" was ErrNoMatch while " March 15, 2024 " was a date.
//
// Every path is trimmed now, and the four that behave differently underneath
// are all here: the trie, the variable-width numeric detector, epoch, and
// natural language. Epoch is the one the card missed, and it refused padding in
// both directions.
func TestSurroundingWhitespaceIsPadding(t *testing.T) {
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	values := []struct {
		name, input, want string
	}{
		{"trie/ISO date", "2024-03-15", "2024-03-15 00:00:00"},
		{"trie/ISO datetime", "2024-03-15T10:30:00Z", "2024-03-15 10:30:00"},
		{"trie/SQL", "2024-03-15 10:30:00", "2024-03-15 10:30:00"},
		{"trie/time only", "10:30:00", "2026-01-01 10:30:00"},
		{"numeric/US slash", "03/15/2024", "2024-03-15 00:00:00"},
		{"numeric/variable width", "3/15/2024", "2024-03-15 00:00:00"},
		{"textual/month first", "March 15, 2024", "2024-03-15 00:00:00"},
		{"textual/day first", "15 Mar 2024", "2024-03-15 00:00:00"},
		{"textual/RFC 2822", "Fri, 15 Mar 2024 10:30:00 +0000", "2024-03-15 10:30:00"},
		{"epoch/seconds", "1710504800", "2024-03-15 12:13:20"},
		{"epoch/milliseconds", "1710504800000", "2024-03-15 12:13:20"},
		{"natural/relative", "3 days ago", "2026-08-17 12:00:00"},
	}

	for _, v := range values {
		for _, p := range paddingBytes {
			for _, shape := range []struct {
				name, in string
			}{
				{"leading", p.b + v.input},
				{"trailing", v.input + p.b},
				{"both", p.b + v.input + p.b},
				{"repeated", p.b + p.b + p.b + v.input + p.b + p.b},
			} {
				name := v.name + "/" + p.name + "/" + shape.name
				got, err := ParseWith(shape.in, WithBaseTime(base))
				if err != nil {
					t.Errorf("%s: ParseWith(%q) = %v, want %s", name, shape.in, err, v.want)
					continue
				}
				if s := got.Time.UTC().Format("2006-01-02 15:04:05"); s != v.want {
					t.Errorf("%s: ParseWith(%q) = %s, want %s", name, shape.in, s, v.want)
				}
			}
		}
	}
}

// TestPaddingDoesNotChangeTheAnswer is the property behind the table above,
// asserted rather than enumerated: a padded value and its trimmed form parse to
// the same instant, carry the same layout label, and report the same flags.
//
// The flags are the half worth stating. Padding is not evidence, so it cannot
// make an ambiguous input certain or a certain one a guess, and a change that
// trimmed inside a detector rather than ahead of them all could do either.
func TestPaddingDoesNotChangeTheAnswer(t *testing.T) {
	base := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	for _, in := range []string{
		"2024-03-15", "2024-03-15T10:30:00+05:30", "03/15/2024", "01/02/2024",
		"March 15, 2024", "15 Mar 2024", "March 15", "01MAY10", "MAY70",
		"10:30:00", "1710504800", "3 days ago", "20240315",
		"Friday, 15-Mar-24 10:30:00 UTC",
	} {
		want, err := ParseWith(in, WithBaseTime(base))
		if err != nil {
			t.Errorf("ParseWith(%q): %v", in, err)
			continue
		}
		for _, pad := range []string{" ", "\t\n", "  \r\n"} {
			padded := pad + in + pad
			got, err := ParseWith(padded, WithBaseTime(base))
			if err != nil {
				t.Errorf("ParseWith(%q): %v, want the same answer as %q", padded, err, in)
				continue
			}
			if !got.Time.Equal(want.Time) {
				t.Errorf("ParseWith(%q) = %v, ParseWith(%q) = %v", padded, got.Time, in, want.Time)
			}
			if got.Layout.String() != want.Layout.String() {
				t.Errorf("ParseWith(%q) layout %v, ParseWith(%q) layout %v",
					padded, got.Layout, in, want.Layout)
			}
			if got.Ambiguous != want.Ambiguous {
				t.Errorf("ParseWith(%q).Ambiguous = %v, ParseWith(%q) = %v: padding is not "+
					"evidence and cannot decide a guess", padded, got.Ambiguous, in, want.Ambiguous)
			}
			if got.Kind != want.Kind {
				t.Errorf("ParseWith(%q).Kind = %v, ParseWith(%q) = %v", padded, got.Kind, in, want.Kind)
			}
		}
	}
}

// TestPaddedRowReusesTheDetectedLayout is the half of F9 that is easy to miss
// and is the reason a column works at all.
//
// A spreadsheet export does not pad consistently: row 1 arrives as "2024-03-15"
// and row 900 as "2024-03-15 ". Detection trimming alone leaves the layout from
// row 1 refusing row 900, which Parser survives by falling back to detection,
// at the cost of the cache on every padded row and of an outright error for a
// caller holding the layout themselves.
func TestPaddedRowReusesTheDetectedLayout(t *testing.T) {
	for _, seed := range []string{"2024-03-15", " 2024-03-15", "2024-03-15\r\n"} {
		result, err := Parse(seed)
		if err != nil {
			t.Errorf("Parse(%q): %v", seed, err)
			continue
		}
		if !result.Layout.Reusable() {
			t.Errorf("Parse(%q).Layout.Reusable() = false", seed)
			continue
		}
		for _, row := range []string{"2024-03-16", " 2024-03-16", "2024-03-16 ", "\t2024-03-16\r\n"} {
			got, err := result.Layout.Parse(row)
			if err != nil {
				t.Errorf("layout from %q on %q: %v", seed, row, err)
				continue
			}
			if s := got.UTC().Format("2006-01-02"); s != "2024-03-16" {
				t.Errorf("layout from %q on %q = %s, want 2024-03-16", seed, row, s)
			}

			gotB, err := result.Layout.ParseBytes([]byte(row))
			if err != nil {
				t.Errorf("layout from %q on ParseBytes(%q): %v", seed, row, err)
				continue
			}
			if !gotB.Equal(got) {
				t.Errorf("layout from %q: Parse(%q) = %v, ParseBytes = %v", seed, row, got, gotB)
			}
		}
	}

	// The column, end to end, mixing padded and unpadded rows. Every row has to
	// agree with what detection alone would have said for it.
	p := NewParser()
	col := []string{"2024-03-15", "2024-03-16 ", " 2024-03-17", "2024-03-18\r\n", "2024-03-19"}
	times, errs := p.ParseColumn(col)
	for i, in := range col {
		if errs[i] != nil {
			t.Errorf("row %d %q: %v", i, in, errs[i])
			continue
		}
		fresh, err := Parse(in)
		if err != nil {
			t.Errorf("row %d %q: %v", i, in, err)
			continue
		}
		if !times[i].Equal(fresh.Time) {
			t.Errorf("row %d %q: Parser gave %v, detection gives %v", i, in, times[i], fresh.Time)
		}
	}
}

// TestCompiledLayoutKeepsItsWhitespace is why Layout.Parse trims for a detected
// layout and not for every layout.
//
// A layout compiled from a Go layout string describes what the caller wrote,
// spaces included, and both of the layouts below work today and agree with
// time.Parse. A blanket trim at the entry to Layout.Parse turns each into
// "layout describes 11 of 10 bytes", which is refusing input the stdlib
// accepts, and staying equivalent to the stdlib on a layout the caller handed
// us is what the four FuzzCompile_* targets exist for.
//
// The last case is the other direction and matters just as much. Compile
// against an unpadded layout still refuses a padded input, exactly as
// time.Parse does, so a compiled layout is neither more nor less lenient than
// it was.
func TestCompiledLayoutKeepsItsWhitespace(t *testing.T) {
	for _, c := range []struct {
		layout, input string
		wantErr       bool
	}{
		{" 2006-01-02", " 2024-03-15", false},
		{"2006-01-02 ", "2024-03-15 ", false},
		{"2006-01-02", "2024-03-15", false},
		{"2006-01-_2", "2024-03- 5", false},
		{"2006-01-_2", "2024-03-  ", true}, // a space where the digit goes

		// The trim would have made each of these succeed, and time.Parse
		// refuses all four.
		{"2006-01-02", "2024-03-15 ", true},
		{"2006-01-02", " 2024-03-15", true},
		{" 2006-01-02", "2024-03-15", true},
		{"2006-01-02 ", "2024-03-15", true},
	} {
		l, err := Compile(c.layout)
		if err != nil {
			t.Errorf("Compile(%q): %v", c.layout, err)
			continue
		}
		if l.trimsPadding {
			t.Errorf("Compile(%q) produced a layout that trims; only a detected one may",
				c.layout)
		}

		got, err := l.Parse(c.input)
		if c.wantErr {
			if err == nil {
				t.Errorf("Compile(%q).Parse(%q) = %v, want an error: time.Parse refuses it and "+
					"a compiled layout is documented as no more lenient than the stdlib",
					c.layout, c.input, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("Compile(%q).Parse(%q): %v", c.layout, c.input, err)
			continue
		}

		// And the answer itself, against the oracle rather than against a
		// literal, so a change to either side has to move both.
		want, stdErr := time.Parse(c.layout, c.input)
		if stdErr != nil {
			t.Errorf("test premise gone: time.Parse(%q, %q) = %v", c.layout, c.input, stdErr)
			continue
		}
		if !got.Equal(want) {
			t.Errorf("Compile(%q).Parse(%q) = %v, time.Parse = %v", c.layout, c.input, got, want)
		}
	}
}

// TestNBSPIsNotPadding pins the decision rather than the implementation.
//
// U+00A0 arrives from Word, from Excel and from copy-paste, which are the same
// sources as the padded column F9 exists for, and it is excluded anyway: it is
// a character rather than layout, a value containing one is arguably not the
// value, and admitting it turns a byte loop into a decode. Every wider reading
// costs more: the rest of Unicode Zs would make trimming a table question in a
// library that imports neither regexp nor unicode.
//
// It refused before F9 on every path, textual included, so this is what did not
// change and not what did.
func TestNBSPIsNotPadding(t *testing.T) {
	for _, in := range []string{
		" 2024-03-15",
		"2024-03-15 ",
		" March 15, 2024 ",
		" 2024-03-15", // figure space
		" 2024-03-15", // narrow no-break space
		"　2024-03-15", // ideographic space
	} {
		if got, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) = %v, want an error: only the six ASCII bytes are padding",
				in, got.Time)
		}
	}
}

// TestWhitespaceOnlyInputIsRefused is the edge the trim loops are written
// against each other for: an input that is entirely padding trims to "" rather
// than running off either end, and "" is refused by every detector.
func TestWhitespaceOnlyInputIsRefused(t *testing.T) {
	for _, in := range []string{
		"", " ", "  ", "\t", "\n", "\r\n", " \t\r\n\v\f ",
		strings.Repeat(" ", 1000),
	} {
		if got, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) = %v, want an error", in, got.Time)
		} else if !errors.Is(err, ErrNoMatch) {
			t.Errorf("Parse(%q) = %v, want ErrNoMatch", in, err)
		}
	}

	// Directly, because the loops are the thing being checked and Parse would
	// refuse an empty string whether or not they were right.
	for _, in := range []string{"", " ", "\t\r\n\v\f "} {
		if got := trimPadding(in); got != "" {
			t.Errorf("trimPadding(%q) = %q, want %q", in, got, "")
		}
		if got := trimPaddingBytes([]byte(in)); len(got) != 0 {
			t.Errorf("trimPaddingBytes(%q) = %q, want empty", in, got)
		}
	}
}

// TestParseErrorCarriesTheUntrimmedInput is the answer to the card's second
// decision, which asked whether an offset should be reported against the
// original or the trimmed value.
//
// The error names the value the caller passed, because that is the one they
// hold and will log. fieldError's offset is relative to what the executor was
// given, which is the trimmed span, and the doc comment on Layout.Parse says
// so. Reporting a whole input and a bounded message is the same split
// *ParseError already documents.
func TestParseErrorCarriesTheUntrimmedInput(t *testing.T) {
	const in = "  not a date at all  "
	_, err := Parse(in)
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("Parse(%q) = %T %v, want *ParseError", in, err, err)
	}
	if pe.Input != in {
		t.Errorf("ParseError.Input = %q, want %q: the caller logs what they passed",
			pe.Input, in)
	}

	// Same through a reused layout, which trims on its own path.
	result, err := Parse("2024-03-15")
	if err != nil {
		t.Fatal(err)
	}
	const row = "  not a date  "
	_, err = result.Layout.Parse(row)
	if !errors.As(err, &pe) {
		t.Fatalf("Layout.Parse(%q) = %T %v, want *ParseError", row, err, err)
	}
	if pe.Input != row {
		t.Errorf("ParseError.Input = %q, want %q", pe.Input, row)
	}
}

// TestTrimPaddingCoversExactlyTheSixBytes is the gate on the set itself. A byte
// added to isPadding without a reason is a byte this catches, and the two
// halves of the check are what stop it being a restatement of the loop: every
// byte in the set trims, and no other byte in 0..255 does.
func TestTrimPaddingCoversExactlyTheSixBytes(t *testing.T) {
	want := map[byte]bool{' ': true, '\t': true, '\n': true, '\r': true, '\v': true, '\f': true}
	if len(want) != len(paddingBytes) {
		t.Fatalf("paddingBytes has %d entries and the set has %d", len(paddingBytes), len(want))
	}
	for c := 0; c < 256; c++ {
		b := byte(c)
		trimmed := trimPadding(string(b)+"x"+string(b)) == "x"
		if trimmed != want[b] {
			t.Errorf("trimPadding trims byte %d (%q) = %v, want %v", c, string(b), trimmed, want[b])
		}
		if isPadding(b) != want[b] {
			t.Errorf("isPadding(%d) = %v, want %v", c, isPadding(b), want[b])
		}
	}
}
