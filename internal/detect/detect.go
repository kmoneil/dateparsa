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
	Def *compile.FormatDef

	// Ambig reports that THIS input needed a guess: "01/02/2024" could be
	// either reading and the configured preference picked one.
	Ambig bool

	// AmbigProne reports that the FORMAT can need a guess, whether or not this
	// input did. "25/12/2024" is unambiguous because 25 is not a month, but it
	// is the same DD/DD/DDDD shape as "01/02/2024" and resolves by value.
	//
	// The distinction exists because Parser caches a layout and reuses it
	// without re-detecting. Ambiguity belongs to the input, so a layout cannot
	// answer it for the next value; what a layout can carry is whether the
	// question arises at all. Strict mode uses this to decline the cache.
	AmbigProne bool
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

	// Step 2: Compute signature and walk the trie. The signature scan also
	// detects whether the input contains letters (sig.HasLetter), eliminating
	// the need for a separate hasLetter pass over the input.
	sig := Scan(s)
	entry := globalTrie.lookup(&sig)

	// Step 2b: If the trie missed and the string contains letters, try textual month.
	// This is ordered after the trie so that formats with timezone names (e.g.
	// "2024-03-15 10:30:00 UTC") are matched by the trie first.
	//
	// sig.HasLetter alone was the condition, and it is computed from a pass that
	// stops at maxSigLen, so it means "has a letter in the first 64 bytes". A
	// date behind 64 bytes of anything else never reached this detector at all:
	// Parse(strings.Repeat(" ", 64) + "March 15, 2024") fell through to natural
	// language, which read "March 15" and answered with the base year rather
	// than the 2024 in front of it. See hasLetterPastSignature for why the
	// second look lives here and not in Scan.
	if entry == nil && (sig.HasLetter || hasLetterPastSignature(s)) {
		if result, ok := detectTextualMonth(s, cfg); ok {
			return result, true
		}
	}

	if entry == nil {
		// Step 3b: Try ISO 8601 with variable-length fractional seconds.
		if r, ok := detectISO8601Frac(s); ok {
			return r, true
		}
		// Step 3c: Try variable-width numeric formats (e.g. 3/15/2024, 3/15/24).
		if r, ok := detectVariableNumeric(s, cfg); ok {
			return r, true
		}
		// Step 3d: Try Go time.String() output and other complex formats.
		if r, ok := detectGoTimeString(s); ok {
			return r, true
		}
		// Step 3e: Try date + tz offset without time (e.g. 2020-07-20+08:00).
		if r, ok := detectDatePlusTZ(s); ok {
			return r, true
		}
		return Result{}, false
	}

	// Step 4: Handle ambiguous formats.
	if entry.ambig {
		return resolveAmbiguous(s, cfg)
	}

	// Step 5: Return the pre-built FormatDef (zero allocation for trie-matched formats).
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

// lit returns a one-byte literal field carrying the byte the detector checked,
// so the executor can check it too. A separator is what tells two formats
// apart: ISO_ORDINAL described its year and its day-of-year and nothing at all
// for the '-' between them, so a layout built from "0000-001" accepted the
// compact date "00000101" and read its "0101" as day-of-year 101.
func lit(s string, off int) compile.Field {
	return compile.Field{Kind: compile.FLiteral, Offset: off, Len: 1, Aux: uint16(s[off])}
}

// coverGaps appends a skip for every run of bytes no field reads, so the
// program accounts for the whole input.
//
// The textual formats locate their fields by scanning, so what sits between
// them is punctuation, a weekday name, or an " at ", and none of it is read.
// Width is still worth fixing: a run of a different length shifts every field
// after it, which is how a layout built from "Fri Jul 03 2015" would land its
// day field in the middle of "Wednesday". Content is not worth checking here,
// unlike the separators above, because nothing else parses these bytes
// differently: "March 15; 2024" has only one reading whatever the punctuation.
func coverGaps(fields []compile.Field, s string) []compile.Field {
	covered := make([]bool, len(s))
	for _, f := range fields {
		w := f.Len
		if fw, fixed := compile.FixedWidth(f.Kind); fixed {
			w = fw
		}
		for i := f.Offset; i < f.Offset+w && i < len(s); i++ {
			covered[i] = true
		}
	}
	for i := 0; i < len(s); {
		if covered[i] {
			i++
			continue
		}
		start := i
		for i < len(s) && !covered[i] {
			i++
		}
		fields = append(fields, skip(start, i-start))
	}
	return fields
}

