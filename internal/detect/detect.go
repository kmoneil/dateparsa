package detect

import (
	"strings"
	"time"

	"github.com/kmoneil/dateparsa/internal/compile"
)

// globalTrie is built once at init time.
var globalTrie *trie

func init() {
	globalTrie = buildTrie()
}

// Result holds the outcome of format detection.
type Result struct {
	Def   *compile.FormatDef
	Ambig bool
}

// Config passes user preferences into the detection layer.
type Config struct {
	PreferDayFirst  bool
	PreferYearFirst bool
	Timezone        *time.Location
}

// Detect analyzes a date string and returns the matching FormatDef.
// Returns nil if no structured format matches.
func Detect(s string, cfg Config) *Result {
	// Step 1: Try special formats that contain letters but aren't textual months.
	if r := detectISOWeekOrOrdinal(s); r != nil {
		return r
	}

	// Step 2: Try textual month formats if the string contains letters.
	if hasLetter(s) {
		if result := detectTextualMonth(s, cfg); result != nil {
			return result
		}
	}

	// Step 3: Compute signature and walk the trie.
	sig := Scan(s)
	entry := globalTrie.lookup(&sig)
	if entry == nil {
		return nil
	}

	// Step 3: Handle ambiguous formats.
	if entry.ambig {
		return resolveAmbiguous(s, entry, cfg)
	}

	// Step 4: Build the FormatDef from the matched entry.
	def := &compile.FormatDef{
		Name:     entry.name,
		GoLayout: entry.goLayout,
		Fields:   entry.fields,
	}
	return &Result{Def: def}
}

