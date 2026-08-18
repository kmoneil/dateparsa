package flextime

import (
	"encoding/json"
	"testing"

	"github.com/kmoneil/dateparsa"
)

func FuzzScanString(f *testing.F) {
	seeds := []string{
		"2024-03-15",
		"2024-03-15T10:30:00Z",
		"03/15/2024",
		"1710505800",
		"",
		"not a date",
		"2024-13-45",
		"99/99/9999",

		// Panicked through dateparsa.Parse in detect.trimAtSuffix, which took
		// the index of " at " from a strings.ToLower copy and sliced the input
		// with it. Kept here as well as in the root package: a database column
		// is exactly where a byte sequence like this arrives without anyone
		// having chosen it.
		"deC0000\xcd\xcd\xcd At 0",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		var ft FlexTime
		err := ft.Scan(input)
		if err == nil {
			if !ft.Valid() {
				t.Error("Scan succeeded but Valid() is false")
			}
			v, verr := ft.Value()
			if verr != nil {
				t.Errorf("Value() error after successful Scan: %v", verr)
			}
			if v == nil {
				t.Error("Value() returned nil after successful Scan")
			}
		}
	})
}

func FuzzUnmarshalJSON(f *testing.F) {
	seeds := []string{
		`"2024-03-15T10:30:00Z"`,
		`"03/15/2024"`,
		`1710505800`,
		`1710505800.123`,
		`null`,
		`""`,
		`"garbage"`,
		`true`,
		`{}`,
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		var ft FlexTime
		err := ft.UnmarshalJSON(data)
		if err == nil && ft.Valid() {
			b, merr := ft.MarshalJSON()
			if merr != nil {
				t.Errorf("MarshalJSON error after successful Unmarshal: %v", merr)
			}
			if len(b) == 0 {
				t.Error("MarshalJSON returned empty bytes")
			}
		}
	})
}

// FuzzJSONUnquoteAgreesWithEncodingJSON is the fuzzing half of
// TestUnquoteJSONStringAgreesWithEncodingJSON. Refusal is always allowed, since
// the decoder runs on refusal; acceptance is a claim that encoding/json would
// return the same string, and that is what this checks.
//
// The seeds are the shapes the table found by hand, plus the two the byte scan
// exists for: a body carrying UTF-8, and a body carrying bytes that are not.
func FuzzJSONUnquoteAgreesWithEncodingJSON(f *testing.F) {
	seeds := []string{
		`"2024-03-15T10:30:00Z"`,
		`""`,
		`"Z"`,
		`"a\"b"`,
		`"a"b"`,
		`"x`,
		"\"caf\xc3\xa9\"",
		"\"caf\xff\"",
		"\"tab\there\"",
		"\"\x00\"",
		`"x" `,
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		got, ok := unquoteJSONString(data)
		if !ok {
			return
		}
		var want string
		if err := json.Unmarshal(data, &want); err != nil {
			t.Fatalf("unquoteJSONString(%q) accepted %q, encoding/json refuses it: %v", data, got, err)
		}
		if got != want {
			t.Fatalf("unquoteJSONString(%q) = %q, encoding/json says %q", data, got, want)
		}
	})
}