// skip covers a run the format does not read and does not constrain, such as
// the weekday name of an RFC 2822 date. It fixes the run's width without
// looking at it, which is what stops a wider one shifting every field after it.
func skip(off, length int) compile.Field {
	return compile.Field{Kind: compile.FSkip, Offset: off, Len: length}
}

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
		lit(s, 4),
		compile.Field{Kind: compile.FMonth2, Offset: 5, Len: 2},
		lit(s, 7),
		compile.Field{Kind: compile.FDay2, Offset: 8, Len: 2},
		lit(s, 10),
		compile.Field{Kind: compile.FHour24, Offset: 11, Len: 2},
		lit(s, 13),
		compile.Field{Kind: compile.FMinute2, Offset: 14, Len: 2},
		lit(s, 16),
		compile.Field{Kind: compile.FSecond2, Offset: 17, Len: 2},
		lit(s, 19),
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
		} else {
			// Anything else after the fraction, a space included, is not this
			// format. A space means a Go time.String or a SQL value with a
			// separated zone, and detectGoTimeString reads the offset those
			// carry. Returning a def covering only the prefix here read
			// "2012-08-03 18:31:59.257000000 +0300 MSK" as UTC, three hours
			// off, and did it only when a fraction was present: the same value
			// without one reached detectGoTimeString and came out right.
			return Result{}, false
		}
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
		lit(s, 4),
		compile.Field{Kind: compile.FMonth2, Offset: 5, Len: 2},
		lit(s, 7),
		compile.Field{Kind: compile.FDay2, Offset: 8, Len: 2},
		lit(s, 10),
		compile.Field{Kind: compile.FHour24, Offset: 11, Len: 2},
		lit(s, 13),
		compile.Field{Kind: compile.FMinute2, Offset: 14, Len: 2},
		lit(s, 16),
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
			fields = append(fields, lit(s, pos),
				compile.Field{Kind: compile.FFracSec, Offset: fracStart, Len: fracEnd - fracStart})
			pos = fracEnd
		}
	}

	// Skip space.
	if pos < n && s[pos] == ' ' {
		fields = append(fields, lit(s, pos))
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

	// The timezone name and any trailing content ("m=+0.000000001") are
	// ignored: the offset above already fixes the instant. FTail records that
	// as a decision the program carries, rather than leaving it to the
	// executor not to check.
	if pos < n {
		fields = append(fields, compile.Field{Kind: compile.FTail, Offset: pos})
	}

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
		lit(s, 4),
		{Kind: compile.FMonth2, Offset: 5, Len: 2},
		lit(s, 7),
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
	if !allDigits(yearStr) || !allDigits(monStr) || !allDigits(dayStr) {
		return Result{}, false
	}

	// Build fields using byte offsets.
	fields := []compile.Field{
		{Kind: compile.FYear4, Offset: 0, Len: len(yearStr)},
		skip(yearIdx, len("年")),
		{Kind: compile.FMonth1or2, Offset: yearIdx + len("年"), Len: len(monStr)},
		skip(monthIdx, len("月")),
		{Kind: compile.FDay1or2, Offset: monthIdx + len("月"), Len: len(dayStr)},
		skip(dayIdx, len("日")),
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
	ambigResult, ok := resolveAmbiguous(datePart, cfg)
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
				Fields: coverGaps(combined, s),
			}
		}
	}

	return ambigResult, true
}

// datePart holds a parsed component of an ambiguous date string with its position.
type datePart struct {
	value  int
	offset int
	length int
}

