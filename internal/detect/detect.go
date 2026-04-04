package detect

import (
	"strings"
	"time"

	"github.com/kmoneil/dateparsa/internal/compile"
	"github.com/kmoneil/dateparsa/internal/locale"
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
	Locales         []*locale.Data // Locale data for month/day name lookup
}

// Detect analyzes a date string and returns the matching FormatDef.
// Returns ok=false if no structured format matches.
func Detect(s string, cfg Config) (Result, bool) {
	// Step 1: Try special formats that contain letters but aren't textual months.
	if r, ok := detectISOWeekOrOrdinal(s); ok {
		return r, true
	}

	// Step 1b: Try CJK ideographic dates before textual month (年月日).
	if r, ok := detectCJKDate(s); ok {
		return r, true
	}

	// Step 2: Try textual month formats if the string contains letters.
	if hasLetter(s) {
		if result, ok := detectTextualMonth(s, cfg); ok {
			return result, true
		}
	}

	// Step 3: Compute signature and walk the trie.
	sig := Scan(s)
	entry := globalTrie.lookup(&sig)
	if entry == nil {
		// Step 3b: Try ISO 8601 with variable-length fractional seconds.
		if r, ok := detectISO8601Frac(s); ok {
			return r, true
		}
		// Step 3c: Try variable-width numeric formats (e.g. 3/15/2024, 3/15/24).
		if r, ok := detectVariableNumeric(s, cfg); ok {
			return r, true
		}
		// Step 3c: Try Go time.String() output and other complex formats.
		if r, ok := detectGoTimeString(s); ok {
			return r, true
		}
		// Step 3d: Try date + tz offset without time (e.g. 2020-07-20+08:00).
		if r, ok := detectDatePlusTZ(s); ok {
			return r, true
		}
		// Step 3e: Try CJK ideographic dates (e.g. 2014年04月08日).
		if r, ok := detectCJKDate(s); ok {
			return r, true
		}
		return Result{}, false
	}

	// Step 3: Handle ambiguous formats.
	if entry.ambig {
		return resolveAmbiguous(s, entry, cfg)
	}

	// Step 4: Return the pre-built FormatDef (zero allocation for trie-matched formats).
	if entry.def != nil {
		return Result{Def: entry.def}, true
	}
	// Fallback for entries without pre-built defs.
	def := &compile.FormatDef{
		Name:     entry.name,
		GoLayout: entry.goLayout,
		Fields:   entry.fields,
	}
	return Result{Def: def}, true
}

// resolveAmbiguous handles DD/DD/DDDD type signatures where the format
// could be MM/DD/YYYY or DD/MM/YYYY.
// detectISO8601Frac handles ISO 8601/RFC 3339 with variable-length fractional seconds:
// "2024-03-15T10:30:00.123Z", "2024-03-15T10:30:00.123456+05:30",
// "2024-03-15 10:30:00.123456789Z", etc.
// Matches: YYYY-MM-DD[T ]HH:MM:SS.{1-9 digits}[Z|±HH:MM|±HHMM]
func detectISO8601Frac(s string) (Result, bool) {
	n := len(s)
	// Minimum: "YYYY-MM-DDTHH:MM:SS.fZ" = 22 chars
	if n < 22 {
		return Result{}, false
	}
	// Check the fixed prefix: YYYY-MM-DD[T ]HH:MM:SS.
	if !(isDigit(s[0]) && s[4] == '-' && s[7] == '-' &&
		(s[10] == 'T' || s[10] == ' ') &&
		s[13] == ':' && s[16] == ':' && s[19] == '.') {
		return Result{}, false
	}

	// Count fractional digits after the dot.
	fracStart := 20
	fracEnd := fracStart
	for fracEnd < n && isDigit(s[fracEnd]) {
		fracEnd++
	}
	fracLen := fracEnd - fracStart
	if fracLen < 1 || fracLen > 9 {
		return Result{}, false
	}

	fields := make([]compile.Field, 0, 10)
	fields = append(fields,
		compile.Field{Kind: compile.FYear4, Offset: 0, Len: 4},
		compile.Field{Kind: compile.FMonth2, Offset: 5, Len: 2},
		compile.Field{Kind: compile.FDay2, Offset: 8, Len: 2},
		compile.Field{Kind: compile.FHour24, Offset: 11, Len: 2},
		compile.Field{Kind: compile.FMinute2, Offset: 14, Len: 2},
		compile.Field{Kind: compile.FSecond2, Offset: 17, Len: 2},
		compile.Field{Kind: compile.FFracSec, Offset: fracStart, Len: fracLen},
	)

	// Parse timezone immediately after fractional seconds (no space).
	// If there's a space, bail — let detectGoTimeString handle it.
	pos := fracEnd
	if pos < n {
		if s[pos] == 'Z' && (pos+1 == n) {
			fields = append(fields, compile.Field{Kind: compile.FTZZ, Offset: pos, Len: 1})
		} else if s[pos] == '+' || s[pos] == '-' {
			tzLen := n - pos
			if tzLen == 5 || tzLen == 6 {
				fields = append(fields, compile.Field{Kind: compile.FTZOffset, Offset: pos, Len: tzLen})
			} else {
				return Result{}, false // complex tz — let other handlers deal with it
			}
		} else if s[pos] != ' ' {
			return Result{}, false // unexpected character after frac
		}
		// Space after frac = Go time.String or SQL+tz — fall through to other handlers.
	}

	def := &compile.FormatDef{Name: "ISO8601_FRAC", Fields: fields}
	return Result{Def: def}, true
}

