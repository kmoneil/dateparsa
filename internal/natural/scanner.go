// Package natural parses English natural language date expressions
// such as "3 days ago", "next friday", "yesterday at 5pm".
package natural

import (
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/kmoneil/dateparsa/internal/locale"
)

// TokenKind identifies the type of a token in a natural language date expression.
type TokenKind byte

const (
	TokNumber    TokenKind = iota // An integer: "3", "10"
	TokUnit                       // A time unit: "day", "days", "week", "month", "year", "hour", "minute", "second"
	TokDirection                  // A direction: "ago", "from now", "in" (prefix)
	TokRelWord                    // A relative keyword: "yesterday", "today", "tomorrow", "now"
	TokSelector                   // "last", "next", "this"
	TokWeekday                    // "monday" .. "sunday"
	TokMonth                      // "january" .. "december"
	TokBoundary                   // "beginning", "start", "end"
	TokOf                         // "of"
	TokAt                         // "at"
	TokNoon                       // "noon"
	TokMidnight                   // "midnight"
	TokAMPM                       // "am", "pm"
	TokTime                       // Inline time: "5pm", "14:00", "5:30pm"
	TokHalf                       // "half" (halves the next unit)
	TokTimeOfDay                  // "morning", "afternoon", "evening", "night"
	TokAnd                        // "and" (for compound durations)
	TokUnknown                    // Anything else
)

// Token is a single token from a natural language date expression.
type Token struct {
	Kind TokenKind

	// Pos is the byte offset of the token in the string the scanner walked,
	// and Raw is that token's text.
	//
	// For Scan that string is the input: lowerASCII maps each byte to one byte,
	// so the offsets carry across. For ScanLocale it is not. That scanner runs
	// strings.ToLower and then foldAccents first, and both change lengths: a
	// two-byte "é" folds to a one-byte "e", and ToLower turns an incomplete
	// UTF-8 sequence into a three-byte U+FFFD. ScanLocale("\xc30A", ar) returns
	// a token at Pos 4 for three bytes of input.
	//
	// Mapping back would mean folding with an index, an allocation per locale
	// parse, and nothing outside these two functions reads Pos. So the comment
	// is what changed, not the code. If a caller ever needs to report which
	// part of an input a parse used, that is the moment to build the index; see
	// W11 in the backlog.
	Pos int
	Raw string

	// Semantic values (set by kind):
	IntVal  int // TokNumber: the parsed integer
	UnitVal Unit
	DirVal  Direction
	RelVal  RelWord // TokRelWord: which relative keyword
	WdayVal int     // TokWeekday: 0=Sunday .. 6=Saturday (matches time.Weekday)
	MonVal  int     // TokMonth: 1=January .. 12=December
	SelVal  Selector
	BndVal  Boundary
	Hour    int // TokTime: hour (0-23)
	Min     int // TokTime: minute (0-59)
	AMPM    int // TokAMPM/TokTime: 1=AM, 2=PM
}

// RelWord identifies a specific relative keyword.
type RelWord byte

const (
	RelNow RelWord = iota
	RelToday
	RelYesterday
	RelTomorrow
	RelTonight
)

// Unit represents a time unit.
type Unit byte

const (
	UnitSecond Unit = iota
	UnitMinute
	UnitHour
	UnitDay
	UnitWeek
	UnitMonth
	UnitYear
)

// Direction represents a temporal direction.
type Direction byte

const (
	DirAgo     Direction = iota // past
	DirFromNow                  // future
	DirIn                       // future (prefix: "in 3 days")
)

// Selector represents last/next/this.
type Selector byte

const (
	SelLast Selector = iota
	SelNext
	SelThis
)

// Boundary represents beginning/end of a period.
type Boundary byte

const (
	BndStart Boundary = iota
	BndEnd
)

// lowerASCII performs an in-place ASCII-only lowering into a stack-friendly buffer.
// Returns the lowered string, or "" if non-ASCII case mapping would change length.
// For pure-ASCII inputs (common in English NL), this avoids the strings.ToLower heap allocation.
func lowerASCII(s string) string {
	// Check for non-ASCII bytes that could change length under Unicode lowering.
	hasUpper := false
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			// Fall back to strings.ToLower for non-ASCII.
			lower := strings.ToLower(s)
			if len(lower) != len(s) {
				return ""
			}
			return lower
		}
		if s[i] >= 'A' && s[i] <= 'Z' {
			hasUpper = true
		}
	}
	if !hasUpper {
		return s // already lowercase, zero alloc
	}
	// ASCII-only with uppercase: allocate and fold.
	buf := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 0x20
		}
		buf[i] = c
	}
	return string(buf)
}

