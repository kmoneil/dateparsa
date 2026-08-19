package detect

import (
	"github.com/kmoneil/dateparsa/internal/compile"
	"github.com/kmoneil/dateparsa/internal/locale"
)

// A skipped run is a stretch of bytes a textual format locates its fields
// around and never reads: the "Fri, " of an RFC 2822 date, the " of " in "the
// 3rd of March 2024", the punctuation between two numbers. coverGaps fixes its
// width so a longer one cannot shift every field after it, and until C26 the
// width was the only thing anybody checked. Its comment said the content was
// not worth checking because "March 15; 2024" has one reading whatever the
// punctuation, which is true of punctuation and false of English:
//
//	"first monday of march 2024"    parsed as MONTH_YEAR, 2024-03-01
//	"third thursday of november 24" parsed as MONTH_YEAR, 2024-11-01
//	"last day of february 2024"     parsed as MONTH_YEAR, 2024-02-01
//	"end of march 2024"             parsed as MONTH_YEAR, 2024-03-01
//
// Three days out, twenty days out, twenty-eight days out, with a nil error and
// Ambiguous false, because none of those runs holds a digit and a digit was the
// only content a skip was ever checked for (C24's rule).
//
// So a skipped run may not hold a word that decides what a date expression
// means. A selector, an ordinal, a boundary, a relative word and a unit name
// all decide one. Three classes of word deliberately still may:
//
//   - Weekday and month names. "Fri, 15 Mar 2024" is RFC 2822 and its weekday
//     name is the thing skips exist for.
//   - "of" and "at". "the 3rd of March 2024" and "March 15, 2024 at 10:30" are
//     ordinary spellings of a date and both put the word in a skipped run.
//   - Every word the library does not recognise: "invoice 15 March 2024 paid"
//     still parses. Whether a date inside arbitrary text should parse at all is
//     a separate question and not this file's to answer.
//
// What that trades away is stated here rather than discovered later. Input that
// was answered correctly by accident is now refused:
//
//	"Last modified: March 15, 2024"   refused on "last"
//	"On Wed 8 March in the year 2020" refused on "year"
//
// Both are free text carrying a date, both got the right answer, and nothing in
// the run itself tells them apart from the four above.
//
// Refusing here sends the value on to the epoch reading and then to natural
// language, which is where an expression written in words belongs. Natural
// language answers none of the four today: it answers "end of month" and not
// "end of march 2024". So the four are errors now rather than wrong days, which
// is the trade this file makes and the whole of what it claims. Answering them
// is a natural-language pattern and a separate piece of work.

// meaningWords are the English words a skipped run may not hold, other than
// those a configured locale contributes.
//
// English is here as a literal rather than read from locale.Data because
// English is always in play: the trie and the textual-month detector recognise
// English month names whether or not a caller named a locale, and the default
// locale set is empty rather than English.
//
// The ordinals are the one group with no counterpart anywhere else in the
// library. Nothing tokenises "first" today, in this package or in
// internal/natural, which is why "first monday of march 2024" reached a format
// at all. They stop at fifth because a month holds at most five of any weekday.
var meaningWords = map[string]struct{}{
	// Ordinals.
	"first": {}, "second": {}, "third": {}, "fourth": {}, "fifth": {},

	// Boundaries, which internal/natural scans as TokBoundary.
	"beginning": {}, "start": {}, "end": {},

	// Times of day, TokTimeOfDay, TokNoon, TokMidnight and TokHalf.
	"morning": {}, "afternoon": {}, "evening": {}, "night": {},
	"noon": {}, "midnight": {}, "half": {},

	// Selectors and relative words. These duplicate locale.EN, deliberately:
	// see the note above about the default locale set.
	"last": {}, "previous": {}, "past": {}, "next": {}, "coming": {}, "this": {},
	"now": {}, "today": {}, "yesterday": {}, "tomorrow": {},

	// Unit names.
	// "second" is above, as an ordinal.
	"seconds": {}, "sec": {}, "secs": {},
	"minute": {}, "minutes": {}, "min": {}, "mins": {},
	"hour": {}, "hours": {}, "hr": {}, "hrs": {},
	"day": {}, "days": {},
	"week": {}, "weeks": {},
	"month": {}, "months": {},
	"year": {}, "years": {},
}

// maxMeaningWord is the longest word the map can hold, and the width of the
// stack buffer a candidate is lowercased into. A longer word cannot match the
// map, so it is compared against the configured locales only.
const maxMeaningWord = 16

