package compare

import (
	"testing"
	"time"

	araddon "github.com/araddon/dateparse"
	"github.com/kmoneil/dateparsa"
)

// BenchmarkOneShot is one call on one value with no reuse: the format is
// detected and the value parsed, every time.
//
// This is the comparison that matches how araddon is used, and how dateparsa is
// used by a caller who has a single date rather than a column of them.
func BenchmarkOneShot(b *testing.B) {
	for _, c := range Corpus {
		b.Run(c.Name+"/dateparsa", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_, _ = dateparsa.Parse(c.Input)
			}
		})
		b.Run(c.Name+"/araddon", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_, _ = araddon.ParseAny(c.Input)
			}
		})
	}
}

// BenchmarkReuse is the per-row cost after the format is known, which is the
// case a column of a million rows is made of.
//
// Each library uses its own mechanism, and each is given its best one.
// dateparsa returns a compiled Layout from the first parse and re-runs it.
// araddon has no compiled form; what it offers is ParseFormat, which returns a
// Go layout string for time.Parse, so that is what this times. Neither
// mechanism is charged for the detection here, which BenchmarkColumn does.
//
// The two entries araddon's layout cannot round-trip are skipped for araddon
// only, and TestAraddonReuseGaps says which and why.
func BenchmarkReuse(b *testing.B) {
	for _, c := range Corpus {
		result, err := dateparsa.Parse(c.Input)
		if err != nil {
			b.Fatalf("dateparsa.Parse(%q): %v", c.Input, err)
		}
		if _, err := result.Layout.Parse(c.Input); err == nil {
			b.Run(c.Name+"/dateparsa", func(b *testing.B) {
				layout := result.Layout
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					_, _ = layout.Parse(c.Input)
				}
			})
		} else {
			// LayoutEpoch and LayoutNaturalLanguage refuse by design. Naming
			// the skip keeps it from reading as an omission.
			b.Run(c.Name+"/dateparsa", func(b *testing.B) {
				b.Skipf("%v is a sentinel layout and refuses to re-parse", result.Layout)
			})
		}

		if layout, ok := araddonCanReuse(c.Input); ok {
			b.Run(c.Name+"/araddon", func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					_, _ = time.Parse(layout, c.Input)
				}
			})
		} else {
			b.Run(c.Name+"/araddon", func(b *testing.B) {
				b.Skip("ParseFormat's layout does not re-parse the input; see TestAraddonReuseGaps")
			})
		}
	}
}

// columnRows is how many values a benchmarked column holds. Big enough that the
// one detection is amortised the way it is in a real import, small enough that
// the benchmark finishes.
const columnRows = 10000

// BenchmarkColumn is the whole job rather than one call of it: a column of
// values in one unknown format, parsed end to end.
//
// This is the case the library exists for and the only one where the two
// designs actually differ. Both are given the same rows and both use their own
// intended mechanism, detection included, so the detection cost is paid by both
// exactly once.
//
// dateparsa gets two entries because it offers two ways to do this and they
// cost different amounts: Parser caches the layout and re-detects when a row
// stops matching, while holding the Layout by hand does not.
//
// The destination slice is allocated once, outside the timer, for the two
// hand-rolled loops. It was inside, and that made the benchmark useless: a
// fresh 240KB slice per iteration is tens of megabytes of garbage over a run,
// and the GC that follows swamped the parsing. It reported 109 ns a row for a
// format BenchmarkReuse measures at 17.7. A caller fills a column into one
// slice, so hoisting it is also what the real code does.
//
// dateparsa_Parser cannot be given the same treatment, because ParseColumn
// allocates and returns its own two slices. Its number therefore includes an
// allocation the other two hoisted out, and is not comparable with them on
// that basis; it is comparable on the parsing, which is the bulk of it.
func BenchmarkColumn(b *testing.B) {
	for _, c := range Corpus {
		rows := make([]string, columnRows)
		for i := range rows {
			rows[i] = c.Input
		}

		b.Run(c.Name+"/dateparsa_Parser", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				p := dateparsa.NewParser()
				p.ParseColumn(rows)
			}
		})

		b.Run(c.Name+"/dateparsa_Layout", func(b *testing.B) {
			out := make([]time.Time, len(rows))
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				result, err := dateparsa.Parse(rows[0])
				if err != nil {
					b.Fatal(err)
				}
				out[0] = result.Time
				layout := result.Layout
				for i := 1; i < len(rows); i++ {
					t, err := layout.Parse(rows[i])
					if err != nil {
						// A sentinel layout, or a row the layout does not
						// describe. Fall back the way a caller would.
						r, rerr := dateparsa.Parse(rows[i])
						if rerr != nil {
							b.Fatal(rerr)
						}
						t = r.Time
					}
					out[i] = t
				}
			}
		})

		b.Run(c.Name+"/araddon", func(b *testing.B) {
			out := make([]time.Time, len(rows))
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				layout, reusable := araddonCanReuse(rows[0])
				for i := range rows {
					if reusable {
						if t, err := time.Parse(layout, rows[i]); err == nil {
							out[i] = t
							continue
						}
					}
					t, err := araddon.ParseAny(rows[i])
					if err != nil {
						b.Fatal(err)
					}
					out[i] = t
				}
			}
		})
	}
}

// BenchmarkMiss is a value in a date column that is not a date, which every
// real import is full of and neither library's own benchmarks measure.
//
// A miss costs more than a hit in both libraries, because it is the whole
// detection path with nothing found at the end of it.
func BenchmarkMiss(b *testing.B) {
	for _, m := range Misses {
		b.Run(m.Name+"/dateparsa", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_, _ = dateparsa.Parse(m.Input)
			}
		})
		b.Run(m.Name+"/araddon", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_, _ = araddon.ParseAny(m.Input)
			}
		})
	}
}