// Scan tokenizes a natural language date string into a token sequence.
// All matching is case-insensitive. Unknown words become TokUnknown.
func Scan(s string) []Token {
	lower := lowerASCII(s)
	if lower == "" {
		return nil
	}

	tokens := make([]Token, 0, 6)
	i := 0
	n := len(lower)

	for i < n {
		// Skip whitespace.
		if lower[i] == ' ' || lower[i] == '\t' || lower[i] == ',' {
			i++
			continue
		}

		// Number (possibly followed by unit suffix like "5pm", "3rd").
		if lower[i] >= '0' && lower[i] <= '9' {
			tok := scanNumber(lower, i)
			tokens = append(tokens, tok)
			i = tok.Pos + len(tok.Raw)
			continue
		}

		// Word.
		if isAlpha(lower[i]) {
			start := i
			for i < n && isAlpha(lower[i]) {
				i++
			}
			word := lower[start:i]

			// Two-word phrases: "from now".
			if word == "from" && i < n {
				j := i
				for j < n && (lower[j] == ' ' || lower[j] == '\t') {
					j++
				}
				if j+3 <= n && lower[j:j+3] == "now" && (j+3 == n || !isAlpha(lower[j+3])) {
					tokens = append(tokens, Token{
						Kind:   TokDirection,
						Pos:    start,
						Raw:    lower[start : j+3],
						DirVal: DirFromNow,
					})
					i = j + 3
					continue
				}
			}

			tok := classifyWord(word, start)
			tokens = append(tokens, tok)
			continue
		}

		// Time pattern: H:MM or HH:MM
		if lower[i] == ':' {
			// This shouldn't start a token. Skip.
			i++
			continue
		}

		// Skip other characters.
		i++
	}

	return tokens
}

// convertAMPM adjusts a 12-hour clock value to 24-hour based on an AM/PM indicator.
// ampm: 1=AM, 2=PM, 0=unset (no conversion).
func convertAMPM(hour, ampm int) int {
	if ampm == 2 && hour != 12 {
		return hour + 12
	}
	if ampm == 1 && hour == 12 {
		return 0
	}
	return hour
}

// scanNumber extracts a number token, handling suffixes like "pm", "am", "st", "nd", "rd", "th".
func scanNumber(s string, start int) Token {
	i := start
	n := len(s)
	val := 0
	for i < n && s[i] >= '0' && s[i] <= '9' {
		val = val*10 + int(s[i]-'0')
		i++
	}
	numEnd := i

	// Check for time pattern: digits followed by ":" and more digits (e.g., "14:00", "5:30").
	if tok, ok := tryTimePattern(s, start, val, i, n); ok {
		return tok
	}
	i = numEnd

	// Check for AM/PM suffix directly after the number (e.g., "5pm", "10am").
	if tok, ok := tryAMPMSuffix(s, start, val, i, n); ok {
		return tok
	}

	// Skip ordinal suffixes: "st", "nd", "rd", "th".
	if i+2 <= n {
		suffix := s[i : i+2]
		if suffix == "st" || suffix == "nd" || suffix == "rd" || suffix == "th" {
			return Token{Kind: TokNumber, Pos: start, Raw: s[start : i+2], IntVal: val}
		}
	}

	return Token{Kind: TokNumber, Pos: start, Raw: s[start:i], IntVal: val}
}

// tryTimePattern checks for HH:MM[am/pm] at position i and returns a TokTime token.
func tryTimePattern(s string, start, hour, i, n int) (Token, bool) {
	if i >= n || s[i] != ':' || i+1 >= n || s[i+1] < '0' || s[i+1] > '9' {
		return Token{}, false
	}
	i++ // skip ':'
	min := 0
	minStart := i
	for i < n && s[i] >= '0' && s[i] <= '9' {
		min = min*10 + int(s[i]-'0')
		i++
	}
	if i-minStart != 2 || hour > 23 || min > 59 {
		return Token{}, false
	}
	ampm := 0
	raw := s[start:i]
	if i+2 <= n {
		suffix := s[i : i+2]
		if suffix == "am" {
			ampm = 1
			i += 2
			raw = s[start:i]
		} else if suffix == "pm" {
			ampm = 2
			i += 2
			raw = s[start:i]
		}
	}
	hour = convertAMPM(hour, ampm)
	return Token{Kind: TokTime, Pos: start, Raw: raw, Hour: hour, Min: min, AMPM: ampm}, true
}

