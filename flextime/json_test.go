package flextime

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/kmoneil/dateparsa"
)

func TestMarshalJSON(t *testing.T) {
	tests := []struct {
		name string
		ft   FlexTime
		want string
	}{
		{
			name: "valid time",
			ft:   New(time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)),
			want: `"2024-03-15T10:30:00Z"`,
		},
		{
			name: "with nanos",
			ft:   New(time.Date(2024, 3, 15, 10, 30, 0, 123456789, time.UTC)),
			want: `"2024-03-15T10:30:00.123456789Z"`,
		},
		{
			name: "null",
			ft:   FlexTime{},
			want: "null",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := json.Marshal(tt.ft)
			if err != nil {
				t.Fatalf("MarshalJSON error: %v", err)
			}
			if string(b) != tt.want {
				t.Errorf("MarshalJSON = %s, want %s", string(b), tt.want)
			}
		})
	}
}

func TestUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    time.Time
		valid   bool
		wantErr bool
	}{
		{
			name:  "RFC3339 string",
			input: `"2024-03-15T10:30:00Z"`,
			want:  time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC),
			valid: true,
		},
		{
			name:  "ISO date string",
			input: `"2024-03-15"`,
			want:  time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
			valid: true,
		},
		{
			name:  "US date string",
			input: `"03/15/2024"`,
			want:  time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC),
			valid: true,
		},
		{
			name:  "Unix timestamp number",
			input: "1710505800",
			want:  time.Unix(1710505800, 0),
			valid: true,
		},
		{
			name:  "Unix timestamp float",
			input: "1710505800.5",
			want:  time.Unix(1710505800, 500000000),
			valid: true,
		},
		{
			name:  "null",
			input: "null",
			valid: false,
		},
		{
			name:    "invalid string",
			input:   `"not a date"`,
			wantErr: true,
		},
		{
			name:    "invalid JSON",
			input:   "{bad}",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ft FlexTime
			err := json.Unmarshal([]byte(tt.input), &ft)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalJSON error: %v", err)
			}
			if ft.Valid() != tt.valid {
				t.Errorf("Valid() = %v, want %v", ft.Valid(), tt.valid)
			}
			if tt.valid && !ft.Time().Equal(tt.want) {
				t.Errorf("Time() = %v, want %v", ft.Time(), tt.want)
			}
		})
	}
}

func TestJSONRoundTrip(t *testing.T) {
	type record struct {
		CreatedAt FlexTime `json:"created_at"`
		UpdatedAt FlexTime `json:"updated_at"`
		DeletedAt FlexTime `json:"deleted_at"`
	}

	original := record{
		CreatedAt: New(time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)),
		UpdatedAt: New(time.Date(2024, 3, 16, 12, 0, 0, 0, time.UTC)),
		DeletedAt: FlexTime{},
	}

	b, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded record
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if !decoded.CreatedAt.Time().Equal(original.CreatedAt.Time()) {
		t.Errorf("CreatedAt mismatch: got %v, want %v",
			decoded.CreatedAt.Time(), original.CreatedAt.Time())
	}
	if !decoded.UpdatedAt.Time().Equal(original.UpdatedAt.Time()) {
		t.Errorf("UpdatedAt mismatch")
	}
	if decoded.DeletedAt.Valid() {
		t.Error("DeletedAt should be invalid (null)")
	}
}

// TestUnmarshalJSONStringAllocates gates the one allocation UnmarshalJSON makes
// on a plain ASCII timestamp in a steady stream of one format: the string the
// quoted body is copied into.
//
// It made four originally. Two went when the body stopped being decoded through
// encoding/json, which has nothing to do for a body with no escape in it, and
// the third was the Layout, which is now detected once and held in jsonParser
// rather than built and discarded per value.
//
// AllocsPerRun's warmup call is what primes that cache, so this measures the
// second value onward, which is the case the cache exists for. The first value
// after a format change costs the Layout as well.
//
// The number is exact rather than an upper bound, for the reason
// TestLayoutParseZeroAlloc is: an escape analysis change three packages away is
// how this regresses, and a bound hides that.
func TestUnmarshalJSONStringAllocates(t *testing.T) {
	data := []byte(`"2024-03-15T10:30:00Z"`)
	var ft FlexTime
	got := testing.AllocsPerRun(1000, func() {
		if err := ft.UnmarshalJSON(data); err != nil {
			t.Fatalf("UnmarshalJSON: %v", err)
		}
	})
	if got != 1 {
		t.Errorf("UnmarshalJSON allocated %v times, want 1", got)
	}
}

