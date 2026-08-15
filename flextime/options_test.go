package flextime

import (
	"sync"
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

// TestScannerConfiguredCostsNoMorePerRow is the gate on the option list being
// built once. A Scanner that rebuilds it inside Scan allocates the slice and a
// closure per option on every row, so a configured Scanner cost four
// allocations a row more than a default one for configuration that was fixed
// when NewScanner returned.
//
// It asserts equality against the default Scanner rather than a fixed count,
// because the count that survives belongs to the root package and moves when
// that package's allocation behaviour changes. What this package promises is
// that configuring a Scanner is free per row, and that is the equality.
func TestScannerConfiguredCostsNoMorePerRow(t *testing.T) {
	loc := time.FixedZone("CET", 3600)
	def := NewScanner()
	cfg := NewScanner(WithPreferDayFirst(true), WithTimezone(loc))

	var ft FlexTime
	perRow := func(s *Scanner) float64 {
		return testing.AllocsPerRun(100, func() {
			if err := s.Scan(&ft, "2024-03-15"); err != nil {
				t.Fatalf("Scan: %v", err)
			}
		})
	}

	if got, want := perRow(cfg), perRow(def); got != want {
		t.Errorf("configured Scanner allocates %.0f times per row, default allocates %.0f; "+
			"configuration is fixed at NewScanner and may not cost anything per row", got, want)
	}
}

// TestScannerColumnFormatChangesMidFile is the test the cache makes necessary.
// A Scanner detects a column's format once and reuses it, so a column that
// changes format partway through is where reuse could return a wrong instant
// instead of an error. _plans/two-pass-column.md designs for this shape and
// nothing in this package covered it.
//
// The rows alternate deliberately rather than switching once: a layout that
// stuck would be caught on the row after it, not fifty rows later.
func TestScannerColumnFormatChangesMidFile(t *testing.T) {
	want := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	wantTime := time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)

	rows := []struct {
		in   string
		want time.Time
	}{
		{"2024-03-15", want},
		{"2024-03-15", want},
		{"20240315", want},
		{"2024-03-15 10:30:00", wantTime},
		{"2024-03-15", want},
		{"March 15, 2024", want},
		{"2024-03-15T10:30:00Z", wantTime},
		{"20240315", want},
		{"2024-03-15", want},
	}

	s := NewScanner()
	for i, row := range rows {
		var ft FlexTime
		if err := s.Scan(&ft, row.in); err != nil {
			t.Fatalf("row %d, Scan(%q): %v", i, row.in, err)
		}
		if !ft.Time().Equal(row.want) {
			t.Errorf("row %d, Scan(%q) = %v, want %v", i, row.in, ft.Time(), row.want)
		}
	}
}

// TestScannerConcurrentUse holds the promise on the Scanner doc comment. A
// Scanner caches a layout now, and the type was safe for concurrent use before
// it did.
func TestScannerConcurrentUse(t *testing.T) {
	rows := []struct {
		in   string
		want time.Time
	}{
		{"2024-03-15", time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
		{"20240315", time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
		{"2024-03-15 10:30:00", time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)},
		{"2024-03-15T10:30:00Z", time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)},
	}

	s := NewScanner()
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			row := rows[g%len(rows)]
			for range 400 {
				var ft FlexTime
				if err := s.Scan(&ft, row.in); err != nil {
					t.Errorf("goroutine %d: Scan(%q): %v", g, row.in, err)
					return
				}
				if !ft.Time().Equal(row.want) {
					t.Errorf("goroutine %d: Scan(%q) = %v, want %v", g, row.in, ft.Time(), row.want)
					return
				}
			}
		}(g)
	}
	wg.Wait()
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
