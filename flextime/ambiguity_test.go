package flextime

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kmoneil/dateparsa"
)

// TestAmbiguityReachesEveryEntryPoint is C18's assertion. Every path that parses
// a string has to carry the flag dateparsa returns beside the time.
//
// "01/02/2024" is either the second of January or the first of February.
// dateparsa.Parse reports Ambiguous true for it; all four of these dropped that
// and reported Valid true with nothing else.
func TestAmbiguityReachesEveryEntryPoint(t *testing.T) {
	const in = "01/02/2024"

	// Sanity: the root package still calls this ambiguous. If it stops, this
	// test is passing for the wrong reason and should say so rather than go
	// green.
	if r, err := dateparsa.Parse(in); err != nil || !r.Ambiguous {
		t.Fatalf("test premise gone: dateparsa.Parse(%q) ambiguous=%v err=%v",
			in, r.Ambiguous, err)
	}

	t.Run("Scan", func(t *testing.T) {
		var ft FlexTime
		if err := ft.Scan(in); err != nil {
			t.Fatal(err)
		}
		assertAmbiguous(t, ft)
	})

	t.Run("ScanBytes", func(t *testing.T) {
		var ft FlexTime
		if err := ft.Scan([]byte(in)); err != nil {
			t.Fatal(err)
		}
		assertAmbiguous(t, ft)
	})

	t.Run("UnmarshalJSON", func(t *testing.T) {
		var ft FlexTime
		if err := json.Unmarshal([]byte(`"`+in+`"`), &ft); err != nil {
			t.Fatal(err)
		}
		assertAmbiguous(t, ft)
	})

	t.Run("UnmarshalText", func(t *testing.T) {
		var ft FlexTime
		if err := ft.UnmarshalText([]byte(in)); err != nil {
			t.Fatal(err)
		}
		assertAmbiguous(t, ft)
	})

	t.Run("Scanner", func(t *testing.T) {
		var ft FlexTime
		if err := NewScanner().Scan(&ft, in); err != nil {
			t.Fatal(err)
		}
		assertAmbiguous(t, ft)
	})

	t.Run("ScannerConfigured", func(t *testing.T) {
		// The value is the other reading and the flag still stands: choosing by
		// preference is still choosing.
		var ft FlexTime
		if err := NewScanner(WithPreferDayFirst(true)).Scan(&ft, in); err != nil {
			t.Fatal(err)
		}
		want := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
		if !ft.Time().Equal(want) {
			t.Errorf("Time() = %v, want %v", ft.Time(), want)
		}
		assertAmbiguous(t, ft)
	})
}

func assertAmbiguous(t *testing.T, ft FlexTime) {
	t.Helper()
	if !ft.Valid() {
		t.Error("Valid() = false, want true")
	}
	if !ft.Ambiguous() {
		t.Error("Ambiguous() = false, want true")
	}
}

// TestUnambiguousStaysUnambiguous is the other direction, so the flag cannot be
// satisfied by returning true for everything.
func TestUnambiguousStaysUnambiguous(t *testing.T) {
	for _, in := range []string{
		"2024-03-15",              // shape fixes every field
		"2024-03-15T10:30:00Z",    //
		"25/12/2024",              // 25 cannot be a month
		"March 15, 2024",          // four-digit year, so 15 is not one
		"1710500000",              // epoch
		"2024-03-15 10:30:00.123", //
	} {
		var ft FlexTime
		if err := ft.Scan(in); err != nil {
			t.Errorf("Scan(%q): %v", in, err)
			continue
		}
		if ft.Ambiguous() {
			t.Errorf("Scan(%q): Ambiguous() = true, want false", in)
		}
	}
}

