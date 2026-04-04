package dateparsa

import (
	"github.com/kmoneil/dateparsa/internal/locale"

	// Blank import to register all built-in locale data via init().
	_ "github.com/kmoneil/dateparsa/internal/locale/data"
)

// Locale represents a language/region for date parsing.
// Locale data is compiled into the binary — no runtime file loading.
type Locale struct {
	data *locale.Data
}

// String returns the BCP 47 tag for this locale (e.g. "fr", "de").
// Returns "" for the zero-value Locale (no data loaded).
func (l Locale) String() string {
	if l.data == nil {
		return ""
	}
	return l.data.Tag
}

// Pre-built locales for convenience. These are initialized lazily on first access
// via LookupLocale, but since all data is registered at init() time, they are
// safe to use immediately.
var (
	EN, _ = LookupLocale("en")
	ES, _ = LookupLocale("es")
	FR, _ = LookupLocale("fr")
	DE, _ = LookupLocale("de")
	IT, _ = LookupLocale("it")
	PT, _ = LookupLocale("pt")
	NL, _ = LookupLocale("nl")
	RU, _ = LookupLocale("ru")
	ZH, _ = LookupLocale("zh")
	JA, _ = LookupLocale("ja")
	KO, _ = LookupLocale("ko")
	AR, _ = LookupLocale("ar")
	HI, _ = LookupLocale("hi")
	PL, _ = LookupLocale("pl")
	SV, _ = LookupLocale("sv")
	DA, _ = LookupLocale("da")
	NO, _ = LookupLocale("no")
	FI, _ = LookupLocale("fi")
	TR, _ = LookupLocale("tr")
	UK, _ = LookupLocale("uk")
)

// LookupLocale returns a Locale by BCP 47 tag (e.g. "en", "fr", "de", "ja").
// Returns (Locale{}, false) if the locale is not supported.
func LookupLocale(tag string) (Locale, bool) {
	d := locale.Lookup(tag)
	if d == nil {
		return Locale{}, false
	}
	return Locale{data: d}, true
}

// Locales returns all supported locale tags.
func Locales() []string {
	return locale.Tags()
}

// WithLocales sets the locales to try, in preference order.
// Default: [English]. Natural language parsing and month/day name
// recognition use these locales.
func WithLocales(locales ...Locale) Option {
	return func(c *config) {
		c.locales = locales
	}
}
