// Package epoch detects and parses Unix timestamps.
//
// Supports seconds, milliseconds, microseconds, nanoseconds, and
// fractional seconds (e.g. 1710500000.123).
package epoch

import (
	"time"
)

// Kind identifies the precision of a detected timestamp.
type Kind int

const (
	KindNone   Kind = iota
	KindSec         // 10-digit: seconds since epoch
	KindMilli       // 13-digit: milliseconds
	KindMicro       // 16-digit: microseconds
	KindNano        // 19-digit: nanoseconds
	KindFrac        // Fractional: 1710500000.123
)

// Result holds a detected epoch timestamp.
type Result struct {
	Time time.Time
	Kind Kind
}

// Detect checks if the string is a Unix timestamp and parses it.
// Returns nil if the string is not a valid timestamp.
//
// Detection heuristics:
//   - All digits (optionally with one '.') and optional leading '-'
//   - Length determines precision: 10=sec, 13=milli, 16=micro, 19=nano
//   - Fractional part detected by presence of '.'
//   - Range check: seconds must be in [0, 1e11) for positive, [-1e11, 0) for negative
//     This avoids false positives on short numbers like "2024" or "20240315"
func Detect(s string) *Result {
	if len(s) == 0 {
		return nil
	}

	neg := false
	start := 0
	if s[0] == '-' {
		neg = true
		start = 1
		if len(s) == 1 {
			return nil
		}
	}

	// Scan for digits and at most one dot.
	dotPos := -1
	for i := start; i < len(s); i++ {
		if s[i] == '.' {
			if dotPos >= 0 {
				return nil // two dots
			}
			dotPos = i
		} else if s[i] < '0' || s[i] > '9' {
			return nil // non-digit
		}
	}

	if dotPos >= 0 {
		return parseFractional(s, start, dotPos, neg)
	}

	return parseInteger(s, start, neg)
}

func parseInteger(s string, start int, neg bool) *Result {
	digitLen := len(s) - start

	// Determine precision by digit count.
	var kind Kind
	switch {
	case digitLen == 10:
		kind = KindSec
	case digitLen == 13:
		kind = KindMilli
	case digitLen == 16:
		kind = KindMicro
	case digitLen == 19:
		kind = KindNano
	default:
		// 11-12 digits could still be seconds if in range.
		if digitLen >= 10 && digitLen <= 12 {
			kind = KindSec
		} else {
			return nil
		}
	}

	val := parseInt(s, start)
	if neg {
		val = -val
	}

	var t time.Time
	switch kind {
	case KindSec:
		// Range check: valid epoch seconds roughly 1973-03-03 to 5138-11-16.
		// We use a practical range of [-1e11, 1e11].
		if val > 1e11 || val < -1e11 {
			return nil
		}
		t = time.Unix(val, 0).UTC()
	case KindMilli:
		sec := val / 1000
		nsec := (val % 1000) * 1e6
		if neg && nsec < 0 {
			nsec = -nsec
		}
		t = time.Unix(sec, nsec).UTC()
	case KindMicro:
		sec := val / 1e6
		nsec := (val % 1e6) * 1e3
		if neg && nsec < 0 {
			nsec = -nsec
		}
		t = time.Unix(sec, nsec).UTC()
	case KindNano:
		sec := val / 1e9
		nsec := val % 1e9
		if neg && nsec < 0 {
			nsec = -nsec
		}
		t = time.Unix(sec, nsec).UTC()
	}

	return &Result{Time: t, Kind: kind}
}

func parseFractional(s string, start, dotPos int, neg bool) *Result {
	intPart := parseInt(s, start)
	if neg {
		intPart = -intPart
	}

	// Integer part must look like epoch seconds (roughly 10 digits).
	intDigits := dotPos - start
	if intDigits < 9 || intDigits > 12 {
		return nil
	}

	// Range check on the integer part.
	if intPart > 1e11 || intPart < -1e11 {
		return nil
	}

	// Parse fractional part as nanoseconds.
	fracStr := s[dotPos+1:]
	if len(fracStr) == 0 {
		return nil
	}
	fracVal := parseInt(fracStr, 0)
	// Scale to nanoseconds.
	for i := len(fracStr); i < 9; i++ {
		fracVal *= 10
	}
	// Truncate if more than 9 digits.
	for i := len(fracStr); i > 9; i-- {
		fracVal /= 10
	}

	t := time.Unix(intPart, fracVal).UTC()
	return &Result{Time: t, Kind: KindFrac}
}

// parseInt parses an integer from s[start:]. No overflow check — caller
// ensures the digit count is bounded.
func parseInt(s string, start int) int64 {
	var val int64
	for i := start; i < len(s); i++ {
		if s[i] == '.' {
			break
		}
		val = val*10 + int64(s[i]-'0')
	}
	return val
}
