package dateparsa

import (
	"fmt"
	"math/rand"
	"testing"
	"time"
)

// BenchmarkParse_ISO8601 benchmarks parsing an ISO 8601 date (detection + parse).
func BenchmarkParse_ISO8601(b *testing.B) {
	for b.Loop() {
		Parse("2024-03-15")
	}
}

// BenchmarkParse_ISO8601_DateTime benchmarks ISO datetime with Z.
func BenchmarkParse_ISO8601_DateTime(b *testing.B) {
	for b.Loop() {
		Parse("2024-03-15T10:30:00Z")
	}
}

// BenchmarkParse_RFC3339 benchmarks RFC3339 with timezone offset.
func BenchmarkParse_RFC3339(b *testing.B) {
	for b.Loop() {
		Parse("2024-03-15T10:30:00+05:30")
	}
}

// BenchmarkParse_SQLDatetime benchmarks SQL datetime format.
func BenchmarkParse_SQLDatetime(b *testing.B) {
	for b.Loop() {
		Parse("2024-03-15 10:30:00")
	}
}

// BenchmarkParse_TextualMonth benchmarks textual month parsing.
func BenchmarkParse_TextualMonth(b *testing.B) {
	for b.Loop() {
		Parse("March 15, 2024")
	}
}

// BenchmarkParse_AmbiguousSlash benchmarks ambiguous numeric dates.
func BenchmarkParse_AmbiguousSlash(b *testing.B) {
	for b.Loop() {
		Parse("03/15/2024")
	}
}

// The four below measure a parse that fails, which nothing here used to do.
//
// Every other benchmark in this file parses something that succeeds, and a real
// ingest column is not like that: it has empty cells, "N/A", free text, and rows
// where somebody typed a note into a date field. Each of those walks the whole
// detection cascade, then epoch.Detect, then natural.Parse, and then allocates a
// *ParseError, so a miss costs several times what a hit does and grew without
// anybody being able to see it.
//
// The inputs are fixed rather than generated, so a regression reproduces.
//
// missText is deliberately long enough to make the per-byte constant visible
// next to the short forms. The cost is linear in input length, measured at 15.4
// to 19.8 MB/s over inputs from 68 to 4352 bytes, so what these track is the
// constant and not the shape.
const (
	missShort = "N/A"
	missText  = "not a date at all"
	missLong  = "the quick brown fox jumps over the lazy dog and then some more text "
)

// BenchmarkParse_Miss_Short benchmarks the empty-ish cell an import is full of.
func BenchmarkParse_Miss_Short(b *testing.B) {
	for b.Loop() {
		_, _ = Parse(missShort)
	}
}

// BenchmarkParse_Miss_Text benchmarks free text in a date column.
func BenchmarkParse_Miss_Text(b *testing.B) {
	for b.Loop() {
		_, _ = Parse(missText)
	}
}

// BenchmarkParse_Miss_Long reports MB/s as well, which is what makes the
// per-byte constant comparable against the short forms.
func BenchmarkParse_Miss_Long(b *testing.B) {
	b.SetBytes(int64(len(missLong)))
	for b.Loop() {
		_, _ = Parse(missLong)
	}
}

// BenchmarkParse_Miss_Locales is the same miss with locales configured, which is
// what makes the per-locale rescan in natural.Parse visible: it scans once for
// English and once more for each locale, building a token slice every time, for
// an input that fails all of them.
func BenchmarkParse_Miss_Locales(b *testing.B) {
	for b.Loop() {
		_, _ = ParseWith(missText, WithLocales(FR, DE, ES))
	}
}

// BenchmarkParse_Miss_Locales_NonLatin is the same miss against locales written
// in another script.
//
// The phrase table is bucketed by the first byte of the phrase, and a Cyrillic
// or CJK phrase starts with a UTF-8 lead byte that no ASCII input can carry, so
// this is the case the index dismisses outright. It is also the case that
// benefits least from the index when the input *is* in that script, since a
// whole table shares two or three lead bytes between them: see
// BenchmarkParse_Locale_RussianNL for that side of it.
func BenchmarkParse_Miss_Locales_NonLatin(b *testing.B) {
	opts := []Option{WithLocales(RU, ZH, JA)}
	for b.Loop() {
		_, _ = ParseWith(missText, opts...)
	}
}