// FuzzUnmarshalJSONAcrossFormats holds down the one property the package-level
// layout cache in json.go must have: what a value unmarshals to cannot depend on
// what was unmarshalled before it.
//
// The cache is shared by every caller in the binary, so "before it" is not
// something a caller controls. That is the whole reason this target exists
// rather than a table test: the sequence is the input.
//
// Each iteration parses the second value twice, once through a cache primed by
// the first value and once through a cache that was just reset, and requires the
// two to agree on the error, the instant and the guess flag. Resetting is what
// makes the comparison honest; without it the "cold" side would be primed by
// whatever the previous iteration left behind.
func FuzzUnmarshalJSONAcrossFormats(f *testing.F) {
	seeds := [][2]string{
		{`"2024-03-15T10:30:00Z"`, `"03/15/2024"`},
		{`"03/15/2024"`, `"01/02/2024"`},
		{`"25/12/2024"`, `"01/02/2024"`},
		{`"2024-03-15"`, `"2024-03-15 10:30:00"`},
		{`"March 15, 2024"`, `"15 Mar 2024"`},
		{`"MAY70"`, `"MAY10"`},
		{`"10:30:45"`, `"12/25/24"`},
		{`"20-1-00"`, `"10:01:00"`},
		{`1710505800`, `"2024-03-15"`},
		{`"2024-03-15"`, `null`},
		{`""`, `""`},

		// Found by this target in 26s, and it is a defect in the target rather
		// than in the cache: "in0S" is "in 0 s", a relative expression resolved
		// against time.Now() on each call, so the warm and cold parses differ by
		// the microseconds between them. The guard below skips anything that is
		// not absolute. Kept as a seed because the guard is the kind of thing
		// somebody deletes as redundant.
		{`0`, `"in0S"`},

		// Found by this target in 105s and it is not a defect: a Go time string
		// layout accepts the trailing space that detection refuses, and returns
		// the instant detection would have returned. The assertion is
		// one-directional because of this input.
		{`"0000-01-01 00:00:00  0000"`, `"0000-01-01 00:00:00 "`},
	}
	for _, s := range seeds {
		f.Add([]byte(s[0]), []byte(s[1]))
	}

	f.Fuzz(func(t *testing.T, primeWith, applyTo []byte) {
		// A relative expression is resolved against time.Now() on every call, so
		// two parses of one differ by the time between them whatever the cache
		// does. Only an absolute value has an answer this can compare.
		if !jsonValueIsAbsolute(applyTo) {
			return
		}

		jsonParser.Reset()
		var primed FlexTime
		_ = primed.UnmarshalJSON(primeWith)

		var warm FlexTime
		warmErr := warm.UnmarshalJSON(applyTo)

		jsonParser.Reset()
		var cold FlexTime
		coldErr := cold.UnmarshalJSON(applyTo)

		// One direction. A value the cold path parses has to survive the cache
		// unchanged; a value only the warm path parses is the reuse
		// over-acceptance the root package has documented since C10, where a
		// layout that fits accepts bytes detection would refuse and answers with
		// the same instant. "0000-01-01 00:00:00 " after a Go time string is the
		// shape, and it is in the seeds.
		if coldErr == nil && warmErr != nil {
			t.Fatalf("primed with %q, parsing %q: warm refused what cold parsed: %v",
				primeWith, applyTo, warmErr)
		}
		if warmErr != nil || coldErr != nil {
			return
		}
		if !warm.Time().Equal(cold.Time()) {
			t.Errorf("primed with %q, parsing %q: warm = %v, cold = %v",
				primeWith, applyTo, warm.Time(), cold.Time())
		}
		if warm.Ambiguous() != cold.Ambiguous() {
			t.Errorf("primed with %q, parsing %q: warm Ambiguous = %v, cold = %v",
				primeWith, applyTo, warm.Ambiguous(), cold.Ambiguous())
		}
		if warm.Valid() != cold.Valid() {
			t.Errorf("primed with %q, parsing %q: warm Valid = %v, cold = %v",
				primeWith, applyTo, warm.Valid(), cold.Valid())
		}
	})
}

// jsonValueIsAbsolute reports whether applyTo, as UnmarshalJSON would read it,
// parses to something whose answer does not move between two calls. It returns
// true for anything that fails to parse, because a failure is deterministic and
// the target compares those too.
func jsonValueIsAbsolute(data []byte) bool {
	s, ok := unquoteJSONString(data)
	if !ok {
		var decoded string
		if err := json.Unmarshal(data, &decoded); err != nil {
			return true // a number, a null, or malformed: no clock involved
		}
		s = decoded
	}
	r, err := dateparsa.Parse(s)
	if err != nil {
		return true
	}
	return r.Kind == dateparsa.KindAbsolute
}
