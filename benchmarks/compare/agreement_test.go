package compare

import (
	"testing"
	"time"

	araddon "github.com/araddon/dateparse"
	"github.com/kmoneil/dateparsa"
)

// TestBothParseTheCorpus is what keeps the benchmarks honest.
//
// A benchmark of a format one library refuses is not a comparison of parsing:
// the refusing side is measuring its detection cascade and its error
// construction instead, which is cheaper than a parse in one library and dearer
// in the other. This asserts both accept every corpus entry, so that failure
// shows up here, as a fact about capability, rather than in a benchmark table as
// a number nobody can interpret.
func TestBothParseTheCorpus(t *testing.T) {
	for _, c := range Corpus {
		t.Run(c.Name, func(t *testing.T) {
			if _, err := dateparsa.Parse(c.Input); err != nil {
				t.Errorf("dateparsa refuses %q: %v", c.Input, err)
			}
			if _, err := araddon.ParseAny(c.Input); err != nil {
				t.Errorf("araddon refuses %q: %v", c.Input, err)
			}
		})
	}
}

// TestBothRefuseTheMisses is the same guarantee for the miss benchmarks. A
// "miss" one library silently parses is not a miss, and its cost is not the cost
// of a failed detection.
func TestBothRefuseTheMisses(t *testing.T) {
	for _, m := range Misses {
		t.Run(m.Name, func(t *testing.T) {
			if got, err := dateparsa.Parse(m.Input); err == nil {
				t.Errorf("dateparsa parsed %q as %v, so it is not a miss", m.Input, got.Time)
			}
			if got, err := araddon.ParseAny(m.Input); err == nil {
				t.Errorf("araddon parsed %q as %v, so it is not a miss", m.Input, got)
			}
		})
	}
}

// TestAgreement reports where the two libraries return different instants for
// the same input.
//
// It reports rather than fails. Disagreement is not automatically a bug in
// either: the two resolve an ambiguous DD/MM against MM/DD by different rules
// and default to different timezones, both deliberately. What it is, is the list
// a caller migrating between them has to look at, and it is worth having written
// down next to the speed numbers, because speed is the less interesting half of
// choosing a parser.
func TestAgreement(t *testing.T) {
	for _, c := range Corpus {
		ours, oerr := dateparsa.Parse(c.Input)
		theirs, terr := araddon.ParseAny(c.Input)
		if oerr != nil || terr != nil {
			continue // TestBothParseTheCorpus owns that failure
		}
		if !ours.Time.Equal(theirs) {
			t.Logf("DISAGREE %-20s %-32q dateparsa=%v araddon=%v",
				c.Name, c.Input, ours.Time.UTC(), theirs.UTC())
		}
	}
}

// TestReuseIsTheSameAnswer checks the two reuse mechanisms the benchmarks time.
//
// dateparsa hands back a compiled Layout; araddon hands back a Go layout string
// for time.Parse. Timing those against each other is only meaningful if each
// actually reproduces what its own one-shot call returned, so this asserts that
// before the benchmark claims anything.
func TestReuseIsTheSameAnswer(t *testing.T) {
	for _, c := range Corpus {
		t.Run(c.Name, func(t *testing.T) {
			ours, err := dateparsa.Parse(c.Input)
			if err != nil {
				t.Skipf("dateparsa refuses %q", c.Input)
			}
			if reusable(ours.Layout) {
				got, err := ours.Layout.Parse(c.Input)
				if err != nil {
					t.Errorf("dateparsa Layout.Parse(%q): %v", c.Input, err)
				} else if !got.Equal(ours.Time) {
					t.Errorf("dateparsa Layout.Parse(%q) = %v, Parse = %v",
						c.Input, got, ours.Time)
				}
			}

			theirs, err := araddon.ParseAny(c.Input)
			if err != nil {
				t.Skipf("araddon refuses %q", c.Input)
			}
			layout, err := araddon.ParseFormat(c.Input)
			if err != nil {
				t.Logf("araddon has no reusable layout for %q: %v", c.Input, err)
				return
			}
			got, err := time.Parse(layout, c.Input)
			if err != nil {
				t.Logf("araddon layout %q does not re-parse %q: %v", layout, c.Input, err)
				return
			}
			if !got.Equal(theirs) {
				t.Logf("araddon layout %q re-parses %q as %v, ParseAny said %v",
					layout, c.Input, got, theirs)
			}
		})
	}
}

// TestAraddonReuseGaps pins the corpus entries whose araddon layout does not
// re-parse the input it was derived from.
//
// This is the reason BenchmarkReuse skips those two for araddon rather than
// timing them: a layout that returns an error is not a reuse path, and timing
// one would measure time.Parse's error construction and call it a comparison.
//
// It is pinned rather than derived so that the list is a stated finding. Both
// are capability gaps in the mechanism that corresponds to dateparsa's Layout:
//
//	ANSIC         ParseFormat drops the weekday, so the layout it returns is
//	              narrower than the input and time.Parse refuses the "Mon".
//	unix_seconds  ParseFormat returns the digits themselves as the layout, and
//	              time.Parse reads them as a reference time with a month of 17.
//
// dateparsa answers the first with a normal reusable layout and the second with
// LayoutEpoch, which is a sentinel that refuses to re-parse by design, because
// an epoch has no format to reuse. Refusing and returning something that does
// not work are different answers.
func TestAraddonReuseGaps(t *testing.T) {
	want := map[string]bool{"ANSIC": true, "unix_seconds": true}

	got := map[string]bool{}
	for _, c := range Corpus {
		layout, err := araddon.ParseFormat(c.Input)
		if err != nil {
			got[c.Name] = true
			continue
		}
		if _, err := time.Parse(layout, c.Input); err != nil {
			got[c.Name] = true
		}
	}

	for name := range want {
		if !got[name] {
			t.Errorf("%s: araddon's layout now re-parses its own input. "+
				"Remove it from the pinned list and from araddonCanReuse.", name)
		}
	}
	for name := range got {
		if !want[name] {
			t.Errorf("%s: araddon's layout no longer re-parses its own input, "+
				"which is a new gap. Add it to the pinned list and to "+
				"araddonCanReuse, and say so in README.", name)
		}
	}
}

// araddonCanReuse reports whether araddon's ParseFormat yields a layout that
// re-parses the input. TestAraddonReuseGaps pins which entries this excludes.
func araddonCanReuse(input string) (string, bool) {
	layout, err := araddon.ParseFormat(input)
	if err != nil {
		return "", false
	}
	if _, err := time.Parse(layout, input); err != nil {
		return "", false
	}
	return layout, true
}

// reusable reports whether a Layout can re-parse. Epoch and natural-language
// results are sentinels with no program, by design, and asking one to re-parse
// is an error rather than a slow path.
func reusable(l *dateparsa.Layout) bool {
	_, err := l.Parse("")
	if err == nil {
		return true
	}
	// A sentinel refuses with a message naming itself; a real layout refuses ""
	// because it does not describe zero bytes.
	s := l.String()
	return s != "UNIX_TIMESTAMP" && s != "NATURAL_LANGUAGE"
}
