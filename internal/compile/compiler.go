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
	FTail        // Everything from Offset to the end of the input is ignored

	numFieldKinds // sentinel — must be last
)

// fieldKindToOp maps each FieldKind to its corresponding OpCode.
//
// The two enums are NOT parallel and must not be treated as such. FieldKind 3
// is FMonth1or2 while OpCode 3 is OpMonthName, and they diverge again at 4, 5,
// 16, and 17. Replacing this table with OpCode(f.Kind) compiles, vets, lints,
// and turns every month into a day. This table is the only correspondence
// between them, and TestFieldKindToOpIsComplete is what checks it stays one.
//
// A FieldKind added without an entry here gets the array's zero value, which is
// OpYear4, and silently parses a year at that field's offset. The same test
// catches that.
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
	FTail:        OpTail,
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

// FixedWidth returns the number of input bytes the instruction for k reads,
// and whether that number is fixed at all.
//
// It exists so a test can assert what this package assumes and what the
// detectors have to honour: a Field's declared Len is the number of bytes its
// op will read. buildDatePartFields broke that by giving a three-character
// part an FDay2 with Len 3, and since OpDay2 reads exactly two, the value
// detection validated was not the value the program returned. Nothing failed,
// because the program was wrong the same way every time it ran, so comparing
// Parse against Layout.Parse could not see it either.
//
// Variable-width kinds report false: their width comes from the input or from
// Len itself, so there is nothing to cross-check.
func FixedWidth(k FieldKind) (int, bool) {
	switch k {
	case FYear4:
		return 4, true
	case FYear2, FMonth2, FDay2, FHour24, FHour12, FMinute2, FSecond2, FAMPM, FISOWeek:
		return 2, true
	case FTZZ, FISOWeekDay:
		return 1, true
	default:
		// FMonth1or2, FDay1or2, FHour1or2 read one byte or two.
		// FMonthName, FFracSec, FTZOffset, FTZName, FOrdinalDay, FLiteral,
		// FSkip read exactly Len. FTZZOrOffset and FTail vary with the input.
		return 0, false
	}
}
