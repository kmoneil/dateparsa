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

// TestLayoutParseZeroAlloc verifies that Layout.Parse allocates nothing.
func TestLayoutParseZeroAlloc(t *testing.T) {
	result, err := Parse("2024-03-15T10:30:00Z")
	if err != nil {
		t.Fatal(err)
	}
	layout := result.Layout
	allocs := testing.AllocsPerRun(1000, func() {
		layout.Parse("2025-01-01T00:00:00Z")
	})
	if allocs > 0 {
		t.Errorf("Layout.Parse allocated %f times, want 0", allocs)
	}
}