// detectGoTimeString handles Go's time.Time.String() output:
// "2012-08-03 18:31:59.257000000 +0000 UTC"
// "2015-02-08 03:02:00 +0300 MSK"
// Pattern: YYYY-MM-DD HH:MM:SS[.nnnnnnnnn] +HHMM [TZName]
func detectGoTimeString(s string) (Result, bool) {
	n := len(s)
	// Minimum: "YYYY-MM-DD HH:MM:SS +HHMM" = 25 chars
	if n < 25 {
		return Result{}, false
	}
	// Must start with YYYY-MM-DD HH:MM:SS pattern.
	if !(isDigit(s[0]) && s[4] == '-' && s[7] == '-' && s[10] == ' ' &&
		s[13] == ':' && s[16] == ':') {
		return Result{}, false
	}

	fields := make([]compile.Field, 0, 10)
	fields = append(fields,
		compile.Field{Kind: compile.FYear4, Offset: 0, Len: 4},
		compile.Field{Kind: compile.FMonth2, Offset: 5, Len: 2},
		compile.Field{Kind: compile.FDay2, Offset: 8, Len: 2},
		compile.Field{Kind: compile.FHour24, Offset: 11, Len: 2},
		compile.Field{Kind: compile.FMinute2, Offset: 14, Len: 2},
		compile.Field{Kind: compile.FSecond2, Offset: 17, Len: 2},
	)

	pos := 19
	// Optional fractional seconds.
	if pos < n && s[pos] == '.' {
		fracStart := pos + 1
		fracEnd := fracStart
		for fracEnd < n && isDigit(s[fracEnd]) {
			fracEnd++
		}
		if fracEnd > fracStart {
			fields = append(fields, compile.Field{Kind: compile.FFracSec, Offset: fracStart, Len: fracEnd - fracStart})
			pos = fracEnd
		}
	}

	// Skip space.
	if pos < n && s[pos] == ' ' {
		pos++
	}

	// Timezone offset: +HHMM or +HH:MM
	if pos < n && (s[pos] == '+' || s[pos] == '-') {
		tzStart := pos
		pos++
		// Count digits and optional colon.
		digits := 0
		for pos < n && (isDigit(s[pos]) || s[pos] == ':') {
			pos++
			digits++
		}
		if digits >= 4 {
			fields = append(fields, compile.Field{Kind: compile.FTZOffset, Offset: tzStart, Len: pos - tzStart})
		}
	}

	// Skip timezone name and any trailing content (e.g. "m=+0.000000001").
	// We don't need to parse it — the offset is sufficient.

	def := &compile.FormatDef{Name: "GO_TIME_STRING", Fields: fields}
	return Result{Def: def}, true
}

