package dateparsa

import (
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

// zeroAllocSample is fixed so a failure reproduces. It carries a fraction and a
// day and month over 12, so the formats that read those are exercised.
var zeroAllocSample = time.Date(2024, 3, 15, 10, 30, 45, 123000000, time.UTC)

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
}

// TestLayoutParseZeroAlloc verifies that Layout.Parse allocates nothing, for
// every format in the round-trip table and for the extras above.
//
// CLAUDE.md calls this "the reason the library exists" and runs it in CI, in
// make ci, and in the pre-commit hook. It used to parse one input,
// "2024-03-15T10:30:00Z", and assert on that one layout: one format of 31, and
// not one of the ones that allocated.
//
// It runs without -race on purpose. The race detector allocates, so folding
// this into a -race leg reports green while measuring nothing.
func TestLayoutParseZeroAlloc(t *testing.T) {
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
	}

	for _, spec := range roundTripFormats {
		input := zeroAllocSample.Format(spec.goFmt)
		if spec.render != nil {
			input = spec.render(zeroAllocSample)
		}
		t.Run(spec.name, func(t *testing.T) {
			check(t, spec.name, input, spec.opts...)
		})
	}

	for _, e := range zeroAllocExtras {
		t.Run(e.name, func(t *testing.T) {
			check(t, e.name, e.input)
		})
	}
}
