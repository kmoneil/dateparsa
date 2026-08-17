package detect

import (
	"testing"

	"github.com/kmoneil/dateparsa/internal/compile"
)

// fallbackCorpus is one input per shape the fallback detectors produce. The
// trie formats are not here: they return a pre-built def and allocate nothing.
var fallbackCorpus = []string{
	"03/15/2024",
	"3/15/24",
	"03/15/2024 10:30:00",
	"3/15/2024 1:02:03 PM",
	"15.03.2024",
	"March 15, 2024",
	"Mar 15, 2024",
	"15 March 2024",
	"March 2024",
	"15 March",
	"December 23rd, 2024",
	"September 17, 2012 at 10:09am",
	"Fri, 15 Mar 2024 10:30:00 UTC",
	"Fri, 15 Mar 2024 10:30:00 +0000",
	"Mon Jan  2 15:04:05 2006",
	"Fri Jul 03 2015 18:04:07 GMT+0100",
	"10/Oct/2000:13:55:36 -0700",
	"2024-W05-1",
	"2024-W05",
	"2024-075",
	"2020-07-20+08:00",
	"2024年3月15日",
	"2024-03-15T10:30:00.123456789+05:30",
	"2024-03-15T10:30:00.123Z",
}

// TestFallbackFieldCountsFitTheScratch measures what newResult is sized for.
//
// The two scratch sizes are a tuning decision taken against these numbers, and
// a format that outgrows the larger one still parses: the append in newResult
// moves to the heap and costs one more allocation. So this reports rather than
// forbids, except at the hard limit, where Compile refuses the format outright
// and the input stops parsing at all.
func TestFallbackFieldCountsFitTheScratch(t *testing.T) {
	small, large, over := 0, 0, 0
	for _, in := range fallbackCorpus {
		r, ok := Detect(in, Config{})
		if !ok {
			t.Errorf("Detect(%q) found nothing; the corpus should be parseable", in)
			continue
		}
		n := len(r.Def.Fields)
		switch {
		case n > compile.MaxInstructions:
			t.Errorf("Detect(%q) [%s] built %d fields, over the %d Compile accepts",
				in, r.Def.Name, n, compile.MaxInstructions)
		case n > scratchFields:
			over++
			t.Logf("%-38q %-22s %2d fields, over scratchFields (%d): costs an "+
				"extra allocation", in, r.Def.Name, n, scratchFields)
		case n > smallScratchFields:
			large++
		default:
			small++
		}
	}
	t.Logf("fallback formats: %d fit the small scratch, %d the large, %d overflow",
		small, large, over)
	if over > 0 {
		t.Logf("if that count grows, raise scratchFields and say what moved")
	}
}

// TestNewResultCopiesTheFields is what makes a caller's local array safe.
//
// Every fallback detector builds its fields in an array in its own frame and
// hands the slice to newResult. If newResult retained that slice instead of
// copying, the Def would point at a frame that has returned, and the fields it
// described would be whatever the next call put there. That is a wrong date
// rather than a crash, so it is checked rather than reasoned about.
func TestNewResultCopiesTheFields(t *testing.T) {
	src := []compile.Field{
		{Kind: compile.FYear4, Offset: 0, Len: 4},
		{Kind: compile.FMonth2, Offset: 5, Len: 2},
	}
	r := newResult("X", "", src, AmbigNone, false)

	// Scribble over the source the way a reused frame would.
	for i := range src {
		src[i] = compile.Field{Kind: compile.FSkip, Offset: 99, Len: 99}
	}

	if got := r.Def.Fields[0]; got.Kind != compile.FYear4 || got.Offset != 0 || got.Len != 4 {
		t.Errorf("newResult kept a reference to its argument: field 0 is now %+v", got)
	}
	if got := r.Def.Fields[1]; got.Kind != compile.FMonth2 || got.Offset != 5 {
		t.Errorf("newResult kept a reference to its argument: field 1 is now %+v", got)
	}
}

