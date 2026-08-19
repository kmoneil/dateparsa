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
