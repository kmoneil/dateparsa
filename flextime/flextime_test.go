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

	// Validity is half the value. Equal compared only the instant, so a SQL
	// NULL was equal to a real zero time, which is the comparison a caller
	// scanning a nullable column is most likely to write.
	var null FlexTime
	zero := New(time.Time{})
	if null.Equal(zero) {
		t.Error("an invalid FlexTime is Equal to a valid zero time")
	}
	if zero.Equal(null) {
		t.Error("Equal is not symmetric across validity")
	}
	if !null.Equal(FlexTime{}) {
		t.Error("two invalid FlexTimes should be Equal")
	}
	if !zero.Equal(New(time.Time{})) {
		t.Error("two valid zero times should be Equal")
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
