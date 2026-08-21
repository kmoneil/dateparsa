package dateparsa

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// The corpus golden: what every input in a fixed set parses to, committed, so
// that changing any of it is a line in a diff rather than a thing a fuzzer might
// find.
//
// CLAUDE.md's second idea is that a wrong time returned confidently is worse
// than an error, because a date decides an expiry, a retention window, a billing
// period. Everything that guards it today is a test somebody wrote for a case
// somebody thought of, or a fuzzer that has to rediscover the case on every run
// against a corpus that is not the same corpus twice. Neither answers "did this
// change what any input parses to", which is the question a reviewer has.
//
// This does. It records input, instant, error, format name and the ambiguity
// flag for a few thousand inputs, and fails on any difference. A change that
// moves one is not blocked: it regenerates the file, and the diff is then the
// clearest possible statement of what the change did, in the commit where a
// reviewer can see it. Same contract as testdata/codegen/gates.txt and
// benchmarks/baseline.txt: regenerating is deliberate and says why.
//
// It is not a substitute for the fuzzers. They find inputs nobody listed; this
// pins what the listed ones do. The two fail in different directions and the
// gap between them is the reason to have both.
const corpusGoldenPath = "testdata/corpus/golden.txt"

// corpusInputs is the fixed set. Three sources, none of them a list written by
// hand for this test:
//
//   - every input the coverage list advertises, which is the public promise
//   - the round-trip generator's samples at its own seed, which reach the month
//     ends and the zone offsets a hand-written list does not
//   - every committed fuzz seed and crasher, which are the inputs that have
//     already broken something once
//
// A format added to any of those three is covered here without anybody
// remembering, which is the property that makes this worth committing.
func corpusInputs(t *testing.T) []string {
	t.Helper()
	seen := make(map[string]bool)
	var out []string
	add := func(s string) {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}

	for _, tc := range coverageCases {
		add(tc.input)
	}

	// Boundaries, which a generator reaches only by luck and a corpus of past
	// crashers reaches only if somebody already broke them.
	//
	// The two-digit year pivot is here because it was missing: moving
	// NormalizeTwoDigitYear's threshold from 69 to 70, which is a twenty-year
	// error on one input, changed nothing in the first generated corpus. A
	// regression net that a deliberate defect walks through is not one, and
	// finding that out cost one sed and one test run.
	for _, s := range []string{
		// The two-digit year pivot, both sides and both ends.
		"31/12/68", "31/12/69", "31/12/70", "31/12/99", "31/12/00",
		"68-12-31", "69-12-31", "70-12-31",
		// Leap days, and the century rule 1900 and 2100 get wrong.
		"2024-02-29", "2000-02-29", "2023-02-28", "1904-02-29",
		"2024-02-28T23:59:59Z", "2024-03-01T00:00:00Z",
		// Midnight and noon, which the AM/PM conversion treats specially.
		"12:00 AM", "12:00 PM", "12:59 AM", "01:00 AM", "11:59 PM",
		// Zone offsets: the ends, off the fifteen-minute grid, and zero.
		"2024-06-15T12:00:00+14:00", "2024-06-15T12:00:00-12:00",
		"2024-06-15T12:00:00+05:30", "2024-06-15T12:00:00-00:44",
		"2024-06-15T12:00:00+05:53", "2024-06-15T12:00:00+00:00",
		"2024-06-15T12:00:00-00:00", "2024-06-15T12:00:00Z",
		// Fractional seconds at every width a nanosecond holds.
		"2024-06-15 12:00:00.1", "2024-06-15 12:00:00.123",
		"2024-06-15 12:00:00.123456", "2024-06-15 12:00:00.123456789",
		// Year ends, including the ones the day-count table brackets.
		"0001-01-01", "1799-12-31", "1800-01-01", "2399-12-31", "2400-01-01",
		"9999-12-31", "1970-01-01T00:00:00Z", "1969-12-31T23:59:59Z",
		// The respellings, which decide what GoLayout reports.
		"2024/06/15", "2024.06.15", "2024/06/15 12:00:00.123",
		"2024/06/15 12:00:00/123",
	} {
		add(s)
	}

	// The generator, at a seed of this test's own so that changing the
	// round-trip iteration count does not rewrite this file.
	rng := rand.New(rand.NewSource(20260821))
	base := time.Date(2024, 3, 15, 10, 30, 45, 123456789, time.UTC)
	for _, spec := range roundTripFormats {
		for i := range 40 {
			d := base.AddDate(rng.Intn(200)-100, rng.Intn(24), rng.Intn(400))
			d = d.Add(time.Duration(rng.Int63n(int64(48 * time.Hour))))
			add(renderSample(spec, d, rng))
			_ = i
		}
	}

	// Every committed corpus file. These are the inputs that already broke
	// something, so they are the ones most worth pinning.
	_ = filepath.Walk("testdata/fuzz", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil //nolint:nilerr // a missing corpus is not this test's failure
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			// go test fuzz corpus files write one Go literal per argument.
			if !strings.HasPrefix(line, "string(") {
				continue
			}
			var v string
			if _, err := fmt.Sscanf(line, "string(%q)", &v); err == nil {
				add(v)
			}
		}
		return nil
	})

	sort.Strings(out)
	return out
}

