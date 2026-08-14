package compile

import (
	"strings"

	"github.com/kmoneil/dateparsa/internal/locale"
)

// englishMonths holds every English spelling detection will match, indexed by
// month minus one. September has three: detection carries "sept" alongside
// "september" and "sep", so verification has to as well or "sept. 1, 2020"
// would be refused by the layout it was detected from.
var englishMonths = [12][]string{
	{"january", "jan"},
	{"february", "feb"},
	{"march", "mar"},
	{"april", "apr"},
	{"may"},
	{"june", "jun"},
	{"july", "jul"},
	{"august", "aug"},
	{"september", "sept", "sep"},
	{"october", "oct"},
	{"november", "nov"},
	{"december", "dec"},
}

// monthNameMatches reports whether s[off:off+length] names the given month.
//
// OpMonthName used to skip this and take the month from the instruction alone,
// which is correct for the input a layout was detected from and wrong for every
// other one. A Layout is a reusable format, so a layout detected from
// "March 15, 2024" read "April 20, 2024" as 2024-03-20 and returned no error.
// Whether that was caught depended on the width of the month name: "December"
// shifted the fields after it and failed the day parse, so it fell back to
// detection and came out right, while April, being as wide as March, did not.
//
// Only the one month the instruction already names is checked, never all
// twelve, so an English input costs one or two comparisons and a mismatch is a
// refusal rather than a different month. Nothing here allocates:
// strings.EqualFold folds in place.
func monthNameMatches(s string, off, length, month int) bool {
	if month < 1 || month > 12 || length <= 0 || off+length > len(s) {
		return false
	}
	got := s[off : off+length]

	for _, want := range englishMonths[month-1] {
		if strings.EqualFold(got, want) {
			return true
		}
	}
	return locale.MatchesMonth(got, month)
}
