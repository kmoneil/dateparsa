package detect

import (
	"testing"

	"github.com/kmoneil/dateparsa/internal/locale"
	_ "github.com/kmoneil/dateparsa/internal/locale/data"
)

// TestSkipRun_RefusesAWordThatDecidesTheDay is C26.
//
// Every input here parsed before the check existed, and the first five returned
// a day that is not the day the words name.
func TestSkipRun_RefusesAWordThatDecidesTheDay(t *testing.T) {
	cfg := Config{}
	refused := []struct {
		input string
		was   string // what it used to answer, for the reader
	}{
		{"first monday of march 2024", "2024-03-01, and the first Monday is the 4th"},
		{"third thursday of november 2024", "2024-11-01, and the third Thursday is the 21st"},
		{"last friday in march 2024", "2024-03-01, and the last Friday is the 29th"},
		{"last day of february 2024", "2024-02-01, and the last day is the 29th"},
		{"end of march 2024", "2024-03-01, and the end is the 31st"},

		// Correct before this check and refused by it. Listed so the trade is
		// in the test file and not only in a comment: nothing in the run tells
		// these apart from the five above.
		{"Last modified: March 15, 2024", "2024-03-15, correctly, by accident"},
		{"On Wed 8 March in the year 2020", "2020-03-08, correctly, by accident"},

		// One word from each remaining class, so a class dropped from
		// meaningWords fails here rather than silently.
		{"this March 15, 2024", "a selector"},
		{"next March 15, 2024", "a selector"},
		{"beginning of March 2024", "a boundary"},
		{"March 15, 2024 morning", "a time of day"},
		{"March 15, 2024 midnight", "a bare midnight"},
		{"half of March 2024", "the half quantifier"},
		{"yesterday March 15, 2024", "a relative word"},
		{"a week from March 15, 2024", "a unit name"},
	}
	for _, tt := range refused {
		if r, ok := Detect(tt.input, cfg); ok {
			t.Errorf("Detect(%q) = %s, want refused (was %s)", tt.input, r.Def.Name, tt.was)
		}
	}
}

// TestSkipRun_KeepsTheWordsAFormatNeeds is the other half, and it is the half
// that decides whether the rule above is usable. Each of these puts a word in a
// skipped run and each is an ordinary way to write a date.
func TestSkipRun_KeepsTheWordsAFormatNeeds(t *testing.T) {
	cfg := Config{}
	kept := []struct {
		input string
		why   string
	}{
		{"Fri, 15 Mar 2024 10:30:00 +0000", "RFC 2822's weekday name is what a skip is for"},
		{"Fri Mar 15 10:30:00 UTC 2024", "the same, unix date"},
		{"Friday, 15-Mar-24 10:30:00 UTC", "RFC 850"},
		{"the 3rd of March 2024", `"of" in an ordinary English date`},
		{"15th of March 2024", "the same"},
		{"March 15, 2024 at 10:30", `" at " is named in coverGaps's own comment`},
		{"15/Mar/2024:10:30:00 +0000", "common log format, punctuation only"},
		{"invoice 15 March 2024 paid", "unrecognised words still pass, which is F4's question"},
		{"2014年04月08日", "CJK, where the unit name is the separator"},
	}
	for _, tt := range kept {
		if _, ok := Detect(tt.input, cfg); !ok {
			t.Errorf("Detect(%q) refused, want accepted: %s", tt.input, tt.why)
		}
	}
}

// TestSkipRun_LocaleWords covers the half of the word set that comes from the
// configured locale rather than from the English map.
func TestSkipRun_LocaleWords(t *testing.T) {
	fr := locale.Lookup("fr")
	if fr == nil {
		t.Fatal("fr locale not registered")
	}
	cfg := Config{Locales: []*locale.Data{fr}}

	// "dernier" is fr Relative.Last. It is not in meaningWords, so this input
	// is refused only if the locale's own words are being read.
	if r, ok := Detect("dernier 15 mars 2024", cfg); ok {
		t.Errorf(`Detect("dernier 15 mars 2024") = %s, want refused on the fr selector`, r.Def.Name)
	}

	// The same input without the selector still parses, so the refusal above
	// is the word and not the locale.
	if _, ok := Detect("15 mars 2024", cfg); !ok {
		t.Error(`Detect("15 mars 2024") refused, want accepted`)
	}

	// A word this locale does not know is not a reason to refuse, even though
	// another locale does know it: "viime" is fi Relative.Last.
	if _, ok := Detect("viime 15 mars 2024", cfg); !ok {
		t.Error(`Detect("viime 15 mars 2024") refused; fr does not know the word and only fr is configured`)
	}
}

func TestWordCarriesMeaning(t *testing.T) {
	en := []*locale.Data{locale.Lookup("en")}
	tests := []struct {
		word string
		want bool
	}{
		{"first", true},
		{"FIRST", true},    // the comparison folds ASCII case
		{"First", true},    //
		{"firstly", false}, // whole words only
		{"last", true},
		{"of", false},
		{"at", false},
		{"the", false},
		{"fri", false},   // a weekday name is not a meaning word
		{"march", false}, // nor a month name
		{"", false},
		{"日", false},                          // no ASCII letter, never compared
		{"abcdefghijklmnopqrstuvwxyz", false}, // longer than the stack buffer
	}
	for _, tt := range tests {
		if got := wordCarriesMeaning(tt.word, en); got != tt.want {
			t.Errorf("wordCarriesMeaning(%q) = %v, want %v", tt.word, got, tt.want)
		}
	}
}

// TestMeaningWordsFitTheBuffer measures the constant rather than trusting it.
// A word longer than maxMeaningWord is never compared against the map, so an
// entry over the limit would be dead and nothing else would say so.
func TestMeaningWordsFitTheBuffer(t *testing.T) {
	for w := range meaningWords {
		if len(w) > maxMeaningWord {
			t.Errorf("meaningWords entry %q is %d bytes, over maxMeaningWord = %d",
				w, len(w), maxMeaningWord)
		}
	}
}