// TestAmbiguousClearsOnReuse is the trap C18's card named. database/sql reuses one
// *FlexTime across every row of a result set, so a flag left set by an earlier row
// would report a guess on a value that never involved one.
//
// Every path is checked, not only the nil one, because each of them writes the
// fields and any of them could write two of the three.
func TestAmbiguousClearsOnReuse(t *testing.T) {
	fill := func(t *testing.T, ft *FlexTime) {
		t.Helper()
		if err := ft.Scan("01/02/2024"); err != nil {
			t.Fatal(err)
		}
		if !ft.Ambiguous() {
			t.Fatal("setup failed: value should be ambiguous")
		}
	}

	cases := []struct {
		name string
		next func(*FlexTime) error
	}{
		{"nil", func(ft *FlexTime) error { return ft.Scan(nil) }},
		{"time.Time", func(ft *FlexTime) error { return ft.Scan(time.Now()) }},
		{"int64", func(ft *FlexTime) error { return ft.Scan(int64(1710500000)) }},
		{"float64", func(ft *FlexTime) error { return ft.Scan(1710500000.5) }},
		{"unambiguous string", func(ft *FlexTime) error { return ft.Scan("2024-03-15") }},
		{"json null", func(ft *FlexTime) error { return json.Unmarshal([]byte("null"), ft) }},
		{"json string", func(ft *FlexTime) error { return json.Unmarshal([]byte(`"2024-03-15"`), ft) }},
		{"json number", func(ft *FlexTime) error { return json.Unmarshal([]byte("1710500000"), ft) }},
		{"empty text", func(ft *FlexTime) error { return ft.UnmarshalText(nil) }},
		{"text", func(ft *FlexTime) error { return ft.UnmarshalText([]byte("2024-03-15")) }},
		{"scanner", func(ft *FlexTime) error { return NewScanner().Scan(ft, "2024-03-15") }},
		{"scanner nil", func(ft *FlexTime) error { return NewScanner().Scan(ft, nil) }},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var ft FlexTime
			fill(t, &ft)
			if err := c.next(&ft); err != nil {
				t.Fatalf("second scan: %v", err)
			}
			if ft.Ambiguous() {
				t.Error("Ambiguous() = true after reuse; a previous row's flag survived")
			}
		})
	}
}

// TestStrictModeRefuses covers WithStrictMode, and the error's inspectability
// rather than only its existence. "A parse error is inspectable" is an invariant,
// and this is a new error path joining that scheme.
func TestStrictModeRefuses(t *testing.T) {
	sc := NewScanner(WithStrictMode(true))

	var ft FlexTime
	err := sc.Scan(&ft, "01/02/2024")
	if err == nil {
		t.Fatalf("strict mode accepted an ambiguous value as %v", ft.Time())
	}
	if !errors.Is(err, dateparsa.ErrAmbiguous) {
		t.Errorf("error does not unwrap to ErrAmbiguous: %v", err)
	}

	var ade *dateparsa.AmbiguousDateError
	if !errors.As(err, &ade) {
		t.Fatalf("error does not unwrap to *AmbiguousDateError: %v", err)
	}
	if len(ade.Interpretations) < 2 {
		t.Errorf("got %d interpretations, want at least 2", len(ade.Interpretations))
	}
	// The interpretations have to differ, or the caller has nothing to decide
	// between. This is C21's defect and it is asserted here too, so that closing
	// C21 cannot silently stop mattering at this boundary.
	if len(ade.Interpretations) == 2 &&
		ade.Interpretations[0].Time.Equal(ade.Interpretations[1].Time) {
		t.Errorf("both interpretations are %v; a caller comparing them would "+
			"conclude the guess was safe", ade.Interpretations[0].Time)
	}

	// An unambiguous value still parses under strict mode.
	var ok FlexTime
	if err := sc.Scan(&ok, "2024-03-15"); err != nil {
		t.Errorf("strict mode refused an unambiguous value: %v", err)
	}
	if !ok.Valid() || ok.Ambiguous() {
		t.Errorf("valid=%v ambiguous=%v, want true/false", ok.Valid(), ok.Ambiguous())
	}
}

// TestStrictModeErrorSurvivesJSONWrapping checks that encoding/json's own wrapping
// does not hide the sentinel. The card flagged it as worth verifying rather than
// assuming, because the %w chain is three deep by the time a caller sees it.
func TestStrictModeErrorSurvivesJSONWrapping(t *testing.T) {
	// UnmarshalJSON has no configuration, so it cannot refuse. What it can do is
	// report, and that is the documented equivalent at this boundary.
	var v struct {
		At FlexTime `json:"at"`
	}
	if err := json.Unmarshal([]byte(`{"at":"01/02/2024"}`), &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !v.At.Ambiguous() {
		t.Error("Ambiguous() = false through a struct field; the flag did not survive")
	}

	// And a parse failure through the same path still unwraps to ErrNoMatch.
	var bad struct {
		At FlexTime `json:"at"`
	}
	err := json.Unmarshal([]byte(`{"at":"not a date"}`), &bad)
	if err == nil {
		t.Fatal("want an error")
	}
	if !errors.Is(err, dateparsa.ErrNoMatch) {
		t.Errorf("error does not unwrap to ErrNoMatch through encoding/json: %v", err)
	}
}

// TestStrictModeIgnoresTypedValues pins that strict mode has nothing to say about a
// value that never involved a guess.
func TestStrictModeIgnoresTypedValues(t *testing.T) {
	sc := NewScanner(WithStrictMode(true))
	for _, src := range []any{
		time.Now(),
		int64(1710500000),
		1710500000.5,
		nil,
	} {
		var ft FlexTime
		if err := sc.Scan(&ft, src); err != nil {
			t.Errorf("strict mode refused %T: %v", src, err)
		}
		if ft.Ambiguous() {
			t.Errorf("%T reported ambiguous", src)
		}
	}
}
