package dateparsa

import "testing"

// FuzzParse ensures that arbitrary inputs never panic.
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

		// If parsing succeeded and a Layout was returned, it must be reusable.
		// Epoch timestamps don't return a Layout (nil is valid for them).
		if result.Layout != nil {
			_, err = result.Layout.Parse(input)
			if err != nil {
				t.Errorf("Layout.Parse(%q) failed after successful Parse: %v", input, err)
			}
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

	f.Fuzz(func(t *testing.T, input string) {
		// Must not panic.
		Detect(input)
	})
}
