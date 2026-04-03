// Package natural parses English natural language date expressions
// such as "3 days ago", "next friday", "yesterday at 5pm".
package natural

import (
	"strings"
)

// TokenKind identifies the type of a token in a natural language date expression.
type TokenKind byte

const (
	TokNumber    TokenKind = iota // An integer: "3", "10"
	TokUnit                      // A time unit: "day", "days", "week", "month", "year", "hour", "minute", "second"
	TokDirection                  // A direction: "ago", "from now", "in" (prefix)
	TokRelWord                   // A relative keyword: "yesterday", "today", "tomorrow", "now"
	TokSelector                  // "last", "next", "this"
	TokWeekday                   // "monday" .. "sunday"
	TokMonth                     // "january" .. "december"
	TokBoundary                  // "beginning", "start", "end"
	TokOf                        // "of"
	TokAt                        // "at"
	TokNoon                      // "noon"
	TokMidnight                  // "midnight"
	TokAMPM                      // "am", "pm"
	TokTime                      // Inline time: "5pm", "14:00", "5:30pm"
	TokUnknown                   // Anything else
)

// Token is a single token from a natural language date expression.
type Token struct {
	Kind TokenKind
	Pos  int    // Byte offset in the original string
	Raw  string // The original text of the token

	// Semantic values (set by kind):
	IntVal  int // TokNumber: the parsed integer
	UnitVal Unit
	DirVal  Direction
	WdayVal int // TokWeekday: 0=Sunday .. 6=Saturday (matches time.Weekday)
	MonVal  int // TokMonth: 1=January .. 12=December
	SelVal  Selector
	BndVal  Boundary
	Hour    int // TokTime: hour (0-23)
	Min     int // TokTime: minute (0-59)
	AMPM    int // TokAMPM/TokTime: 1=AM, 2=PM
}

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
	DirAgo    Direction = iota // past
	DirFromNow                // future
	DirIn                     // future (prefix: "in 3 days")
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

// Scan tokenizes a natural language date string into a token sequence.
// All matching is case-insensitive. Unknown words become TokUnknown.
func Scan(s string) []Token {
	lower := strings.ToLower(s)
	if len(lower) != len(s) {
		// Non-ASCII case mapping changed length — bail to avoid offset mismatch.
		return nil
	}

	var tokens []Token
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
	if i < n && s[i] == ':' && i+1 < n && s[i+1] >= '0' && s[i+1] <= '9' {
		hour := val
		i++ // skip ':'
		min := 0
		minStart := i
		for i < n && s[i] >= '0' && s[i] <= '9' {
			min = min*10 + int(s[i]-'0')
			i++
		}
		if i-minStart == 2 && hour <= 23 && min <= 59 {
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
			if ampm == 2 && hour != 12 {
				hour += 12
			} else if ampm == 1 && hour == 12 {
				hour = 0
			}
			return Token{Kind: TokTime, Pos: start, Raw: raw, Hour: hour, Min: min, AMPM: ampm}
		}
		// Not a valid time — rewind and treat as just a number.
		i = numEnd
	}

	// Check for AM/PM suffix directly after the number (e.g., "5pm", "10am").
	if i+2 <= n {
		suffix := s[i : i+2]
		if suffix == "am" || suffix == "pm" {
			hour := val
			ampm := 1
			if suffix == "pm" {
				ampm = 2
			}
			if ampm == 2 && hour != 12 {
				hour += 12
			} else if ampm == 1 && hour == 12 {
				hour = 0
			}
			return Token{Kind: TokTime, Pos: start, Raw: s[start : i+2], Hour: hour, Min: 0, AMPM: ampm}
		}
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

// classifyWord maps a lowercase word to its token kind.
func classifyWord(word string, pos int) Token {
	tok := Token{Pos: pos, Raw: word}

	switch word {
	// Relative keywords
	case "now", "today":
		tok.Kind = TokRelWord
	case "yesterday":
		tok.Kind = TokRelWord
	case "tomorrow":
		tok.Kind = TokRelWord

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

	default:
		// Check for "a" as a number (e.g., "a week ago" = "1 week ago").
		if word == "a" || word == "an" {
			tok.Kind = TokNumber
			tok.IntVal = 1
		} else {
			tok.Kind = TokUnknown
		}
	}

	return tok
}

func isAlpha(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
