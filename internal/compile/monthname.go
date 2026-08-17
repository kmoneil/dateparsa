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

// IsWordChar reports whether c can be part of a word: a letter, or any byte of
// a UTF-8 sequence.
//
// It lives here and not beside its callers because detect and this package have
// to agree about it exactly. detect finds a month name as a whole word and this
// package verifies one, and where the two disagreed about what a word is, a
// reused layout read a month the detector would not have found. See
// monthNameMatches. Two copies of this rule is the shape of that bug, so there
// is one copy and detect calls it.
func IsWordChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
}

// monthNameMatches reports whether s[off:off+length] names the given month, as
// a whole word.
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
//
// The word boundary is the other half of the same bug and was missing until
// C24. detectTextualMonth finds a month name with a whole-word match, so
// "MArAA1MAY" holds exactly one month name and it is MAY at offset 6, while
// this function looked at the three bytes the instruction named and nothing
// around them. A MONTH_DAY layout detected from "MAr A1AAA" therefore read that
// input as March where detection read it as May, and neither call reported a
// guess. FuzzLayoutReuse found it.
//
// A digit either side is still a match. "MAY10" is an input this library
// accepts, and detection accepts it for exactly this reason: a digit is not a
// word character, so the name is still a whole word. Every path in
// findMonthNameCI requires the same two boundaries, which is why this cannot
// refuse an input detection accepted.
func monthNameMatches(s string, off, length, month int) bool {
	if month < 1 || month > 12 || length <= 0 || off+length > len(s) {
		return false
	}
	if off > 0 && IsWordChar(s[off-1]) {
		return false
	}
	if end := off + length; end < len(s) && IsWordChar(s[end]) {
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