// resolveAmbiguous handles DD/DD/DDDD type signatures where the format
// could be MM/DD/YYYY or DD/MM/YYYY.
func resolveAmbiguous(s string, entry *formatEntry, cfg Config) *Result {
	// Parse the first two numeric components.
	sep := findSep(s)
	if sep < 0 {
		return nil
	}
	sepChar := s[sep]

	// Dot separator strongly implies European DMY convention.
	if sepChar == '.' {
		cfg.PreferDayFirst = true
	}

	parts := splitOnSep(s, sepChar)

	if len(parts) < 3 {
		return nil
	}

	first := parseSmallInt(parts[0])
	second := parseSmallInt(parts[1])
	third := parseSmallInt(parts[2])

	if first < 0 || second < 0 || third < 0 {
		return nil
	}

	// Determine year position and value.
	var year, v1, v2 int
	var yearOffset, v1Offset, v2Offset int
	var yearLen int

	if third > 31 || len(parts[2]) == 4 {
		// Year is last: ??/??/YYYY
		year = third
		v1, v2 = first, second
		v1Offset = 0
		v2Offset = len(parts[0]) + 1
		yearOffset = len(parts[0]) + 1 + len(parts[1]) + 1
		yearLen = len(parts[2])
	} else if first > 31 || len(parts[0]) == 4 {
		// Year is first: YYYY/??/??
		year = first
		v1, v2 = second, third
		yearOffset = 0
		yearLen = len(parts[0])
		v1Offset = len(parts[0]) + 1
		v2Offset = len(parts[0]) + 1 + len(parts[1]) + 1
	} else {
		// All small numbers, truly ambiguous with 2-digit year.
		// Apply preference or default to MM/DD/YY.
		year = third
		if year < 69 {
			year += 2000
		} else {
			year += 1900
		}
		v1, v2 = first, second
		v1Offset = 0
		v2Offset = len(parts[0]) + 1
		yearOffset = len(parts[0]) + 1 + len(parts[1]) + 1
		yearLen = len(parts[2])
	}

	// Resolve month vs day.
	ambig := false
	var monthVal, dayVal int
	var monthOffset, dayOffset int
	var monthLen, dayLen int

	if v1 > 12 {
		// First must be day.
		dayVal, monthVal = v1, v2
		dayOffset, monthOffset = v1Offset, v2Offset
		dayLen, monthLen = len(parts[0]), len(parts[1])
	} else if v2 > 12 {
		// Second must be day.
		monthVal, dayVal = v1, v2
		monthOffset, dayOffset = v1Offset, v2Offset
		monthLen, dayLen = len(parts[0]), len(parts[1])
	} else {
		// Genuinely ambiguous — both could be month or day.
		ambig = true
		if cfg.PreferDayFirst {
			dayVal, monthVal = v1, v2
			dayOffset, monthOffset = v1Offset, v2Offset
			dayLen, monthLen = len(parts[0]), len(parts[1])
		} else {
			monthVal, dayVal = v1, v2
			monthOffset, dayOffset = v1Offset, v2Offset
			monthLen, dayLen = len(parts[0]), len(parts[1])
		}
	}

	if monthVal < 1 || monthVal > 12 || dayVal < 1 || dayVal > 31 {
		return nil
	}

	var fields []compile.Field

	// Build fields in input order.
	type fieldInfo struct {
		offset int
		field  compile.Field
	}
	var infos [3]fieldInfo

	yearFieldKind := compile.FYear4
	if yearLen <= 2 {
		yearFieldKind = compile.FYear2
	}

	monthFieldKind := compile.FMonth2
	if monthLen == 1 {
		monthFieldKind = compile.FMonth1or2
	}

	dayFieldKind := compile.FDay2
	if dayLen == 1 {
		dayFieldKind = compile.FDay1or2
	}

	infos[0] = fieldInfo{yearOffset, compile.Field{Kind: yearFieldKind, Offset: yearOffset, Len: yearLen}}
	infos[1] = fieldInfo{monthOffset, compile.Field{Kind: monthFieldKind, Offset: monthOffset, Len: monthLen}}
	infos[2] = fieldInfo{dayOffset, compile.Field{Kind: dayFieldKind, Offset: dayOffset, Len: dayLen}}

	// Sort by offset.
	for i := 0; i < 2; i++ {
		for j := i + 1; j < 3; j++ {
			if infos[j].offset < infos[i].offset {
				infos[i], infos[j] = infos[j], infos[i]
			}
		}
	}

	// Insert literal separators between fields.
	prevEnd := 0
	for _, info := range infos {
		if info.offset > prevEnd {
			fields = append(fields, compile.Field{Kind: compile.FLiteral, Offset: prevEnd, Len: info.offset - prevEnd})
		}
		fields = append(fields, info.field)
		prevEnd = info.offset + info.field.Len
	}

	_ = year // year value used for validation only; the field extracts it at runtime

	goLayout := ""
	name := "NUMERIC_AMBIG"
	if !ambig {
		if cfg.PreferDayFirst {
			name = "NUMERIC_DMY"
		} else {
			name = "NUMERIC_MDY"
		}
	}

	def := &compile.FormatDef{
		Name:     name,
		GoLayout: goLayout,
		Fields:   fields,
	}
	return &Result{Def: def, Ambig: ambig}
}

