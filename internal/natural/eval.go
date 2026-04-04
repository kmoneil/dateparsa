package natural

import (
	"time"
)

// ResultKind classifies the NL parse result.
type ResultKind int

const (
	KindNone     ResultKind = iota
	KindNow                        // "now", "today"
	KindRelative                   // "3 days ago", "yesterday"
)

// Result holds the outcome of a natural language parse.
type Result struct {
	Time time.Time
	Kind ResultKind
}

// Eval evaluates a token stream against a base time.
// Returns nil if the tokens don't form a recognized NL expression.
func Eval(tokens []Token, base time.Time, preferFuture bool) *Result {
	if len(tokens) == 0 {
		return nil
	}

	// Filter out unknown tokens — if more than half are unknown, bail.
	unknowns := 0
	for _, t := range tokens {
		if t.Kind == TokUnknown {
			unknowns++
		}
	}
	if unknowns > len(tokens)/2 {
		return nil
	}

	// Collapse consecutive TokNumber tokens: "a few" → [1, 3] → keep 3.
	// When "a"/"an" (IntVal=1) precedes another number, drop the "a"/"an".
	tokens = collapseNumbers(tokens)

	// Try each pattern in priority order.
	if r := evalRelWord(tokens, base); r != nil {
		return r
	}
	if r := evalTimeOfDay(tokens, base); r != nil {
		return r
	}
	if r := evalHalf(tokens, base); r != nil {
		return r
	}
	if r := evalCompoundNAgo(tokens, base); r != nil {
		return r
	}
	if r := evalNAgo(tokens, base); r != nil {
		return r
	}
	if r := evalPrefixAgo(tokens, base); r != nil {
		return r
	}
	if r := evalInN(tokens, base); r != nil {
		return r
	}
	if r := evalSelectorWeekday(tokens, base, preferFuture); r != nil {
		return r
	}
	if r := evalMonthDay(tokens, base); r != nil {
		return r
	}
	if r := evalSelectorMonth(tokens, base, preferFuture); r != nil {
		return r
	}
	if r := evalSelectorUnit(tokens, base, preferFuture); r != nil {
		return r
	}
	if r := evalBoundary(tokens, base); r != nil {
		return r
	}
	if r := evalBareWeekday(tokens, base, preferFuture); r != nil {
		return r
	}

	return nil
}

// evalRelWord handles "now", "today", "yesterday", "tomorrow",
// optionally followed by "at <time>".
func evalRelWord(tokens []Token, base time.Time) *Result {
	if tokens[0].Kind != TokRelWord {
		return nil
	}

	t := base
	kind := KindNow

	switch tokens[0].RelVal {
	case RelNow:
		// t = base (as-is, including time)
	case RelToday:
		t = truncateDay(base)
	case RelYesterday:
		t = truncateDay(base).AddDate(0, 0, -1)
		kind = KindRelative
	case RelTomorrow:
		t = truncateDay(base).AddDate(0, 0, 1)
		kind = KindRelative
	case RelTonight:
		t = truncateDay(base).Add(21 * time.Hour)
		kind = KindRelative
	default:
		return nil
	}

	// Check for "at <time>" suffix.
	t = applyTimeSuffix(tokens[1:], t)

	return &Result{Time: t, Kind: kind}
}

// evalNAgo handles "N units ago" and "N units from now".
func evalNAgo(tokens []Token, base time.Time) *Result {
	// Pattern: NUMBER UNIT DIRECTION
	if len(tokens) < 3 {
		return nil
	}
	if tokens[0].Kind != TokNumber || tokens[1].Kind != TokUnit || tokens[2].Kind != TokDirection {
		return nil
	}

	n := tokens[0].IntVal
	unit := tokens[1].UnitVal
	dir := tokens[2].DirVal

	if dir == DirAgo {
		n = -n
	}

	t := addUnit(base, n, unit)

	// Check for "at <time>" suffix.
	t = applyTimeSuffix(tokens[3:], t)

	return &Result{Time: t, Kind: KindRelative}
}