// TestNewResultSurvivesOverflow covers the path where a format needs more
// fields than either scratch holds inline. The append moves to the heap, which
// costs an allocation and must not cost correctness.
func TestNewResultSurvivesOverflow(t *testing.T) {
	for _, n := range []int{1, smallScratchFields, smallScratchFields + 1, scratchFields, scratchFields + 1, scratchFields * 3} {
		src := make([]compile.Field, n)
		for i := range src {
			src[i] = compile.Field{Kind: compile.FSkip, Offset: int32(i), Len: 1}
		}
		r := newResult("X", "", src, AmbigNone, false)
		if len(r.Def.Fields) != n {
			t.Fatalf("%d fields in, %d out", n, len(r.Def.Fields))
		}
		for i := range src {
			if r.Def.Fields[i].Offset != int32(i) {
				t.Fatalf("%d fields: field %d is %+v", n, i, r.Def.Fields[i])
			}
		}
	}
}

// TestWithTimeSuffixCoversResolveAmbiguous holds the shortcut in withTimeSuffix
// to the names it shortcuts. A name it does not know still gets the right
// answer, by concatenation, so this is about the allocation and not about
// correctness.
func TestWithTimeSuffixCoversResolveAmbiguous(t *testing.T) {
	seen := map[string]bool{}
	for _, in := range []string{
		"03/15/2024", "3/15/24", "15/03/2024", "01/02/2024", "15.03.2024",
	} {
		var buf [maxDetectFields]compile.Field
		name, _, _, ok := resolveAmbiguousFields(buf[:0], in, Config{})
		if !ok {
			continue
		}
		seen[name] = true
	}
	if len(seen) == 0 {
		t.Fatal("resolveAmbiguousFields produced no names; the corpus is wrong")
	}
	for name := range seen {
		want := name + "_TIME"
		if got := withTimeSuffix(name); got != want {
			t.Errorf("withTimeSuffix(%q) = %q, want %q", name, got, want)
		}
		// The point of the function is that the answer needs no allocation,
		// which only holds for a name it names.
		allocs := testing.AllocsPerRun(100, func() { _ = withTimeSuffix(name) })
		if allocs > 0 {
			t.Errorf("withTimeSuffix(%q) allocates %.0f times; add it to the switch",
				name, allocs)
		}
	}
}

// TestFallbackDetectionAllocationBudget is the regression gate on the thing
// this file exists for.
//
// A fallback parse builds one heap object for its def and its fields, and the
// caller builds one Layout. Anything above that is a helper that has started
// returning a freshly allocated slice again, which is how it was: a textual
// date cost four allocations and "03/15/2024 10:30:00" cost ten.
func TestFallbackDetectionAllocationBudget(t *testing.T) {
	for _, in := range fallbackCorpus {
		if _, ok := Detect(in, Config{}); !ok {
			continue
		}
		allocs := testing.AllocsPerRun(200, func() {
			_, _ = Detect(in, Config{})
		})
		if allocs > 1 {
			t.Errorf("Detect(%q) allocates %.0f times, want 1", in, allocs)
		}
	}
}

// TestTrieDetectionAllocatesNothing is the other half. A format the trie
// matches returns a def built at init, and the work in this file must not have
// put an allocation in front of it.
func TestTrieDetectionAllocatesNothing(t *testing.T) {
	for _, in := range []string{
		"2024-03-15", "2024-03-15T10:30:00Z", "2024-03-15 10:30:00",
		"20240315", "10:30:00", "2024-03-15T10:30:00",
	} {
		if _, ok := Detect(in, Config{}); !ok {
			t.Errorf("Detect(%q) found nothing", in)
			continue
		}
		allocs := testing.AllocsPerRun(200, func() {
			_, _ = Detect(in, Config{})
		})
		if allocs > 0 {
			t.Errorf("Detect(%q) allocates %.0f times, want 0", in, allocs)
		}
	}
}
