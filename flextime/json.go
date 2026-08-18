package flextime

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/kmoneil/dateparsa/internal/epoch"
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
		ft.set(time.Time{}, false, false)
		return nil
	}

	// Try as quoted string first. A timestamp is printable ASCII with nothing to
	// unescape, so the bytes between the quotes are already the string and the
	// decode encoding/json would perform is a copy of bytes it has just checked.
	// unquoteJSONString refuses anything it is not certain of and the fallback
	// below is what defines the answer in that case.
	if len(data) > 0 && data[0] == '"' {
		s, ok := unquoteJSONString(data)
		if !ok {
			var decoded string
			if err := json.Unmarshal(data, &decoded); err != nil {
				return fmt.Errorf("flextime: invalid JSON string: %w", err)
			}
			s = decoded
		}
		result, err := jsonParser.Parse(s)
		if err != nil {
			return fmt.Errorf("flextime: %w", err)
		}
		ft.set(result.Time, true, result.Ambiguous)
		return nil
	}

	// Try as number (Unix timestamp).
	//
	// The integer and fractional forms want different readings and a float64 has
	// thrown the distinction away by the time you hold one. An integer takes its
	// precision from its digit count, so 1710500000000 is the millisecond epoch it
	// almost always is rather than the year 56173 it used to be. A value with a
	// fraction or an exponent is seconds, because that is the only thing a
	// fractional timestamp can mean.
	//
	// Which it is comes from looking at the bytes rather than from decoding into a
	// json.Number, which was the first version and allocated the number's text:
	// BenchmarkUnmarshalJSONNumber measured 172.6ns and 3 allocations with it
	// against 150.0ns and 2 before, for a distinction the bytes answer directly.
	if isJSONInteger(data) {
		if v, err := strconv.ParseInt(string(data), 10, 64); err == nil {
			t, ok := epoch.FromInt(v)
			if !ok {
				return fmt.Errorf("flextime: %d is not a timestamp this package accepts", v)
			}
			ft.set(t, true, false)
			return nil
		}
		// Out of int64 range. Fall through: the seconds arm refuses it on range,
		// which is the right outcome by either route.
	}

	var f float64
	if err := json.Unmarshal(data, &f); err != nil {
		return fmt.Errorf("flextime: cannot parse JSON value: %s", string(data))
	}
	t, ok := epoch.FromSeconds(f)
	if !ok {
		return fmt.Errorf("flextime: %v is not a timestamp this package accepts", f)
	}
	ft.set(t, true, false)
	return nil
}

// unquoteJSONString returns the body of a JSON string whose bytes already are the
// string, and reports whether data is one. It is the string half of what
// isJSONInteger does for numbers: read the bytes rather than decode them, in the
// case where decoding cannot change them.
//
// It refuses more than it strictly has to and never guesses, because
// encoding/json is the definition of the answer and this only skips the work
// where the two provably agree. A backslash opens an escape sequence. A quote
// means the string token ended before the last byte, so there is trailing
// content and the value is malformed. A byte under 0x20 is a control character
// that JSON forbids unescaped, which encoding/json rejects. A byte over 0x7f is
// UTF-8, which encoding/json validates and replaces where it is malformed, and
// this cannot reproduce that. Every one of those goes back to the decoder.
func unquoteJSONString(data []byte) (string, bool) {
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return "", false
	}
	body := data[1 : len(data)-1]
	for _, c := range body {
		if c == '\\' || c == '"' || c < 0x20 || c > 0x7f {
			return "", false
		}
	}
	return string(body), true
}

// isJSONInteger reports whether data is a JSON number written without a fraction
// or an exponent, which is the form whose digit count names a precision.
//
// It only has to separate the two number forms, not validate the token: the
// ParseInt on the other side of it settles whether the bytes really are an
// integer, and encoding/json has already rejected anything that is not a JSON
// value. A leading minus is part of the number and not a reason to look further.
func isJSONInteger(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	for _, c := range data {
		if c == '.' || c == 'e' || c == 'E' {
			return false
		}
	}
	return true
}
