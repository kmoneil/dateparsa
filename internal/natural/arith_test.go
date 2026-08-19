package natural

import (
	"testing"
	"time"
)

// daysInMonthRef is this test's own copy of the calendar. It must not call
// anything in the package under test and must not call time.AddDate: the two
// tests that covered "1 month ago" before this file existed asserted against
// base.AddDate(0, -1, 0), which is the operation the code performs, so the
// assertion was AddDate == AddDate and stayed green over a wrong answer for as
// long as the library existed.
func daysInMonthRef(m time.Month, y int) int {
	switch m {
	case time.April, time.June, time.September, time.November:
		return 30
	case time.February:
		if y%4 == 0 && (y%100 != 0 || y%400 == 0) {
			return 29
		}
		return 28
	default:
		return 31
	}
}

// TestAddUnit_MonthAndYearClampToTheMonthEnd walks every day of every month of a
// leap year and a common year against a wide range of shifts, and asserts the
// day clamps to the last day of the target month rather than overflowing into
// the next one.
func TestAddUnit_MonthAndYearClampToTheMonthEnd(t *testing.T) {
	loc := time.UTC
	for _, y := range []int{2024, 2025} { // leap, common
		for m := 1; m <= 12; m++ {
			for d := 1; d <= daysInMonthRef(time.Month(m), y); d++ {
				base := time.Date(y, time.Month(m), d, 10, 30, 45, 123456789, loc)

				for n := -24; n <= 24; n++ {
					// Months.
					got := addUnit(base, n, UnitMonth)
					assertShift(t, base, got, n, "month")

					// Years, which are twelve months and must agree.
					gotY := addUnit(base, n, UnitYear)
					assertShift(t, base, gotY, n*12, "year")
				}
			}
		}
	}
}

func assertShift(t *testing.T, base, got time.Time, months int, unit string) {
	t.Helper()

	total := int(base.Month()) - 1 + months
	ty := base.Year() + floorDivRef(total, 12)
	tm := time.Month(floorModRef(total, 12) + 1)

	wantDay := base.Day()
	if last := daysInMonthRef(tm, ty); wantDay > last {
		wantDay = last
	}
	want := time.Date(ty, tm, wantDay,
		base.Hour(), base.Minute(), base.Second(), base.Nanosecond(), base.Location())

	if !got.Equal(want) {
		t.Fatalf("addUnit(%s, %d, %s) = %s, want %s",
			base.Format(time.RFC3339Nano), months, unit,
			got.Format(time.RFC3339Nano), want.Format(time.RFC3339Nano))
	}
}

func floorDivRef(a, b int) int {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

func floorModRef(a, b int) int {
	r := a % b
	if r != 0 && (r < 0) != (b < 0) {
		r += b
	}
	return r
}

// TestAddUnit_MonthEndSymptoms pins the exact inputs from C25 as literals, so
// the regression lives in the test source and not only in a property.
func TestAddUnit_MonthEndSymptoms(t *testing.T) {
	tests := []struct {
		name string
		base time.Time
		n    int
		unit Unit
		want time.Time
	}{
		{
			"one month before the 31st of March",
			time.Date(2024, 3, 31, 12, 0, 0, 0, time.UTC), -1, UnitMonth,
			time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC),
		},
		{
			"one month after the 31st of January",
			time.Date(2024, 1, 31, 12, 0, 0, 0, time.UTC), 1, UnitMonth,
			time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC),
		},
		{
			"one month after the 31st of March",
			time.Date(2024, 3, 31, 12, 0, 0, 0, time.UTC), 1, UnitMonth,
			time.Date(2024, 4, 30, 12, 0, 0, 0, time.UTC),
		},
		{
			"one month after the 31st of January in a common year",
			time.Date(2025, 1, 31, 12, 0, 0, 0, time.UTC), 1, UnitMonth,
			time.Date(2025, 2, 28, 12, 0, 0, 0, time.UTC),
		},
		{
			"one year after the 29th of February",
			time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC), 1, UnitYear,
			time.Date(2025, 2, 28, 12, 0, 0, 0, time.UTC),
		},
		{
			"one year before the 29th of February",
			time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC), -1, UnitYear,
			time.Date(2023, 2, 28, 12, 0, 0, 0, time.UTC),
		},
		{
			"four years after the 29th of February lands on a leap day",
			time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC), 4, UnitYear,
			time.Date(2028, 2, 29, 12, 0, 0, 0, time.UTC),
		},
		{
			"two months before the 31st of March needs no clamp",
			time.Date(2024, 3, 31, 12, 0, 0, 0, time.UTC), -2, UnitMonth,
			time.Date(2024, 1, 31, 12, 0, 0, 0, time.UTC),
		},
		{
			"a shift across a year boundary going back",
			time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC), -1, UnitMonth,
			time.Date(2023, 12, 15, 12, 0, 0, 0, time.UTC),
		},
		{
			"a shift of a whole year in months",
			time.Date(2024, 3, 31, 12, 0, 0, 0, time.UTC), -12, UnitMonth,
			time.Date(2023, 3, 31, 12, 0, 0, 0, time.UTC),
		},
		{
			"1900 is not a leap year",
			time.Date(1900, 1, 31, 12, 0, 0, 0, time.UTC), 1, UnitMonth,
			time.Date(1900, 2, 28, 12, 0, 0, 0, time.UTC),
		},
		{
			"2000 is a leap year",
			time.Date(2000, 1, 31, 12, 0, 0, 0, time.UTC), 1, UnitMonth,
			time.Date(2000, 2, 29, 12, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := addUnit(tt.base, tt.n, tt.unit)
			if !got.Equal(tt.want) {
				t.Errorf("addUnit(%s, %d) = %s, want %s",
					tt.base.Format(time.RFC3339), tt.n,
					got.Format(time.RFC3339), tt.want.Format(time.RFC3339))
			}
		})
	}
}

// TestAddUnit_NonCalendarUnitsAreUnchanged guards the arms the fix does not
// touch. A duration-based unit must stay a duration: adding a day across a
// month end is not a clamping question.
func TestAddUnit_NonCalendarUnitsAreUnchanged(t *testing.T) {
	base := time.Date(2024, 1, 31, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		n    int
		unit Unit
		want time.Time
	}{
		{1, UnitDay, time.Date(2024, 2, 1, 12, 0, 0, 0, time.UTC)},
		{1, UnitWeek, time.Date(2024, 2, 7, 12, 0, 0, 0, time.UTC)},
		{1, UnitHour, time.Date(2024, 1, 31, 13, 0, 0, 0, time.UTC)},
		{-90, UnitMinute, time.Date(2024, 1, 31, 10, 30, 0, 0, time.UTC)},
		{30, UnitSecond, time.Date(2024, 1, 31, 12, 0, 30, 0, time.UTC)},
	}
	for _, tt := range tests {
		if got := addUnit(base, tt.n, tt.unit); !got.Equal(tt.want) {
			t.Errorf("addUnit(base, %d, %v) = %s, want %s",
				tt.n, tt.unit, got.Format(time.RFC3339), tt.want.Format(time.RFC3339))
		}
	}
}
