package compile

import "time"

// FormatField describes a component within a date format.
type FieldKind byte

const (
	FYear4     FieldKind = iota
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
	FTZZ      // Literal 'Z'
	FTZOffset // ±HH:MM or ±HHMM
	FTZName      // Timezone abbreviation
	FLiteral     // A literal character to skip
	FSkip        // Skip N bytes
	FISOWeek     // ISO week number
	FISOWeekDay  // ISO weekday (1=Mon)
	FOrdinalDay  // Ordinal day of year (1-366)
)

// Field describes one component in a format definition.
type Field struct {
	Kind   FieldKind
	Offset int    // Byte offset in the input
	Len    int    // Expected length (0 = variable)
	Aux    uint16 // Pre-resolved value (month number for MonthName, literal byte, etc.)
}

// FormatDef defines a date format as a sequence of fields.
type FormatDef struct {
	Name     string   // e.g. "ISO8601_DATE"
	GoLayout string   // Go time layout equivalent, if any
	Fields   []Field
}

// Compile turns a FormatDef into an executable Program.
func Compile(def *FormatDef, tz *time.Location) Program {
	var p Program
	p.Tz = tz

	for _, f := range def.Fields {
		if p.N >= MaxInstructions {
			break
		}
		inst := Inst{
			Offset: byte(f.Offset),
			Len:    byte(f.Len),
			Aux:    f.Aux,
		}
		switch f.Kind {
		case FYear4:
			inst.Op = OpYear4
		case FYear2:
			inst.Op = OpYear2
		case FMonth2:
			inst.Op = OpMonth2
		case FMonth1or2:
			inst.Op = OpMonth1or2
		case FMonthName:
			inst.Op = OpMonthName
		case FDay2:
			inst.Op = OpDay2
		case FDay1or2:
			inst.Op = OpDay1or2
		case FHour24:
			inst.Op = OpHour24
		case FHour12:
			inst.Op = OpHour12
		case FHour1or2:
			inst.Op = OpHour1or2
		case FMinute2:
			inst.Op = OpMinute2
		case FSecond2:
			inst.Op = OpSecond2
		case FFracSec:
			inst.Op = OpFracSec
		case FAMPM:
			inst.Op = OpAMPM
		case FTZZ:
			inst.Op = OpTZZ
		case FTZOffset:
			inst.Op = OpTZOffset
		case FTZName:
			inst.Op = OpTZName
		case FLiteral:
			inst.Op = OpLiteral
		case FSkip:
			inst.Op = OpSkip
		case FISOWeek:
			inst.Op = OpISOWeek
		case FISOWeekDay:
			inst.Op = OpISOWeekDay
		case FOrdinalDay:
			inst.Op = OpOrdinalDay
		}
		p.Insts[p.N] = inst
		p.N++
	}

	return p
}
