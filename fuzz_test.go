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