// skipRunCarriesMeaning reports whether any run of bytes def does not read
// holds a word that changes what the date means.
//
// It runs once per successful detection, which is once per column rather than
// once per row: a compiled Layout carries the program and never comes back
// here. A separator-only skip, which is every skip a trie format produces,
// costs one isWordChar call per byte and finds no word to compare.
func skipRunCarriesMeaning(s string, def *compile.FormatDef, locales []*locale.Data) bool {
	if def == nil {
		return false
	}
	for _, f := range def.Fields {
		if f.Kind != compile.FSkip {
			continue
		}
		off, end := int(f.Offset), int(f.Offset)+int(f.Len)
		if off < 0 || off > len(s) {
			continue
		}
		if end > len(s) {
			end = len(s)
		}
		if runCarriesMeaning(s[off:end], locales) {
			return true
		}
	}
	return false
}

// runCarriesMeaning walks one run's words. A word is a maximal stretch of
// isWordChar bytes, which is compile.IsWordChar, the same definition the
// executor uses to decide where a month name begins and ends.
//
// A word holding no ASCII letter is not compared, and that is the rule this
// file cannot do without. In Chinese and Japanese the unit name and the date
// separator are the same character: 年 is the word "year" and the particle that
// ends the year field, 月 is "month", 日 is "day". A whole-word comparison there
// matches every CJK date ever written, and the first version of this file
// refused "2014年04月08日" and "2024년 1월 15일" for exactly that reason.
//
// The cost is that a locale whose script is not Latin gets no protection from
// this check: Russian "прошлый" and Arabic "الماضي" are not compared. The
// defect this file exists for is English-shaped, the Latin locales keep the
// check for their unaccented forms, and refusing every date in five scripts to
// reach the accented ones is not a trade worth making.
func runCarriesMeaning(run string, locales []*locale.Data) bool {
	for i := 0; i < len(run); {
		if !isWordChar(run[i]) {
			i++
			continue
		}
		j, ascii := i, false
		for j < len(run) && isWordChar(run[j]) {
			if isLetter(run[j]) {
				ascii = true
			}
			j++
		}
		if ascii && wordCarriesMeaning(run[i:j], locales) {
			return true
		}
		i = j
	}
	return false
}

// wordCarriesMeaning compares one word against the English map and against
// every configured locale's relative keywords.
//
// Allocation-free on both halves: the lowercased copy goes in a stack array,
// and a map lookup keyed by a byte slice converted to a string does not copy.
// Detection is not the hot path, but a cold parse's allocation count is a
// number README quotes.
func wordCarriesMeaning(w string, locales []*locale.Data) bool {
	if len(w) == 0 {
		return false
	}
	if len(w) <= maxMeaningWord {
		var buf [maxMeaningWord]byte
		for i := 0; i < len(w); i++ {
			c := w[i]
			if c >= 'A' && c <= 'Z' {
				c += 'a' - 'A'
			}
			buf[i] = c
		}
		if _, ok := meaningWords[string(buf[:len(w)])]; ok {
			return true
		}
	}
	for _, loc := range locales {
		if loc == nil {
			continue
		}
		if relativeWordMatches(&loc.Relative, w) {
			return true
		}
	}
	return false
}

// relativeWordMatches checks the locale's own words for the same classes the
// English map covers.
//
// Ago and InFuture are not checked, and that is a decision rather than an
// omission. They are the shortest entries in the data and the ones most likely
// to be an ordinary word of the language: "en" and "em" in Spanish and
// Portuguese, "in" in German, "om" in the Nordic locales, "za" in Polish. What
// they would catch, a phrase counting units in a direction, holds a unit name
// as well, and the unit names are checked.
//
// Multi-word entries such as the French "il y a" and the Swedish "den här"
// never match here, because the comparison is one word at a time. They are left
// in the lists they belong to rather than filtered out, so that the data stays
// the data.
func relativeWordMatches(r *locale.RelativeKeywords, w string) bool {
	return matchesAny(r.Last, w) ||
		matchesAny(r.Next, w) ||
		matchesAny(r.This, w) ||
		matchesAny(r.Now, w) ||
		matchesAny(r.Today, w) ||
		matchesAny(r.Yesterday, w) ||
		matchesAny(r.Tomorrow, w) ||
		matchesAny(r.Seconds, w) ||
		matchesAny(r.Minutes, w) ||
		matchesAny(r.Hours, w) ||
		matchesAny(r.Days, w) ||
		matchesAny(r.Weeks, w) ||
		matchesAny(r.Months, w) ||
		matchesAny(r.Years, w)
}

func matchesAny(list []string, w string) bool {
	for _, e := range list {
		if len(e) == len(w) && equalsFoldASCII(w, e) {
			return true
		}
	}
	return false
}
