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
// The sentinels carry no program and refuse by design.
func reusable(l *Layout) bool {
	return l != nil && l != LayoutEpoch && l != LayoutNaturalLanguage
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