// evalInN handles "in N units".
func evalInN(tokens []Token, base time.Time) *Result {
	// Pattern: DIRECTION(in) NUMBER UNIT
	if len(tokens) < 3 {
		return nil
	}
	if tokens[0].Kind != TokDirection || tokens[0].DirVal != DirIn {
		return nil
	}
	if tokens[1].Kind != TokNumber || tokens[2].Kind != TokUnit {
		return nil
	}

	n := tokens[1].IntVal
	unit := tokens[2].UnitVal
	t := addUnit(base, n, unit)

	t = applyTimeSuffix(tokens[3:], t)

	return &Result{Time: t, Kind: KindRelative}
}

// evalSelectorWeekday handles "last friday", "next tuesday", "this monday".
func evalSelectorWeekday(tokens []Token, base time.Time, preferFuture bool) *Result {
	if len(tokens) < 2 {
		return nil
	}
	if tokens[0].Kind != TokSelector || tokens[1].Kind != TokWeekday {
		return nil
	}

	sel := tokens[0].SelVal
	targetWday := time.Weekday(tokens[1].WdayVal)
	t := resolveWeekday(base, targetWday, sel, preferFuture)

	t = applyTimeSuffix(tokens[2:], t)

	return &Result{Time: t, Kind: KindRelative}
}

// evalSelectorMonth handles "last january", "next march".
func evalSelectorMonth(tokens []Token, base time.Time, preferFuture bool) *Result {
	if len(tokens) < 2 {
		return nil
	}
	if tokens[0].Kind != TokSelector || tokens[1].Kind != TokMonth {
		return nil
	}

	sel := tokens[0].SelVal
	targetMonth := time.Month(tokens[1].MonVal)
	t := resolveMonth(base, targetMonth, sel, preferFuture)

	return &Result{Time: t, Kind: KindRelative}
}

// evalSelectorUnit handles "last week", "next month", "last year".
func evalSelectorUnit(tokens []Token, base time.Time, preferFuture bool) *Result {
	if len(tokens) < 2 {
		return nil
	}
	if tokens[0].Kind != TokSelector || tokens[1].Kind != TokUnit {
		return nil
	}

	sel := tokens[0].SelVal
	unit := tokens[1].UnitVal

	n := -1
	if sel == SelNext {
		n = 1
	} else if sel == SelThis {
		n = 0
	}

	var t time.Time
	switch unit {
	case UnitWeek:
		if n == 0 {
			t = startOfWeek(base)
		} else {
			t = startOfWeek(base).AddDate(0, 0, n*7)
		}
	case UnitMonth:
		if n == 0 {
			t = startOfMonth(base)
		} else {
			t = startOfMonth(base).AddDate(0, n, 0)
		}
	case UnitYear:
		if n == 0 {
			t = startOfYear(base)
		} else {
			t = startOfYear(base).AddDate(n, 0, 0)
		}
	case UnitDay:
		t = truncateDay(base).AddDate(0, 0, n)
	default:
		return nil
	}

	return &Result{Time: t, Kind: KindRelative}
}

// evalBoundary handles "beginning of month", "end of year", "start of day".
func evalBoundary(tokens []Token, base time.Time) *Result {
	// Pattern: BOUNDARY "of" UNIT
	if len(tokens) < 3 {
		return nil
	}
	if tokens[0].Kind != TokBoundary || tokens[1].Kind != TokOf || tokens[2].Kind != TokUnit {
		return nil
	}

	bnd := tokens[0].BndVal
	unit := tokens[2].UnitVal

	var t time.Time
	switch unit {
	case UnitDay:
		if bnd == BndStart {
			t = truncateDay(base)
		} else {
			t = truncateDay(base).Add(24*time.Hour - time.Nanosecond)
		}
	case UnitWeek:
		if bnd == BndStart {
			t = startOfWeek(base)
		} else {
			t = startOfWeek(base).AddDate(0, 0, 7).Add(-time.Nanosecond)
		}
	case UnitMonth:
		if bnd == BndStart {
			t = startOfMonth(base)
		} else {
			t = startOfMonth(base).AddDate(0, 1, 0).Add(-time.Nanosecond)
		}
	case UnitYear:
		if bnd == BndStart {
			t = startOfYear(base)
		} else {
			t = startOfYear(base).AddDate(1, 0, 0).Add(-time.Nanosecond)
		}
	default:
		return nil
	}

	return &Result{Time: t, Kind: KindRelative}
}

