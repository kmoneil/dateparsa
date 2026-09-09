// Package locale provides locale data for date parsing.
// Locale data is compiled into the binary — no runtime file loading.
package locale

import (
	"sort"
	"strings"
	"sync"
)

// Data holds all locale-specific data needed for date parsing.
type Data struct {
	Tag  string // BCP 47 tag, e.g. "fr", "de", "ja"
	Name string // English name, e.g. "French", "German"

	// Month names: wide (full) and abbreviated.
	// Index 0 = January, 11 = December.
	MonthsWide [12]string
	MonthsAbbr [12]string

	// Weekday names: wide (full) and abbreviated.
	// Index 0 = Sunday, 6 = Saturday (matches time.Weekday).
	WeekdaysWide [7]string
	WeekdaysAbbr [7]string

	// AM/PM markers.
	AM string
	PM string

	// Relative time keywords for natural language parsing.
	Relative RelativeKeywords
}

// RelativeKeywords holds locale-specific keywords for NL date expressions.
type RelativeKeywords struct {
	Now       []string // "now", "maintenant", "ahora"
	Today     []string // "today", "aujourd'hui", "hoy"
	Yesterday []string // "yesterday", "hier", "ayer"
	Tomorrow  []string // "tomorrow", "demain", "mañana"
	Ago       []string // "ago", "il y a" (patterns: "N units <ago>")
	InFuture  []string // "in", "dans", "en" (patterns: "<in> N units")
	Last      []string // "last", "dernier", "pasado"
	Next      []string // "next", "prochain", "próximo"
	This      []string // "this", "ce", "este"

	// Unit names (singular, plural). Key = canonical unit.
	Seconds []string
	Minutes []string
	Hours   []string
	Days    []string
	Weeks   []string
	Months  []string
	Years   []string
}

// registry maps BCP 47 tags to locale data.
var registry = map[string]*Data{}

// Register adds a locale to the global registry.
//
// Init-time only, and not safe to call concurrently with Lookup, Tags, or
// another Register. The map has no lock because the only callers are the
// init() functions in internal/locale/data, which the runtime serialises, and
// every read happens afterwards. Nothing enforces that, and this function is
// exported from the package, so it is written here rather than assumed.
func Register(d *Data) {
	tag := strings.ToLower(d.Tag)
	registry[tag] = d
}

// Lookup returns the Data for a BCP 47 tag, or nil if not found.
// Tries exact match first, then base language (e.g. "fr-FR" -> "fr").
func Lookup(tag string) *Data {
	tag = strings.ToLower(tag)
	if d, ok := registry[tag]; ok {
		return d
	}
	// Try base language.
	if i := strings.IndexByte(tag, '-'); i > 0 {
		if d, ok := registry[tag[:i]]; ok {
			return d
		}
	}
	return nil
}

