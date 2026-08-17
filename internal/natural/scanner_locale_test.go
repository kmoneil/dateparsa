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
// These answers are now chosen rather than inherited from the sort, which is
// what W13 was. The four different-kind pairs are settled by kindRank, and the
// one same-kind pair, Hindi कल, is merged by mergeSameKindDuplicates into a
// single entry carrying both readings, so its token has Alt set and Parse
// reports the pair instead of picking one.
//
// The table still exists for the same reason: a phrase whose meaning is decided
// anywhere but in the data is a date that can move without anybody editing a
// date. Changing one of these lines is a decision about what an input parses
// to, so make it deliberately and give it a BREAKING CHANGE footer, because a
// caller's database is what records the difference.
type duplicateSpelling struct {
	tag    string
	phrase string
	want   Token // Pos, Raw and Alt are ignored

	// wantAlt is the other reading for a merged same-kind pair, and zero when
	// the entry is a different-kind pair with no alternative to carry.
	wantAlt RelWord
}

var duplicateSpellings = []duplicateSpelling{
	// Same kind, so no grammar can pick: merged, and both readings travel.
	// Yesterday is primary because Relative.Yesterday is listed before
	// Relative.Tomorrow, which is a decision about insertion order and not
	// about which one a Hindi speaker meant. Parse says it guessed.
	{"hi", "कल", Token{Kind: TokRelWord, RelVal: RelYesterday}, RelTomorrow},

	// Different kinds, so the grammar picks by position and kindRank picks
	// which one is in the table. Every one of these is the answer the unstable
	// sort used to give, kept because it is also the one that keeps the
	// compositional forms parsing: "1 ora fa" needs ora to be the hour.
	{"it", "ora", Token{Kind: TokUnit, UnitVal: UnitHour}, 0},
	{"ja", "今", Token{Kind: TokSelector, SelVal: SelThis}, 0},
	{"ja", "日", Token{Kind: TokUnit, UnitVal: UnitDay}, 0},
	{"ko", "일", Token{Kind: TokUnit, UnitVal: UnitDay}, 0},
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
		gotAlt := got.Alt
		got.Pos, got.Raw, got.Alt = 0, "", nil
		if got != d.want {
			t.Errorf("%s: ScanLocale(%q) = %+v, pinned to %+v.\n"+
				"If this is a deliberate change, update duplicateSpellings. "+
				"If nothing here was meant to change, the length sort reordered "+
				"under you, or kindRank did.",
				d.tag, d.phrase, got, d.want)
		}

		switch {
		case d.wantAlt == 0 && gotAlt != nil:
			t.Errorf("%s: ScanLocale(%q) now carries a second reading %+v, and "+
				"this entry pins none. A different-kind pair became a same-kind "+
				"one, so the grammar can no longer tell them apart.",
				d.tag, d.phrase, *gotAlt)
		case d.wantAlt != 0 && gotAlt == nil:
			t.Errorf("%s: ScanLocale(%q) lost its second reading, pinned to %v. "+
				"Parse can no longer report the ambiguity and will answer one "+
				"of two days with no sign it guessed.", d.tag, d.phrase, d.wantAlt)
		case d.wantAlt != 0 && gotAlt.RelVal != d.wantAlt:
			t.Errorf("%s: ScanLocale(%q) second reading is %v, pinned to %v",
				d.tag, d.phrase, gotAlt.RelVal, d.wantAlt)
		}
	}
}

// TestScanLocale_NoUnpinnedDuplicates is the half that keeps working after
// somebody adds a locale.
//
// A phrase registered twice in one locale with different tokens has to be
// resolved by something that decided on purpose: kindRank when the kinds
// differ, mergeSameKindDuplicates when they do not. The five that exist are
// pinned above. This finds any others, so a new locale cannot quietly inherit
// an arbitrary answer, and so a phrase that stops being duplicated does not
// leave a pin behind claiming to hold something.
//
// It reads the table before the merge, because the merge is what it is checking
// has happened.
func TestScanLocale_NoUnpinnedDuplicates(t *testing.T) {
	pinned := map[string]bool{}
	for _, d := range duplicateSpellings {
		pinned[d.tag+"\x00"+d.phrase] = true
	}

	found := map[string]bool{}
	for _, tag := range locale.Tags() {
		loc := locale.Lookup(tag)
		byPhrase := map[string][]Token{}
		for _, w := range buildLocaleWordsUnmerged(loc) {
			tok := w.tok
			tok.Pos, tok.Raw, tok.Alt = 0, "", nil
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