// detectTextualMonth handles formats like "March 15, 2024", "15 Mar 2024",
// "Mar 15, 2024", "Fri, 15 Mar 2024 10:30:00 +0000" (RFC 2822).
// detectISOWeekOrOrdinal detects ISO week dates (2024-W11-5) and ordinal dates (2024-074).
func detectISOWeekOrOrdinal(s string) *Result {
	n := len(s)

	// ISO week date: YYYY-Www-D (10 chars) or YYYY-Www (8 chars)
	// Pattern: 4 digits, '-', 'W', 2 digits, optionally '-', 1 digit
	if n >= 8 && s[4] == '-' && (s[5] == 'W' || s[5] == 'w') {
		if n == 8 && isDigit(s[6]) && isDigit(s[7]) {
			// YYYY-Www (week only, assume day 1=Monday)
			return &Result{Def: &compile.FormatDef{
				Name: "ISO_WEEK",
				Fields: []compile.Field{
					{Kind: compile.FYear4, Offset: 0, Len: 4},
					{Kind: compile.FISOWeek, Offset: 6, Len: 2},
				},
			}}
		}
		if n == 10 && isDigit(s[6]) && isDigit(s[7]) && s[8] == '-' && isDigit(s[9]) {
			// YYYY-Www-D
			return &Result{Def: &compile.FormatDef{
				Name: "ISO_WEEK_DATE",
				Fields: []compile.Field{
					{Kind: compile.FYear4, Offset: 0, Len: 4},
					{Kind: compile.FISOWeek, Offset: 6, Len: 2},
					{Kind: compile.FISOWeekDay, Offset: 9, Len: 1},
				},
			}}
		}
	}

	// ISO ordinal date: YYYY-DDD (8 chars)
	// Pattern: 4 digits, '-', 3 digits
	if n == 8 && s[4] == '-' && isDigit(s[0]) && isDigit(s[5]) && isDigit(s[6]) && isDigit(s[7]) {
		// Verify it's not a regular date (month would be 01-12, not 1xx-3xx).
		// The key: ordinal day is 3 digits (001-366).
		d0 := s[5] - '0'
		d1 := s[6] - '0'
		d2 := s[7] - '0'
		if d0 <= 9 && d1 <= 9 && d2 <= 9 {
			val := int(d0)*100 + int(d1)*10 + int(d2)
			if val >= 1 && val <= 366 {
				return &Result{Def: &compile.FormatDef{
					Name: "ISO_ORDINAL",
					Fields: []compile.Field{
						{Kind: compile.FYear4, Offset: 0, Len: 4},
						{Kind: compile.FOrdinalDay, Offset: 5, Len: 3},
					},
				}}
			}
		}
	}

	return nil
}

func detectTextualMonth(s string, cfg Config) *Result {
	lower := strings.ToLower(s)

	// Try to find a month name in the string.
	monthNum, monthStart, monthEnd := findMonthName(lower)
	if monthNum == 0 {
		return nil
	}

	// ToLower can change byte length for non-ASCII input.
	// If the positions don't map cleanly to the original, bail out.
	if monthEnd > len(s) || monthStart > len(s) || len(lower) != len(s) {
		return nil
	}

	// Extract the rest and figure out the structure.
	// Strategy: remove the month name, find the remaining numeric components.
	before := strings.TrimSpace(s[:monthStart])
	after := strings.TrimSpace(s[monthEnd:])

	// Handle "March 15, 2024" or "Mar 15, 2024"
	// after = "15, 2024"
	// Handle "15 March 2024" or "15 Mar 2024"
	// before = "15"
	// Handle "Fri, 15 Mar 2024 10:30:00 +0000" (RFC 2822)

	var fields []compile.Field
	name := "TEXTUAL_MONTH"
	goLayout := ""

	// Month is already resolved.
	fields = append(fields, compile.Field{
		Kind: compile.FMonthName,
		Aux:  uint16(monthNum),
	})

	// Parse numbers from before and after the month name.
	beforeNums := extractNumbers(before)
	afterStr := strings.TrimLeft(after, ", ")
	afterNums := extractNumbers(afterStr)

	var day, year int

	switch {
	case len(beforeNums) == 0 && len(afterNums) >= 2:
		// "March 15, 2024" pattern
		day = afterNums[0]
		year = afterNums[1]
		name = "MONTH_DAY_YEAR"

	case len(beforeNums) >= 1 && len(afterNums) >= 1:
		// Check for "Fri, 15 Mar 2024" (weekday prefix)
		if len(beforeNums) == 1 && beforeNums[0] <= 31 {
			day = beforeNums[0]
			year = afterNums[0]
			name = "DAY_MONTH_YEAR"
		} else {
			day = beforeNums[len(beforeNums)-1]
			year = afterNums[0]
			name = "DAY_MONTH_YEAR"
		}

	case len(beforeNums) == 0 && len(afterNums) == 1:
		// "March 2024" (month + year only) or "Mar 15" (month + day)
		if afterNums[0] > 31 {
			year = afterNums[0]
			day = 1
			name = "MONTH_YEAR"
		} else {
			day = afterNums[0]
			name = "MONTH_DAY"
		}

	case len(beforeNums) == 1 && len(afterNums) == 0:
		// "15 March" (day + month)
		day = beforeNums[0]
		name = "DAY_MONTH"

	default:
		return nil
	}

	if year < 100 && year > 0 {
		if year >= 69 {
			year += 1900
		} else {
			year += 2000
		}
	}

	// For textual month formats, we build a "virtual" program that uses
	// pre-resolved values. The fields here are positional markers but the
	// actual parsing re-scans the input. For Phase 1, we take a simpler approach:
	// build fields that extract components by re-parsing the input string.
	//
	// This is the "slow path" for first parse — the Layout is then reusable
	// for same-format strings where the month name position/length is the same.

	_ = day
	_ = year

	// For now, construct a FormatDef that will use the re-parse approach.
	// The compiler handles MonthName fields specially.
	def := &compile.FormatDef{
		Name:     name,
		GoLayout: goLayout,
		Fields:   fields,
	}

	// We need to build a proper field list by analyzing the actual positions.
	def.Fields = buildTextualFields(s, monthNum, monthStart, monthEnd)

	return &Result{Def: def}
}