// applyTimeSuffix looks for "at <time>", "at noon", "at midnight",
// or a bare TokTime in the remaining tokens, and applies it.
func applyTimeSuffix(tokens []Token, t time.Time) time.Time {
	if len(tokens) == 0 {
		return t
	}

	// Truncate to day before applying time.
	base := truncateDay(t)
	idx := 0

	// Skip "at" if present.
	if tokens[0].Kind == TokAt {
		idx++
		if idx >= len(tokens) {
			return t
		}
	}

	if idx >= len(tokens) {
		return t
	}

	switch tokens[idx].Kind {
	case TokTime:
		return base.Add(time.Duration(tokens[idx].Hour)*time.Hour + time.Duration(tokens[idx].Min)*time.Minute)
	case TokNoon:
		return base.Add(12 * time.Hour)
	case TokMidnight:
		return base
	case TokNumber:
		// Bare number after "at": treat as hour. e.g., "yesterday at 5"
		h := tokens[idx].IntVal
		if h >= 0 && h <= 23 {
			// Check for AM/PM following.
			if idx+1 < len(tokens) && tokens[idx+1].Kind == TokAMPM {
				if tokens[idx+1].AMPM == 2 && h != 12 {
					h += 12
				} else if tokens[idx+1].AMPM == 1 && h == 12 {
					h = 0
				}
			}
			return base.Add(time.Duration(h) * time.Hour)
		}
	}

	return t
}

// evalTimeOfDay handles "this morning", "this afternoon", "this evening",
// "last night", and selector + time-of-day patterns.
func evalTimeOfDay(tokens []Token, base time.Time) *Result {
	// Pattern: SELECTOR TIMEOFDAY (e.g. "this morning", "last night")
	if len(tokens) >= 2 && tokens[0].Kind == TokSelector && tokens[1].Kind == TokTimeOfDay {
		day := truncateDay(base)
		hour := tokens[1].Hour
		if tokens[0].SelVal == SelLast {
			day = day.AddDate(0, 0, -1)
		}
		t := day.Add(time.Duration(hour) * time.Hour)
		return &Result{Time: t, Kind: KindRelative}
	}

	// Bare time-of-day word alone (unlikely in tests but for completeness).
	if len(tokens) == 1 && tokens[0].Kind == TokTimeOfDay {
		day := truncateDay(base)
		t := day.Add(time.Duration(tokens[0].Hour) * time.Hour)
		return &Result{Time: t, Kind: KindRelative}
	}

	return nil
}

// evalHalf handles "half an hour ago", "half a day ago".
// Pattern: HALF NUMBER? UNIT DIRECTION
func evalHalf(tokens []Token, base time.Time) *Result {
	if len(tokens) < 3 || tokens[0].Kind != TokHalf {
		return nil
	}

	idx := 1
	// Skip optional "a"/"an" (TokNumber with IntVal=1)
	if idx < len(tokens) && tokens[idx].Kind == TokNumber && tokens[idx].IntVal == 1 {
		idx++
	}
	if idx >= len(tokens) || tokens[idx].Kind != TokUnit {
		return nil
	}
	unit := tokens[idx].UnitVal
	idx++
	if idx >= len(tokens) || tokens[idx].Kind != TokDirection {
		return nil
	}
	dir := tokens[idx].DirVal

	// Compute half the unit duration.
	var d time.Duration
	switch unit {
	case UnitMinute:
		d = 30 * time.Second
	case UnitHour:
		d = 30 * time.Minute
	case UnitDay:
		d = 12 * time.Hour
	case UnitWeek:
		d = 84 * time.Hour // 3.5 days
	default:
		return nil
	}

	if dir == DirAgo {
		d = -d
	}

	return &Result{Time: base.Add(d), Kind: KindRelative}
}