// Tags returns all registered locale tags, sorted alphabetically.
func Tags() []string {
	tags := make([]string, 0, len(registry))
	for tag := range registry {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags
}

// MonthNumber returns the month number (1-12) for a month name in this locale,
// or 0 if not found. Case-insensitive.
func (d *Data) MonthNumber(name string) int {
	lower := strings.ToLower(name)
	for i := 0; i < 12; i++ {
		if strings.ToLower(d.MonthsWide[i]) == lower || strings.ToLower(d.MonthsAbbr[i]) == lower {
			return i + 1
		}
	}
	return 0
}

// WeekdayNumber returns the weekday number (0=Sunday..6=Saturday) for a
// weekday name in this locale, or -1 if not found. Case-insensitive.
func (d *Data) WeekdayNumber(name string) int {
	lower := strings.ToLower(name)
	for i := 0; i < 7; i++ {
		if strings.ToLower(d.WeekdaysWide[i]) == lower || strings.ToLower(d.WeekdaysAbbr[i]) == lower {
			return i
		}
	}
	return -1
}

// LocaleSet is a set of registered locales, one bit each. It is what a compiled
// program carries so that verifying a month name asks the same question
// detection asked, and the zero value is the answer for a program compiled with
// no locales at all: English and nothing else.
//
// C33 is what it is for. MatchesMonth used to answer "is this name this month in
// any locale I know", all twenty of them, while detection searched the ones the
// caller configured. So a MONTH_DAY layout read "mAI" as May, French and German
// for it, where Parse read the same input as March, and "März 15, 2024" through
// a layout detected from "March 15, 2024" was the fifteenth of March where Parse
// refuses it outright.
//
// A bit set rather than the []*Data itself, because a Program is copied by value
// on every Parse and its size decides a size class: four bytes against a
// twenty-four byte slice header and a pointer for the garbage collector to
// follow.
type LocaleSet uint32

// Bit returns d's place in a LocaleSet, or 0 for a locale that is not
// registered.
//
// The bits are assigned from Tags(), which is sorted, so a locale's bit is the
// same in every process. Locales past the thirty-second share the last bit,
// which makes the set slightly lenient rather than wrong: two locales sharing a
// bit means one is accepted where the other was configured. There are twenty
// today and TestEveryLocaleHasItsOwnBit says so.
//
// Like the month index below, this is built on first use rather than at
// package-variable initialisation, because the locales register from init() in
// internal/locale/data and first use is always after every init has run.
func Bit(d *Data) LocaleSet {
	if d == nil {
		return 0
	}
	localeBitsOnce.Do(buildLocaleBits)
	return localeBits[d]
}

var (
	localeBitsOnce sync.Once
	localeBits     map[*Data]LocaleSet
)

func buildLocaleBits() {
	localeBits = make(map[*Data]LocaleSet, len(registry))
	for i, tag := range Tags() {
		shift := i
		if shift > 31 {
			shift = 31
		}
		localeBits[registry[tag]] = 1 << uint(shift)
	}
}

// monthIndex maps every spelling of every registered month to its number and
// the locales that spell it that way, and monthSpellings lists them per month
// for the case-folding fallback.
//
// Built on first use rather than at package-variable initialisation, because a
// package variable here would be empty: the locales register from init() in
// internal/locale/data, and only a package importing that one is guaranteed to
// see them. First use is always after every init has run.
//
// The index exists because the obvious version, ranging the registry and
// folding case, was both slow and nondeterministic. Go randomises map iteration
// order, so the number of comparisons before reaching the right locale differed
// per call: a German month measured 1.0µs to 1.5µs with an 86% spread. An exact
// lookup is one hash, allocates nothing, and hits for any input spelled the way
// the locale data spells it.
// monthName is one spelling and the locales that use it for the month it is
// filed under. A name shared by two locales, French and German "mai", is one
// entry carrying both bits.
type monthName struct {
	name string
	num  int
	set  LocaleSet
}

var (
	monthIndexOnce sync.Once
	monthIndex     map[string]monthName
	monthSpellings [12][]monthName
)

func buildMonthIndex() {
	localeBitsOnce.Do(buildLocaleBits)
	monthIndex = make(map[string]monthName, len(registry)*24)
	add := func(name string, month int, set LocaleSet) {
		if name == "" {
			return
		}
		if e, seen := monthIndex[name]; seen {
			if e.num == month {
				e.set |= set
				monthIndex[name] = e
			}
		} else {
			monthIndex[name] = monthName{name: name, num: month, set: set}
		}
		monthSpellings[month-1] = append(monthSpellings[month-1], monthName{
			name: name, num: month, set: set,
		})
	}
	// Sorted tags so the fallback scan is in a fixed order run to run.
	for _, tag := range Tags() {
		d := registry[tag]
		set := localeBits[d]
		for i := range 12 {
			for _, n := range [2]string{d.MonthsWide[i], d.MonthsAbbr[i]} {
				add(n, i+1, set)
				if trimmed := strings.TrimRight(n, "."); trimmed != n {
					add(trimmed, i+1, set)
				}
			}
		}
	}
}

// MatchesMonth reports whether name is the given month (1-12) in one of the
// locales in allowed, case-insensitively. An abbreviation is accepted with or
// without its trailing dot, because detection accepts it both ways.
//
// allowed is the whole of C33's fix and an empty set is not a wildcard: a
// program compiled with no locales accepts no locale spelling, which is what
// detection does for the same input.
//
// The exact hit falls through to the fold rather than answering from the map,
// which the version without a set did not have to do. Two locales can spell one
// month two ways that fold together, French "mai" and German "Mai", and those
// are two keys; answering false from the first would refuse German "Mai" for a
// caller who configured German and not French.
func MatchesMonth(name string, month int, allowed LocaleSet) bool {
	if month < 1 || month > 12 || allowed == 0 {
		return false
	}
	monthIndexOnce.Do(buildMonthIndex)

	if e, ok := monthIndex[name]; ok && e.num == month && e.set&allowed != 0 {
		return true
	}
	// Spelled with different case, or spelled by a locale that is not the first
	// to claim these bytes. Fold only against the one month asked about, never
	// all twelve.
	for _, want := range monthSpellings[month-1] {
		if want.set&allowed != 0 && strings.EqualFold(name, want.name) {
			return true
		}
	}
	return false
}