// TestUnquoteJSONStringAgreesWithEncodingJSON is the one-directional rule the
// in-place unquote lives under: it may refuse anything at all, because the
// fallback decodes it, but whatever it accepts has to be exactly what
// encoding/json would have produced. A fast path that accepts a value the
// decoder rejects, or that returns different bytes, is a wrong answer rather
// than a slow one.
func TestUnquoteJSONStringAgreesWithEncodingJSON(t *testing.T) {
	inputs := []string{
		`"2024-03-15T10:30:00Z"`,
		`""`,
		`"03/15/2024"`,
		`"a b c"`,
		`"Z"`,
		`"2024-03-15T10:30:00Z"`,
		`"a\"b"`,
		`"a"b"`,
		`"x`,
		`"`,
		``,
		`"x" `,
		` "x"`,
		`"café"`,
		"\"caf\xc3\xa9\"",
		"\"caf\xff\"",
		"\"tab\there\"",
		"\"nl\nhere\"",
		"\"\x00\"",
		`"15 février 2024"`,
		`null`,
		`1710505800`,
	}

	for _, in := range inputs {
		got, ok := unquoteJSONString([]byte(in))
		var want string
		err := json.Unmarshal([]byte(in), &want)
		switch {
		case ok && err != nil:
			t.Errorf("unquoteJSONString(%q) accepted %q, encoding/json refuses it: %v", in, got, err)
		case ok && got != want:
			t.Errorf("unquoteJSONString(%q) = %q, encoding/json says %q", in, got, want)
		}
	}
}

// TestUnmarshalJSONMatchesTheDecodedPath runs both routes end to end and
// requires the same outcome. The table above compares the two decoders; this
// compares the two parses, because the value a caller sees is what matters and
// the fast path changes which bytes reach dateparsa.Parse.
func TestUnmarshalJSONMatchesTheDecodedPath(t *testing.T) {
	inputs := []string{
		`"2024-03-15T10:30:00Z"`,
		`"2024-03-15T10:30:00Z"`,
		`"03/15/2024"`,
		`""`,
		`"a"b"`,
		`"x" `,
		"\"caf\xff\"",
		"\"2024-03-15\xff\"",
		`"not a date"`,
		`"15 février 2024"`,
	}

	for _, in := range inputs {
		var fast FlexTime
		fastErr := fast.UnmarshalJSON([]byte(in))

		var slow FlexTime
		var decoded string
		slowErr := json.Unmarshal([]byte(in), &decoded)
		if slowErr == nil {
			slowErr = slow.Scan(decoded)
		}

		if (fastErr == nil) != (slowErr == nil) {
			t.Errorf("%q: UnmarshalJSON err = %v, decode-then-Scan err = %v", in, fastErr, slowErr)
			continue
		}
		if fastErr != nil {
			continue
		}
		if !fast.Time().Equal(slow.Time()) {
			t.Errorf("%q: UnmarshalJSON = %v, decode-then-Scan = %v", in, fast.Time(), slow.Time())
		}
	}
}

