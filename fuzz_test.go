package dateparsa

import "testing"

// FuzzParse ensures that arbitrary inputs never panic, and that the Layout
// Parse hands back reproduces the time Parse returned.
//
// The equality half is the one that matters. This target used to call
// Layout.Parse and check only that it returned no error, which asserts that
// the layout is usable and says nothing about whether it is right. The whole
// premise of the library is that the detection result is reusable, and a
// reusable layout that returns a different instant from the call that produced
// it is the failure that premise exists to rule out.
func FuzzParse(f *testing.F) {
	// Seed corpus with valid formats.
	seeds := []string{
		"2024-03-15",
		"2024-03-15T10:30:00Z",
		"2024-03-15T10:30:00+05:30",
		"2024-03-15 10:30:00",
		"March 15, 2024",
		"15 Mar 2024",
		"01/02/2024",
		"15.03.2024",
		"10:30",
		"10:30:00",
		"",
		"not a date",
		"2024-03-15T10:30:00.123456789Z",

		// Panicked in detect.trimAtSuffix with a slice bounds error. It took
		// the index of " at " from a strings.ToLower copy and sliced the
		// original with it; the invalid bytes lower to a three-byte U+FFFD
		// each, so the copy is longer than the input and the index runs past
		// its end. A month name is needed to reach that code at all.
		"dEC0000A\xbe\xc2\xd0 At 0",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		// Must not panic. Errors are fine.
		result, err := Parse(input)
		if err != nil {
			return
		}

		// If parsing succeeded with a reusable layout, it must round-trip.
		// Sentinel layouts (epoch, NL) are not reusable, so skip them.
		if result.Layout == nil ||
			result.Layout == LayoutEpoch ||
			result.Layout == LayoutNaturalLanguage {
			return
		}

		reparsed, err := result.Layout.Parse(input)
		if err != nil {
			t.Errorf("Layout.Parse(%q) failed after successful Parse: %v", input, err)
			return
		}

		// Equal compares instants, so a layout that resolves the same moment
		// in a different location still passes. Anything else is a genuine
		// disagreement between detection and reuse.
		if !reparsed.Equal(result.Time) {
			t.Errorf("layout %s disagrees with Parse on %q:\n"+
				"  Parse        = %v\n"+
				"  Layout.Parse = %v",
				result.Layout, input, result.Time, reparsed)
		}
	})
}

