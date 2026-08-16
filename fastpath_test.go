package dateparsa

import (
	"sort"
	"strings"
	"testing"
	"time"
)

// fastPathFormats is which of the round-trip formats detection compiles into a
// program the straight-line executor can run, and it is checked rather than
// described because nothing else would notice it changing.
//
// A format leaves this set by acquiring a field the slot layout has no place
// for, which can happen without touching internal/compile at all: widening a
// day to 1-or-2 digits, or emitting a separator the fusion in Compile declines
// to fold, moves a format onto the interpreter and costs it about a third of its
// speed. That is a legitimate change to make, and this list is what makes it a
// decision rather than an accident. Update it in the same commit and say which
// way it moved.
//
// The formats not listed are on the interpreter for reasons that are not going
// to change: a month name, a weekday, an ISO week or ordinal day needing
// post-processing, a variable-width numeric, or a trailing tail.
// Twenty of the thirty-one, which is the answer that matters: the formats a
// CSV column or a log file is actually full of are all here.
var fastPathFormats = []string{
	"COMPACT_DATE",
	"COMPACT_DATETIME",
	"COMPACT_DATETIME_Z",
	"EXIF_DATE",
	"EXIF_DATETIME_HMS",
	"ISO8601_DATE",
	"ISO8601_DATETIME",
	"ISO8601_DATETIME_Z",
	"ISO8601_FRAC",
	"ISO_YEAR_MONTH",
	"NUMERIC_DMY",
	"NUMERIC_MDY",
	"SQL_DATETIME",
	"SQL_DATETIME_AMPM",
	"SQL_DATETIME_COMMA_FRAC3",
	"SQL_DATETIME_FRAC3",
	"SQL_DATETIME_TZ_NAME",
	"TIME_HM",
	"TIME_HMS",
	"TIME_HM_AMPM",
}

func TestFastPathCoverage(t *testing.T) {
	sample := time.Date(2024, 3, 15, 10, 30, 45, 123000000, time.UTC)

	var got []string
	for _, spec := range roundTripFormats {
		input := sample.Format(spec.goFmt)
		if spec.render != nil {
			input = spec.render(sample)
		}
		result, err := ParseWith(input, spec.opts...)
		if err != nil {
			t.Errorf("%s: Parse(%q): %v", spec.name, input, err)
			continue
		}
		if result.Layout.program.Width != 0 {
			got = append(got, result.Layout.String())
		}
	}

	got = dedupeSorted(got)
	want := append([]string(nil), fastPathFormats...)
	sort.Strings(want)

	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("the set of formats on the straight-line executor changed.\n"+
			"on the fast path now:\n  %s\nexpected:\n  %s\n"+
			"If this is intended, update fastPathFormats and say in the commit "+
			"which formats moved and why.",
			strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

func dedupeSorted(in []string) []string {
	sort.Strings(in)
	out := in[:0]
	for i, s := range in {
		if i == 0 || s != in[i-1] {
			out = append(out, s)
		}
	}
	return out
}

// TestFastPathAgreesWithDetection is the end-to-end half of the cross-check in
// internal/compile: a value that Parse read one way has to read the same way
// through the Layout that Parse handed back, for every format on the fast path.
//
// The two are not the same code. Parse runs the detector, which validated the
// fields itself before compiling anything; Layout.Parse runs only the program.
// A layout that agrees with detection on the value it was built from and
// disagrees on the next row is the failure this catches, and it is the shape of
// bug that the cached layout in Parser would spread across a whole column.
func TestFastPathAgreesWithDetection(t *testing.T) {
	samples := []time.Time{
		time.Date(2024, 3, 15, 10, 30, 45, 123000000, time.UTC),
		time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(1999, 12, 31, 23, 59, 59, 0, time.UTC),
		time.Date(2000, 2, 29, 12, 0, 0, 0, time.UTC),
		time.Date(2024, 11, 3, 1, 30, 0, 0, time.UTC),
	}

	for _, spec := range roundTripFormats {
		t.Run(spec.name, func(t *testing.T) {
			first := samples[0].Format(spec.goFmt)
			if spec.render != nil {
				first = spec.render(samples[0])
			}
			result, err := ParseWith(first, spec.opts...)
			if err != nil {
				t.Fatalf("Parse(%q): %v", first, err)
			}
			if result.Layout.program.Width == 0 {
				t.Skip("interpreter format, covered by the round-trip suite")
			}

			for _, s := range samples[1:] {
				in := s.Format(spec.goFmt)
				if spec.render != nil {
					in = spec.render(s)
				}
				// A sample that renders to a different width is a different
				// format, not a disagreement: RFC 3339 nano drops trailing
				// zeros, so a whole second renders 10 bytes shorter, and a
				// fixed-width layout is right to refuse it. Parse re-detects and
				// succeeds, which is the documented difference between the two
				// calls rather than something this test is asking about.
				if len(in) != len(first) {
					continue
				}
				direct, derr := ParseWith(in, spec.opts...)
				reused, rerr := result.Layout.Parse(in)

				if (derr == nil) != (rerr == nil) {
					t.Errorf("%q: Parse err=%v, Layout.Parse err=%v", in, derr, rerr)
					continue
				}
				if derr == nil && !direct.Time.Equal(reused) {
					t.Errorf("%q: Parse %v, Layout.Parse %v", in, direct.Time, reused)
				}
			}
		})
	}
}