// resolveAmbiguous handles DD/DD/DDDD type signatures where the format
// could be MM/DD/YYYY or DD/MM/YYYY.
func resolveAmbiguous(s string, cfg Config) (Result, bool) {
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

	monthPart, dayPart, ambig, ok := resolveYearMonthDay(parts, first, second, third, cfg)
	if !ok {
		return Result{}, false
	}

	fields, ok := buildDatePartFields(parts, monthPart, dayPart)
	if !ok {
		return Result{}, false
	}

	name := "NUMERIC_AMBIG"
	if !ambig {
		if cfg.PreferDayFirst {
			name = "NUMERIC_DMY"
		} else {
			name = "NUMERIC_MDY"
		}
	}
	def := &compile.FormatDef{Name: name, Fields: fields}
	return Result{Def: def, Ambig: ambig, AmbigProne: true}, true
}

// resolveYearMonthDay determines which parts are year, month, and day,
// and whether the result is ambiguous.
// resolveYearMonthDay decides which of three numeric parts is the month and
// which is the day, given where the year sits.
//
// It used to return the year as well, and the only caller wrote
// "_ = year // year value used for validation only" and dropped it. Nothing
// validated it, here or anywhere: the fields the caller builds read the year
// out of the input at run time, which is the whole design. The year is still
// identified, because that is what leaves two parts to choose between, but it
// is identified rather than computed.
func resolveYearMonthDay(parts []string, first, second, third int, cfg Config) (month, day datePart, ambig bool, ok bool) {
	// Step 1: Identify year position.
	var v1, v2 int
	var v1Offset, v2Offset int

	if third > 31 || len(parts[2]) == 4 {
		// Year is last: ??/??/YYYY
		v1, v2 = first, second
		v1Offset = 0
		v2Offset = len(parts[0]) + 1
	} else if first > 31 || len(parts[0]) == 4 {
		// Year is first: YYYY/??/??
		v1, v2 = second, third
		v1Offset = len(parts[0]) + 1
		v2Offset = len(parts[0]) + 1 + len(parts[1]) + 1
	} else {
		// All small numbers, truly ambiguous with 2-digit year last.
		v1, v2 = first, second
		v1Offset = 0
		v2Offset = len(parts[0]) + 1
	}

	// Step 2: Resolve month vs day from the two non-year parts.
	p1 := datePart{v1, v1Offset, len(parts[partIndex(v1Offset, parts)])}
	p2 := datePart{v2, v2Offset, len(parts[partIndex(v2Offset, parts)])}

	if v1 > 12 {
		// First must be day.
		day, month = p1, p2
	} else if v2 > 12 {
		// Second must be day.
		month, day = p1, p2
	} else {
		// Genuinely ambiguous — both could be month or day.
		ambig = true
		if cfg.PreferDayFirst {
			day, month = p1, p2
		} else {
			month, day = p1, p2
		}
	}

	if month.value < 1 || month.value > 12 || day.value < 1 || day.value > 31 {
		return datePart{}, datePart{}, false, false
	}

	// A day or a month is written with one digit or two, never three. The
	// field kinds below can only express those two widths, so a wider part
	// produced a field whose declared width and read width disagreed:
	// buildDatePartFields gave "020" an FDay2 with Len 3, OpDay2 read the
	// first two bytes, and Parse("020/01/2024") came back as the second of
	// January having validated the twentieth.
	if month.length > 2 || day.length > 2 {
		return datePart{}, datePart{}, false, false
	}
	return month, day, ambig, true
}

// partIndex returns which index (0, 1, or 2) a given byte offset corresponds to
// in a 3-part date string split by separators.
func partIndex(offset int, parts []string) int {
	pos := 0
	for i, p := range parts {
		if pos == offset {
			return i
		}
		pos += len(p) + 1 // +1 for separator
	}
	return 0
}

