package natural

import (
	"strings"
	"testing"
	"time"

	"github.com/kmoneil/dateparsa/internal/locale"

	// The registry is populated by init() in the data package, which only the
	// root package blank-imports. Without this the tags list is empty, the
	// target skips, and a run over nothing reports PASS in two milliseconds.
	_ "github.com/kmoneil/dateparsa/internal/locale/data"
)

// fuzzBase is fixed. This is the one package in the tree whose answers depend
// on the clock, and a target that read the clock would produce failures nobody
// could reproduce.
var fuzzBase = time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)

// FuzzNaturalRejectsATrailingToken asserts the rule C14 was the absence of,
// without asking the code whether it thinks it followed the rule.
//
// Take any input this package accepts, put one more token on the end that no
// pattern can read, and it has to stop accepting. Before the fix it did not:
// every evaluator matched a prefix and returned without looking at the rest, so
// "3 days ago 2024-01-01" came back as three days before the base time with a
// nil error.
//
// The obvious target, "Result.Consumed equals the token count", cannot fail.
// Eval already refuses anything where those differ, so the check and the rule
// are the same statement and a broken evaluator turns into a refusal rather
// than a counterexample. Breaking two evaluators' arithmetic on purpose and
// watching that version stay green is how this one came to be written instead.
// It is the trap M4 documented, met a second time.
func FuzzNaturalRejectsATrailingToken(f *testing.F) {
	seeds := []string{
		// The C14 inputs. All four returned a time before the fix.
		"3 days ago 2024-01-01",
		"yesterday 2024",
		"next friday 1999",
		"tomorrow at 5pm zzzz",
		"yesterday at",

		// The patterns that have to keep working, one per evaluator. These are
		// the ones that make the target bite: each is an input Eval accepts, so
		// each becomes a case where a trailing token must flip it to a refusal.
		"yesterday", "3 days ago", "in 10 minutes", "next friday at 2pm",
		"beginning of month", "a few days ago", "half an hour ago",
		"2 weeks and 3 days ago", "last monday", "march 15", "december 25th",
		"this morning", "last night", "next week", "sunday", "now",
		"yesterday at 5pm", "yesterday at noon", "tomorrow at midnight",
		"yesterday at 5", "in 2 hours", "next january",

		// Shapes that exercise the scanner rather than the evaluators.
		"", " ", "3", "a", "the", "\x00", "\xff\xfe", "5pm", "12:30",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		tokens := Scan(s)
		if Eval(tokens, fuzzBase, false) == nil {
			return // nothing to make worse
		}

		// TokUnknown is what the scanner produces for a word it does not know,
		// and no evaluator reads one. Appending it cannot make the input more
		// parseable, so continuing to parse means something stopped reading.
		extended := make([]Token, len(tokens), len(tokens)+1)
		copy(extended, tokens)
		extended = append(extended, Token{Kind: TokUnknown, Pos: len(s), Raw: "qqzz"})

		if r := Eval(extended, fuzzBase, false); r != nil {
			t.Errorf("Eval(%q) accepts, and still accepts with a trailing unknown token: %v",
				s, r.Time)
		}
	})
}

// FuzzNaturalScanLocale runs the locale tokenizer over arbitrary bytes.
//
// ScanLocale, foldAccents, isUnicodeWord and prevCharPos walk UTF-8 by hand and
// index by byte, which is the shape that produced the one panic this library
// has had: trimAtSuffix sliced past the end of an invalid sequence and took
// Parse, Detect and a flextime column scan down with it. None of this code has
// ever been fuzzed.
//
// It asserts more than the absence of a panic: a token has to point somewhere
// real in the string the scanner actually walked.
//
// That string is not the input. ScanLocale lowercases and folds accents first,
// and both change byte lengths: "é" folds to a one-byte "e", and ToLower turns
// an incomplete UTF-8 sequence into a three-byte U+FFFD. Token.Pos indexes the
// folded string, while its doc comment claims the original. The first version
// of this assertion believed the comment and failed on "\xc30A" in seconds,
// with a token at Pos 4 of a 3-byte input.
//
// Raw is the same kind of lie in the same function: a matched locale phrase
// carries the dictionary spelling, so ScanLocale("hace 3 dias", es) returns a
// token whose Raw is "días" for four bytes of input that read "dias". Pos plus
// len(Raw) is therefore not a span, which is why this checks only that a token
// starts inside what was walked and that the positions advance.
//
// Nothing outside the scanners reads either field today, so the comments are
// wrong rather than the code broken, and the assertion here is the true
// invariant. W11 is the card for making the comments true.
func FuzzNaturalScanLocale(f *testing.F) {
	seeds := []string{
		"hace 3 dias", "il y a 2 heures", "vor 5 Minuten", "gestern",
		"ayer a las 5", "demain à 14h", "übermorgen",
		"\xc3", "\xc3\x28", "a\xffb", "\xef\xbb\xbf yesterday",
		"", "é", "ééééééééé", "3 jours",

		// Found by this target within seconds of it first running for real.
		// strings.ToLower replaces the incomplete sequence with U+FFFD, which
		// is three bytes where the input had one, so the scanner walks a
		// string longer than the one it was given. See the Pos note below.
		"\xc30A",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	tags := locale.Tags()
	if len(tags) == 0 {
		f.Fatal("no locale data registered: this target would sweep nothing and report a pass")
	}

	f.Fuzz(func(t *testing.T, s string) {
		for _, tag := range tags {
			loc := locale.Lookup(tag)
			if loc == nil {
				continue
			}
			walked := foldAccents(strings.ToLower(s))
			prev := -1
			for _, tok := range ScanLocale(s, loc) {
				if tok.Pos < 0 || tok.Pos >= len(walked) {
					t.Fatalf("ScanLocale(%q, %s): token %q starts at Pos %d, outside the %d bytes it walked",
						s, loc.Tag, tok.Raw, tok.Pos, len(walked))
				}
				if tok.Pos <= prev {
					t.Fatalf("ScanLocale(%q, %s): token %q at Pos %d does not advance past %d",
						s, loc.Tag, tok.Raw, tok.Pos, prev)
				}
				prev = tok.Pos
			}
			// The evaluator has to survive whatever the tokenizer produced.
			Eval(ScanLocale(s, loc), fuzzBase, false)
		}
	})
}
