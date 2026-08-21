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

// A class is three bits, so twenty-one of them fit in a uint64 with one bit
// spare. That is what lets a whole signature be a key rather than a path.
//
// The build fails rather than silently truncating if a seventh class is ever
// added, because a class that did not fit its three bits would collide with
// another one and match the wrong format.
const (
	sigBitsPerClass   = 3
	sigClassMask      = 1<<sigBitsPerClass - 1
	sigClassesPerWord = 64 / sigBitsPerClass // 21
	sigWords          = (maxSigLen + sigClassesPerWord - 1) / sigClassesPerWord
)

var _ [sigClassMask + 1 - numClasses]struct{}

// Signature is the character class sequence for an input string, packed three
// bits to a class. No heap allocation.
//
// It used to be a [64]CharClass that the trie walked one position at a time,
// one dependent pointer load per input byte through 139 nodes of 56 bytes.
// Packed, the whole sequence is a key: the lookup is a hash and a compare, and
// the struct is 48 bytes rather than 80, so Scan returns less than it did.
//
// key[0] holds classes 0 to 20, key[1] the next 21, and so on. Nothing outside
// this file indexes a position: At exists for the tests that render a
// signature, and the lookup compares whole words.
type Signature struct {
	key       [sigWords]uint64
	len       int
	HasLetter bool // true if any byte was classified as CLetter
}

// Len returns the number of character classes in the signature.
func (s *Signature) Len() int { return s.len }

// At returns the character class at index i, which must be under Len, as it had
// to be when this indexed an array.
//
// It unpacks rather than indexes, and that is affordable because it is not on
// the hot path: the lookup compares whole words and never asks about one
// position.
func (s *Signature) At(i int) CharClass {
	w, sh := i/sigClassesPerWord, uint(i%sigClassesPerWord)*sigBitsPerClass
	return CharClass(s.key[w] >> sh & sigClassMask)
}

// classOfByte answers what class a byte takes when nothing around it matters,
// which is every byte but three.
//
// It is a table rather than the switch this used to be, because the switch was
// a chain of range compares run once per input byte and the answer for 253 of
// the 256 bytes is a constant. A load and a test replace it.
//
// 'T', 'Z' and '-' carry ctxBit, because what they are depends on their
// neighbours: 'T' is the ISO separator only between digits, 'Z' is the UTC
// marker only at the end or before a sign, and '-' is a zone sign only after a
// time. Those three take the branch below; everything else does not, and the
// branch predicts perfectly for an input whose bytes are mostly digits.
var classOfByte = buildClassOfByte()

// ctxBit marks a byte whose class Scan has to decide from context. It sits
// above the three bits a class occupies, so masking it off leaves the class the
// table guessed, which is the one those bytes take when context does not apply.
const ctxBit = 0x80

func buildClassOfByte() [256]uint8 {
	var t [256]uint8
	for i := range t {
		c := byte(i)
		switch {
		case c >= '0' && c <= '9':
			t[i] = uint8(CDigit)
		case c == 'T', c == 'Z':
			t[i] = ctxBit | uint8(CLetter)
		case c == '-':
			t[i] = ctxBit | uint8(CSep)
		case c == '/', c == '.':
			t[i] = uint8(CSep)
		case c == ' ', c == '\t':
			t[i] = uint8(CSpace)
		case c == ':':
			t[i] = uint8(CColon)
		case c == '+', c == ',':
			t[i] = uint8(CSpecial)
		default:
			// Letters, non-ASCII bytes and rare punctuation alike. This is the
			// switch's letter arm and its default arm, which agreed.
			t[i] = uint8(CLetter)
		}
	}
	return t
}

// Scan maps each byte of the input to a CharClass, producing a Signature used
// for format lookup. No heap allocation.
//
// Context-dependent classification rules:
//   - 'T' between digits → CSpecial (ISO 8601 date-time separator)
//   - 'Z' at end or before +/- → CSpecial (UTC indicator)
//   - '-' after a time pattern → CSpecial (timezone offset sign)
//   - '+' and ',' → always CSpecial
//
// The work is a fixed amount per byte and stops at maxSigLen, which SECURITY.md
// states as a property and which is why HasLetter means "in the first 64 bytes"
// and hasLetterPastSignature exists.
func Scan(s string) Signature {
	var sig Signature
	n := len(s)
	if n > maxSigLen {
		n = maxSigLen
	}
	sig.len = n

	// The word and the shift walk with i rather than being divided out of it,
	// which is what keeps the packing to an or and an add per byte.
	w, sh := 0, uint(0)
	seen := uint8(0)
	for i := 0; i < n; i++ {
		c := s[i]
		cl := classOfByte[c]

		if cl&ctxBit != 0 {
			cl &^= ctxBit
			switch c {
			case 'T':
				// The ISO 8601 date-time separator, between digits.
				if i > 0 && i < n-1 && isDigit(s[i-1]) && isDigit(s[i+1]) {
					cl = uint8(CSpecial)
				}
			case 'Z':
				// The UTC marker, at the end or before a zone sign.
				if i == n-1 || (i < n-1 && (s[i+1] == '+' || s[i+1] == '-')) {
					cl = uint8(CSpecial)
				}
			default: // '-'
				// A zone sign rather than a date separator, after a time.
				if i > 0 && isTZSignPosition(s, i, n) {
					cl = uint8(CSpecial)
				}
			}
		}

		sig.key[w] |= uint64(cl) << sh
		// One bit per class seen, tested once at the end rather than branched
		// on per byte.
		//
		// The set and not the classes themselves. Or-ing the class values
		// together and testing bit 0 is wrong and looks right: CLetter is 1,
		// but CSpace is 3 and CSpecial is 5, so a tab set the bit and Scan
		// reported a letter in an input that held none, which sends it to the
		// textual detector. FuzzScanMatchesReference found that within a second
		// of being written, which is the argument for keeping the old switch in
		// the test file as a reference.
		seen |= 1 << cl

		sh += sigBitsPerClass
		if sh >= sigClassesPerWord*sigBitsPerClass {
			w, sh = w+1, 0
		}
	}
	sig.HasLetter = seen&(1<<uint8(CLetter)) != 0

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
