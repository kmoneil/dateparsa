package dateparsa

import (
	"strings"
	"testing"
	"time"

	"github.com/kmoneil/dateparsa/internal/compile"
	"github.com/kmoneil/dateparsa/internal/detect"
	"github.com/kmoneil/dateparsa/internal/locale"
)

// TestLayoutReadsOnlyTheLocalesItWasDetectedWith is C33 at the public boundary.
//
// A Layout is a format, and which languages that format is written in is part
// of it. The executor verified a month name against every locale compiled into
// the binary while detection searched the ones the caller configured, so a
// layout accepted spellings detection would never have found and answered with
// a month from a language nobody asked for:
//
//	Parse("März 15, 2024")                      ErrNoMatch
//	MONTH_DAY_YEAR from "March 15, 2024" on it  2024-03-15
//
// The reuse over-acceptance this repository has documented since C10 does not
// cover that: it allows a layout to accept bytes detection would refuse only
// where the instant is the same, and here it is a different month.
func TestLayoutReadsOnlyTheLocalesItWasDetectedWith(t *testing.T) {
	english, err := Parse("March 15, 2024")
	if err != nil {
		t.Fatalf("Parse(%q) = %v", "March 15, 2024", err)
	}

	// Only a spelling as wide as the one the layout holds reaches the byte
	// compare at all: a different width is refused for its width. That is why
	// this defect survived the locale tables being added, so the widths are
	// checked here rather than left to luck.
	for _, in := range []string{"März 15, 2024", "märz 15, 2024"} {
		if len(in) != len("March 15, 2024") {
			t.Fatalf("%q is %d bytes against the layout's %d, so it is refused for its "+
				"width and proves nothing", in, len(in), len("March 15, 2024"))
		}
		if _, err := english.Layout.Parse(in); err == nil {
			t.Errorf("a layout detected with no locales read %q, which Parse refuses", in)
		}
		if _, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) started accepting a German spelling with no locale configured", in)
		}
	}

	// The other direction, which is what makes the set the fix rather than
	// switching the executor to English: a layout detected with a locale still
	// re-parses that locale's rows, which is the whole of Parser's cache for a
	// German column.
	german, err := ParseWith("15 März 2024", WithLocales(DE))
	if err != nil {
		t.Fatalf("ParseWith(%q, WithLocales(DE)) = %v", "15 März 2024", err)
	}
	want := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	if !german.Time.Equal(want) {
		t.Fatalf("ParseWith(%q, WithLocales(DE)) = %v, want %v", "15 März 2024", german.Time, want)
	}
	got, err := german.Layout.Parse("15 März 2024")
	if err != nil {
		t.Errorf("a layout detected with WithLocales(DE) refused the row it came from: %v", err)
	} else if !got.Equal(want) {
		t.Errorf("that layout read %v, want %v", got, want)
	}

	// A locale the caller did not configure is still refused by a layout that
	// carries one. French "mars" is not German.
	if _, err := german.Layout.Parse("15 mars 2024"); err == nil {
		t.Error("a layout detected with WithLocales(DE) read a French month name")
	}
}

// TestParserWithLocalesParsesEveryRow is the regression half stated where a
// caller would notice it. Switching the executor to English only would leave
// each of these detecting once and refusing every row after it.
func TestParserWithLocalesParsesEveryRow(t *testing.T) {
	rows := []struct {
		locale Locale
		inputs []string
		months []time.Month
	}{
		{
			DE,
			[]string{"15 März 2024", "15 Mai 2024", "15 Juni 2024"},
			[]time.Month{time.March, time.May, time.June},
		},
		{
			FR,
			[]string{"15 mars 2024", "15 mai 2024", "15 juin 2024"},
			[]time.Month{time.March, time.May, time.June},
		},
		{
			ES,
			[]string{"15 marzo 2024", "15 mayo 2024", "15 junio 2024"},
			[]time.Month{time.March, time.May, time.June},
		},
	}
	for _, r := range rows {
		p := NewParser(WithLocales(r.locale))
		for i, in := range r.inputs {
			got, err := p.Parse(in)
			if err != nil {
				t.Errorf("%s: Parser.Parse(%q) = %v", r.locale, in, err)
				continue
			}
			if got.Time.Month() != r.months[i] || got.Time.Day() != 15 || got.Time.Year() != 2024 {
				t.Errorf("%s: Parser.Parse(%q) = %v, want 2024-%02d-15", r.locale, in, got.Time, r.months[i])
			}
		}
	}
}

// TestNoInternedFormatReadsAMonthName is what makes an empty locale set the
// right answer for the interned layouts rather than a compromise.
//
// They are built once at init and shared by every caller whatever that caller
// configured, so they cannot carry anybody's locales. That is only safe while no
// prebuilt format holds a month name: the trie is signature-matched and every
// textual format comes from detectTextualMonth, which compiles per call. A
// prebuilt format that did hold one would silently refuse a locale row that
// Parse accepts.
func TestNoInternedFormatReadsAMonthName(t *testing.T) {
	defs := detect.PrebuiltDefs()
	if len(defs) == 0 {
		t.Fatal("no prebuilt defs, so this test proves nothing")
	}
	for _, def := range defs {
		for _, f := range def.Fields {
			if f.Kind == compile.FMonthName {
				t.Errorf("prebuilt format %s reads a month name, so an interned layout "+
					"compiled with no locales would refuse rows Parse accepts", def.Name)
			}
		}
	}
}

// TestLocaleSetFromConfigNamesTheConfiguredLocales pins the plumbing between the
// option and the program, which the tests above reach only through a parse.
func TestLocaleSetFromConfigNamesTheConfiguredLocales(t *testing.T) {
	var none config
	if got := localeSetFromConfig(none); got != 0 {
		t.Errorf("a config with no locales gives set %#x, want 0", got)
	}

	cfg := config{locales: []Locale{DE, FR}}
	set := localeSetFromConfig(cfg)
	for _, l := range []Locale{DE, FR} {
		if bit := locale.Bit(l.data); set&bit == 0 {
			t.Errorf("set %#x from WithLocales(DE, FR) does not hold %s", set, l)
		}
	}
	if bit := locale.Bit(ES.data); set&bit != 0 {
		t.Errorf("set %#x from WithLocales(DE, FR) holds es", set)
	}
	if strings.Contains(DE.String(), "fr") {
		t.Fatal("the locale tags moved under this test")
	}
}