// buildTextualFields constructs compile.Fields for a textual-month date string
// by scanning the actual byte positions.
func buildTextualFields(s string, monthNum int, monthStart, monthEnd int) []compile.Field {
	var fields []compile.Field

	// Scan for numeric tokens in the string, skipping the month name region.
	var nums []numToken

	i := 0
	for i < len(s) {
		if i >= monthStart && i < monthEnd {
			i = monthEnd
			continue
		}
		if s[i] >= '0' && s[i] <= '9' {
			start := i
			val := 0
			for i < len(s) && s[i] >= '0' && s[i] <= '9' {
				val = val*10 + int(s[i]-'0')
				i++
			}
			nums = append(nums, numToken{val, start, i})
		} else {
			i++
		}
	}

	// Add the month name field.
	fields = append(fields, compile.Field{
		Kind:   compile.FMonthName,
		Offset: monthStart,
		Len:    monthEnd - monthStart,
		Aux:    uint16(monthNum),
	})

	// Identify day and year from the numeric tokens.
	switch len(nums) {
	case 2:
		// Two numbers + month name. Smaller is day, larger is year (usually).
		n0, n1 := nums[0], nums[1]
		if n0.value > 31 || (n1.value <= 31 && n0.start > monthEnd) {
			// n0 is year, n1 is day — unlikely but handle it
			fields = append(fields, yearField(n0))
			fields = append(fields, dayField(n1))
		} else if n1.value > 31 {
			fields = append(fields, dayField(n0))
			fields = append(fields, yearField(n1))
		} else {
			// Both small — first before month is day, after month is year, or vice versa.
			if n0.start < monthStart {
				fields = append(fields, dayField(n0))
				fields = append(fields, yearField(n1))
			} else {
				fields = append(fields, dayField(n0))
				fields = append(fields, yearField(n1))
			}
		}

		// Check for time component after the date.
		afterLast := nums[len(nums)-1].end
		timeFields := parseTimeComponent(s, afterLast)
		fields = append(fields, timeFields...)

	case 1:
		n := nums[0]
		if n.value > 31 {
			fields = append(fields, yearField(n))
		} else {
			fields = append(fields, dayField(n))
		}
		// Check for time component after the number.
		timeFields := parseTimeComponent(s, n.end)
		fields = append(fields, timeFields...)

	case 0:
		// Month name only — unusual but valid.

	default:
		// 3+ numbers — could include day, year, and time components.
		// Check if nums look like "day time" (e.g. "15 10:30:00") or "day year time".
		n0, n1 := nums[0], nums[1]

		// Detect if nums[1:] form a time pattern (HH:MM or HH:MM:SS).
		// Check if n1 is followed by a colon and another number — classic time.
		isTimeAtN1 := n1.end < len(s) && s[n1.end] == ':' && len(nums) >= 3

		if isTimeAtN1 && n0.value <= 31 && n1.value <= 23 {
			// Pattern: "day time" (no year), e.g. "Mar 15 10:30:00"
			fields = append(fields, dayField(n0))
			// Time starts at n1, but parseTimeComponent expects to skip to HH:MM.
			// Position the start just before n1 so it finds the digits.
			timeFields := parseTimeComponent(s, n0.end)
			fields = append(fields, timeFields...)
		} else {
			if n1.value > 31 {
				fields = append(fields, dayField(n0))
				fields = append(fields, yearField(n1))
			} else if n0.value > 31 {
				fields = append(fields, yearField(n0))
				fields = append(fields, dayField(n1))
			} else {
				fields = append(fields, dayField(n0))
				fields = append(fields, yearField(n1))
			}

			// Remaining numbers are time components.
			afterSecondNum := nums[1].end
			timeFields := parseTimeComponent(s, afterSecondNum)
			fields = append(fields, timeFields...)
		}
	}

	return fields
}