// tryAMPMSuffix checks for "am"/"pm" suffix directly after a number (e.g., "5pm").
func tryAMPMSuffix(s string, start, val, i, n int) (Token, bool) {
	if i+2 > n {
		return Token{}, false
	}
	suffix := s[i : i+2]
	if suffix != "am" && suffix != "pm" {
		return Token{}, false
	}
	ampm := 1
	if suffix == "pm" {
		ampm = 2
	}
	hour := convertAMPM(val, ampm)
	return Token{Kind: TokTime, Pos: start, Raw: s[start : i+2], Hour: hour, Min: 0, AMPM: ampm}, true
}

// classifyWord maps a lowercase word to its token kind.
func classifyWord(word string, pos int) Token {
	tok := Token{Pos: pos, Raw: word}

	switch word {
	// Relative keywords
	case "now":
		tok.Kind = TokRelWord
		tok.RelVal = RelNow
	case "today":
		tok.Kind = TokRelWord
		tok.RelVal = RelToday
	case "yesterday":
		tok.Kind = TokRelWord
		tok.RelVal = RelYesterday
	case "tomorrow":
		tok.Kind = TokRelWord
		tok.RelVal = RelTomorrow

	// Directions
	case "ago":
		tok.Kind = TokDirection
		tok.DirVal = DirAgo
	case "in":
		tok.Kind = TokDirection
		tok.DirVal = DirIn

	// Selectors
	case "last", "previous", "past":
		tok.Kind = TokSelector
		tok.SelVal = SelLast
	case "next", "coming":
		tok.Kind = TokSelector
		tok.SelVal = SelNext
	case "this":
		tok.Kind = TokSelector
		tok.SelVal = SelThis

	// Units
	case "second", "seconds", "sec", "secs", "s":
		tok.Kind = TokUnit
		tok.UnitVal = UnitSecond
	case "minute", "minutes", "min", "mins":
		tok.Kind = TokUnit
		tok.UnitVal = UnitMinute
	case "hour", "hours", "hr", "hrs":
		tok.Kind = TokUnit
		tok.UnitVal = UnitHour
	case "day", "days":
		tok.Kind = TokUnit
		tok.UnitVal = UnitDay
	case "week", "weeks":
		tok.Kind = TokUnit
		tok.UnitVal = UnitWeek
	case "month", "months":
		tok.Kind = TokUnit
		tok.UnitVal = UnitMonth
	case "year", "years":
		tok.Kind = TokUnit
		tok.UnitVal = UnitYear

	// Weekdays
	case "sunday", "sun":
		tok.Kind = TokWeekday
		tok.WdayVal = 0
	case "monday", "mon":
		tok.Kind = TokWeekday
		tok.WdayVal = 1
	case "tuesday", "tue", "tues":
		tok.Kind = TokWeekday
		tok.WdayVal = 2
	case "wednesday", "wed":
		tok.Kind = TokWeekday
		tok.WdayVal = 3
	case "thursday", "thu", "thur", "thurs":
		tok.Kind = TokWeekday
		tok.WdayVal = 4
	case "friday", "fri":
		tok.Kind = TokWeekday
		tok.WdayVal = 5
	case "saturday", "sat":
		tok.Kind = TokWeekday
		tok.WdayVal = 6

	// Months
	case "january", "jan":
		tok.Kind = TokMonth
		tok.MonVal = 1
	case "february", "feb":
		tok.Kind = TokMonth
		tok.MonVal = 2
	case "march", "mar":
		tok.Kind = TokMonth
		tok.MonVal = 3
	case "april", "apr":
		tok.Kind = TokMonth
		tok.MonVal = 4
	case "may":
		tok.Kind = TokMonth
		tok.MonVal = 5
	case "june", "jun":
		tok.Kind = TokMonth
		tok.MonVal = 6
	case "july", "jul":
		tok.Kind = TokMonth
		tok.MonVal = 7
	case "august", "aug":
		tok.Kind = TokMonth
		tok.MonVal = 8
	case "september", "sep", "sept":
		tok.Kind = TokMonth
		tok.MonVal = 9
	case "october", "oct":
		tok.Kind = TokMonth
		tok.MonVal = 10
	case "november", "nov":
		tok.Kind = TokMonth
		tok.MonVal = 11
	case "december", "dec":
		tok.Kind = TokMonth
		tok.MonVal = 12

	// Boundary
	case "beginning", "start":
		tok.Kind = TokBoundary
		tok.BndVal = BndStart
	case "end":
		tok.Kind = TokBoundary
		tok.BndVal = BndEnd

	// Structural
	case "of":
		tok.Kind = TokOf
	case "at":
		tok.Kind = TokAt
	case "noon":
		tok.Kind = TokNoon
	case "midnight":
		tok.Kind = TokMidnight
	case "am":
		tok.Kind = TokAMPM
		tok.AMPM = 1
	case "pm":
		tok.Kind = TokAMPM
		tok.AMPM = 2

	// Time-of-day words
	case "morning":
		tok.Kind = TokTimeOfDay
		tok.Hour = 8
	case "afternoon":
		tok.Kind = TokTimeOfDay
		tok.Hour = 14
	case "evening":
		tok.Kind = TokTimeOfDay
		tok.Hour = 18
	case "night":
		tok.Kind = TokTimeOfDay
		tok.Hour = 21
	case "tonight":
		tok.Kind = TokRelWord
		tok.RelVal = RelTonight

	// "half" quantifier
	case "half":
		tok.Kind = TokHalf

	// Compound duration conjunction
	case "and":
		tok.Kind = TokAnd

	default:
		return classifyFallback(word, pos)
	}

	return tok
}