// buildDatePartFields constructs compile.Fields for a 3-part date, sorting by
// input position and inserting literal separator fields between them.
func buildDatePartFields(parts []string, month, day datePart) ([]compile.Field, bool) {
	// Determine year part by elimination: whichever offset is not month or day.
	// Both are assigned on every path below, so neither carries an initializer.
	var yearOffset, yearLen int
	p1End := len(parts[0]) + 1
	p2End := p1End + len(parts[1]) + 1
	if month.offset == 0 || day.offset == 0 {
		if month.offset != 0 && day.offset != 0 {
			yearOffset = 0
			yearLen = len(parts[0])
		} else if month.offset == 0 {
			if day.offset == p1End {
				yearOffset = p2End
				yearLen = len(parts[2])
			} else {
				yearOffset = p1End
				yearLen = len(parts[1])
			}
		} else {
			if month.offset == p1End {
				yearOffset = p2End
				yearLen = len(parts[2])
			} else {
				yearOffset = p1End
				yearLen = len(parts[1])
			}
		}
	} else {
		yearOffset = 0
		yearLen = len(parts[0])
	}

	// A year here is written with exactly two digits or exactly four, because
	// those are the only widths FYear2 and FYear4 read. Anything else produced
	// a field whose declared width and read width disagreed, which is the same
	// defect as the over-wide day and month parts refused above. Those two
	// happened to refuse anyway, by running off the end of the input, which is
	// luck rather than a check.
	var yearKind compile.FieldKind
	switch yearLen {
	case 4:
		yearKind = compile.FYear4
	case 2:
		yearKind = compile.FYear2
	default:
		return nil, false
	}
	monthKind := compile.FMonth2
	if month.length == 1 {
		monthKind = compile.FMonth1or2
	}
	dayKind := compile.FDay2
	if day.length == 1 {
		dayKind = compile.FDay1or2
	}

	type posField struct {
		offset int
		field  compile.Field
	}
	infos := [3]posField{
		{yearOffset, compile.Field{Kind: yearKind, Offset: yearOffset, Len: yearLen}},
		{month.offset, compile.Field{Kind: monthKind, Offset: month.offset, Len: month.length}},
		{day.offset, compile.Field{Kind: dayKind, Offset: day.offset, Len: day.length}},
	}

	// Sort 3 elements by offset (simple conditional swaps).
	if infos[0].offset > infos[1].offset {
		infos[0], infos[1] = infos[1], infos[0]
	}
	if infos[1].offset > infos[2].offset {
		infos[1], infos[2] = infos[2], infos[1]
	}
	if infos[0].offset > infos[1].offset {
		infos[0], infos[1] = infos[1], infos[0]
	}

	// Build fields with literal separators between them.
	fields := make([]compile.Field, 0, 8)
	prevEnd := 0
	for _, info := range infos {
		if info.offset > prevEnd {
			fields = append(fields, compile.Field{Kind: compile.FLiteral, Offset: prevEnd, Len: info.offset - prevEnd})
		}
		fields = append(fields, info.field)
		prevEnd = info.offset + info.field.Len
	}
	return fields, true
}

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
					lit(s, 4), lit(s, 5),
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
					lit(s, 4), lit(s, 5),
					{Kind: compile.FISOWeek, Offset: 6, Len: 2},
					lit(s, 8),
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
						lit(s, 4),
						{Kind: compile.FOrdinalDay, Offset: 5, Len: 3},
					},
				}}, true
			}
		}
	}

	return Result{}, false
}

// indexFoldASCII returns the byte index of the first occurrence of sub in s,
// folding ASCII case, or -1 if sub is not present.
//
// It exists because an index taken from a strings.ToLower copy is not valid in
// the original. Lowering can change a string's byte length: every byte of
// invalid UTF-8 lowers to a three-byte U+FFFD, and a few real runes grow too
// (U+0130 becomes two runes). trimAtSuffix used to search the lowered copy and
// slice the original with the result, which panicked with a slice bounds error
// on "dEC0000A\xbe\xc2\xd0 At 0" and on any other input carrying an invalid
// byte before an " at ". Both needles here are ASCII, so folding ASCII on the
// input is exactly equivalent to the search it replaces, minus the copy.
func indexFoldASCII(s, sub string) int {
	if len(sub) == 0 {
		return 0
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if equalsFoldASCII(s[i:i+len(sub)], sub) {
			return i
		}
	}
	return -1
}