// detectDatePlusTZ handles date + timezone offset without time:
// "2020-07-20+08:00"
func detectDatePlusTZ(s string) (Result, bool) {
	n := len(s)
	if n < 15 || n > 16 {
		return Result{}, false
	}
	// Must be YYYY-MM-DD±HH:MM
	if !(isDigit(s[0]) && s[4] == '-' && isDigit(s[5]) && s[7] == '-' && isDigit(s[8])) {
		return Result{}, false
	}
	if s[10] != '+' && s[10] != '-' {
		return Result{}, false
	}

	tzLen := n - 10
	fields := []compile.Field{
		{Kind: compile.FYear4, Offset: 0, Len: 4},
		{Kind: compile.FMonth2, Offset: 5, Len: 2},
		{Kind: compile.FDay2, Offset: 8, Len: 2},
		{Kind: compile.FTZOffset, Offset: 10, Len: tzLen},
	}
	def := &compile.FormatDef{Name: "ISO_DATE_TZ", Fields: fields}
	return Result{Def: def}, true
}

// detectCJKDate handles dates with CJK ideographic characters:
// "2014年04月08日" (year年month月day日)
func detectCJKDate(s string) (Result, bool) {
	// Look for 年 (U+5E74), 月 (U+6708), 日 (U+65E5)
	// In UTF-8: 年=E5B9B4, 月=E69C88, 日=E697A5
	yearIdx := strings.Index(s, "年")
	monthIdx := strings.Index(s, "月")
	dayIdx := strings.Index(s, "日")

	if yearIdx < 0 || monthIdx < 0 || dayIdx < 0 {
		return Result{}, false
	}
	if yearIdx >= monthIdx || monthIdx >= dayIdx {
		return Result{}, false
	}

	// Extract year (digits before 年).
	yearStr := s[:yearIdx]
	if len(yearStr) != 4 {
		return Result{}, false
	}

	// Extract month (digits between 年 and 月).
	monStr := s[yearIdx+len("年") : monthIdx]
	// Extract day (digits between 月 and 日).
	dayStr := s[monthIdx+len("月") : dayIdx]

	// Validate they're all digits.
	for _, c := range []byte(yearStr) {
		if !isDigit(c) {
			return Result{}, false
		}
	}
	for _, c := range []byte(monStr) {
		if !isDigit(c) {
			return Result{}, false
		}
	}
	for _, c := range []byte(dayStr) {
		if !isDigit(c) {
			return Result{}, false
		}
	}

	// Build fields using byte offsets.
	fields := []compile.Field{
		{Kind: compile.FYear4, Offset: 0, Len: len(yearStr)},
		{Kind: compile.FMonth1or2, Offset: yearIdx + len("年"), Len: len(monStr)},
		{Kind: compile.FDay1or2, Offset: monthIdx + len("月"), Len: len(dayStr)},
	}
	def := &compile.FormatDef{Name: "CJK_DATE", Fields: fields}
	return Result{Def: def}, true
}