// FuzzLayoutReuse asserts the property Parser actually depends on: a Layout
// detected from one input, applied to a different input, either refuses or
// agrees with what detection would have produced for that second input.
//
// FuzzParse checks a Layout against the input it was detected from, which is
// the easy direction and holds today (6.6M executions, no counterexample).
// Parser.Parse does something stronger: it caches the last successful Layout
// and applies it to the next value, treating "no error" as "right format". So
// the property that decides whether ParseColumn is correct is a two-input one,
// and nothing was checking it.
//
// Refusing is always allowed. A cached layout that rejects a row is a layout
// Parser falls back from, which is the safe outcome. What must not happen is a
// silent wrong answer: a layout that accepts a row it does not describe and
// returns a time detection would not have chosen.
func FuzzLayoutReuse(f *testing.F) {
	// Pairs where the first value's format is a prefix of the second's. This
	// is the shape a real column takes when a date-only row precedes a
	// timestamp row, and it is where a prefix match costs the caller a time
	// of day and a timezone.
	seeds := [][2]string{
		{"2024-03-15", "2024-03-16T10:30:00Z"},
		{"2024-03-15", "2024-03-17T23:59:59+05:30"},
		{"2024-03-15", "2024-03-15 10:30:00"},
		{"10:30", "10:30:45"},
		{"2024-03-15T10:30:00Z", "2024-03-15"},
		{"March 15, 2024", "15 Mar 2024"},
		{"01/02/2024", "25/12/2024"},
		{"", ""},

		// Found by this target in 88s, and the reason a length comparison
		// between the two inputs is not a shortcut for this property: both are
		// 10 bytes. detectVariableNumeric stops the date at the first space,
		// so Parse("70/01/1 00") succeeds while silently dropping " 00", and
		// the layout it returns describes only 7 of the 10 bytes. Reused, its
		// fields land on the wrong digits: year at 0..2 reads "01", day at 6
		// reads "10", and 1000-01-01 comes back as 2001-01-10.
		{"70/01/1 00", "01/01/1000"},

		// OpMonthName took the month from the instruction and read no bytes, so
		// a layout answered with the month it was detected from. Minimised to a
		// pair of four-byte inputs, because a name of a different width shifts
		// the fields after it and fails the day parse instead.
		{"1MAY", "1MAr"},

		// A three-character day part got an FDay2, which reads two, so the two
		// inputs put their fields in different places and the cached layout
		// read the second one's month out of its day.
		{"020/1/0000", "17/11/0000"},

		// ISO_ORDINAL described its year and its day-of-year and nothing for
		// the '-' between them, so it accepted a compact eight-digit date and
		// read its "0101" as day-of-year 101.
		{"0000-001", "00000101"},

		// A skip fixed a run's width and read nothing, so a digit could sit in
		// it. The first pair widens the day as well, and 3 + 2 + 2 balances
		// exactly, which is why counting bytes could not see it; the second
		// needs no widening at all and reads May 5th where detection reads the
		// 15th.
		{"MAY A1", "MAY1010"},
		{"MAY A1", "MAY 15"},

		// The same rule at the other unread instruction. A trie format's
		// literals name no byte, because the entry matches a signature class
		// and one entry serves every byte in it, so TIME_HMS declared ':' at 2
		// and at 5 and accepted eight digits.
		{"00:00:00", "00000101"},
		{"10:30", "1030"},

		// Refusing a digit was not enough at those same positions. A date and a
		// time are both DD?DD?DD and ':' is not a digit, so each format read
		// the other's input and answered with the wrong kind of value: a date
		// layout returned 2000-01-10 for one minute past ten, and a time layout
		// returned 12:25:24 for Christmas Day. The literal carries the
		// character class its signature matched on now.
		{"20-1-00", "10:01:00"},
		{"10:30:45", "12/25/24"},

		// The same rule at the month name. Detection finds one as a whole word,
		// so "MArAA1MAY" holds exactly one and it is MAY at offset 6, but the
		// executor checked the bytes the instruction named and nothing either
		// side of them. A MONTH_DAY layout read March where detection read May,
		// with no guess reported on either call. C24.
		{"MAr A1AAA", "MArAA1MAY"},
	}
	for _, s := range seeds {
		f.Add(s[0], s[1])
	}

	f.Fuzz(func(t *testing.T, detectFrom, applyTo string) {
		cached, err := Parse(detectFrom)
		if err != nil || !reusable(cached.Layout) {
			return
		}
		fresh, err := Parse(applyTo)
		if err != nil || !reusable(fresh.Layout) {
			return
		}

		got, err := cached.Layout.Parse(applyTo)
		if err != nil {
			return // refusing is always allowed
		}

		// A reported guess is allowed to differ. Where detection says it had to
		// choose which field was which, a cached layout that made the other
		// choice is not wrong, it is the other reading, and the caller was told
		// on both calls. This exclusion is keyed on the flag precisely so that
		// a format which guesses without reporting it still fails here: that is
		// how "MAY70" against "MAY10" was found.
		if cached.Ambiguous || fresh.Ambiguous {
			return
		}

		if !got.Equal(fresh.Time) {
			t.Errorf("layout %s detected from %q accepted %q and disagreed with detection:\n"+
				"  reused %-18s = %v\n"+
				"  fresh  %-18s = %v",
				cached.Layout, detectFrom, applyTo,
				cached.Layout.String(), got,
				fresh.Layout.String(), fresh.Time)
		}
	})
}

// reusable reports whether a Layout can be re-applied to another input.
//
// It asks the layout rather than comparing it against the two sentinels by
// identity, which is what it used to do and what a third sentinel would have
// broken silently.
func reusable(l *Layout) bool {
	return l.Reusable()
}

// FuzzDetect ensures that format detection never panics.
func FuzzDetect(f *testing.F) {
	f.Add("2024-03-15")
	f.Add("March 15, 2024")
	f.Add("")
	f.Add("123456789012345678901234567890") // long numeric
	f.Add("aaaaaaaaaaaaaaaaaaaaaaaaaaaa")   // long alpha
	f.Add("MAY00\xc30\xae\x840000 At 0")    // trimAtSuffix slice bounds panic

	f.Fuzz(func(t *testing.T, input string) {
		// Must not panic.
		Detect(input)
	})
}
