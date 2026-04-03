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

	// Try each pattern in priority order.
	if r := evalRelWord(tokens, base); r != nil {
		return r
	}
	if r := evalNAgo(tokens, base); r != nil {
		return r
	}
	if r := evalInN(tokens, base); r != nil {
		return r
	}
	if r := evalSelectorWeekday(tokens, base, preferFuture); r != nil {
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

	switch tokens[0].Raw {
	case "now":
		// t = base (as-is, including time)
	case "today":
		t = truncateDay(base)
	case "yesterday":
		t = truncateDay(base).AddDate(0, 0, -1)
		kind = KindRelative
	case "tomorrow":
		t = truncateDay(base).AddDate(0, 0, 1)
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
