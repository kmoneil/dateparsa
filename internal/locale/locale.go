// Package locale provides locale data for date parsing.
// Locale data is compiled into the binary — no runtime file loading.
package locale

import "strings"

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

// Tags returns all registered locale tags, sorted.
func Tags() []string {
	tags := make([]string, 0, len(registry))
	for tag := range registry {
		tags = append(tags, tag)
	}
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
