package flextime

import (
	"encoding/json"
	"math"
	"strconv"
	"testing"
	"time"
)

// TestNumericAgreesWithString is C20's assertion. The same timestamp arriving as
// a string, as a driver int64, and as a JSON number has to name one instant.
//
// The value that opened the card is the second row: 1710500000000 is a
// millisecond epoch, which is what Date.now() and System.currentTimeMillis()
// produce, and it was 2024-03-15 through the string arm and year 56173 through
// the other two.
func TestNumericAgreesWithString(t *testing.T) {
	values := []int64{
		1710500000,          // seconds
		1710500000000,       // milliseconds
		1710500000000000,    // microseconds
		1710500000000000000, // nanoseconds
		-1710500000,
		-1710500000000,
		0,
		86400,
	}

	for _, v := range values {
		s := strconv.FormatInt(v, 10)

		var viaString FlexTime
		strErr := viaString.Scan(s)

		var viaInt FlexTime
		intErr := viaInt.Scan(v)

		var viaJSON FlexTime
		jsonErr := json.Unmarshal([]byte(s), &viaJSON)

		// A string of fewer than ten digits is not a timestamp -- "86400" could
		// be anything and "2024" is a year -- so the string arm refuses those
		// while the typed arms accept them. Documented on epoch.FromInt, and the
		// only input class where the three legitimately differ.
		if len(s) < 10 || (v < 0 && len(s) < 11) {
			if intErr != nil || jsonErr != nil {
				t.Errorf("%d: typed arms should accept a short value: int=%v json=%v",
					v, intErr, jsonErr)
			}
			continue
		}

		if strErr != nil || intErr != nil || jsonErr != nil {
			t.Errorf("%d: one arm refused: string=%v int=%v json=%v",
				v, strErr, intErr, jsonErr)
			continue
		}
		if !viaInt.Time().Equal(viaString.Time()) {
			t.Errorf("%d: Scan(int64) = %v, Scan(string) = %v",
				v, viaInt.Time().UTC(), viaString.Time().UTC())
		}
		if !viaJSON.Time().Equal(viaString.Time()) {
			t.Errorf("%d: UnmarshalJSON(number) = %v, Scan(string) = %v",
				v, viaJSON.Time().UTC(), viaString.Time().UTC())
		}
	}
}

// TestNumericRefusesWhatIsNotATimestamp covers the out-of-range half. Every one
// of these was accepted before C20, with a nil error and a year no SQL timestamp
// column can hold, so the failure landed at insert time instead of at parse time.
func TestNumericRefusesWhatIsNotATimestamp(t *testing.T) {
	t.Run("json", func(t *testing.T) {
		for _, body := range []string{
			"1e15", "1e18", "1e300", "-1e300",
			"1.7976931348623157e308",
			"100000000000000", // 15 digits, no precision claims it
			"17105000000000",  // 14 digits
			// MinInt64 only. MaxInt64 is 19 digits and a real nanosecond
			// timestamp -- 2262-04-11 -- and the string arm accepts it too, so
			// it belongs in the agreement test above and not here. MinInt64 has
			// one more digit than the positive side holds, so parseInt refuses
			// the string and FromInt refuses the value to match.
			"-9223372036854775808",
		} {
			var ft FlexTime
			if err := json.Unmarshal([]byte(body), &ft); err == nil {
				t.Errorf("UnmarshalJSON(%s) accepted %v, want an error",
					body, ft.Time().UTC())
			}
		}
	})

	t.Run("int64", func(t *testing.T) {
		for _, v := range []int64{
			math.MinInt64,      // one digit past what the positive side holds
			100000000000000,    // 15 digits, no precision claims it
			17105000000000,     // 14 digits
			171050000000000000, // 18 digits
		} {
			var ft FlexTime
			if err := ft.Scan(v); err == nil {
				t.Errorf("Scan(int64 %d) accepted %v, want an error", v, ft.Time().UTC())
			}
		}
	})

	t.Run("float64", func(t *testing.T) {
		for _, f := range []float64{
			math.NaN(), math.Inf(1), math.Inf(-1),
			math.MaxFloat64, -math.MaxFloat64,
			1e15, -1e15,
		} {
			var ft FlexTime
			if err := ft.Scan(f); err == nil {
				t.Errorf("Scan(float64 %v) accepted %v, want an error", f, ft.Time().UTC())
			}
		}
	})
}

// TestJSONFractionalIsSeconds pins the split UnmarshalJSON makes between an
// integer and a fractional number. A fraction can only mean seconds, because no
// millisecond count is written with a decimal point.
func TestJSONFractionalIsSeconds(t *testing.T) {
	cases := []struct {
		body string
		want time.Time
	}{
		{"1710500000.5", time.Unix(1710500000, 500000000).UTC()},
		{"1710500000.0", time.Unix(1710500000, 0).UTC()},
		{"-1710500000.5", time.Unix(-1710500000, -500000000).UTC()},
		{"1.7105e9", time.Unix(1710500000, 0).UTC()},
	}
	for _, c := range cases {
		var ft FlexTime
		if err := json.Unmarshal([]byte(c.body), &ft); err != nil {
			t.Errorf("UnmarshalJSON(%s): %v", c.body, err)
			continue
		}
		if !ft.Time().Equal(c.want) {
			t.Errorf("UnmarshalJSON(%s) = %v, want %v", c.body, ft.Time().UTC(), c.want)
		}
	}
}

// TestScannerNumericMatchesFlexTime holds the two Scan implementations together.
// Scanner.Scan kept its own copy of the int64 arm, so it stayed on the old
// seconds-whatever-the-magnitude reading while FlexTime.Scan moved.
func TestScannerNumericMatchesFlexTime(t *testing.T) {
	sc := NewScanner()
	for _, v := range []int64{1710500000, 1710500000000, 1710500000000000, 0, math.MaxInt64} {
		var viaScanner, viaValue FlexTime
		errA := sc.Scan(&viaScanner, v)
		errB := viaValue.Scan(v)

		if (errA == nil) != (errB == nil) {
			t.Errorf("%d: Scanner.Scan err=%v, FlexTime.Scan err=%v", v, errA, errB)
			continue
		}
		if errA == nil && !viaScanner.Time().Equal(viaValue.Time()) {
			t.Errorf("%d: Scanner.Scan = %v, FlexTime.Scan = %v",
				v, viaScanner.Time().UTC(), viaValue.Time().UTC())
		}
	}
}

// TestScanRefusalLeavesTheValueUnset checks that a refused numeric value does not
// leave a stale time behind on a *FlexTime that sql.Rows.Scan reuses across rows.
func TestScanRefusalLeavesTheValueUnset(t *testing.T) {
	var ft FlexTime
	if err := ft.Scan(int64(1710500000)); err != nil {
		t.Fatalf("first scan: %v", err)
	}
	first := ft.Time()

	if err := ft.Scan(math.MaxInt64); err == nil {
		t.Fatal("second scan should have failed")
	}
	// The contract this pins: a failed Scan reports the failure and the caller is
	// expected to stop, so the previous value is still there. It is pinned rather
	// than changed because database/sql abandons the row on a Scan error, and
	// zeroing here would differ from every other Scan arm without any caller
	// being able to see the difference.
	if !ft.Time().Equal(first) {
		t.Errorf("a failed Scan modified the value: was %v, now %v", first, ft.Time())
	}
}
