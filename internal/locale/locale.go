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

// monthIndex maps every spelling of every registered month to its number, and
// monthSpellings lists them per month for the case-folding fallback.
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
var (
	monthIndexOnce sync.Once
	monthIndex     map[string]int
	monthSpellings [12][]string
)

func buildMonthIndex() {
	monthIndex = make(map[string]int, len(registry)*24)
	add := func(name string, month int) {
		if name == "" {
			return
		}
		if _, seen := monthIndex[name]; !seen {
			monthIndex[name] = month
		}
		monthSpellings[month-1] = append(monthSpellings[month-1], name)
	}
	// Sorted tags so the fallback scan is in a fixed order run to run.
	for _, tag := range Tags() {
		d := registry[tag]
		for i := range 12 {
			for _, n := range [2]string{d.MonthsWide[i], d.MonthsAbbr[i]} {
				add(n, i+1)
				if trimmed := strings.TrimRight(n, "."); trimmed != n {
					add(trimmed, i+1)
				}
			}
		}
	}
}

// MatchesMonth reports whether name is the given month (1-12) in any registered
// locale, case-insensitively. An abbreviation is accepted with or without its
// trailing dot, because detection accepts it both ways.
func MatchesMonth(name string, month int) bool {
	if month < 1 || month > 12 {
		return false
	}
	monthIndexOnce.Do(buildMonthIndex)

	if m, ok := monthIndex[name]; ok {
		return m == month
	}
	// Spelled with different case. Fold only against the one month asked
	// about, never all twelve.
	for _, want := range monthSpellings[month-1] {
		if strings.EqualFold(name, want) {
			return true
		}
	}
	return false
}
