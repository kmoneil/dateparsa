package natural

import (
	"strings"
	"testing"

	"github.com/kmoneil/dateparsa/internal/locale"
	_ "github.com/kmoneil/dateparsa/internal/locale/data"
)

// TestScanLocale_MultiWordPhrases checks that every phrase containing a space
// is still scanned as one token.
//
// The phrase table is not a word list. Twenty-two of the thousand-odd phrases
// hold a space, "il y a" in French and "i morgen" in Danish among them, and
// every scheme that starts by splitting the input on whitespace loses them.
// The first-byte index does not split anything, which is why it was the one
// chosen; this test is what says so out loud, and it derives its cases from the
// locale data so that a phrase added later is covered without anybody
// remembering to add a case.
func TestScanLocale_MultiWordPhrases(t *testing.T) {
	checked := 0
	for _, tag := range locale.Tags() {
		loc := locale.Lookup(tag)
		for _, w := range getLocaleWords(loc).words {
			if !strings.Contains(w.phrase, " ") {
				continue
			}
			checked++
			tokens := ScanLocale(w.phrase, loc)
			if len(tokens) != 1 {
				t.Errorf("%s: ScanLocale(%q) = %d tokens, want 1: %+v",
					tag, w.phrase, len(tokens), tokens)
				continue
			}
			// Raw is the text that matched in the folded input, so an
			// accented phrase comes back folded: "i går" matches as "i gar".
			// What matters is that all of it matched, not which spelling.
			if want := foldAccents(w.phrase); tokens[0].Raw != want {
				t.Errorf("%s: ScanLocale(%q) matched %q, want the whole phrase %q",
					tag, w.phrase, tokens[0].Raw, want)
			}
			if tokens[0].Kind == TokUnknown {
				t.Errorf("%s: ScanLocale(%q) gave TokUnknown", tag, w.phrase)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no multi-word phrases found; the test is not testing anything")
	}
	t.Logf("checked %d multi-word phrases", checked)
}

// TestScanLocale_PhraseIndex checks the first-byte index against the table it
// indexes: every phrase has to be reachable from the bucket its first byte
// selects, and a bucket may not hold a phrase that starts with another byte.
//
// The scanner only ever looks in one bucket, so a phrase filed under the wrong
// one is a phrase that stops matching, which no format test would necessarily
// notice: the input simply comes back as an unknown word.
func TestScanLocale_PhraseIndex(t *testing.T) {
	for _, tag := range locale.Tags() {
		lw := getLocaleWords(locale.Lookup(tag))

		total := 0
		for b := range 256 {
			bucket := lw.bucket(byte(b))
			total += len(bucket)
			for _, w := range bucket {
				if w.phrase[0] != byte(b) {
					t.Errorf("%s: bucket %#02x holds %q", tag, b, w.phrase)
				}
			}
			// Longest first, or a longer phrase loses to a prefix of itself:
			// "i morgen" has to be tried before "i".
			for i := 1; i < len(bucket); i++ {
				if len(bucket[i-1].phrase) < len(bucket[i].phrase) {
					t.Errorf("%s: bucket %#02x has %q before the longer %q",
						tag, b, bucket[i-1].phrase, bucket[i].phrase)
				}
			}
		}
		if total != len(lw.words) {
			t.Errorf("%s: buckets cover %d phrases, the table has %d", tag, total, len(lw.words))
		}
		if len(lw.words) == 0 {
			t.Errorf("%s: empty phrase table", tag)
		}
	}
}

// TestScanLocale_LongestPhraseWins covers the cases the index could break
// first: a phrase that is a prefix of a longer one, in a locale where both are
// real.
func TestScanLocale_LongestPhraseWins(t *testing.T) {
	tests := []struct {
		tag   string
		input string
		want  string // the Raw of the first token
	}{
		{"fr", "il y a 3 jours", "il y a"},
		{"da", "i morgen", "i morgen"},
		{"da", "i dag", "i dag"},
		{"no", "i går", "i gar"}, // accents fold before matching
		{"sv", "i morgon", "i morgon"},
		{"sv", "det här", "det har"},
		{"es", "dentro de 3 dias", "dentro de"},
		{"pt", "dentro de 2 horas", "dentro de"},
		{"hi", "बीता कल", "बीता कल"},
	}
	for _, tt := range tests {
		loc := locale.Lookup(tt.tag)
		if loc == nil {
			t.Errorf("locale %s is not registered", tt.tag)
			continue
		}
		tokens := ScanLocale(tt.input, loc)
		if len(tokens) == 0 {
			t.Errorf("%s: ScanLocale(%q) gave no tokens", tt.tag, tt.input)
			continue
		}
		if tokens[0].Raw != tt.want {
			t.Errorf("%s: ScanLocale(%q) first token Raw = %q, want %q",
				tt.tag, tt.input, tokens[0].Raw, tt.want)
		}
	}
}

// duplicateSpelling is one phrase that a locale registers twice with different
// meanings, together with the token the scanner returns for it today.
//
// Today is the operative word. Nothing chose these answers: the phrase table is
// sorted by length with sort.Slice, which is not stable, so among equal-length
// phrases the winner is whatever pdqsort leaves in front. Three of the five
// change under sort.SliceStable, measured. What this table does is stop that
// from moving silently, so a toolchain upgrade fails a test instead of changing
// a date.
//
// The entries are recorded, not endorsed. Hindi lists कल under both yesterday
// and tomorrow because Hindi uses one word for both, and it answers tomorrow
// here for no better reason than the sort. Italian "ora" is the hour rather
// than "now" the same way. Deciding what they should be is W13 in the backlog;
// changing one of these lines is that decision, so make it deliberately and
// give it a BREAKING CHANGE footer, because a caller's database is what records
// the difference.
type duplicateSpelling struct {
	tag    string
	phrase string
	want   Token // Pos and Raw are ignored
}

var duplicateSpellings = []duplicateSpelling{
	{"hi", "कल", Token{Kind: TokRelWord, RelVal: RelTomorrow}},
	{"it", "ora", Token{Kind: TokUnit, UnitVal: UnitHour}},
	{"ja", "今", Token{Kind: TokSelector, SelVal: SelThis}},
	{"ja", "日", Token{Kind: TokUnit, UnitVal: UnitDay}},
	{"ko", "일", Token{Kind: TokUnit, UnitVal: UnitDay}},
}

// TestScanLocale_DuplicateSpellingsArePinned holds each of those answers.
func TestScanLocale_DuplicateSpellingsArePinned(t *testing.T) {
	for _, d := range duplicateSpellings {
		loc := locale.Lookup(d.tag)
		if loc == nil {
			t.Errorf("locale %s is not registered", d.tag)
			continue
		}
		tokens := ScanLocale(d.phrase, loc)
		if len(tokens) != 1 {
			t.Errorf("%s: ScanLocale(%q) = %d tokens, want 1: %+v",
				d.tag, d.phrase, len(tokens), tokens)
			continue
		}
		got := tokens[0]
		got.Pos, got.Raw = 0, ""
		if got != d.want {
			t.Errorf("%s: ScanLocale(%q) = %+v, pinned to %+v.\n"+
				"If this is a deliberate change, update duplicateSpellings and see W13. "+
				"If nothing here was meant to change, the length sort reordered under you: "+
				"it is sort.Slice, and equal-length phrases have no defined order.",
				d.tag, d.phrase, got, d.want)
		}
	}
}

// TestScanLocale_NoUnpinnedDuplicates is the half that keeps working after
// somebody adds a locale.
//
// A phrase registered twice in one locale with different tokens gets its
// meaning from the sort, which decides nothing on purpose. The five that exist
// are pinned above. This finds any others, so a new locale cannot quietly
// inherit an arbitrary answer, and so a phrase that stops being duplicated does
// not leave a pin behind claiming to hold something.
func TestScanLocale_NoUnpinnedDuplicates(t *testing.T) {
	pinned := map[string]bool{}
	for _, d := range duplicateSpellings {
		pinned[d.tag+"\x00"+d.phrase] = true
	}

	found := map[string]bool{}
	for _, tag := range locale.Tags() {
		loc := locale.Lookup(tag)
		byPhrase := map[string][]Token{}
		for _, w := range getLocaleWords(loc).words {
			tok := w.tok
			tok.Pos, tok.Raw = 0, ""
			byPhrase[w.phrase] = append(byPhrase[w.phrase], tok)
		}
		for phrase, toks := range byPhrase {
			differs := false
			for _, tok := range toks[1:] {
				if tok != toks[0] {
					differs = true
				}
			}
			if !differs {
				continue
			}
			key := tag + "\x00" + phrase
			found[key] = true
			if !pinned[key] {
				t.Errorf("%s: %q is registered %d times with different meanings and nothing pins it, "+
					"so which one wins is decided by an unstable sort. "+
					"Decide what it should mean, then add it to duplicateSpellings. See W13.",
					tag, phrase, len(toks))
			}
		}
	}
	for _, d := range duplicateSpellings {
		if !found[d.tag+"\x00"+d.phrase] {
			t.Errorf("%s: %q is pinned but is no longer registered twice with different meanings. "+
				"The locale data changed; drop the pin.", d.tag, d.phrase)
		}
	}
}
