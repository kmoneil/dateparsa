package compile

import "time"

// FormatField describes a component within a date format.
type FieldKind byte

const (
	FYear4 FieldKind = iota
	FYear2
	FMonth2
	FMonth1or2
	FMonthName // Textual month — resolved at detection time
	FDay2
	FDay1or2
	FHour24
	FHour12
	FHour1or2
	FMinute2
	FSecond2
	FFracSec
	FAMPM
	FTZZ         // Literal 'Z'
	FTZOffset    // ±HH:MM or ±HHMM
	FTZName      // Timezone abbreviation
	FLiteral     // A literal character to skip
	FSkip        // Skip N bytes
	FISOWeek     // ISO week number
	FISOWeekDay  // ISO weekday (1=Mon)
	FOrdinalDay  // Ordinal day of year (1-366)
	FTZZOrOffset // 'Z' → UTC, or ±HH:MM/±HHMM offset (conditional)

	numFieldKinds // sentinel — must be last
)

// fieldKindToOp maps each FieldKind to its corresponding OpCode.
// Indexed by FieldKind; the FieldKind and OpCode enums are defined in parallel.
var fieldKindToOp = [numFieldKinds]OpCode{
	FYear4:       OpYear4,
	FYear2:       OpYear2,
	FMonth2:      OpMonth2,
	FMonth1or2:   OpMonth1or2,
	FMonthName:   OpMonthName,
	FDay2:        OpDay2,
	FDay1or2:     OpDay1or2,
	FHour24:      OpHour24,
	FHour12:      OpHour12,
	FHour1or2:    OpHour1or2,
	FMinute2:     OpMinute2,
	FSecond2:     OpSecond2,
	FFracSec:     OpFracSec,
	FAMPM:        OpAMPM,
	FTZZ:         OpTZZ,
	FTZOffset:    OpTZOffset,
	FTZName:      OpTZName,
	FLiteral:     OpLiteral,
	FSkip:        OpSkip,
	FISOWeek:     OpISOWeek,
	FISOWeekDay:  OpISOWeekDay,
	FOrdinalDay:  OpOrdinalDay,
	FTZZOrOffset: OpTZZOrOffset,
}

// Field describes one component in a format definition.
type Field struct {
	Kind   FieldKind
	Offset int    // Byte offset in the input
	Len    int    // Expected length (0 = variable)
	Aux    uint16 // Pre-resolved value (month number for MonthName, literal byte, etc.)
}

// FormatDef defines a date format as a sequence of fields.
type FormatDef struct {
	Name     string // e.g. "ISO8601_DATE"
	GoLayout string // Go time layout equivalent, if any
	Fields   []Field
}

// Compile turns a FormatDef into an executable Program.
//
// needsBaseYear reports that the format carries no year field, so the caller
// may want to set Program.BaseYear before running it. It is returned rather
// than resolved here because the answer costs a clock read, and the caller is
// the only one who knows whether it has a configured base time to use instead.
// Reporting it from this loop keeps that read off the formats that do carry a
// year, which is nearly all of them.
func Compile(def *FormatDef, tz *time.Location) (p Program, needsBaseYear bool) {
	p.Tz = tz
	needsBaseYear = true

	for _, f := range def.Fields {
		if p.N >= MaxInstructions {
			break
		}
		if f.Kind == FYear4 || f.Kind == FYear2 {
			needsBaseYear = false
		}
		p.Insts[p.N] = Inst{
			Op:     fieldKindToOp[f.Kind],
			Offset: byte(f.Offset),
			Len:    byte(f.Len),
			Aux:    f.Aux,
		}
		p.N++
	}

	return p, needsBaseYear
}
