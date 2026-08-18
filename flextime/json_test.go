package flextime

import (
	"encoding/json"
	"testing"
	"time"
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

// TestUnmarshalJSONStringAllocates gates the two allocations UnmarshalJSON makes
// on a plain ASCII timestamp: the string the body is copied into, and the Layout
// dateparsa.Parse returns. It made four before the in-place unquote, because
// encoding/json allocated a decode state and unquoted into a string of its own
// for a body that has nothing to unquote.
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
	if got != 2 {
		t.Errorf("UnmarshalJSON allocated %v times, want 2", got)
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