// BenchmarkLayout_Parse benchmarks the compiled Layout hot path.
func BenchmarkLayout_Parse(b *testing.B) {
	result, err := Parse("2024-03-15T10:30:00Z")
	if err != nil {
		b.Fatal(err)
	}
	layout := result.Layout
	b.ResetTimer()
	for b.Loop() {
		layout.Parse("2025-01-01T00:00:00Z")
	}
}

// BenchmarkLayout_Parse_ISODate benchmarks compiled parse for ISO date only.
func BenchmarkLayout_Parse_ISODate(b *testing.B) {
	result, err := Parse("2024-03-15")
	if err != nil {
		b.Fatal(err)
	}
	layout := result.Layout
	b.ResetTimer()
	for b.Loop() {
		layout.Parse("2025-06-15")
	}
}

// BenchmarkDetect_Only benchmarks detection without parsing.
func BenchmarkDetect_Only(b *testing.B) {
	for b.Loop() {
		Detect("2024-03-15T10:30:00Z")
	}
}

// BenchmarkVsStdlib benchmarks stdlib time.Parse with a known layout for comparison.
func BenchmarkVsStdlib(b *testing.B) {
	for b.Loop() {
		time.Parse(time.RFC3339, "2024-03-15T10:30:00Z")
	}
}

// BenchmarkParser_Cached benchmarks the Parser with cached layout (batch scenario).
func BenchmarkParser_Cached(b *testing.B) {
	p := NewParser()
	// Warm up the cache.
	p.Parse("2024-03-15T10:30:00Z")
	b.ResetTimer()
	for b.Loop() {
		p.Parse("2025-01-01T00:00:00Z")
	}
}

// Benchmark10M_Layout_Parse benchmarks parsing 10M rows with a compiled layout.
func Benchmark10M_Layout_Parse(b *testing.B) {
	result, err := Parse("2024-03-15T10:30:00Z")
	if err != nil {
		b.Fatal(err)
	}
	layout := result.Layout

	const rows = 10_000_000
	input := "2025-01-15T08:30:00Z"

	b.ResetTimer()
	for range b.N {
		for range rows {
			layout.Parse(input)
		}
	}
}

// Benchmark10M_Parser_ParseColumn benchmarks batch parsing 10M rows.
func Benchmark10M_Parser_ParseColumn(b *testing.B) {
	const rows = 10_000_000
	values := make([]string, rows)
	for i := range values {
		values[i] = "2024-06-15T12:00:00Z"
	}

	b.ResetTimer()
	for range b.N {
		p := NewParser()
		p.ParseColumn(values)
	}
}

// BenchmarkLayout_Parse_TZName_CET is here because the number it holds is the
// one that went unwatched. Resolving CET fell past the pre-built abbreviation
// table into time.LoadLocation, which reads the zone file on every call and does
// not cache: 18035 ns and 24 allocations, against 58 ns and 0 for the same
// format with a name the table knew.
func BenchmarkLayout_Parse_TZName_CET(b *testing.B) {
	result, err := Parse("2024-03-15 10:30:00 CET")
	if err != nil {
		b.Fatal(err)
	}
	layout := result.Layout
	b.ResetTimer()
	for b.Loop() {
		layout.Parse("2024-03-15 10:30:00 CET")
	}
}

// BenchmarkLayout_Parse_TZOffset_OffGrid is the offset half of the benchmark
// above, and the same shape of hole: an offset off the 15-minute grid missed
// the pre-built table and built a Location per call, three allocations, while
// the on-grid benchmark beside it reported zero.
//
// The pair is the point. Both run the same program over the same format and
// differ by seven minutes of zone offset.
func BenchmarkLayout_Parse_TZOffset_OffGrid(b *testing.B) {
	const in = "2024-03-15T10:30:00+05:53" // Bombay, until 1955
	result, err := Parse(in)
	if err != nil {
		b.Fatal(err)
	}
	layout := result.Layout
	b.ResetTimer()
	for b.Loop() {
		layout.Parse(in)
	}
}

