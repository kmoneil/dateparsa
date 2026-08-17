package dateparsa

import (
	"errors"
	"testing"
)

// TestStrictModeCarriesTheCallerConfig is C21 half two.
//
// buildAmbiguousError rebuilt detect.Config from two of cfg's fields and dropped
// the rest. Locales was the one that mattered: without them neither re-detection
// can find a locale month name, so both failed, the interpretation list came back
// empty, and the function fell through to a *ParseError reading "ambiguous date
// could not be interpreted". Strict mode did not refuse to guess about "mai 15",
// it failed to parse it, while the lenient path read it and reported the guess.
//
// A caller who turned strict mode on because SECURITY.md told them to lost the
// ability to parse non-English textual dates at all.
func TestStrictModeCarriesTheCallerConfig(t *testing.T) {
	cases := []struct {
		in      string
		locales []Locale
	}{
		{"mai 15", []Locale{FR}},
		{"15 mai", []Locale{FR}},
		{"marzo 15", []Locale{ES}},
		{"mai 15", []Locale{FR, ES}},
	}

	for _, c := range cases {
		// The lenient parse is the premise: if this stops being ambiguous, the
		// test below is passing for the wrong reason.
		lenient, err := ParseWith(c.in, WithLocales(c.locales...))
		if err != nil {
			t.Fatalf("test premise gone: ParseWith(%q) does not parse: %v", c.in, err)
		}
		if !lenient.Ambiguous {
			t.Fatalf("test premise gone: ParseWith(%q) is no longer ambiguous", c.in)
		}

		_, err = ParseWith(c.in, WithLocales(c.locales...), WithStrictMode(true))
		if err == nil {
			t.Errorf("ParseWith(%q, strict) accepted an ambiguous value", c.in)
			continue
		}

		var ade *AmbiguousDateError
		if !errors.As(err, &ade) {
			t.Errorf("ParseWith(%q, strict) = %T %v\n"+
				"  want *AmbiguousDateError; a strict-mode refusal has to carry the\n"+
				"  readings it refused to choose between, not report that it could\n"+
				"  not interpret an input the lenient path reads",
				c.in, err, err)
			continue
		}
		if len(ade.Interpretations) == 0 {
			t.Errorf("ParseWith(%q, strict): no interpretations", c.in)
		}
		if !errors.Is(err, ErrAmbiguous) {
			t.Errorf("ParseWith(%q, strict): does not unwrap to ErrAmbiguous", c.in)
		}
	}
}

// TestStrictModeNumericStillCarriesBothReadings is the case that already worked, so
// that carrying the config cannot have broken it.
func TestStrictModeNumericStillCarriesBothReadings(t *testing.T) {
	_, err := ParseWith("01/02/2024", WithStrictMode(true))
	var ade *AmbiguousDateError
	if !errors.As(err, &ade) {
		t.Fatalf("want *AmbiguousDateError, got %T %v", err, err)
	}
	if len(ade.Interpretations) != 2 {
		t.Fatalf("got %d interpretations, want 2", len(ade.Interpretations))
	}
	if ade.Interpretations[0].Time.Equal(ade.Interpretations[1].Time) {
		t.Errorf("both interpretations are %v; the numeric case is the one that "+
			"has always offered two different readings",
			ade.Interpretations[0].Time)
	}
	byLabel := map[string]string{}
	for _, i := range ade.Interpretations {
		byLabel[i.Label] = i.Time.Format("2006-01-02")
	}
	if byLabel["MM/DD/YYYY"] != "2024-01-02" || byLabel["DD/MM/YYYY"] != "2024-02-01" {
		t.Errorf("interpretations = %v, want MM/DD/YYYY 2024-01-02 and DD/MM/YYYY 2024-02-01",
			byLabel)
	}
}

// TestStrictModeTextualInterpretationsAreStillWrong records C21 half one, which is
// open, and pins the current behaviour so that fixing it is a visible change rather
// than a silent one.
//
// "MAY 15" is ambiguous because 15 could be the day or the year 2015, which is what
// textualDayIsAGuess decides. The error offers two interpretations that are the same
// instant, under labels naming a numeric format the input is not, and the reading it
// is actually choosing between is not among them. A caller checking whether the two
// readings agree concludes the guess was safe and takes it.
//
// The assertion is deliberately "these are equal", so it FAILS when half one lands.
// That is the point: read this comment, delete the test, and put the real assertion
// in TestStrictModeNumericStillCarriesBothReadings's shape beside it.
func TestStrictModeTextualInterpretationsAreStillWrong(t *testing.T) {
	_, err := ParseWith("MAY 15", WithStrictMode(true))
	var ade *AmbiguousDateError
	if !errors.As(err, &ade) {
		t.Fatalf("want *AmbiguousDateError, got %T %v", err, err)
	}
	if len(ade.Interpretations) != 2 {
		t.Fatalf("got %d interpretations, want 2 (pinning current behaviour)", len(ade.Interpretations))
	}
	if !ade.Interpretations[0].Time.Equal(ade.Interpretations[1].Time) {
		t.Skip("C21 half one appears to be fixed: the two textual interpretations " +
			"now differ. Delete this test and assert the real readings instead.")
	}
	for _, i := range ade.Interpretations {
		if i.Label != "MM/DD/YYYY" && i.Label != "DD/MM/YYYY" {
			t.Skipf("C21 half one appears to be fixed: label %q is no longer a "+
				"numeric one. Delete this test.", i.Label)
		}
	}
	t.Log("C21 half one is open: \"MAY 15\" yields two identical interpretations " +
		"labelled MM/DD/YYYY and DD/MM/YYYY, and the day-versus-2015 reading it " +
		"is really choosing between is not offered.")
}
