package compile

// OpCode identifies a parse instruction.
type OpCode byte

const (
	OpYear4       OpCode = iota // Extract 4-digit year at offset
	OpYear2                     // Extract 2-digit year at offset
	OpMonth2                    // Extract 2-digit month at offset
	OpMonthName                 // Month was pre-resolved during detection
	OpDay2                      // Extract 2-digit day at offset
	OpHour24                    // Extract 2-digit hour (24h) at offset
	OpHour12                    // Extract 2-digit hour (12h) at offset
	OpMinute2                   // Extract 2-digit minute at offset
	OpSecond2                   // Extract 2-digit second at offset
	OpFracSec                   // Extract fractional seconds at offset, with length
	OpAMPM                      // Extract AM/PM at offset
	OpTZZ                       // Literal 'Z' means UTC
	OpTZOffset                  // Extract ±HH:MM or ±HHMM timezone offset
	OpTZName                    // Extract timezone abbreviation (e.g. "UTC", "EST")
	OpLiteral                   // Skip a literal byte at offset
	OpSkip                      // Skip N bytes
	OpDay1or2                   // Extract 1-or-2 digit day
	OpMonth1or2                 // Extract 1-or-2 digit month
	OpHour1or2                  // Extract 1-or-2 digit hour
	OpISOWeek                   // Extract ISO week number (01-53) at offset
	OpISOWeekDay                // Extract ISO weekday (1-7, Mon=1) at offset
	OpOrdinalDay                // Extract ordinal day of year (001-366) at offset
	OpTZZOrOffset               // 'Z' → UTC, or parse ±offset of Len bytes
	OpTail                      // Consume the rest of the input without reading it
	OpDaySpacePad               // Extract a space-padded day: " 5" or "15"
)

// Inst is a single parse instruction.
// Kept small (6 bytes) so a Program fits in a few cache lines.
type Inst struct {
	Op     OpCode
	Offset byte   // Byte offset into the input string
	Len    byte   // Length of the field (0 = implied by Op)
	Aux    uint16 // Auxiliary data: see below
}

// What Aux means depends on Op, and there are three readings.
//
//	OpMonthName   the month, 1-12, resolved when the format was detected
//	OpLiteral     0 for "any non-digit", otherwise the exact byte to match
//	numeric ops   0 for nothing, otherwise a separator fused onto the field
//
// The third is why the separators in a trie format do not cost an instruction
// each. Every literal in every trie format is a single byte sitting exactly
// where a fixed-width numeric field ends, which is 105 of the 267 fields those
// formats declare, and each one was a loop iteration and an opcode dispatch to
// read one byte and check it was not a digit. Compile folds them into the field
// in front of them, so ISO8601_DATETIME_Z is 7 instructions rather than 12 and
// ISO8601_DATE is 3 rather than 5.
//
// Aux is free on the numeric ops and always has been: before this only
// OpMonthName and OpLiteral ever read it. So the fusion costs Inst nothing,
// which matters because Program is copied by value on every detection and
// MaxInstructions is capped at 24 for that reason.
const (
	// sepNone is Aux on a numeric op that carries no fused separator.
	sepNone = 0

	// sepAnyNonDigit is Aux on a numeric op whose fused separator may be any
	// byte that is not a digit.
	//
	// It is out of byte range on purpose, so it cannot collide with the exact
	// byte encodings below it. The trie entries need this reading rather than an
	// exact byte: an entry matches a signature of character classes and one
	// entry serves every byte in the class, so ISO8601_DATE reads "2024-03-15",
	// "2024/03/15" and "2024.03.15" through the same fields and naming '-' would
	// refuse two inputs detection accepts. What no class at a separator position
	// contains is a digit, and that is the half worth enforcing: a digit is a
	// numeric token, and which token is where is what picks the format.
	sepAnyNonDigit = 256
)

// fusesSeparator reports whether a field of kind k can carry a fused separator,
// and how many bytes it reads itself.
//
// Only the fixed-width numeric kinds qualify. Their width is known here, their
// Aux is unused, and their arm in the executor sets w to a constant. The
// variable-width kinds are excluded deliberately: their width comes from the
// input and interacts with the executor's delta, and no format in the tree needs
// them fused, so they keep their separator as its own instruction.
func fusesSeparator(k FieldKind) (int, bool) {
	switch k {
	case FYear4:
		return 4, true
	case FYear2, FMonth2, FDay2, FHour24, FHour12, FMinute2, FSecond2, FISOWeek,
		FDaySpacePad:
		return 2, true
	}
	return 0, false
}