// detectVariableNumeric handles numeric dates with variable-width fields:
// "3/15/2024", "3/15/24", "3/15/2024 10:30:00 AM", etc.
// Recognizes patterns of 2-3 numeric parts separated by / or -.
func detectVariableNumeric(s string, cfg Config) (Result, bool) {
	if len(s) < 3 {
		return Result{}, false
	}

	// Find the separator (must be / or -).
	sepIdx := -1
	for i := 0; i < len(s) && i < 4; i++ {
		if s[i] == '/' || s[i] == '-' {
			sepIdx = i
			break
		}
	}
	if sepIdx < 1 {
		return Result{}, false
	}
	sep := s[sepIdx]

	// Split the date portion (stop at space for compound formats).
	dateEnd := len(s)
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' || s[i] == '\t' {
			dateEnd = i
			break
		}
	}
	datePart := s[:dateEnd]

	parts := splitOnSep(datePart, sep)
	if len(parts) < 2 || len(parts) > 3 {
		return Result{}, false
	}

	// Validate all parts are numeric and reasonable length.
	for _, p := range parts {
		if len(p) == 0 || len(p) > 4 {
			return Result{}, false
		}
		for j := 0; j < len(p); j++ {
			if p[j] < '0' || p[j] > '9' {
				return Result{}, false
			}
		}
	}

	// For 2-part dates, not a full date. Skip.
	if len(parts) < 3 {
		return Result{}, false
	}

	// Use resolveAmbiguous logic on the date portion.
	// But first, verify this looks like a numeric date, not an ISO date
	// (ISO dates are 4-digit year first with - separator, already handled by trie).
	if sep == '-' && len(parts[0]) == 4 {
		return Result{}, false // YYYY-MM-DD handled by trie
	}

	// Dot separator with 4-digit last part also handled by trie.
	if sep == '.' {
		return Result{}, false
	}

	// Build the result using resolveAmbiguous on the date portion.
	ambigResult, ok := resolveAmbiguous(datePart, nil, cfg)
	if !ok {
		return Result{}, false
	}

	// If there's a time component after the date, parse it.
	if dateEnd < len(s) {
		timeFields := parseTimeComponent(s, dateEnd)
		if len(timeFields) > 0 {
			combined := make([]compile.Field, 0, len(ambigResult.Def.Fields)+len(timeFields))
			combined = append(combined, ambigResult.Def.Fields...)
			combined = append(combined, timeFields...)
			ambigResult.Def = &compile.FormatDef{
				Name:   ambigResult.Def.Name + "_TIME",
				Fields: combined,
			}
		}
	}

	return ambigResult, true
}

func resolveAmbiguous(s string, entry *formatEntry, cfg Config) (Result, bool) {
	// Parse the first two numeric components.
	sep := findSep(s)
	if sep < 0 {
		return Result{}, false
	}
	sepChar := s[sep]

	// Dot separator strongly implies European DMY convention.
	if sepChar == '.' {
		cfg.PreferDayFirst = true
	}

	parts := splitOnSep(s, sepChar)

	if len(parts) < 3 {
		return Result{}, false
	}

	first := parseSmallInt(parts[0])
	second := parseSmallInt(parts[1])
	third := parseSmallInt(parts[2])

	if first < 0 || second < 0 || third < 0 {
		return Result{}, false
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
		return Result{}, false
	}

	fields := make([]compile.Field, 0, 8)

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
	return Result{Def: def, Ambig: ambig}, true
}

// detectTextualMonth handles formats like "March 15, 2024", "15 Mar 2024",
// "Mar 15, 2024", "Fri, 15 Mar 2024 10:30:00 +0000" (RFC 2822).
// detectISOWeekOrOrdinal detects ISO week dates (2024-W11-5) and ordinal dates (2024-074).
func detectISOWeekOrOrdinal(s string) (Result, bool) {
	n := len(s)

	// ISO week date: YYYY-Www-D (10 chars) or YYYY-Www (8 chars)
	// Pattern: 4 digits, '-', 'W', 2 digits, optionally '-', 1 digit
	if n >= 8 && s[4] == '-' && (s[5] == 'W' || s[5] == 'w') {
		if n == 8 && isDigit(s[6]) && isDigit(s[7]) {
			// YYYY-Www (week only, assume day 1=Monday)
			return Result{Def: &compile.FormatDef{
				Name: "ISO_WEEK",
				Fields: []compile.Field{
					{Kind: compile.FYear4, Offset: 0, Len: 4},
					{Kind: compile.FISOWeek, Offset: 6, Len: 2},
				},
			}}, true
		}
		if n == 10 && isDigit(s[6]) && isDigit(s[7]) && s[8] == '-' && isDigit(s[9]) {
			// YYYY-Www-D
			return Result{Def: &compile.FormatDef{
				Name: "ISO_WEEK_DATE",
				Fields: []compile.Field{
					{Kind: compile.FYear4, Offset: 0, Len: 4},
					{Kind: compile.FISOWeek, Offset: 6, Len: 2},
					{Kind: compile.FISOWeekDay, Offset: 9, Len: 1},
				},
			}}, true
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
				return Result{Def: &compile.FormatDef{
					Name: "ISO_ORDINAL",
					Fields: []compile.Field{
						{Kind: compile.FYear4, Offset: 0, Len: 4},
						{Kind: compile.FOrdinalDay, Offset: 5, Len: 3},
					},
				}}, true
			}
		}
	}

	return Result{}, false
}

