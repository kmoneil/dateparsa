package flextime

import (
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/kmoneil/dateparsa"
	"github.com/kmoneil/dateparsa/internal/epoch"
)

// Scan implements sql.Scanner.
// Accepts: time.Time, string, []byte, int64 (Unix timestamp), float64 (Unix
// seconds with fractional), nil (SQL NULL).
//
// An int64 takes its precision from how many decimal digits it is written with,
// so 13 digits are milliseconds and 19 are nanoseconds, the same reading a string
// of those digits gets. It used to be seconds whatever the magnitude, which made
// 1710500000000 the year 56173 here and 2024-03-15 through the string arm two
// cases above.
//
// A float64 is seconds, fraction included, because that is all a 53-bit mantissa
// can carry and because it is what the column type means. NaN and the infinities
// are refused rather than converted; the conversion was implementation-defined
// and gave different instants on arm64 and amd64.
func (ft *FlexTime) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		ft.set(time.Time{}, false, false)
		return nil

	case time.Time:
		ft.set(v, true, false)
		return nil

	case string:
		return ft.scanString(v)

	case []byte:
		return ft.scanString(string(v))

	case int64:
		t, ok := epoch.FromInt(v)
		if !ok {
			return fmt.Errorf("flextime: %d is not a timestamp this package accepts", v)
		}
		ft.set(t, true, false)
		return nil

	case float64:
		t, ok := epoch.FromSeconds(v)
		if !ok {
			return fmt.Errorf("flextime: %v is not a timestamp this package accepts", v)
		}
		ft.set(t, true, false)
		return nil

	default:
		return fmt.Errorf("flextime: unsupported Scan type %T", src)
	}
}

// Value implements driver.Valuer.
// Returns the time as a time.Time, which most drivers handle natively.
// Returns nil if the FlexTime is not valid (represents SQL NULL).
func (ft FlexTime) Value() (driver.Value, error) {
	if !ft.valid {
		return nil, nil
	}
	return ft.t, nil
}

func (ft *FlexTime) scanString(s string) error {
	if s == "" {
		return fmt.Errorf("flextime: cannot scan empty string")
	}
	result, err := dateparsa.Parse(s)
	if err != nil {
		// Name the boundary and nothing else. The wrapped error already quotes
		// the input, bounded, and repeating it here doubled the length of an
		// error message over an input that is assumed hostile.
		return fmt.Errorf("flextime: %w", err)
	}
	ft.set(result.Time, true, result.Ambiguous)
	return nil
}
