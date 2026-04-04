package flextime

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/kmoneil/dateparsa"
)

// MarshalJSON implements json.Marshaler.
// Encodes as RFC3339Nano string (or custom format via WithJSONFormat).
// Encodes as JSON null if not valid.
func (ft FlexTime) MarshalJSON() ([]byte, error) {
	if !ft.valid {
		return []byte("null"), nil
	}
	layout := time.RFC3339Nano
	if ft.opts != nil && ft.opts.jsonFormat != "" {
		layout = ft.opts.jsonFormat
	}
	return json.Marshal(ft.t.Format(layout))
}

// UnmarshalJSON implements json.Unmarshaler.
// Accepts: RFC3339, ISO8601, Unix timestamp (number), null,
// and any other format dateparsa can detect.
func (ft *FlexTime) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		ft.valid = false
		ft.t = time.Time{}
		return nil
	}

	// Try as quoted string first.
	if len(data) > 0 && data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return fmt.Errorf("flextime: invalid JSON string: %w", err)
		}
		result, err := dateparsa.Parse(s)
		if err != nil {
			return fmt.Errorf("flextime: cannot parse %q: %w", s, err)
		}
		ft.t = result.Time
		ft.valid = true
		return nil
	}

	// Try as number (Unix timestamp).
	var f float64
	if err := json.Unmarshal(data, &f); err != nil {
		return fmt.Errorf("flextime: cannot parse JSON value: %s", string(data))
	}
	sec, frac := math.Modf(f)
	nsec := math.Round(frac * 1e9)
	ft.t = time.Unix(int64(sec), int64(nsec))
	ft.valid = true
	return nil
}