func BenchmarkLayout_Parse_TZOffset_OnGrid(b *testing.B) {
	const in = "2024-03-15T10:30:00+05:30"
	result, err := Parse(in)
	if err != nil {
		b.Fatal(err)
	}
	layout := result.Layout
	b.ResetTimer()
	for b.Loop() {
		layout.Parse(in)
	}
}

// zeroAllocSample is fixed so a failure reproduces. It carries a fraction and a
// day and month over 12, so the formats that read those are exercised.
var zeroAllocSample = time.Date(2024, 3, 15, 10, 30, 45, 123000000, time.UTC)

// zeroAllocGenerated is how many generated samples each format is checked over,
// on top of the fixed one. Eight is enough to reach both widths of a 1-or-2
// digit field and, for a zone-bearing format, to land off the 15-minute grid
// with near certainty: 105 of the 2879 offsets the generator draws from are on
// it, so eight draws miss the grid 74 times in 100 at worst and always in
// practice at this seed.
const zeroAllocGenerated = 8

// zeroAllocExtras are inputs the round-trip table cannot produce.
//
// The table drives most of this test, but a format's spec renders one sample
// and that sample takes one path through the executor. SQL_TZ_NAME renders
// "UTC", which is the first case of lookupTZAbbr, so the table alone reported 0
// for all 31 formats while "2024-03-15 10:30:00 CET" allocated 24 times: the
// name fell past the pre-built table into time.LoadLocation, which reads the
// zone file on every call and does not cache.
//
// That is the shape of assertion this repository has now written three times
// and had wrong twice, so it is worth naming: a table-driven gate covers the
// paths its samples happen to take, not the paths that exist. Anything with a
// branch inside the executor needs an input per branch.
var zeroAllocExtras = []struct{ name, input string }{
	{"TZ_NAME/UTC", "2024-03-15 10:30:00 UTC"},
	{"TZ_NAME/GMT", "2024-03-15 10:30:00 GMT"},
	{"TZ_NAME/EST", "2024-03-15 10:30:00 EST"},
	{"TZ_NAME/PDT", "2024-03-15 10:30:00 PDT"},
	{"TZ_NAME/HST", "2024-03-15 10:30:00 HST"},
	{"TZ_NAME/UCT", "2024-03-15 10:30:00 UCT"},

	// The four that fell through to the filesystem. W2.
	{"TZ_NAME/CET", "2024-03-15 10:30:00 CET"},
	{"TZ_NAME/EET", "2024-03-15 10:30:00 EET"},
	{"TZ_NAME/MET", "2024-03-15 10:30:00 MET"},
	{"TZ_NAME/WET", "2024-03-15 10:30:00 WET"},

	// Offsets, both widths and both signs, and the conditional zone on each
	// of its two branches.
	{"TZ_OFFSET/+05:30", "2024-03-15T10:30:00+05:30"},
	{"TZ_OFFSET/-08:00", "2024-03-15T10:30:00-08:00"},
	{"TZ_OFFSET/+0530", "2024-03-15 10:30:00 +0530"},
	{"TZ_Z", "2024-03-15T10:30:00Z"},

	// Off the 15-minute grid, which is the other side of the branch above and
	// the one M9 was filed for: every sample in this file used to sit on the
	// grid, so the fallback three lines below the one they exercise had never
	// been measured. It allocated three times per call.
	//
	// Historical zones are the case. Not a fiction, and not one row: a column
	// of pre-1955 Indian records carries +05:53 on every row.
	{"TZ_OFFSET/+05:53", "2024-03-15T10:30:00+05:53"}, // Bombay, until 1955
	{"TZ_OFFSET/-00:44", "2024-03-15T10:30:00-00:44"}, // Monrovia, until 1972
	{"TZ_OFFSET/+04:51", "2024-03-15T10:30:00+04:51"}, // Kathmandu, until 1920
	{"TZ_OFFSET/+0537", "2024-03-15 10:30:00 +0537"},  // off-grid, no colon

	// Past the pre-built table's range rather than off its grid. It stops at
	// +14:00 and parseTZOffset accepts out to 23:59, so these take the same
	// fallback for a different reason.
	{"TZ_OFFSET/+23:59", "2024-03-15T10:30:00+23:59"},
	{"TZ_OFFSET/-23:59", "2024-03-15T10:30:00-23:59"},

	// Variable-width numeric, which reaches the 1-or-2 ops and their delta.
	{"NUMERIC/1or2", "3/15/2024"},
	{"NUMERIC/2", "03/15/2024"},

	// The textual family, which is where the skips and the month-name check
	// live.
	{"TEXTUAL/ordinal", "December 23rd, 2024"},
	{"TEXTUAL/weekday", "Fri Jul 03 2015 18:04:07 GMT+0100"},
	{"TEXTUAL/at", "September 17, 2012 at 10:09am"},

	// Fractions at each width the executor branches on.
	{"FRAC/3", "2024-03-15 10:30:00.123"},
	{"FRAC/6", "2024-03-15 10:30:00.123456"},
	{"FRAC/9", "2024-03-15T10:30:00.123456789Z"},

	// Either side of stringCopyStackMax, which is where ParseBytes stops
	// converting for free. Everything above is at or under it except
	// TEXTUAL/weekday at 33, and one input a byte over a boundary is not
	// coverage of the far side of it.
	{"LONG/frac9+offset", "2024-03-15T10:30:00.123456789+05:30"}, // 35
	{"SHORT/at the limit", "2024-03-15T10:30:00.12345+05:30"},    // 32

	// Padded rows, which F9 made parseable and which the generator cannot
	// produce: it renders a value, and padding is not part of one. A CSV
	// column and a log line are where these come from, and both are named in
	// README.md's first paragraph.
	//
	// The last two are the pair that matters for ParseBytes. Trimming happens
	// before the copy, so a 34-byte input that holds a 30-byte value copies 30
	// and allocates nothing, where the same 34 bytes of value would allocate
	// once. If trimming ever moves to after the copy, that one flips to 1 and
	// this says so.
	{"PADDED/leading space", " 2024-03-15"},
	{"PADDED/trailing space", "2024-03-15 "},
	{"PADDED/both", "  2024-03-15  "},
	{"PADDED/crlf", "2024-03-15\r\n"},
	{"PADDED/tab both ends", "\t2024-03-15T10:30:00Z\t"},
	{"PADDED/textual", "  March 15, 2024  "},
	{"PADDED/over 32 padded, under 32 trimmed", "  2024-03-15T10:30:00Z  "},  // 24 bytes of value in 28
	{"PADDED/over 32 either way", "  2024-03-15T10:30:00.123456789+05:30  "}, // 35 bytes of value in 39
}

