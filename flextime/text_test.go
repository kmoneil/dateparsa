package flextime

import (
	"testing"
	"time"
)

func TestMarshalText(t *testing.T) {
	ft := New(time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC))
	b, err := ft.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText error: %v", err)
	}
	want := "2024-03-15T10:30:00Z"
	if string(b) != want {
		t.Errorf("MarshalText = %q, want %q", string(b), want)
	}
}

func TestMarshalTextInvalid(t *testing.T) {
	var ft FlexTime
	b, err := ft.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText error: %v", err)
	}
	if string(b) != "" {
		t.Errorf("MarshalText of invalid = %q, want empty", string(b))
	}
}

func TestUnmarshalText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  time.Time
	}{
		{"ISO date", "2024-03-15", time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
		{"RFC3339", "2024-03-15T10:30:00Z", time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)},
		{"US date", "03/15/2024", time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ft FlexTime
			err := ft.UnmarshalText([]byte(tt.input))
			if err != nil {
				t.Fatalf("UnmarshalText(%q) error: %v", tt.input, err)
			}
			if !ft.Valid() {
				t.Error("expected Valid() == true")
			}
			if !ft.Time().Equal(tt.want) {
				t.Errorf("Time() = %v, want %v", ft.Time(), tt.want)
			}
		})
	}
}

func TestUnmarshalTextEmpty(t *testing.T) {
	var ft FlexTime
	err := ft.UnmarshalText([]byte(""))
	if err == nil {
		t.Error("expected error for empty text, got nil")
	}
}
