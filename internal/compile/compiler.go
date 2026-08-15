package compile

import (
	"fmt"
	"time"
)

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
	FDaySpacePad // Two bytes: a space and a digit, or two digits

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
	FDaySpacePad: OpDaySpacePad,
}

// Field describes one component in a format definition.
//
// Offset and Len are int32 rather than int, which makes a Field 16 bytes
// instead of 32. Compile refuses any field whose Offset or Len exceeds
// maxFieldByte, which is 255, so the other 60 bits were headroom nothing could
// reach. Every fallback detector allocates a slice of these and they are the
// bulk of what a non-trie parse allocates.
//
// Not int16: it measures the same 16 bytes at this alignment and would add a
// truncation that a long input could plausibly reach.
type Field struct {
	Kind   FieldKind
	Offset int32  // Byte offset in the input
	Len    int32  // Expected length (0 = variable)
	Aux    uint16 // Pre-resolved value: month number, literal byte, or class. See instructions.go
}

// FormatDef defines a date format as a sequence of fields.
type FormatDef struct {
	Name     string // e.g. "ISO8601_DATE"
	GoLayout string // Go time layout equivalent, if any
	Fields   []Field
}

// Compile turns a FormatDef into an executable Program, or refuses a def the
// Program cannot represent.
//
// It used to refuse nothing. Over MaxInstructions it stopped filling and
// returned what it had, so Compile("The current date and time: 2006-01-02")
// came back with a nil error and a layout that answered year zero for every
// input: ParseGoLayout emits one instruction per unrecognised layout byte, and
// 27 of them ran the count out before the first field. The end-of-input check
// added in c4851ae turned that from a wrong date into an error at parse time,
// which is better and still wrong. A constructor that cannot honour a layout
// should say so when it is handed the layout.
//
// Offset and Len are bytes, so a def addressing past 255 is refused too rather
// than wrapping. Detection reaches that with a long enough input: it is what
// stops a 300-byte prefix putting a month name at byte 44.
//
// needsBaseYear reports that the format carries no year field, so the caller
// may want to set Program.BaseYear before running it. It is returned rather
// than resolved here because the answer costs a clock read, and the caller is
// the only one who knows whether it has a configured base time to use instead.
// Reporting it from this loop keeps that read off the formats that do carry a
// year, which is nearly all of them.
func Compile(def *FormatDef, tz *time.Location) (p Program, needsBaseYear bool, err error) {
	// The error paths return the named results rather than a fresh Program{}.
	// A Program is 168 bytes, and materialising a second zero one puts the cost
	// of the refusal on every call that does not refuse.
	if n := len(def.Fields); n > MaxInstructions {
		err = fmt.Errorf("compile: format %s needs %d instructions, the limit is %d",
			def.Name, n, MaxInstructions)
		return p, false, err
	}

	p.Tz = tz
	needsBaseYear = true

	// The refusal above counts fields, deliberately, and not the instructions
	// the loop below emits. Fusion makes some formats fit that did not, and
	// letting that raise the effective budget would mean the error names a
	// number the caller cannot derive from the layout they wrote. A caller can
	// count tokens; they cannot predict which separators fuse.
	for i := 0; i < len(def.Fields); i++ {
		f := def.Fields[i]
		if f.Offset > maxFieldByte || f.Len > maxFieldByte {
			err = fmt.Errorf(
				"compile: format %s has a field at offset %d of length %d, past the %d it can address",
				def.Name, f.Offset, f.Len, maxFieldByte)
			return p, false, err
		}
		if f.Kind == FYear4 || f.Kind == FYear2 {
			needsBaseYear = false
		}

		aux := f.Aux

		// Fuse a following single-byte literal into this field, so the executor
		// reads the separator without spending a loop iteration and a dispatch
		// on it. See the Aux convention in instructions.go.
		//
		// The literal has to sit exactly where this field ends. A gap would mean
		// some byte belongs to neither, and the executor's coverage counters
		// cannot see that: they sum widths, which is why every detector emits a
		// field for every byte and why TestEveryInputByteIsDescribedExactlyOnce
		// exists. Requiring adjacency here keeps the sum exact, because the
		// fused instruction accounts for both widths.
		if w, ok := fusesSeparator(f.Kind); ok && i+1 < len(def.Fields) {
			if next := def.Fields[i+1]; next.Kind == FLiteral &&
				next.Len == 1 && next.Offset == f.Offset+w {
				if next.Aux == 0 {
					aux = sepAnyNonDigit
				} else {
					aux = next.Aux
				}
				i++ // the literal is this instruction's now
			}
		}

		p.Insts[p.N] = Inst{
			Op:     fieldKindToOp[f.Kind],
			Offset: byte(f.Offset),
			Len:    byte(f.Len),
			Aux:    aux,
		}
		p.N++
	}

	return p, needsBaseYear, nil
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
	case FYear2, FMonth2, FDay2, FHour24, FHour12, FMinute2, FSecond2, FAMPM, FISOWeek,
		FDaySpacePad:
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
