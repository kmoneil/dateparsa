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