func detectTextualMonth(s string, cfg Config) (Result, bool) {
	// Find month name using case-insensitive matching directly on the input.
	// No strings.ToLower allocation needed.
	monthNum, monthStart, monthEnd := findMonthNameCI(s, cfg.Locales)
	if monthNum == 0 {
		return Result{}, false
	}

	// Extract the rest and figure out the structure.
	// Strategy: remove the month name, find the remaining numeric components.
	before := strings.TrimSpace(s[:monthStart])
	after := strings.TrimSpace(s[monthEnd:])

	// If the after-month portion contains " at " (NL time indicator like
	// "december 25th at 5pm") and there's no explicit 4-digit year, bail
	// so the NL parser can handle it properly.
	afterLower := strings.ToLower(after)
	if strings.Contains(afterLower, " at ") && !hasFourDigitYear(after) {
		return Result{}, false
	}

	// Handle "March 15, 2024" or "Mar 15, 2024"
	// after = "15, 2024"
	// Handle "15 March 2024" or "15 Mar 2024"
	// before = "15"
	// Handle "Fri, 15 Mar 2024 10:30:00 +0000" (RFC 2822)

	fields := make([]compile.Field, 0, 6)
	var name string
	goLayout := ""

	// Month is already resolved.
	fields = append(fields, compile.Field{
		Kind: compile.FMonthName,
		Aux:  uint16(monthNum),
	})

	// Parse numbers from before and after the month name.
	beforeNums := extractNumbers(before)
	afterStr := strings.TrimLeft(after, ", ")
	// Stop at "at" only when it would misinterpret a bare time number as year.
	// e.g. "25th at 5pm" → trim to "25th" (1 number, no year).
	// But NOT "17, 2012 at 10:09am" → keep as "17, 2012 at 10:09am" (has year).
	afterForNums := afterStr
	if atIdx := strings.Index(strings.ToLower(afterStr), " at "); atIdx >= 0 {
		textBeforeAt := afterStr[:atIdx]
		numsBeforeAt := extractNumbers(textBeforeAt)
		if len(numsBeforeAt) <= 1 || !hasFourDigitYear(textBeforeAt) {
			afterForNums = afterStr[:atIdx]
		}
	}
	afterNums := extractNumbers(afterForNums)

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
		return Result{}, false
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

	return Result{Def: def}, true
}

// buildTextualFields constructs compile.Fields for a textual-month date string
// by scanning the actual byte positions.
func buildTextualFields(s string, monthNum int, monthStart, monthEnd int) []compile.Field {
	// Use stack-allocated fixed-size arrays to avoid heap allocations.
	fields := make([]compile.Field, 0, 10)

	// Scan for numeric tokens in the string, skipping the month name region.
	nums := make([]numToken, 0, 6)

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
			// Pattern: "day time [year]", e.g. "Mar 15 10:30:00 2024"
			fields = append(fields, dayField(n0))
			timeFields := parseTimeComponent(s, n0.end)
			fields = append(fields, timeFields...)

			// Look for a trailing year after the time+tz fields.
			// Scan remaining nums for a 4-digit number (year).
			for _, num := range nums {
				if num.start > n0.end && num.value >= 1000 && num.value <= 9999 {
					// Check this number isn't already part of the time fields.
					isTimeField := false
					for _, tf := range timeFields {
						if int(tf.Offset) == num.start {
							isTimeField = true
							break
						}
					}
					if !isTimeField {
						fields = append(fields, yearField(num))
						break
					}
				}
			}
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
	fields := make([]compile.Field, 0, 8)

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

		// Check for AM/PM first (before timezone, since "AM"/"PM" would be
		// misidentified as timezone abbreviations).
		ampmHandled := false
		if j+2 <= len(s) {
			c0 := s[j] | 0x20
			c1 := s[j+1] | 0x20
			if (c0 == 'a' || c0 == 'p') && c1 == 'm' && (j+2 == len(s) || !isLetter(s[j+2])) {
				if len(fields) > 0 && fields[0].Kind == compile.FHour24 {
					fields[0].Kind = compile.FHour12
				}
				fields = append(fields, compile.Field{Kind: compile.FAMPM, Offset: j, Len: 2})
				ampmHandled = true
			}
		}

		if !ampmHandled && j < len(s) {
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
	}

	return fields
}

