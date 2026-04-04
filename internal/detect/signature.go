package detect

// CharClass represents the character class of a byte in a date string.
type CharClass byte

const (
	CDigit   CharClass = iota // 0-9
	CLetter                   // a-zA-Z (not T or Z in certain positions)
	CSep                      // - / .
	CSpace                    // space, tab
	CColon                    // :
	CSpecial                  // T Z + , and other significant punctuation

	numClasses = 6
)

const maxSigLen = 64

// Signature is a stack-allocated buffer holding the character class sequence
// for an input string. No heap allocation.
type Signature struct {
	buf [maxSigLen]CharClass
	len int
}

// Len returns the number of character classes in the signature.
func (s *Signature) Len() int { return s.len }

// At returns the character class at index i.
func (s *Signature) At(i int) CharClass { return s.buf[i] }

// Scan maps each byte of the input to a CharClass.
// Operates on the raw bytes — no UTF-8 decode needed for the signature
// since all date-relevant characters are ASCII.
//
// Special handling:
//   - 'T' between digits is CSpecial (ISO 8601 separator)
//   - 'Z' at end of string or before +/- is CSpecial (UTC indicator)
//   - '+' and '-' after digits/T/Z are CSpecial (timezone offset sign)
//   - ',' after digits is CSpecial (fractional separator or RFC 2822)
func Scan(s string) Signature {
	var sig Signature
	n := len(s)
	if n > maxSigLen {
		n = maxSigLen
	}
	sig.len = n

	for i := 0; i < n; i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			sig.buf[i] = CDigit

		case c == 'T':
			// 'T' is special when it appears between digit sequences (ISO separator)
			if i > 0 && i < n-1 && isDigit(s[i-1]) && isDigit(s[i+1]) {
				sig.buf[i] = CSpecial
			} else {
				sig.buf[i] = CLetter
			}

		case c == 'Z':
			// 'Z' is special at end of string or before +/-
			if i == n-1 || (i < n-1 && (s[i+1] == '+' || s[i+1] == '-')) {
				sig.buf[i] = CSpecial
			} else {
				sig.buf[i] = CLetter
			}

		case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Y'):
			// Letters (A-Y excluding T handled above, Z handled above)
			sig.buf[i] = CLetter

		case c == '-' || c == '/':
			// These could be separators OR timezone offset signs.
			// After T/Z/digits at a timezone position, treat as special.
			if c == '-' && i > 0 && isTZSignPosition(s, i, n) {
				sig.buf[i] = CSpecial
			} else {
				sig.buf[i] = CSep
			}

		case c == '.':
			sig.buf[i] = CSep

		case c == ' ' || c == '\t':
			sig.buf[i] = CSpace

		case c == ':':
			sig.buf[i] = CColon

		case c == '+':
			sig.buf[i] = CSpecial

		case c == ',':
			sig.buf[i] = CSpecial

		default:
			// Non-ASCII bytes, rare punctuation: treat as letter for now.
			sig.buf[i] = CLetter
		}
	}

	return sig
}

// isTZSignPosition checks if a '-' or '+' at position i is likely a timezone
// offset sign rather than a date separator. Heuristic: it follows a time-like
// pattern (digits:digits or 'Z' or 'T').
func isTZSignPosition(s string, i, n int) bool {
	// If preceded by Z, it's definitely timezone.
	if i > 0 && s[i-1] == 'Z' {
		return true
	}
	// If preceded by a colon pattern like HH:MM:SS-, it's timezone.
	if i >= 8 {
		// Look for time-like pattern before this position.
		// e.g., "...10:30:00-" or "...10:30:00.123-"
		for j := i - 1; j >= 0 && j >= i-7; j-- {
			if s[j] == ':' {
				return true
			}
		}
	}
	return false
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

// allDigits returns true if s is non-empty and every byte is an ASCII digit.
func allDigits(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isDigit(s[i]) {
			return false
		}
	}
	return true
}
