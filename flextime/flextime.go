// Package flextime provides a FlexTime type that wraps time.Time with
// automatic format detection. It implements sql.Scanner, driver.Valuer,
// json.Marshaler, json.Unmarshaler, encoding.TextMarshaler, and
// encoding.TextUnmarshaler.
//
// FlexTime lives in a subpackage to avoid pulling database/sql into the
// dependency tree for users who do not need it.
//
// Zero value is valid Go and represents an unset time (SQL NULL equivalent).
package flextime

import (
	"fmt"
	"time"

	"github.com/kmoneil/dateparsa"
)

// FlexTime wraps a time.Time with automatic format detection.
// Zero value is valid and represents the zero time.Time.
type FlexTime struct {
	t     time.Time
	valid bool
	opts  *options // nil for default behavior; set via NewWithOptions
}

// New creates a FlexTime from an existing time.Time.
func New(t time.Time) FlexTime {
	return FlexTime{t: t, valid: true}
}

// Now returns a FlexTime set to the current time.
func Now() FlexTime {
	return FlexTime{t: time.Now(), valid: true}
}

// Time returns the underlying time.Time.
// Returns the zero time if FlexTime has not been set.
func (ft FlexTime) Time() time.Time {
	return ft.t
}

// Valid reports whether the FlexTime holds a successfully parsed time.
// A zero-value FlexTime or one that scanned a NULL is not valid.
func (ft FlexTime) Valid() bool {
	return ft.valid
}

// IsZero reports whether the underlying time is the zero value.
func (ft FlexTime) IsZero() bool {
	return ft.t.IsZero()
}

// Equal reports whether ft and other represent the same time instant.
func (ft FlexTime) Equal(other FlexTime) bool {
	return ft.t.Equal(other.t)
}

// String returns the time formatted as RFC3339Nano.
// Returns "<nil>" if not valid.
func (ft FlexTime) String() string {
	if !ft.valid {
		return "<nil>"
	}
	return ft.t.Format(time.RFC3339Nano)
}

// MarshalText implements encoding.TextMarshaler.
// Encodes as RFC3339Nano. Returns empty bytes if not valid.
func (ft FlexTime) MarshalText() ([]byte, error) {
	if !ft.valid {
		return []byte{}, nil
	}
	return []byte(ft.t.Format(time.RFC3339Nano)), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
// Parses using dateparsa auto-detection.
func (ft *FlexTime) UnmarshalText(data []byte) error {
	s := string(data)
	if s == "" {
		return fmt.Errorf("flextime: cannot parse empty text")
	}
	result, err := dateparsa.Parse(s)
	if err != nil {
		return fmt.Errorf("flextime: cannot parse %q: %w", s, err)
	}
	ft.t = result.Time
	ft.valid = true
	return nil
}