type numToken struct {
	value int
	start int
	end   int
}

// English month names — fallback when no locale is specified.
var defaultMonthNames = map[string]int{
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

// findMonthName finds the first month name in the lowercase string,
// searching both the default English names and any locale-specific names.
// Returns (month number 1-12, start index, end index) or (0, 0, 0) if not found.
// findMonthNameCI is like findMonthName but does case-insensitive matching
// directly on the input string without allocating a lowered copy.
func findMonthNameCI(s string, locales []*locale.Data) (int, int, int) {
	// Search English names (case-insensitive).
	for name, num := range defaultMonthNames {
		if idx, end, ok := matchWordCI(s, name); ok {
			return num, idx, end
		}
	}
	// Search locale-specific names.
	for _, loc := range locales {
		for i := 0; i < 12; i++ {
			if name := loc.MonthsWide[i]; name != "" {
				if idx, end, ok := matchWordCI(s, name); ok {
					return i + 1, idx, end
				}
			}
			if name := loc.MonthsAbbr[i]; name != "" {
				if idx, end, ok := matchWordCI(s, name); ok {
					return i + 1, idx, end
				}
				// Try without trailing dot.
				clean := strings.TrimRight(name, ".")
				if clean != name {
					if idx, end, ok := matchWordCI(s, clean); ok {
						return i + 1, idx, end
					}
				}
			}
		}
	}
	return 0, 0, 0
}

// matchWordCI finds `word` as a whole word in `s`, case-insensitive.
// Returns (start, end, true) on match, or (0, 0, false).
func matchWordCI(s, word string) (int, int, bool) {
	wlen := len(word)
	if wlen == 0 || wlen > len(s) {
		return 0, 0, false
	}
	for i := 0; i <= len(s)-wlen; i++ {
		if !equalsFoldASCII(s[i:i+wlen], word) {
			continue
		}
		// Check word boundaries.
		if (i == 0 || !isWordChar(s[i-1])) &&
			(i+wlen == len(s) || !isWordChar(s[i+wlen])) {
			return i, i + wlen, true
		}
	}
	return 0, 0, false
}

// equalsFoldASCII compares two strings case-insensitively for ASCII.
// For non-ASCII bytes, compares exact. Both strings must be same length.
func equalsFoldASCII(a, b string) bool {
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca == cb {
			continue
		}
		// ASCII case fold.
		if ca >= 'A' && ca <= 'Z' {
			ca += 0x20
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 0x20
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// searchMonthMap scans for any month name from the map in the string.
// isWordChar returns true for characters that can be part of a word
// (letters, including non-ASCII bytes which may be part of UTF-8 sequences).
func isWordChar(c byte) bool {
	return isLetter(c) || c >= 0x80
}

func hasLetter(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 0x80 {
			// Non-ASCII byte — could be UTF-8 letter (Cyrillic, etc.)
			return true
		}
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' {
			// Skip 'T' and 'Z' when they appear in ISO-like positions.
			if c == 'T' && i > 0 && i < len(s)-1 && isDigit(s[i-1]) && isDigit(s[i+1]) {
				continue
			}
			if c == 'Z' && (i == len(s)-1 || (i < len(s)-1 && (s[i+1] == '+' || s[i+1] == '-'))) {
				continue
			}
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

// hasFourDigitYear checks if a string contains a 4-digit number (likely a year).
func hasFourDigitYear(s string) bool {
	i := 0
	for i < len(s) {
		if s[i] >= '0' && s[i] <= '9' {
			start := i
			for i < len(s) && s[i] >= '0' && s[i] <= '9' {
				i++
			}
			if i-start == 4 {
				return true
			}
		} else {
			i++
		}
	}
	return false
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