// TestUnmarshalJSONAcrossFormats runs values whose formats alternate through the
// one cache UnmarshalJSON now keeps, and requires each of them to come back as
// dateparsa.Parse reads it on its own.
//
// The cache is a package-level Parser, so this is not a property of one caller's
// sequence: any value in the process can be the one that primed it. The order
// below deliberately alternates rather than grouping, so every value but the
// first is parsed against a layout detected from a different format.
func TestUnmarshalJSONAcrossFormats(t *testing.T) {
	values := []string{
		`"2024-03-15T10:30:00Z"`,
		`"03/15/2024"`,
		`"2024-03-15"`,
		`"March 15, 2024"`,
		`"2024-03-15 10:30:00"`,
		`"15 Mar 2024"`,
		`"2024-03-15T10:30:00Z"`,
		`"20240315"`,
		`"2024-03-15"`,
		`"10:30:45"`,
		`"2024-03-15T10:30:00Z"`,
	}

	for i := 0; i < 3; i++ {
		for _, v := range values {
			var ft FlexTime
			if err := ft.UnmarshalJSON([]byte(v)); err != nil {
				t.Fatalf("UnmarshalJSON(%s): %v", v, err)
			}

			body := v[1 : len(v)-1]
			want, err := dateparsa.Parse(body)
			if err != nil {
				t.Fatalf("dateparsa.Parse(%q): %v", body, err)
			}
			if !ft.Time().Equal(want.Time) {
				t.Errorf("round %d, %s: UnmarshalJSON = %v, dateparsa.Parse = %v",
					i, v, ft.Time(), want.Time)
			}
			if ft.Ambiguous() != want.Ambiguous {
				t.Errorf("round %d, %s: Ambiguous = %v, dateparsa.Parse says %v",
					i, v, ft.Ambiguous(), want.Ambiguous)
			}
		}
	}
}

// TestUnmarshalJSONAmbiguityThroughTheCache pins the half of the cache that a
// wrong answer would hide. "01/02/2024" is a guess whichever way it is read, and
// a layout detected from an unambiguous value of the same shape must not make it
// look decided. Parser.Parse handles this by refusing to reuse an
// ambiguity-prone layout at all; this asserts the outcome rather than the
// mechanism, because the mechanism is in another package.
func TestUnmarshalJSONAmbiguityThroughTheCache(t *testing.T) {
	// Prime with a value of the same shape whose day cannot be a month.
	var primed FlexTime
	if err := primed.UnmarshalJSON([]byte(`"03/15/2024"`)); err != nil {
		t.Fatalf("priming UnmarshalJSON: %v", err)
	}
	if primed.Ambiguous() {
		t.Errorf(`"03/15/2024" reported Ambiguous, 15 cannot be a month`)
	}

	var ft FlexTime
	if err := ft.UnmarshalJSON([]byte(`"01/02/2024"`)); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	want, err := dateparsa.Parse("01/02/2024")
	if err != nil {
		t.Fatalf("dateparsa.Parse: %v", err)
	}
	if !ft.Ambiguous() {
		t.Error(`"01/02/2024" through a primed cache reported Ambiguous false, want true`)
	}
	if !ft.Time().Equal(want.Time) {
		t.Errorf("UnmarshalJSON = %v, dateparsa.Parse = %v", ft.Time(), want.Time)
	}
}

// TestUnmarshalJSONConcurrentFormats is the shape the shared cache is actually
// exposed to: unrelated goroutines unmarshalling different formats through one
// package-level Parser. Each goroutine checks its own value, so a layout that
// leaked an answer across the cache fails here rather than in production. Run
// under -race, which the suite is.
func TestUnmarshalJSONConcurrentFormats(t *testing.T) {
	cases := []struct {
		json string
		want time.Time
	}{
		{`"2024-03-15T10:30:00Z"`, time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)},
		{`"03/15/2024"`, time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
		{`"2024-03-15"`, time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
		{`"March 15, 2024"`, time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
		{`"2024-03-15 10:30:00"`, time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)},
		{`"20240315"`, time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
	}

	var wg sync.WaitGroup
	for _, tc := range cases {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				var ft FlexTime
				if err := ft.UnmarshalJSON([]byte(tc.json)); err != nil {
					t.Errorf("UnmarshalJSON(%s): %v", tc.json, err)
					return
				}
				if !ft.Time().Equal(tc.want) {
					t.Errorf("UnmarshalJSON(%s) = %v, want %v", tc.json, ft.Time(), tc.want)
					return
				}
			}
		}()
	}
	wg.Wait()
}