// trimAtSuffix strips the " at ..." portion from a string when it would
// cause a bare time number to be misidentified as a year.
// e.g. "25th at 5pm" → "25th" (no year present, trim to avoid confusion).
// But "17, 2012 at 10:09am" → unchanged (4-digit year present, keep for parsing).
func trimAtSuffix(s string) string {
	atIdx := indexFoldASCII(s, " at ")
	if atIdx < 0 {
		return s
	}
	textBeforeAt := s[:atIdx]
	numsBeforeAt := extractNumbers(textBeforeAt)
	if len(numsBeforeAt) <= 1 || !hasFourDigitYear(textBeforeAt) {
		return textBeforeAt
	}
	return s
}

// detectTextualMonth handles formats with named months:
// "March 15, 2024", "15 Mar 2024", "Mar 15, 2024",
// "Fri, 15 Mar 2024 10:30:00 +0000" (RFC 2822), "March 2024", "15 March".
func detectTextualMonth(s string, cfg Config) (Result, bool) {
	monthNum, monthStart, monthEnd := findMonthNameCI(s, cfg.Locales)
	if monthNum == 0 {
		return Result{}, false
	}

	// If the post-month text contains " at " without a 4-digit year,
	// this is a NL expression like "december 25th at 5pm" — bail so the
	// NL parser handles it.
	after := strings.TrimSpace(s[monthEnd:])
	if indexFoldASCII(after, " at ") >= 0 && !hasFourDigitYear(after) {
		return Result{}, false
	}

	// Classify the surrounding structure to determine the format name.
	name := classifyTextualPattern(s, monthStart, monthEnd)
	if name == "" {
		return Result{}, false
	}

	// Build field list from actual byte positions.
	fields := coverGaps(buildTextualFields(s, monthNum, monthStart, monthEnd), s)
	def := &compile.FormatDef{Name: name, Fields: fields}
	prone, guess := textualDayIsAGuess(fields)
	return Result{Def: def, Ambig: guess, AmbigProne: prone}, true
}

// textualDayIsAGuess reports whether the number this format read as a day could
// equally have been read as a two-digit year.
//
// A bare month and number is classified by value: over 31 it is a year, at or
// under 31 it is a day. So "MAY70" is May 1970 and "MAY10" is the tenth of May,
// and nothing said the second was a choice. A caller who cached the layout from
// the first row of a column then got 2010-05-01 out of "MAY10" while Parse gave
// 2026-05-10, with Ambiguous false on both.
//
// The width of the number is what separates a guess from a certainty:
//
//	"March 5"      one digit, and no year is written with one    day
//	"March 15"     could be the fifteenth, could be 2015         GUESS
//	"March 32"     over 31, so not a day                         year
//	"March 2015"   four digits                                   year
//
// Two digits is the whole of the ambiguous set, because NormalizeTwoDigitYear
// maps every two-digit value to some year, so "could be a year" excludes
// nothing that "could be a day" admits.
// It returns prone, meaning the day-or-year question arises for this shape at
// all, and guess, meaning this input actually needed it answered.
func textualDayIsAGuess(fields []compile.Field) (prone, guess bool) {
	for i := range fields {
		switch fields[i].Kind {
		case compile.FYear2:
			// A year is written out, so the other number is not one, and this
			// input needed no guess. The format still did: buildTextualFields
			// calls a bare number over 31 a year and one at or under 31 a day,
			// and both emit a two-byte field at the same offset. The programs
			// are the same shape, so a layout built under one reading accepts
			// input that wanted the other and answers with the wrong instant.
			//
			// "MAY70" builds a year field and reads "MAY10" as 2010-05-01,
			// where detection reads the tenth of May. "March 32" reads
			// "March 31" as 2031-03-01 against the thirty-first of March. Both
			// used to come back with Ambiguous false and no error, because
			// prone was false here and Parser reuses a layout it is not told to
			// re-detect.
			return true, false
		case compile.FYear4:
			// Four digits cannot be a day, so nothing about this shape is
			// decided by value and no later row can flip it.
			return false, false
		case compile.FDay1or2:
			prone = true
		case compile.FDay2:
			prone = true
			if fields[i].Len == 2 {
				guess = true
			}
		}
	}
	return prone, guess
}

