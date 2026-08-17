package natural

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"
)

var boundsBase = time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)

func parseNL(s string) *Result {
	return Parse(s, Config{BaseTime: boundsBase})
}

// TestQuantityOverflowIsRefused is W15's assertion by input.
func TestQuantityOverflowIsRefused(t *testing.T) {
	for _, in := range []string{
		"9223372036854775807 days ago",
		"10000000000 seconds ago",
		"99999999999999999999 seconds ago",
		"99999999999999999999 years ago",
		"in 99999999999999999999 weeks",
		"1000000000000 months ago",
		"1234567 hours ago", // seven digits: the first width that wraps
	} {
		if r := parseNL(in); r != nil {
			t.Errorf("Parse(%q) = %v, want refused", in, r.Time)
		}
	}
}

// TestDirectionIsPreserved is the general property, and it is the one wrapping
// violates. A year range would be the obvious assertion and would be the wrong one:
// SECURITY.md puts a correct-but-implausible date out of scope, so "999999 years
// ago" landing in the year -997973 is a correct answer to a silly question. What can
// never be correct is an expression naming the past that resolves to the future.
//
// This is what "9223372036854775807 days ago" returning tomorrow violated, and it
// holds for every unit at the largest quantity the scanner now admits.
func TestDirectionIsPreserved(t *testing.T) {
	units := []string{"second", "minute", "hour", "day", "week", "month", "year"}
	quantities := []string{"1", "2", "999", "99999", "999999"}

	for _, u := range units {
		for _, q := range quantities {
			past := fmt.Sprintf("%s %ss ago", q, u)
			if r := parseNL(past); r != nil && r.Time.After(boundsBase) {
				t.Errorf("Parse(%q) = %v, which is after the base time %v",
					past, r.Time, boundsBase)
			}

			future := fmt.Sprintf("in %s %ss", q, u)
			if r := parseNL(future); r != nil && r.Time.Before(boundsBase) {
				t.Errorf("Parse(%q) = %v, which is before the base time %v",
					future, r.Time, boundsBase)
			}
		}
	}
}

// TestCompoundDirectionIsPreserved covers evalCompoundNAgo, which sums several
// quantities. The card flagged it: the bound has to hold for the sum and not only
// per term, and a bound tight enough per term is what makes the sum safe too, since
// each term is applied to the running total rather than accumulated first.
func TestCompoundDirectionIsPreserved(t *testing.T) {
	for _, in := range []string{
		"1 hour and 3 minutes ago",
		"999999 hours and 999999 hours ago",
		"999999 years and 999999 years ago",
		"999999 weeks and 999999 weeks and 999999 weeks ago",
		"in 999999 hours and 999999 hours",
	} {
		r := parseNL(in)
		if r == nil {
			continue // refusing is always allowed
		}
		if strings.HasPrefix(in, "in ") {
			if r.Time.Before(boundsBase) {
				t.Errorf("Parse(%q) = %v, before the base time", in, r.Time)
			}
			continue
		}
		if r.Time.After(boundsBase) {
			t.Errorf("Parse(%q) = %v, after the base time", in, r.Time)
		}
	}
}

// TestSixDigitsIsTheLargestSafeWidth pins why maxQuantityDigits is 6 and not a
// rounder number, so that raising it fails here rather than in the arithmetic.
func TestSixDigitsIsTheLargestSafeWidth(t *testing.T) {
	if maxQuantityDigits != 6 {
		t.Fatalf("maxQuantityDigits is %d.\n"+
			"The hour arm of addUnit multiplies by 3.6e12, so it wraps int64 above\n"+
			"2,562,047. Seven digits reach 3.6e19 against a ceiling of 9.223e18.\n"+
			"If this is being raised, check every arm of addUnit first.",
			maxQuantityDigits)
	}

	// The largest admitted quantity, on the tightest unit, must still be in the
	// past by the amount it says.
	r := parseNL("999999 hours ago")
	if r == nil {
		t.Fatal("999999 hours ago should parse")
	}
	want := boundsBase.Add(-999999 * time.Hour)
	if !r.Time.Equal(want) {
		t.Errorf("999999 hours ago = %v, want %v", r.Time, want)
	}
}

// TestOrdinaryExpressionsStillParse is the other direction, so the bounds cannot be
// satisfied by refusing everything.
func TestOrdinaryExpressionsStillParse(t *testing.T) {
	for _, in := range []string{
		"3 days ago", "in 2 weeks", "5 minutes ago", "a few days ago",
		"yesterday", "tomorrow", "next friday", "last month",
		"1 hour and 3 minutes ago", "half an hour ago",
		"999 days ago", "999999 days ago",
	} {
		if r := parseNL(in); r == nil {
			t.Errorf("Parse(%q) was refused", in)
		}
	}
}

// TestInputLengthIsBounded is W14 piece one.
func TestInputLengthIsBounded(t *testing.T) {
	// At the bound, a valid compound still parses, which is what fixes the size of
	// the bound: a compound relative expression is the one legitimately long input.
	terms := 0
	var b strings.Builder
	for b.Len() < MaxInputLen-30 {
		if terms > 0 {
			b.WriteString(" and ")
		}
		b.WriteString("1 day")
		terms++
	}
	b.WriteString(" ago")
	long := b.String()
	if len(long) > MaxInputLen {
		t.Fatalf("test built a %d-byte input, over the %d bound", len(long), MaxInputLen)
	}
	if r := parseNL(long); r == nil {
		t.Errorf("a %d-byte compound of %d terms was refused; the bound is meant to "+
			"admit the longest expression anybody writes", len(long), terms)
	} else {
		t.Logf("%d-byte compound of %d terms parses: %v", len(long), terms, r.Time)
	}

	// Past the bound, nothing is tokenised at all.
	if r := parseNL(strings.Repeat("a ", MaxInputLen)); r != nil {
		t.Errorf("an input past the bound returned %v", r.Time)
	}
}

// TestLongInputDoesNotAmplify is the gate on W14's measurement, modelled on
// TestLayoutParseZeroAlloc: a documented bound that nothing checks is a bound that
// stops being true.
//
// Before the cap, 1 MiB of words allocated 281 MB, a multiple of 268. The assertion
// is deliberately loose, because what it has to catch is a return to per-word
// allocation and not a small regression.
func TestLongInputDoesNotAmplify(t *testing.T) {
	const size = 1 << 20
	in := strings.Repeat("a ", size/2)

	var m0, m1 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m0)
	for range 100 {
		_ = parseNL(in)
	}
	runtime.ReadMemStats(&m1)

	perCall := float64(m1.TotalAlloc-m0.TotalAlloc) / 100
	if perCall > float64(len(in)) {
		t.Errorf("Parse of a %d-byte input allocated %.0f bytes per call, "+
			"which is %.1fx the input.\n"+
			"The bound in MaxInputLen is meant to make this a comparison and a "+
			"return, not a tokenisation.",
			len(in), perCall, perCall/float64(len(in)))
	}
	t.Logf("%d-byte input: %.0f bytes allocated per call (%.4fx)",
		len(in), perCall, perCall/float64(len(in)))
}
