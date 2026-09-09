package locale_test

import (
	"strings"
	"testing"

	"github.com/kmoneil/dateparsa/internal/locale"

	// The data registers itself through init(), so it has to be imported for
	// there to be anything to check.
	_ "github.com/kmoneil/dateparsa/internal/locale/data"
)

// The month and weekday tables are generated from CLDR by a generator that is
// not in this repository, so nothing here can check a spelling against CLDR.
// What it can check is that the tables are internally consistent, and that is
// the part a regeneration or a hand edit gets wrong: a name landing one slot
// out reads as the wrong month, which is a wrong date rather than an error.
//
// These walk every registered locale rather than the handful somebody wrote
// cases for. Half the locales had no test of any kind before this.

func TestEveryMonthNameMapsToItsOwnMonth(t *testing.T) {
	for _, tag := range locale.Tags() {
		d := locale.Lookup(tag)
		if d == nil {
			t.Errorf("%s is in Tags() and Lookup returns nothing", tag)
			continue
		}
		for i := range 12 {
			for _, name := range []string{d.MonthsWide[i], d.MonthsAbbr[i]} {
				if name == "" {
					t.Errorf("%s: month %d has an empty spelling", tag, i+1)
					continue
				}
				if got := d.MonthNumber(name); got != i+1 {
					t.Errorf("%s: %q is month %d in the table and MonthNumber says %d",
						tag, name, i+1, got)
				}
			}
		}
	}
}

func TestEveryWeekdayNameMapsToItsOwnWeekday(t *testing.T) {
	for _, tag := range locale.Tags() {
		d := locale.Lookup(tag)
		for i := range 7 {
			for _, name := range []string{d.WeekdaysWide[i], d.WeekdaysAbbr[i]} {
				if name == "" {
					t.Errorf("%s: weekday %d has an empty spelling", tag, i)
					continue
				}
				if got := d.WeekdayNumber(name); got != i {
					t.Errorf("%s: %q is weekday %d in the table and WeekdayNumber says %d",
						tag, name, i, got)
				}
			}
		}
	}
}

// A spelling shared by two locales for two different months cannot be resolved
// from the name alone. buildMonthIndex keeps the first writer and the tags are
// walked in sorted order, so the winner would be alphabetical rather than
// meaningful, and one of the two languages would silently read the wrong month.
func TestNoTwoLocalesSpellDifferentMonthsTheSameWay(t *testing.T) {
	type spelling struct {
		tag   string
		month int
	}
	seen := map[string][]spelling{}

	for _, tag := range locale.Tags() {
		d := locale.Lookup(tag)
		for i := range 12 {
			for _, name := range []string{d.MonthsWide[i], d.MonthsAbbr[i]} {
				if name == "" {
					continue
				}
				key := strings.ToLower(strings.TrimRight(name, "."))
				seen[key] = append(seen[key], spelling{tag, i + 1})
			}
		}
	}

	for name, uses := range seen {
		for _, u := range uses {
			if u.month != uses[0].month {
				t.Errorf("%q is a different month depending on the locale: %v", name, uses)
				break
			}
		}
	}
}

// TestEveryLocaleHasItsOwnBit is what LocaleSet's arithmetic rests on. Bits are
// assigned from the sorted tag list, one per locale, and locales past the
// thirty-second would share the last one. Twenty registered today, so nothing
// shares anything; if that ever changes, two locales become interchangeable to
// MatchesMonth and a caller who configured one gets the other's month names.
func TestEveryLocaleHasItsOwnBit(t *testing.T) {
	tags := locale.Tags()
	if len(tags) > 32 {
		t.Errorf("%d locales are registered and a LocaleSet holds 32; the ones past "+
			"the thirty-second share a bit and are accepted for each other", len(tags))
	}

	seen := map[locale.LocaleSet]string{}
	for _, tag := range tags {
		d := locale.Lookup(tag)
		bit := locale.Bit(d)
		if bit == 0 {
			t.Errorf("%s is registered and has no bit", tag)
			continue
		}
		if other, dup := seen[bit]; dup {
			t.Errorf("%s and %s share bit %#x", tag, other, bit)
			continue
		}
		seen[bit] = tag
	}
	if locale.Bit(nil) != 0 {
		t.Error("Bit(nil) is not the empty set")
	}
}

// TestMatchesMonthAsksTheSetItWasGiven is C33 at this package's boundary.
//
// The set is not a filter over an answer that was already right: it is the
// question. A month name is a month name in some language, and which languages
// a caller is parsing is the only thing that says whether this one counts.
// Before the set existed this function knew all twenty, while detection searched
// the ones configured, so a compiled layout accepted a spelling detection would
// never have found.
func TestMatchesMonthAsksTheSetItWasGiven(t *testing.T) {
	de, fr := locale.Bit(locale.Lookup("de")), locale.Bit(locale.Lookup("fr"))
	if de == 0 || fr == 0 {
		t.Fatal("de and fr are registered locales; one of them has no bit")
	}

	tests := []struct {
		name    string
		month   int
		allowed locale.LocaleSet
		want    bool
	}{
		// The empty set is a caller who configured no locale, and it is not a
		// wildcard. Nothing in any locale table answers it.
		{"März", 3, 0, false},
		{"mars", 3, 0, false},
		{"mai", 5, 0, false},

		{"März", 3, de, true},
		{"märz", 3, de, true},  // the fold, which is not ASCII here
		{"MÄRZ", 3, de, true},  //
		{"März", 3, fr, false}, // German spelling, French configured
		{"mars", 3, fr, true},  // and the other way round
		{"mars", 3, de, false}, //
		{"März", 5, de, false}, // right locale, wrong month
		{"Jan.", 1, de, true},  // the abbreviation as the table spells it
		{"Jan", 1, de, true},   // and without its dot, which detection accepts
		{"nonsense", 3, de | fr, false},

		// "mai" is May in both, spelled the same in one and capitalised in the
		// other, which is two keys in the index and one of them is reached only
		// by the fold. Answering from the map alone refuses this pair.
		{"mai", 5, fr, true},
		{"mai", 5, de, true},
		{"Mai", 5, fr, true},
		{"Mai", 5, de, true},
	}
	for _, tt := range tests {
		if got := locale.MatchesMonth(tt.name, tt.month, tt.allowed); got != tt.want {
			t.Errorf("MatchesMonth(%q, %d, %#x) = %v, want %v",
				tt.name, tt.month, tt.allowed, got, tt.want)
		}
	}
}