// classifyTextualPattern determines the format name based on how numbers
// are arranged around the month name. This decides whether the format is
// "MONTH_DAY_YEAR", "DAY_MONTH_YEAR", "MONTH_YEAR", "MONTH_DAY", or "DAY_MONTH".
//
// Returns "" if the surrounding structure doesn't match any known pattern.
func classifyTextualPattern(s string, monthStart, monthEnd int) string {
	before := strings.TrimSpace(s[:monthStart])
	after := strings.TrimSpace(s[monthEnd:])

	beforeNums := extractNumbers(before)
	afterStr := strings.TrimLeft(after, ", ")
	afterNums := extractNumbers(trimAtSuffix(afterStr))

	switch {
	case len(beforeNums) == 0 && len(afterNums) >= 2:
		// "March 15, 2024"
		return "MONTH_DAY_YEAR"

	case len(beforeNums) >= 1 && len(afterNums) >= 1:
		// "15 Mar 2024" or "Fri, 15 Mar 2024"
		return "DAY_MONTH_YEAR"

	case len(beforeNums) == 0 && len(afterNums) == 1:
		// "March 2024" (value > 31 → year) or "March 15" (value ≤ 31 → day)
		if afterNums[0] > 31 {
			return "MONTH_YEAR"
		}
		return "MONTH_DAY"

	case len(beforeNums) == 1 && len(afterNums) == 0:
		// "15 March"
		return "DAY_MONTH"

	default:
		return ""
	}
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
			fields = appendDay(fields, s, n1)
		} else if n1.value > 31 {
			fields = appendDay(fields, s, n0)
			fields = append(fields, yearField(n1))
		} else {
			// Both small, and the first number is the day wherever it sits.
			//
			// This was an if/else on n0.start < monthStart with two identical
			// arms, under a comment promising "or vice versa". Both arms are
			// reachable: "15 MAY 20" takes the first and "MAY15 20" the second,
			// the difference being whether a number precedes the name or abuts
			// it. Neither wants the year first, because the branch above this
			// one has already claimed every case where n0 sits past the end of
			// the month name, which is the only way a year leads two small
			// numbers here. So the position does not decide anything and the
			// vice versa never happens.
			fields = appendDay(fields, s, n0)
			fields = append(fields, yearField(n1))
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
			fields = appendDay(fields, s, n)
		}
		// Check for time component after the number.
		timeFields := parseTimeComponent(s, n.end)
		fields = append(fields, timeFields...)

	case 0:
		// Month name only — unusual but valid.

	default:
		fields = appendMultiNumFields(s, nums, fields)
	}

	return fields
}

// appendMultiNumFields handles the 3+ numeric tokens case in buildTextualFields.
// Patterns: "day time [year]" (e.g., "Mar 15 10:30:00 2024") or "day year time".
func appendMultiNumFields(s string, nums []numToken, fields []compile.Field) []compile.Field {
	n0, n1 := nums[0], nums[1]

	// Detect if nums[1:] form a time pattern (HH:MM or HH:MM:SS).
	isTimeAtN1 := n1.end < len(s) && s[n1.end] == ':' && len(nums) >= 3

	if isTimeAtN1 && n0.value <= 31 && n1.value <= 23 {
		// Pattern: "day time [year]", e.g. "Mar 15 10:30:00 2024"
		fields = appendDay(fields, s, n0)
		timeFields := parseTimeComponent(s, n0.end)
		fields = append(fields, timeFields...)
		if yr := findTrailingYear(nums, timeFields, n0.end); yr != nil {
			fields = append(fields, yearField(*yr))
		}
		return fields
	}

	// Pattern: "day year time" or "year day time".
	if n1.value > 31 {
		fields = appendDay(fields, s, n0)
		fields = append(fields, yearField(n1))
	} else if n0.value > 31 {
		fields = append(fields, yearField(n0))
		fields = appendDay(fields, s, n1)
	} else {
		fields = appendDay(fields, s, n0)
		fields = append(fields, yearField(n1))
	}
	timeFields := parseTimeComponent(s, nums[1].end)
	fields = append(fields, timeFields...)
	return fields
}

