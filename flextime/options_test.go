package flextime

import (
	"testing"
	"time"
)

func TestNewWithOptions(t *testing.T) {
	now := time.Now()
	ft := NewWithOptions(now, WithTimezone(time.UTC))
	if !ft.Valid() {
		t.Error("NewWithOptions should produce valid FlexTime")
	}
	if !ft.Time().Equal(now) {
		t.Error("Time() should return the original time")
	}
}

func TestScannerScan(t *testing.T) {
	scanner := NewScanner(WithPreferDayFirst(true))
	var ft FlexTime
	// 01/02/2024: with day-first, this is Feb 1 not Jan 2
	err := scanner.Scan(&ft, "01/02/2024")
	if err != nil {
		t.Fatalf("Scanner.Scan error: %v", err)
	}
	if !ft.Valid() {
		t.Error("expected valid")
	}
	want := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	if !ft.Time().Equal(want) {
		t.Errorf("Time() = %v, want %v (day-first)", ft.Time(), want)
	}
}

func TestScannerScanNil(t *testing.T) {
	scanner := NewScanner()
	var ft FlexTime
	err := scanner.Scan(&ft, nil)
	if err != nil {
		t.Fatalf("Scanner.Scan(nil) error: %v", err)
	}
	if ft.Valid() {
		t.Error("expected invalid for NULL")
	}
}

func TestScannerWithTimezone(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	scanner := NewScanner(WithTimezone(loc))
	var ft FlexTime
	err := scanner.Scan(&ft, "2024-03-15 10:30:00")
	if err != nil {
		t.Fatalf("Scanner.Scan error: %v", err)
	}
	if ft.Time().Location().String() != loc.String() {
		t.Errorf("Location = %v, want %v", ft.Time().Location(), loc)
	}
}

func TestWithJSONFormat(t *testing.T) {
	ft := NewWithOptions(
		time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC),
		WithJSONFormat(time.RFC822),
	)
	b, err := ft.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON error: %v", err)
	}
	want := `"15 Mar 24 10:30 UTC"`
	if string(b) != want {
		t.Errorf("MarshalJSON = %s, want %s", string(b), want)
	}
}