// evalCompoundNAgo handles compound durations like "1 hour and 3 minutes ago".
// Pattern: (NUMBER UNIT AND)+ NUMBER UNIT DIRECTION
// Also: DIRECTION(in) (NUMBER UNIT AND)+ NUMBER UNIT
func evalCompoundNAgo(tokens []Token, base time.Time) *Result {
	// Check prefix "in" form: IN (N UNIT AND)* N UNIT
	startIdx := 0
	prefixIn := false
	if len(tokens) >= 5 && tokens[0].Kind == TokDirection && tokens[0].DirVal == DirIn {
		startIdx = 1
		prefixIn = true
	}

	// We need at least N UNIT AND N UNIT DIR (6 tokens from startIdx), or
	// for prefix form: IN N UNIT AND N UNIT (6 tokens total).
	remaining := tokens[startIdx:]
	if len(remaining) < 5 {
		return nil
	}

	// Collect N UNIT pairs separated by AND.
	type pair struct {
		n    int
		unit Unit
	}
	var pairs []pair
	i := 0
	for {
		if i+1 >= len(remaining) {
			return nil
		}
		if remaining[i].Kind != TokNumber || remaining[i+1].Kind != TokUnit {
			return nil
		}
		pairs = append(pairs, pair{remaining[i].IntVal, remaining[i+1].UnitVal})
		i += 2

		// Check for AND followed by more pairs.
		if i < len(remaining) && remaining[i].Kind == TokAnd {
			i++
			continue
		}
		break
	}

	if len(pairs) < 2 {
		return nil // Not a compound — let other handlers deal with it.
	}

	// Determine direction.
	var dir Direction
	if prefixIn {
		dir = DirIn
	} else {
		if i >= len(remaining) || remaining[i].Kind != TokDirection {
			return nil
		}
		dir = remaining[i].DirVal
	}

	// Accumulate duration.
	t := base
	for _, p := range pairs {
		n := p.n
		if dir == DirAgo {
			n = -n
		}
		t = addUnit(t, n, p.unit)
	}

	return &Result{Time: t, Kind: KindRelative}
}

// evalPrefixAgo handles prefix-ago patterns used in French/Spanish/German:
// DIRECTION(ago) NUMBER UNIT → "il y a 2 heures", "hace 3 dias", "vor 5 Minuten"
func evalPrefixAgo(tokens []Token, base time.Time) *Result {
	if len(tokens) < 3 {
		return nil
	}
	if tokens[0].Kind != TokDirection || tokens[0].DirVal != DirAgo {
		return nil
	}
	if tokens[1].Kind != TokNumber || tokens[2].Kind != TokUnit {
		return nil
	}

	n := -tokens[1].IntVal
	unit := tokens[2].UnitVal
	t := addUnit(base, n, unit)
	return &Result{Time: t, Kind: KindRelative}
}

// evalMonthDay handles "december 25th", "november 1st", optionally with "at TIME".
// Pattern: MONTH NUMBER [AT TIME]
func evalMonthDay(tokens []Token, base time.Time) *Result {
	if len(tokens) < 2 {
		return nil
	}
	if tokens[0].Kind != TokMonth || tokens[1].Kind != TokNumber {
		return nil
	}

	month := time.Month(tokens[0].MonVal)
	day := tokens[1].IntVal
	if day < 1 || day > 31 {
		return nil
	}

	t := time.Date(base.Year(), month, day, 0, 0, 0, 0, base.Location())
	t = applyTimeSuffix(tokens[2:], t)

	return &Result{Time: t, Kind: KindRelative}
}