// findTrailingYear scans for a 4-digit year among nums that is not already
// claimed by timeFields (i.e., not at the same offset as a time field).
func findTrailingYear(nums []numToken, timeFields []compile.Field, afterOffset int) *numToken {
	for i := range nums {
		num := &nums[i]
		if num.start <= afterOffset || num.value < 1000 || num.value > 9999 {
			continue
		}
		isTimeField := false
		for _, tf := range timeFields {
			if int(tf.Offset) == num.start {
				isTimeField = true
				break
			}
		}
		if !isTimeField {
			return num
		}
	}
	return nil
}

func yearField(n numToken) compile.Field {
	kind := compile.FYear4
	if n.end-n.start <= 2 {
		kind = compile.FYear2
	}
	return compile.Field{Kind: kind, Offset: n.start, Len: n.end - n.start}
}

// ordinalSuffixLen returns 2 when an English ordinal suffix follows a day
// number at position at, and 0 otherwise. The suffix carries no value, but it
// is part of the input and a program has to account for every byte it accepts.
func ordinalSuffixLen(s string, at int) int {
	if at+2 > len(s) {
		return 0
	}
	c0 := s[at] | 0x20
	c1 := s[at+1] | 0x20
	if c1 != 't' && c1 != 'd' && c1 != 'h' {
		return 0
	}
	switch {
	case c0 == 's' && c1 == 't', // 1st
		c0 == 'n' && c1 == 'd', // 2nd
		c0 == 'r' && c1 == 'd', // 3rd
		c0 == 't' && c1 == 'h': // 4th
		// Only when the suffix is not the start of a longer word.
		return boundaryAfter(s, at+2)
	}
	return 0
}

func boundaryAfter(s string, at int) int {
	if at == len(s) || !isLetter(s[at]) {
		return 2
	}
	return 0
}

// appendDay appends the day field for n, plus a skip covering any ordinal
// suffix that follows it.
func appendDay(fields []compile.Field, s string, n numToken) []compile.Field {
	fields = append(fields, dayField(n))
	if w := ordinalSuffixLen(s, n.end); w > 0 {
		fields = append(fields, compile.Field{Kind: compile.FSkip, Offset: n.end, Len: w})
	}
	return fields
}

func dayField(n numToken) compile.Field {
	kind := compile.FDay2
	if n.end-n.start == 1 {
		kind = compile.FDay1or2
	}
	return compile.Field{Kind: kind, Offset: n.start, Len: n.end - n.start}
}

// parseTimeComponent looks for HH:MM or HH:MM:SS or HH:MM:SS.fff patterns
// starting from offset `from` in the string, and returns nil where there is no
// time to describe.
//
// The slice is built after the HH:MM test and not before it. Most of what
// reaches here carries no time at all: "March 15, 2024" is the benchmarked
// textual input and every textual detection runs this once, so the eight fields
// it used to reserve up front were 256 bytes allocated, never written, and
// returned empty. Every caller either checks the length or appends the result,
// and appending a nil slice appends nothing.
func parseTimeComponent(s string, from int) []compile.Field {
	// Skip whitespace, punctuation, and colons to find time start.
	// Colons appear as time separators in CLF: "2024:10:30:00".
	i := from
	for i < len(s) && (s[i] == ' ' || s[i] == ',' || s[i] == '\t' || s[i] == ':') {
		i++
	}

	// Look for "at " prefix.
	if i+3 <= len(s) && equalsFoldASCII(s[i:i+3], "at ") {
		i += 3
	}

	// Find HH:MM pattern.
	if i+5 <= len(s) && isDigit(s[i]) && isDigit(s[i+1]) && s[i+2] == ':' && isDigit(s[i+3]) && isDigit(s[i+4]) {
		fields := make([]compile.Field, 0, 8)
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

		// Skip trailing whitespace before AM/PM or timezone suffix.
		for j < len(s) && s[j] == ' ' {
			j++
		}
		return appendTimeSuffix(s, j, fields)
	}

	return nil
}

type numToken struct {
	value int
	start int
	end   int
}