// corpusBase is the base time every line is parsed against.
//
// It has to be fixed or the file is not a golden. A format with no year field
// takes its year from the clock, and a natural-language expression resolves
// against it entirely, so Parse would write 2026 into this file today and 2027
// into it in January: a thousand lines of diff caused by a calendar. The first
// generated run showed "11:13 PM" as 2026-01-01T23:13, which is what made this
// obvious, and a golden that rewrites itself annually is a golden people learn
// to regenerate without reading.
//
// WithBaseTime is the same code path with a deterministic input, which is what
// a golden needs and what the fuzzers deliberately do not use.
var corpusBase = time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)

// corpusLine is one input's whole observable result, on one line.
//
// The instant is written in UTC with nanoseconds and the zone offset beside it,
// because those are three different ways a change can move an answer and a
// caller reads all three. The error is its sentinel, not its message: a message
// is prose that improves, and failing on prose would make this file churn for
// changes that move nothing.
func corpusLine(input string) string {
	r, err := ParseWith(input, WithBaseTime(corpusBase))
	switch {
	case err == nil:
		name := "-"
		if r.Layout != nil {
			name = r.Layout.String()
		}
		return fmt.Sprintf("%q\tok\t%s\t%s\t%s\tambiguous=%v\tkind=%v",
			input,
			r.Time.UTC().Format("2006-01-02T15:04:05.000000000Z"),
			r.Time.Format("-07:00"),
			name,
			r.Ambiguous,
			r.Kind)
	default:
		return fmt.Sprintf("%q\t%s", input, corpusErrKind(err))
	}
}

// corpusErrKind names the failure by its sentinel, which is the part of an
// error a caller branches on. errors.Is is what this library promises works,
// so it is what this asks.
func corpusErrKind(err error) string {
	switch {
	case errors.Is(err, ErrAmbiguous):
		return "ambiguous"
	case errors.Is(err, ErrNoMatch):
		return "nomatch"
	default:
		return "error"
	}
}

// TestCorpusGolden is the regression net: any change to what a listed input
// parses to shows up here, as a line, in the change that caused it.
func TestCorpusGolden(t *testing.T) {
	inputs := corpusInputs(t)
	if len(inputs) < 500 {
		t.Fatalf("corpus is %d inputs; something stopped contributing to it", len(inputs))
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# %d inputs. Regenerate with: go test -run TestCorpusGolden -corpus-update\n", len(inputs))
	fmt.Fprintf(&b, "# input\tresult\tinstant (UTC)\toffset\tformat\tambiguous\tkind\n")
	for _, in := range inputs {
		b.WriteString(corpusLine(in))
		b.WriteString("\n")
	}
	got := b.String()

	if os.Getenv("CORPUS_UPDATE") != "" {
		if err := os.MkdirAll(filepath.Dir(corpusGoldenPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(corpusGoldenPath, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s with %d inputs", corpusGoldenPath, len(inputs))
		return
	}

	wantBytes, err := os.ReadFile(corpusGoldenPath)
	if err != nil {
		t.Fatalf("%v\n\nregenerate with: make corpus-update", err)
	}
	want := string(wantBytes)
	if got == want {
		return
	}

	// Report the first differing lines rather than the whole file, and say how
	// many there are, because "one input moved" and "everything moved" want
	// different reactions from a reader.
	gotLines, wantLines := strings.Split(got, "\n"), strings.Split(want, "\n")
	diffs := 0
	var shown strings.Builder
	for i := 0; i < len(gotLines) || i < len(wantLines); i++ {
		g, w := "", ""
		if i < len(gotLines) {
			g = gotLines[i]
		}
		if i < len(wantLines) {
			w = wantLines[i]
		}
		if g == w {
			continue
		}
		diffs++
		if diffs <= 20 {
			fmt.Fprintf(&shown, "  - %s\n  + %s\n", w, g)
		}
	}
	t.Errorf("%d line(s) of the corpus golden changed:\n%s\n"+
		"Each line is what an input parses to. If the change is intended, "+
		"regenerate with `make corpus-update` in the same commit and say in the "+
		"message which inputs moved and why.", diffs, shown.String())
}
