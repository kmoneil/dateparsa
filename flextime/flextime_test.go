package flextime

import (
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	now := time.Now()
	ft := New(now)
	if !ft.Valid() {
		t.Error("New() should produce valid FlexTime")
	}
	if !ft.Time().Equal(now) {
		t.Error("Time() should return the original time")
	}
}

func TestNow(t *testing.T) {
	before := time.Now()
	ft := Now()
	after := time.Now()
	if !ft.Valid() {
		t.Error("Now() should produce valid FlexTime")
	}
	if ft.Time().Before(before) || ft.Time().After(after) {
		t.Error("Now() time should be between before and after")
	}
}

func TestZeroValue(t *testing.T) {
	var ft FlexTime
	if ft.Valid() {
		t.Error("zero value should not be valid")
	}
	if !ft.IsZero() {
		t.Error("zero value should be zero time")
	}
	if ft.String() != "<nil>" {
		t.Errorf("String() = %q, want %q", ft.String(), "<nil>")
	}
}

func TestEqual(t *testing.T) {
	t1 := time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)
	t2 := time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)
	t3 := time.Date(2024, 3, 16, 10, 30, 0, 0, time.UTC)

	if !New(t1).Equal(New(t2)) {
		t.Error("equal times should be Equal")
	}
	if New(t1).Equal(New(t3)) {
		t.Error("different times should not be Equal")
	}
}

func TestString(t *testing.T) {
	ft := New(time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC))
	got := ft.String()
	want := "2024-03-15T10:30:00Z"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