func yearField(n numToken) compile.Field {
	kind := compile.FYear4
	if n.end-n.start <= 2 {
		kind = compile.FYear2
	}
	return compile.Field{Kind: kind, Offset: n.start, Len: n.end - n.start}
}

func dayField(n numToken) compile.Field {
	kind := compile.FDay2
	if n.end-n.start == 1 {
		kind = compile.FDay1or2
	}
	return compile.Field{Kind: kind, Offset: n.start, Len: n.end - n.start}
}

// parseTimeComponent looks for HH:MM or HH:MM:SS or HH:MM:SS.fff patterns
// starting from offset `from` in the string.
func parseTimeComponent(s string, from int) []compile.Field {
	var fields []compile.Field

	// Skip whitespace, punctuation, and colons to find time start.
	// Colons appear as time separators in CLF: "2024:10:30:00".
	i := from
	for i < len(s) && (s[i] == ' ' || s[i] == ',' || s[i] == '\t' || s[i] == ':') {
		i++
	}

	// Look for "at " prefix.
	if i+3 <= len(s) && strings.ToLower(s[i:i+3]) == "at " {
		i += 3
	}

	// Find HH:MM pattern.
	if i+5 <= len(s) && isDigit(s[i]) && isDigit(s[i+1]) && s[i+2] == ':' && isDigit(s[i+3]) && isDigit(s[i+4]) {
		fields = append(fields, compile.Field{Kind: compile.FHour24, Offset: i, Len: 2})
		fields = append(fields, compile.Field{Kind: compile.FMinute2, Offset: i + 3, Len: 2})

		j := i + 5
		// Check for :SS
		if j+3 <= len(s) && s[j] == ':' && isDigit(s[j+1]) && isDigit(s[j+2]) {
			fields = append(fields, compile.Field{Kind: compile.FSecond2, Offset: j + 1, Len: 2})
			j += 3

			// Check for .fractional
			if j+1 < len(s) && s[j] == '.' {
				fracStart := j + 1
				fracEnd := fracStart
				for fracEnd < len(s) && isDigit(s[fracEnd]) {
					fracEnd++
				}
				if fracEnd > fracStart {
					fields = append(fields, compile.Field{Kind: compile.FFracSec, Offset: fracStart, Len: fracEnd - fracStart})
					j = fracEnd
				}
			}
		}

		// Check for timezone.
		for j < len(s) && s[j] == ' ' {
			j++
		}

		if j < len(s) {
			if s[j] == 'Z' && (j+1 == len(s) || !isLetter(s[j+1])) {
				fields = append(fields, compile.Field{Kind: compile.FTZZ, Offset: j, Len: 1})
			} else if s[j] == '+' || s[j] == '-' {
				// Timezone offset.
				remaining := len(s) - j
				if remaining >= 5 {
					tzLen := 5
					if remaining >= 6 && s[j+3] == ':' {
						tzLen = 6
					}
					fields = append(fields, compile.Field{Kind: compile.FTZOffset, Offset: j, Len: tzLen})
				}
			} else if isLetter(s[j]) {
				// Timezone abbreviation.
				tzStart := j
				for j < len(s) && isLetter(s[j]) {
					j++
				}
				fields = append(fields, compile.Field{Kind: compile.FTZName, Offset: tzStart, Len: j - tzStart})
			}
		}

		// Check for AM/PM.
		k := i + 5
		if len(fields) > 2 { // has seconds
			k = fields[2].Offset + fields[2].Len
		}
		for k < len(s) && s[k] == ' ' {
			k++
		}
		if k+2 <= len(s) {
			upper0 := s[k] | 0x20
			upper1 := s[k+1] | 0x20
			if (upper0 == 'a' || upper0 == 'p') && upper1 == 'm' {
				// Replace the hour field with 12h variant.
				if len(fields) > 0 && fields[0].Kind == compile.FHour24 {
					fields[0].Kind = compile.FHour12
				}
				fields = append(fields, compile.Field{Kind: compile.FAMPM, Offset: k, Len: 2})
			}
		}
	}

	return fields
}

