package dateparsa

import (
	"errors"
	"testing"
	"time"
)

// ambigBase is a fixed reference so the two readings of an ambiguous word are
// two known dates rather than two moving ones.
var ambigBase = time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)

// TestHindiKalIsReportedAmbiguous is the regression this file exists for.
//
// Hindi writes yesterday and tomorrow with the same word, कल, and picks between
// them with the verb. A date string has no verb. The library answered tomorrow,
// with Ambiguous false and no error, and the answer was not even chosen: the
// phrase table was ordered by a sort documented as unstable, so a toolchain
// upgrade could have moved it to yesterday without a line changing.
//
// A wrong day reported as certain is the failure this library is built to
// refuse, and Ambiguous is the field that refuses it.
func TestHindiKalIsReportedAmbiguous(t *testing.T) {
	r, err := ParseWith("कल", WithLocales(HI), WithBaseTime(ambigBase))
	if err != nil {
		t.Fatalf("ParseWith(कल): %v", err)
	}
	if !r.Ambiguous {
		t.Errorf("Ambiguous = false; कल means both yesterday and tomorrow, "+
			"so answering %v without saying so is the bug this test holds",
			r.Time.Format("2006-01-02"))
	}
	// The primary reading is a decision: Relative.Yesterday is listed first.
	want := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	if !r.Time.Equal(want) {
		t.Errorf("ParseWith(कल) = %v, want %v", r.Time, want)
	}
}

// TestHindiKalStrictModeCarriesBothDays holds the other half of the contract.
// Reporting a guess is one thing; strict mode exists so a caller can refuse it
// and see what the choices were.
func TestHindiKalStrictModeCarriesBothDays(t *testing.T) {
	_, err := ParseWith("कल", WithLocales(HI), WithBaseTime(ambigBase),
		WithStrictMode(true))
	if err == nil {
		t.Fatal("strict mode accepted कल; it is a guess between two days")
	}
	if !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("error does not unwrap to ErrAmbiguous: %v", err)
	}

	var ambig *AmbiguousDateError
	if !errors.As(err, &ambig) {
		t.Fatalf("error is not *AmbiguousDateError: %T %v", err, err)
	}
	if len(ambig.Interpretations) != 2 {
		t.Fatalf("got %d interpretations, want 2: %+v",
			len(ambig.Interpretations), ambig.Interpretations)
	}

	got := map[string]bool{}
	for _, in := range ambig.Interpretations {
		got[in.Time.Format("2006-01-02")] = true
	}
	if !got["2026-08-16"] || !got["2026-08-18"] {
		t.Errorf("interpretations are %v, want both 2026-08-16 and 2026-08-18", got)
	}
}

// TestHindiQualifiedFormsAreNotAmbiguous is what stops the fix over-reporting.
//
// Hindi disambiguates in writing with a qualifier, बीता कल for the past one and
// आने वाला कल for the coming one. Both are longer phrases and match ahead of
// the bare word, so both have exactly one reading and neither is a guess. A fix
// that flagged these would make Ambiguous useless by crying wolf.
func TestHindiQualifiedFormsAreNotAmbiguous(t *testing.T) {
	for _, c := range []struct {
		in   string
		want time.Time
	}{
		{"बीता कल", time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)},
		{"आने वाला कल", time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)},
		{"आज", time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC)},
	} {
		r, err := ParseWith(c.in, WithLocales(HI), WithBaseTime(ambigBase))
		if err != nil {
			t.Errorf("ParseWith(%q): %v", c.in, err)
			continue
		}
		if r.Ambiguous {
			t.Errorf("ParseWith(%q) reports ambiguous; the qualifier decides it", c.in)
		}
		if !r.Time.Equal(c.want) {
			t.Errorf("ParseWith(%q) = %v, want %v", c.in, r.Time, c.want)
		}
	}
}

// TestNonAmbiguousLocalePhrasesStayUnambiguous covers the four duplicate
// spellings whose two readings are different kinds.
//
// Those are settled by the grammar and by kindRank, not by reporting a guess,
// so none of them may start claiming to be ambiguous. "1 ora fa" is the one to
// watch: it only parses while "ora" is the hour unit, and the obvious fix for
// the Hindi problem, making the phrase-table sort stable, hands "ora" to
// RelWord Now and breaks it. That was measured, which is why the sort is
// still sort.Slice with an explicit tie-break instead.
func TestNonAmbiguousLocalePhrasesStayUnambiguous(t *testing.T) {
	for _, c := range []struct {
		loc  Locale
		in   string
		want time.Time
	}{
		{IT, "1 ora fa", time.Date(2026, 8, 17, 11, 0, 0, 0, time.UTC)},
		{IT, "2 ore fa", time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)},
		{IT, "tra 2 ore", time.Date(2026, 8, 17, 14, 0, 0, 0, time.UTC)},
		{IT, "ieri", time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)},
		{JA, "今日", time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC)},
		{JA, "昨日", time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)},
		{KO, "어제", time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)},
	} {
		r, err := ParseWith(c.in, WithLocales(c.loc), WithBaseTime(ambigBase))
		if err != nil {
			t.Errorf("ParseWith(%q): %v", c.in, err)
			continue
		}
		if r.Ambiguous {
			t.Errorf("ParseWith(%q) reports ambiguous; its two readings are "+
				"different kinds and the grammar picks", c.in)
		}
		if !r.Time.Equal(c.want) {
			t.Errorf("ParseWith(%q) = %v, want %v", c.in, r.Time, c.want)
		}
	}
}

// TestEnglishNaturalLanguageIsNeverAmbiguous is the control. English has no
// duplicate spellings, so nothing on that path should have started reporting
// guesses, and strict mode should still accept it.
func TestEnglishNaturalLanguageIsNeverAmbiguous(t *testing.T) {
	for _, in := range []string{
		"yesterday", "tomorrow", "today", "3 days ago", "next friday",
		"in 10 minutes", "beginning of month", "yesterday at 5pm",
	} {
		r, err := ParseWith(in, WithBaseTime(ambigBase))
		if err != nil {
			t.Errorf("ParseWith(%q): %v", in, err)
			continue
		}
		if r.Ambiguous {
			t.Errorf("ParseWith(%q) reports ambiguous", in)
		}
		if _, err := ParseWith(in, WithBaseTime(ambigBase), WithStrictMode(true)); err != nil {
			t.Errorf("strict mode refused %q: %v", in, err)
		}
	}
}
