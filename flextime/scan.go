package flextime

import (
	"database/sql/driver"
	"fmt"
	"math"
	"time"

	"github.com/kmoneil/dateparsa"
)

// Scan implements sql.Scanner.
// Accepts: time.Time, string, []byte, int64 (Unix seconds),
// float64 (Unix seconds with fractional), nil (SQL NULL).
func (ft *FlexTime) Scan(src interface{}) error {
	switch v := src.(type) {
	case nil:
		ft.t = time.Time{}
		ft.valid = false
		return nil

	case time.Time:
		ft.t = v
		ft.valid = true
		return nil

	case string:
		return ft.scanString(v)

	case []byte:
		return ft.scanString(string(v))

	case int64:
		ft.t = time.Unix(v, 0)
		ft.valid = true
		return nil

	case float64:
		sec, frac := math.Modf(v)
		nsec := math.Round(frac * 1e9)
		ft.t = time.Unix(int64(sec), int64(nsec))
		ft.valid = true
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
		return fmt.Errorf("flextime: cannot parse %q: %w", s, err)
	}
	ft.t = result.Time
	ft.valid = true
	return nil
}