// appendTimeSuffix checks for AM/PM or timezone suffix at position j in s,
// and appends the appropriate field(s) to fields. If AM/PM is found, the first
// field's Kind is changed from FHour24 to FHour12.
func appendTimeSuffix(s string, j int, fields []compile.Field) []compile.Field {
	if j >= len(s) {
		return fields
	}

	// Check AM/PM first — "AM"/"PM" would be misidentified as timezone abbreviations.
	if j+2 <= len(s) {
		c0 := s[j] | 0x20
		c1 := s[j+1] | 0x20
		if (c0 == 'a' || c0 == 'p') && c1 == 'm' && (j+2 == len(s) || !isLetter(s[j+2])) {
			if len(fields) > 0 && fields[0].Kind == compile.FHour24 {
				fields[0].Kind = compile.FHour12
			}
			return append(fields, compile.Field{Kind: compile.FAMPM, Offset: j, Len: 2})
		}
	}

	// Timezone: Z, ±HHMM/±HH:MM, or abbreviation.
	if s[j] == 'Z' && (j+1 == len(s) || !isLetter(s[j+1])) {
		return append(fields, compile.Field{Kind: compile.FTZZ, Offset: j, Len: 1})
	}
	if s[j] == '+' || s[j] == '-' {
		remaining := len(s) - j
		if remaining >= 5 {
			tzLen := 5
			if remaining >= 6 && s[j+3] == ':' {
				tzLen = 6
			}
			return append(fields, compile.Field{Kind: compile.FTZOffset, Offset: j, Len: tzLen})
		}
	}
	if isLetter(s[j]) {
		tzEnd := j
		for tzEnd < len(s) && isLetter(s[tzEnd]) {
			tzEnd++
		}
		// "GMT+0100" (JS Date.toString) is the name and the offset together,
		// and the offset is what decides the instant. Reading only the name
		// gave UTC for a value an hour ahead of it.
		if rem := len(s) - tzEnd; rem >= 5 && (s[tzEnd] == '+' || s[tzEnd] == '-') {
			tzLen := 5
			if rem >= 6 && s[tzEnd+3] == ':' {
				tzLen = 6
			}
			fields = append(fields, compile.Field{Kind: compile.FSkip, Offset: j, Len: tzEnd - j})
			return append(fields, compile.Field{Kind: compile.FTZOffset, Offset: tzEnd, Len: tzLen})
		}
		return append(fields, compile.Field{Kind: compile.FTZName, Offset: j, Len: tzEnd - j})
	}
	return fields
}

// monthEntry pairs a lowercase month name with its month number.
type monthEntry struct {
	name string
	num  int
}

// defaultMonthNames is sorted longest-first for greedy matching.
// Using a slice instead of a map gives deterministic iteration order
// and eliminates hash-table traversal overhead on the hot path.
var defaultMonthNames = []monthEntry{
	{"september", 9},
	{"february", 2},
	{"november", 11},
	{"december", 12},
	{"january", 1},
	{"october", 10},
	{"august", 8},
	{"march", 3},
	{"april", 4},
	{"june", 6},
	{"july", 7},
	{"sept", 9},
	{"jan", 1},
	{"feb", 2},
	{"mar", 3},
	{"apr", 4},
	{"may", 5},
	{"jun", 6},
	{"jul", 7},
	{"aug", 8},
	{"sep", 9},
	{"oct", 10},
	{"nov", 11},
	{"dec", 12},
}

// findMonthNameCI finds the first month name in the string using
// case-insensitive matching directly on the input (no lowered copy).
// Returns (month number 1-12, start index, end index) or (0, 0, 0) if not found.
func findMonthNameCI(s string, locales []*locale.Data) (int, int, int) {
	// Search English names (case-insensitive), longest first.
	for i := range defaultMonthNames {
		entry := &defaultMonthNames[i]
		if idx, end, ok := matchWordCI(s, entry.name); ok {
			return entry.num, idx, end
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

// isWordChar returns true for characters that can be part of a word
// (letters, including non-ASCII bytes which may be part of UTF-8 sequences).
func isWordChar(c byte) bool {
	return isLetter(c) || c >= 0x80
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
