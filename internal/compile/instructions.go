package compile

// OpCode identifies a parse instruction.
type OpCode byte

const (
	OpYear4      OpCode = iota // Extract 4-digit year at offset
	OpYear2                    // Extract 2-digit year at offset
	OpMonth2                   // Extract 2-digit month at offset
	OpMonthName                // Month was pre-resolved during detection
	OpDay2                     // Extract 2-digit day at offset
	OpHour24                   // Extract 2-digit hour (24h) at offset
	OpHour12                   // Extract 2-digit hour (12h) at offset
	OpMinute2                  // Extract 2-digit minute at offset
	OpSecond2                  // Extract 2-digit second at offset
	OpFracSec                  // Extract fractional seconds at offset, with length
	OpAMPM                     // Extract AM/PM at offset
	OpTZZ                      // Literal 'Z' means UTC
	OpTZOffset                 // Extract ±HH:MM or ±HHMM timezone offset
	OpTZName                   // Extract timezone abbreviation (e.g. "UTC", "EST")
	OpLiteral                  // Skip a literal byte at offset
	OpSkip                     // Skip N bytes
	OpDay1or2                  // Extract 1-or-2 digit day
	OpMonth1or2                // Extract 1-or-2 digit month
	OpHour1or2                 // Extract 1-or-2 digit hour
	OpISOWeek                  // Extract ISO week number (01-53) at offset
	OpISOWeekDay               // Extract ISO weekday (1-7, Mon=1) at offset
	OpOrdinalDay               // Extract ordinal day of year (001-366) at offset
	OpTZZOrOffset              // 'Z' → UTC, or parse ±offset of Len bytes
)

// Inst is a single parse instruction.
// Kept small (8 bytes) so a Program fits in a few cache lines.
type Inst struct {
	Op     OpCode
	Offset byte   // Byte offset into the input string
	Len    byte   // Length of the field (0 = implied by Op)
	Aux    uint16 // Auxiliary data: pre-resolved month (1-12), literal byte, etc.
}