type numToken struct {
	value int
	start int
	end   int
}

// Month name lookup tables.
var monthNames = map[string]int{
	"january": 1, "jan": 1,
	"february": 2, "feb": 2,
	"march": 3, "mar": 3,
	"april": 4, "apr": 4,
	"may": 5,
	"june": 6, "jun": 6,
	"july": 7, "jul": 7,
	"august": 8, "aug": 8,
	"september": 9, "sep": 9, "sept": 9,
	"october": 10, "oct": 10,
	"november": 11, "nov": 11,
	"december": 12, "dec": 12,
}

// findMonthName finds the first month name in the lowercase string.
// Returns (month number 1-12, start index, end index) or (0, 0, 0) if not found.
func findMonthName(lower string) (int, int, int) {
	// Try longer names first to avoid partial matches.
	for i := 0; i < len(lower); i++ {
		if lower[i] < 'a' || lower[i] > 'z' {
			continue
		}
		// Try to match a month name starting at position i.
		for name, num := range monthNames {
			if i+len(name) <= len(lower) {
				candidate := lower[i : i+len(name)]
				if candidate == name {
					// Ensure it's a whole word.
					if (i == 0 || !isLetter(lower[i-1])) &&
						(i+len(name) == len(lower) || !isLetter(lower[i+len(name)])) {
						return num, i, i + len(name)
					}
				}
			}
		}
	}
	return 0, 0, 0
}

func hasLetter(s string) bool {
	for i := 0; i < len(s); i++ {
		if isLetter(s[i]) {
			return true
		}
	}
	return false
}

func isLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// extractNumbers extracts all decimal numbers from a string.
func extractNumbers(s string) []int {
	var nums []int
	i := 0
	for i < len(s) {
		if s[i] >= '0' && s[i] <= '9' {
			val := 0
			for i < len(s) && s[i] >= '0' && s[i] <= '9' {
				val = val*10 + int(s[i]-'0')
				i++
			}
			nums = append(nums, val)
		} else {
			i++
		}
	}
	return nums
}

func findSep(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '/' || s[i] == '-' || s[i] == '.' {
			return i
		}
	}
	return -1
}

func splitOnSep(s string, sep byte) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func parseSmallInt(s string) int {
	if len(s) == 0 || len(s) > 4 {
		return -1
	}
	val := 0
	for i := 0; i < len(s); i++ {
		d := s[i] - '0'
		if d > 9 {
			return -1
		}
		val = val*10 + int(d)
	}
	return val
}