// stringCopyStackMax is the largest []byte the runtime turns into a string
// without allocating, which it does out of a fixed stack buffer when escape
// analysis proves the string does not outlive the call. runtime.tmpBuf is
// [32]byte and slicebytetostring uses it when the length fits.
//
// It is a runtime implementation detail rather than a language guarantee. If a
// Go release moves it, TestLayoutParseZeroAlloc says so by name and by length,
// which is the failure this constant is here to make legible.
const stringCopyStackMax = 32

// TestLayoutParseZeroAlloc verifies that Layout.Parse allocates nothing, for
// every format in the round-trip table and for the extras above, and that
// Layout.ParseBytes allocates nothing beyond the copy it is documented to make.
//
// CLAUDE.md calls this "the reason the library exists" and runs it in CI, in
// make ci, and in the pre-commit hook. It used to parse one input,
// "2024-03-15T10:30:00Z", and assert on that one layout: one format of 31, and
// not one of the ones that allocated.
//
// It runs without -race on purpose. The race detector allocates, so folding
// this into a -race leg reports green while measuring nothing.
func TestLayoutParseZeroAlloc(t *testing.T) {
	// Both sides of the ParseBytes boundary have to be reached, or the half
	// that is not becomes a branch nothing runs. Exactly one input in the whole
	// set is over 32 bytes today, so this is one deletion away from vacuous.
	var shortInputs, longInputs int

	check := func(t *testing.T, name, input string, opts ...Option) {
		t.Helper()
		result, err := ParseWith(input, opts...)
		if err != nil {
			t.Errorf("%s: Parse(%q): %v", name, input, err)
			return
		}
		if !reusable(result.Layout) {
			return // a sentinel layout refuses to re-parse by design
		}
		layout := result.Layout
		allocs := testing.AllocsPerRun(1000, func() {
			_, _ = layout.Parse(input)
		})
		if allocs > 0 {
			t.Errorf("%s [%v]: Layout.Parse(%q) allocated %.0f times, want 0",
				name, layout, input, allocs)
		}

		// ParseBytes is a second entry point running a second function,
		// Program.ExecuteBytes, and nothing measured it. It is also the entry
		// point a caller reaches for *because* they want to avoid a copy.
		//
		// It does not allocate zero. It converts to string, and the runtime
		// answers that conversion out of a 32-byte stack buffer when the string
		// does not escape and out of the heap when it will not fit, so the cost
		// is 0 up to stringCopyStackMax and exactly 1 above it. The gate is that
		// number rather than zero: a 0 that becomes 1 under the limit means the
		// string started escaping, and a 1 that becomes 2 anywhere means
		// something new was allocated, and both are regressions this would
		// otherwise miss. The existing extras list already carries a 33-byte
		// input, TEXTUAL/weekday, so asserting a flat zero here fails on the
		// spot.
		// Against the trimmed length, not len(input). ExecuteBytes converts
		// what it is given, and since F9 Layout.ParseBytes trims before it
		// hands the slice over, so the copy is the value and not the padding
		// around it. Counting the padding here would demand an allocation for
		// an input that does not make one.
		b := []byte(input)
		valueLen := len(trimPadding(input))
		want := 0.0
		if valueLen > stringCopyStackMax {
			want = 1.0
			longInputs++
		} else {
			shortInputs++
		}
		allocsB := testing.AllocsPerRun(1000, func() {
			_, _ = layout.ParseBytes(b)
		})
		if allocsB != want {
			t.Errorf("%s [%v]: Layout.ParseBytes(%q) allocated %.0f times, want %.0f for a "+
				"%d-byte value in a %d-byte input",
				name, layout, input, allocsB, want, valueLen, len(input))
		}
	}

	// The fixed sample first, then samples this test did not choose.
	//
	// A list of inputs known to pass is not a gate, and this one has been
	// widened twice after it reported green over a defect: M2 for the zone
	// names, M9 for the zone offsets. Both times the sample set was chosen by
	// somebody who knew which inputs satisfied it, so it could not discover
	// one that did not. The generator can: it is the same rng and the same
	// seed the round-trip suite runs on, and for a zone-bearing format it now
	// draws an offset from the whole range rather than rendering UTC.
	//
	// Deterministic, so a failure reproduces: seed 42, and the subtests run in
	// order without t.Parallel.
	rng := rand.New(rand.NewSource(42))
	for _, spec := range roundTripFormats {
		t.Run(spec.name, func(t *testing.T) {
			check(t, spec.name+"/fixed", renderSample(spec, zeroAllocSample, rng), spec.opts...)
			for i := range zeroAllocGenerated {
				check(t, fmt.Sprintf("%s/gen%d", spec.name, i),
					renderSample(spec, randomTime(rng), rng), spec.opts...)
			}
		})
	}

	for _, e := range zeroAllocExtras {
		t.Run(e.name, func(t *testing.T) {
			check(t, e.name, e.input)
		})
	}

	if shortInputs == 0 || longInputs == 0 {
		t.Errorf("the ParseBytes gate saw %d inputs at or under %d bytes and %d over: "+
			"both are needed, add one to zeroAllocExtras",
			shortInputs, stringCopyStackMax, longInputs)
	}
}