// evalBareWeekday handles a bare weekday name like "sunday" with no selector.
// Defaults to most recent past occurrence, or next future if preferFuture.
func evalBareWeekday(tokens []Token, base time.Time, preferFuture bool) *Result {
	if len(tokens) != 1 || tokens[0].Kind != TokWeekday {
		return nil
	}

	sel := SelLast
	if preferFuture {
		sel = SelNext
	}

	targetWday := time.Weekday(tokens[0].WdayVal)
	t := resolveWeekday(base, targetWday, sel, preferFuture)
	return &Result{Time: t, Kind: KindRelative}
}

// collapseNumbers removes redundant "a"/"an" (IntVal=1) tokens when
// they immediately precede another TokNumber (e.g., "a few" → keep "few").
// Returns the input slice unchanged (zero allocation) when no collapsing is needed.
func collapseNumbers(tokens []Token) []Token {
	// Quick scan: check if any collapsing is needed.
	needsCollapse := false
	for i := 0; i < len(tokens)-1; i++ {
		if tokens[i].Kind == TokNumber && tokens[i].IntVal == 1 &&
			(tokens[i].Raw == "a" || tokens[i].Raw == "an") &&
			tokens[i+1].Kind == TokNumber {
			needsCollapse = true
			break
		}
	}
	if !needsCollapse {
		return tokens
	}

	result := make([]Token, 0, len(tokens))
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Kind == TokNumber && tokens[i].IntVal == 1 &&
			(tokens[i].Raw == "a" || tokens[i].Raw == "an") &&
			i+1 < len(tokens) && tokens[i+1].Kind == TokNumber {
			continue // skip "a"/"an" before another number
		}
		result = append(result, tokens[i])
	}
	return result
}

// --- Time arithmetic helpers ---

func addUnit(t time.Time, n int, unit Unit) time.Time {
	switch unit {
	case UnitSecond:
		return t.Add(time.Duration(n) * time.Second)
	case UnitMinute:
		return t.Add(time.Duration(n) * time.Minute)
	case UnitHour:
		return t.Add(time.Duration(n) * time.Hour)
	case UnitDay:
		return t.AddDate(0, 0, n)
	case UnitWeek:
		return t.AddDate(0, 0, n*7)
	case UnitMonth:
		return t.AddDate(0, n, 0)
	case UnitYear:
		return t.AddDate(n, 0, 0)
	}
	return t
}

func resolveWeekday(base time.Time, target time.Weekday, sel Selector, preferFuture bool) time.Time {
	t := truncateDay(base)
	current := base.Weekday()
	diff := int(target) - int(current)

	switch sel {
	case SelNext:
		if diff <= 0 {
			diff += 7
		}
	case SelLast:
		if diff >= 0 {
			diff -= 7
		}
	case SelThis:
		// "this monday" — the upcoming occurrence in the current week.
		if diff < 0 {
			diff += 7
		}
	}

	return t.AddDate(0, 0, diff)
}

func resolveMonth(base time.Time, target time.Month, sel Selector, preferFuture bool) time.Time {
	year := base.Year()
	loc := base.Location()

	switch sel {
	case SelNext:
		if base.Month() >= target {
			year++
		}
	case SelLast:
		if base.Month() <= target {
			year--
		}
	case SelThis:
		// "this january" — same year
	}

	return time.Date(year, target, 1, 0, 0, 0, 0, loc)
}

func truncateDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

func startOfWeek(t time.Time) time.Time {
	d := truncateDay(t)
	wd := d.Weekday()
	// Start of week = Monday.
	offset := int(wd) - int(time.Monday)
	if offset < 0 {
		offset += 7
	}
	return d.AddDate(0, 0, -offset)
}

func startOfMonth(t time.Time) time.Time {
	y, m, _ := t.Date()
	return time.Date(y, m, 1, 0, 0, 0, 0, t.Location())
}

func startOfYear(t time.Time) time.Time {
	return time.Date(t.Year(), 1, 1, 0, 0, 0, 0, t.Location())
}
