package flextime

import (
	"testing"
)

func FuzzScanString(f *testing.F) {
	seeds := []string{
		"2024-03-15",
		"2024-03-15T10:30:00Z",
		"03/15/2024",
		"1710505800",
		"",
		"not a date",
		"2024-13-45",
		"99/99/9999",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		var ft FlexTime
		err := ft.Scan(input)
		if err == nil {
			if !ft.Valid() {
				t.Error("Scan succeeded but Valid() is false")
			}
			v, verr := ft.Value()
			if verr != nil {
				t.Errorf("Value() error after successful Scan: %v", verr)
			}
			if v == nil {
				t.Error("Value() returned nil after successful Scan")
			}
		}
	})
}

func FuzzUnmarshalJSON(f *testing.F) {
	seeds := []string{
		`"2024-03-15T10:30:00Z"`,
		`"03/15/2024"`,
		`1710505800`,
		`1710505800.123`,
		`null`,
		`""`,
		`"garbage"`,
		`true`,
		`{}`,
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		var ft FlexTime
		err := ft.UnmarshalJSON(data)
		if err == nil && ft.Valid() {
			b, merr := ft.MarshalJSON()
			if merr != nil {
				t.Errorf("MarshalJSON error after successful Unmarshal: %v", merr)
			}
			if len(b) == 0 {
				t.Error("MarshalJSON returned empty bytes")
			}
		}
	})
}
