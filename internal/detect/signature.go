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
	buf       [maxSigLen]CharClass
	len       int
	HasLetter bool // true if any byte was classified as CLetter
}

// Len returns the number of character classes in the signature.
func (s *Signature) Len() int { return s.len }

// At returns the character class at index i.
func (s *Signature) At(i int) CharClass { return s.buf[i] }

// Scan maps each byte of the input to a CharClass, producing a Signature
// used for trie-based format lookup. No heap allocation.
//
// Context-dependent classification rules:
//   - 'T' between digits → CSpecial (ISO 8601 date-time separator)
//   - 'Z' at end or before +/- → CSpecial (UTC indicator)
//   - '-' after a time pattern → CSpecial (timezone offset sign)
//   - '+' and ',' → always CSpecial
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

		// ── Context-dependent: T, Z ─────────────────────────────
		// 'T' between digits → ISO 8601 separator ("2024-03-15T10:30:00").
		// Otherwise it's a letter ("Tuesday", "TZ").
		case c == 'T':
			if i > 0 && i < n-1 && isDigit(s[i-1]) && isDigit(s[i+1]) {
				sig.buf[i] = CSpecial
			} else {
				sig.buf[i] = CLetter
				sig.HasLetter = true
			}

		// 'Z' at end or before +/- → UTC marker ("...00Z", "...Z+05:00").
		// Otherwise it's a letter ("Zone").
		case c == 'Z':
			if i == n-1 || (i < n-1 && (s[i+1] == '+' || s[i+1] == '-')) {
				sig.buf[i] = CSpecial
			} else {
				sig.buf[i] = CLetter
				sig.HasLetter = true
			}

		// ── Letters ──────────────────────────────────────────────
		case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Y'):
			sig.buf[i] = CLetter
			sig.HasLetter = true

		// ── Context-dependent: dash ──────────────────────────────
		// '-' is a date separator unless after a time pattern ("10:30:00-05:00").
		// '/' is always a date separator.
		case c == '-' || c == '/':
			if c == '-' && i > 0 && isTZSignPosition(s, i, n) {
				sig.buf[i] = CSpecial
			} else {
				sig.buf[i] = CSep
			}

		// ── Simple one-to-one mappings ───────────────────────────
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
			// Non-ASCII bytes, rare punctuation: treat as letter.
			sig.buf[i] = CLetter
			sig.HasLetter = true
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

// maybeLetter reports whether Scan could classify c as CLetter, for use past
// the bytes Scan actually classified.
//
// It lists the arms of Scan's switch that produce something other than CLetter
// and calls everything else a letter, which includes the default arm: non-ASCII
// bytes and rare punctuation are CLetter there too.
//
// 'T' and 'Z' are counted as letters here even though Scan makes them CSpecial
// in the middle of a timestamp, because that classification depends on their
// neighbours and this predicate does not look at them. Over-reporting is the
// safe direction: the only thing the answer gates is whether a fallback
// detector gets to look at the input, and that detector can refuse.
// TestMaybeLetterCoversScan checks the direction holds for all 256 bytes.
func maybeLetter(c byte) bool {
	switch c {
	case '-', '/', '.', ' ', '\t', ':', '+', ',':
		return false
	}
	return !isDigit(c)
}

// hasLetterPastSignature reports whether a letter sits past the bytes Scan
// classified. It returns false for anything Scan saw in full.
//
// Signature.HasLetter comes from a pass that stops at maxSigLen, so on its own
// it answers "has a letter in the first 64 bytes". Detect gated the whole
// textual family on it, and a date with 64 bytes of anything but letters in
// front of it was handed to natural language instead, which read "March 15" and
// answered with the base year for an input carrying 2024.
//
// This is deliberately not folded into Scan. Scan runs on every structured
// parse and does a fixed amount of work whatever the input length, which
// SECURITY.md states as a property. Asking the question here instead keeps the
// scan linear only on the path that already failed the trie, and that path
// falls through to natural language, which is linear and uncapped anyway.
func hasLetterPastSignature(s string) bool {
	for i := maxSigLen; i < len(s); i++ {
		if maybeLetter(s[i]) {
			return true
		}
	}
	return false
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