// classifyFallback handles words not in the main switch: "a"/"an" as number 1,
// written-out numbers ("two", "three", etc.), or unknown tokens.
func classifyFallback(word string, pos int) Token {
	if word == "a" || word == "an" {
		return Token{Kind: TokNumber, Pos: pos, Raw: word, IntVal: 1}
	}
	if n, ok := wordToNumber(word); ok {
		return Token{Kind: TokNumber, Pos: pos, Raw: word, IntVal: n}
	}
	return Token{Kind: TokUnknown, Pos: pos, Raw: word}
}

// wordToNumber maps written-out number words and special quantifiers to integers.
func wordToNumber(word string) (int, bool) {
	switch word {
	case "one":
		return 1, true
	case "two":
		return 2, true
	case "three":
		return 3, true
	case "four":
		return 4, true
	case "five":
		return 5, true
	case "six":
		return 6, true
	case "seven":
		return 7, true
	case "eight":
		return 8, true
	case "nine":
		return 9, true
	case "ten":
		return 10, true
	case "eleven":
		return 11, true
	case "twelve":
		return 12, true
	case "fifteen":
		return 15, true
	case "twenty":
		return 20, true
	case "thirty":
		return 30, true
	case "few":
		return 3, true
	}
	return 0, false
}

func isAlpha(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// localeWord maps a lowercase word/phrase to a token.
type localeWord struct {
	phrase string
	tok    Token
}

// localeWords is a locale's phrase table, bucketed by the first byte of the
// phrase.
//
// A phrase can only match at a position whose byte equals its own first byte,
// so bucketing on that byte is exact: the scanner visits the phrases that can
// match and no others. Bucket b is words[index[b]:index[b+1]], and index has
// 257 entries so that the last bucket has an end.
type localeWords struct {
	words []localeWord
	index [257]int32
}

// bucket returns the phrases that can start with byte b, longest first.
func (lw *localeWords) bucket(b byte) []localeWord {
	i := int(b)
	return lw.words[lw.index[i]:lw.index[i+1]]
}

// buildLocaleWords builds a word list from locale relative keywords.
func buildLocaleWords(loc *locale.Data) []localeWord {
	var words []localeWord

	add := func(phrases []string, kind TokenKind, setFn func(*Token)) {
		for _, p := range phrases {
			tok := Token{Kind: kind, Raw: p}
			if setFn != nil {
				setFn(&tok)
			}
			words = append(words, localeWord{strings.ToLower(p), tok})
		}
	}

	add(loc.Relative.Now, TokRelWord, func(t *Token) { t.RelVal = RelNow })
	add(loc.Relative.Today, TokRelWord, func(t *Token) { t.RelVal = RelToday })
	add(loc.Relative.Yesterday, TokRelWord, func(t *Token) { t.RelVal = RelYesterday })
	add(loc.Relative.Tomorrow, TokRelWord, func(t *Token) { t.RelVal = RelTomorrow })
	add(loc.Relative.Ago, TokDirection, func(t *Token) { t.DirVal = DirAgo })
	add(loc.Relative.InFuture, TokDirection, func(t *Token) { t.DirVal = DirIn })
	add(loc.Relative.Last, TokSelector, func(t *Token) { t.SelVal = SelLast })
	add(loc.Relative.Next, TokSelector, func(t *Token) { t.SelVal = SelNext })
	add(loc.Relative.This, TokSelector, func(t *Token) { t.SelVal = SelThis })
	add(loc.Relative.Seconds, TokUnit, func(t *Token) { t.UnitVal = UnitSecond })
	add(loc.Relative.Minutes, TokUnit, func(t *Token) { t.UnitVal = UnitMinute })
	add(loc.Relative.Hours, TokUnit, func(t *Token) { t.UnitVal = UnitHour })
	add(loc.Relative.Days, TokUnit, func(t *Token) { t.UnitVal = UnitDay })
	add(loc.Relative.Weeks, TokUnit, func(t *Token) { t.UnitVal = UnitWeek })
	add(loc.Relative.Months, TokUnit, func(t *Token) { t.UnitVal = UnitMonth })
	add(loc.Relative.Years, TokUnit, func(t *Token) { t.UnitVal = UnitYear })

	// Weekdays.
	for i, name := range loc.WeekdaysWide {
		if name != "" {
			words = append(words, localeWord{strings.ToLower(name), Token{Kind: TokWeekday, Raw: name, WdayVal: i}})
		}
	}
	for i, name := range loc.WeekdaysAbbr {
		if name != "" {
			words = append(words, localeWord{strings.ToLower(name), Token{Kind: TokWeekday, Raw: name, WdayVal: i}})
		}
	}

	// Add ASCII-folded variants for accented phrases.
	var extras []localeWord
	for _, w := range words {
		folded := foldAccents(w.phrase)
		if folded != w.phrase {
			extras = append(extras, localeWord{folded, w.tok})
		}
	}
	words = append(words, extras...)

	// Sort by length descending so longer phrases match first.
	sort.Slice(words, func(i, j int) bool {
		return len(words[i].phrase) > len(words[j].phrase)
	})

	return words
}

// indexLocaleWords buckets a sorted word list by the first byte of each phrase.
//
// The bucketing is a counting sort, and it is stable: within a bucket the
// phrases keep the order buildLocaleWords put them in, so the scanner still
// sees them longest first, and two equal phrases carrying different tokens
// still resolve the way they did before there was an index. There are five of
// those across the twenty locales, "ora" in Italian among them, and which one
// wins is decided by the length sort above, which is not stable. Reordering
// here would change an answer.
//
// A phrase of zero length is dropped. Nothing in the locale data has one, and
// if something did the scanner would match it at any position that is not a
// word character, advance by its zero length, and never terminate.
func indexLocaleWords(words []localeWord) *localeWords {
	lw := &localeWords{}

	var counts [256]int32
	kept := 0
	for _, w := range words {
		if len(w.phrase) == 0 {
			continue
		}
		counts[w.phrase[0]]++
		kept++
	}

	var next [256]int32
	sum := int32(0)
	for b := range 256 {
		lw.index[b] = sum
		next[b] = sum
		sum += counts[b]
	}
	lw.index[256] = sum

	lw.words = make([]localeWord, kept)
	for _, w := range words {
		if len(w.phrase) == 0 {
			continue
		}
		b := w.phrase[0]
		lw.words[next[b]] = w
		next[b]++
	}

	return lw
}

// localeWordCache caches the indexed word list per locale Data pointer.
// Built once per locale on first use, reused thereafter. Safe for concurrent access
// since sync.Map handles concurrent reads and writes.
var localeWordCache sync.Map // map[*locale.Data]*localeWords

// getLocaleWords returns the cached, first-byte-indexed word list for a locale,
// building it on first use. What it returns is read-only: a scan takes a bucket
// out of it and never writes.
func getLocaleWords(loc *locale.Data) *localeWords {
	if v, ok := localeWordCache.Load(loc); ok {
		return v.(*localeWords)
	}
	words := indexLocaleWords(buildLocaleWords(loc))
	localeWordCache.Store(loc, words)
	return words
}

// ScanLocale tokenizes a natural language date string using locale-specific keywords.
func ScanLocale(s string, loc *locale.Data) []Token {
	if loc == nil {
		return nil
	}

	lower := strings.ToLower(s)
	// Fold accents for matching (e.g., "días" → "dias").
	lower = foldAccents(lower)

	words := getLocaleWords(loc)
	var tokens []Token
	i := 0
	n := len(lower)

	for i < n {
		// Skip whitespace and commas.
		if lower[i] == ' ' || lower[i] == '\t' || lower[i] == ',' {
			i++
			continue
		}

		// Try matching a locale phrase at current position. Only the phrases
		// that begin with the byte under i can match, and the index holds
		// those; for an English miss against a Russian table there are none.
		matched := false
		for _, w := range words.bucket(lower[i]) {
			wlen := len(w.phrase)
			if i+wlen <= n && lower[i:i+wlen] == w.phrase {
				// Check word boundary.
				if (i+wlen == n || !isUnicodeWord(lower, i+wlen)) &&
					(i == 0 || !isUnicodeWord(lower, prevCharPos(lower, i))) {
					// Raw is the text that matched, not the phrase from the
					// table. Those differ once accents are folded: matching
					// "dias" against the Spanish table used to hand back a
					// token whose Raw was "días", five bytes for four bytes of
					// input, so Pos plus len(Raw) was not a span.
					tok := w.tok
					tok.Pos = i
					tok.Raw = lower[i : i+wlen]
					tokens = append(tokens, tok)
					i += wlen
					matched = true
					break
				}
			}
		}
		if matched {
			continue
		}

		// Number.
		if lower[i] >= '0' && lower[i] <= '9' {
			tok := scanNumber(lower, i)
			tokens = append(tokens, tok)
			i = tok.Pos + len(tok.Raw)
			continue
		}

		// Skip to next whitespace/number (unknown word).
		start := i
		for i < n && lower[i] != ' ' && lower[i] != '\t' && lower[i] != ',' && !(lower[i] >= '0' && lower[i] <= '9') {
			i++
		}
		tokens = append(tokens, Token{Kind: TokUnknown, Pos: start, Raw: lower[start:i]})
	}

	return tokens
}

// isUnicodeWord checks if the byte at position pos in s starts a word character
// (letter or digit), handling UTF-8.
func isUnicodeWord(s string, pos int) bool {
	if pos >= len(s) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(s[pos:])
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// foldAccents strips common accented latin characters to their ASCII base.
// This allows matching "dias" against "días", "Minuten" against "Minüten", etc.
func foldAccents(s string) string {
	// Look before building. Nothing folds in an all-ASCII string, which is
	// every English input and every input that is not a date at all, and this
	// used to fill a Builder with the whole string and then throw it away on
	// each one: 31.9% of the objects allocated by a miss with three locales
	// configured.
	//
	// strings.ToLower, which the only per-call caller runs on the line above
	// this one, is already this shape. It returns its argument when an ASCII
	// string holds nothing to change.
	folds := false
	for _, r := range s {
		if foldRune(r) != r {
			folds = true
			break
		}
	}
	if !folds {
		return s
	}

	// Every fold maps a two-byte Latin-1 rune to a one-byte ASCII one, so the
	// result is never longer than the input and one Grow covers it. Without it
	// the Builder reallocates as it grows.
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		b.WriteRune(foldRune(r))
	}
	return b.String()
}

func foldRune(r rune) rune {
	switch {
	case r >= 'à' && r <= 'å':
		return 'a'
	case r == 'æ':
		return 'a'
	case r == 'ç':
		return 'c'
	case r >= 'è' && r <= 'ë':
		return 'e'
	case r >= 'ì' && r <= 'ï':
		return 'i'
	case r == 'ñ':
		return 'n'
	case r >= 'ò' && r <= 'ö':
		return 'o'
	case r >= 'ù' && r <= 'ü':
		return 'u'
	case r == 'ý' || r == 'ÿ':
		return 'y'
	}
	return r
}

// prevCharPos returns the start position of the previous UTF-8 character.
func prevCharPos(s string, pos int) int {
	if pos <= 0 {
		return 0
	}
	pos--
	for pos > 0 && !utf8.RuneStart(s[pos]) {
		pos--
	}
	return pos
}
