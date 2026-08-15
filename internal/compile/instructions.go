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
//	OpLiteral     what the byte at this offset has to be
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
//
// Both readings that describe a byte use the same three ranges, so litAccepts
// answers for either:
//
//	0          OpLiteral: any byte that is not a digit. Numeric ops: no separator
//	1..255     that exact byte
//	256 and up auxClassBase set, and the low byte a mask of LitClass bits
const (
	// sepNone is Aux on a numeric op that carries no fused separator.
	sepNone = 0

	// auxClassBase marks an Aux that holds a class mask rather than a byte. It
	// is the first bit above byte range, so a class cannot collide with an
	// exact byte and the mask is what is left when the value is truncated to
	// one. That truncation is the whole encoding: the executor reaches the mask
	// with a uint8 conversion and no shift.
	auxClassBase = 256

	// sepAnyNonDigit accepts any byte that is not a digit, which is what a
	// literal or a fused separator asked for before the classes existed.
	//
	// It is the loosest class and no trie format uses it now. It stays because
	// it is the right answer for a run whose class is not known: what no
	// separator position contains is a digit, and that much is always worth
	// enforcing, since a digit is a numeric token and which token sits where is
	// what picks the format.
	sepAnyNonDigit = auxClassBase | 1<<uint16(ClassAny)
)

// LitClass is the set of bytes a literal or a fused separator accepts. It
// mirrors the character classes detect.Scan assigns, because a trie entry
// matches a signature of those classes and one entry serves every byte in the
// class: ISO8601_DATE reads "2024-03-15", "2024/03/15" and "2024.03.15" through
// the same fields, so naming '-' would refuse two inputs detection accepts.
//
// Asking only that the byte is not a digit was too loose in the other
// direction. NUMERIC_MDY and TIME_HMS are both DD?DD?DD, and ':' is not a
// digit, so a layout detected from "20-1-00" read "10:01:00" as the tenth of
// January 2000 and a layout detected from "10:30:45" read "12/25/24" as
// 12:25:24. The class is what tells those two formats apart.
//
// The enum lives here rather than in detect because detect imports compile.
// detect maps its own CharClass onto these; the two sets have to stay in step,
// and TestLitClassSupersetsScan is what checks that they do.
type LitClass uint16

const (
	ClassAny     LitClass = iota // any byte that is not a digit
	ClassSep                     // - / .
	ClassSpace                   // space, tab
	ClassColon                   // :
	ClassSpecial                 // T Z - + ,
	ClassLetter                  // anything Scan would call a letter

	numLitClasses // sentinel, must be last
)

// AuxFor returns the Aux value for a literal that has to match class k.
//
// A class holding exactly one byte compiles to that byte instead. ClassColon is
// the only one today: Scan gives CColon to ':' and to nothing else, so the two
// encodings accept the same input and the executor settles the byte with a
// compare rather than a table lookup. That is most of the colons in the
// datetime formats.
func AuxFor(k LitClass) uint16 {
	if b := classSoleByte[k]; b >= 0 {
		return uint16(b)
	}
	return AuxClass(k)
}

// AuxClass returns the Aux value that makes a literal or a fused separator
// accept every byte in c, whether or not c holds only one.
func AuxClass(c LitClass) uint16 { return auxClassBase | 1<<uint16(c) }

// AuxAccepts reports whether c satisfies an Aux code.
//
// It exists for the test in detect that holds each class to be a superset of
// what Scan assigns to the CharClass it maps from. That test asks about the Aux
// the trie actually stamps rather than about the class, so the sole-byte form
// above is covered by it too.
func AuxAccepts(aux uint16, c byte) bool { return litAccepts(aux, c) }

// classSoleByte holds the one byte a class accepts, for the classes that accept
// exactly one, and -1 for the rest. Counted off litClassSet rather than written
// down, so it cannot come to disagree with it.
var classSoleByte = buildClassSoleBytes()

func buildClassSoleBytes() [numLitClasses]int16 {
	var sole [numLitClasses]int16
	for k := LitClass(0); k < numLitClasses; k++ {
		sole[k] = -1
		n, last := 0, 0
		for i := 0; i < 256; i++ {
			if litClassSet[i]&uint8(1<<k) != 0 {
				n, last = n+1, i
			}
		}
		if n == 1 {
			sole[k] = int16(last)
		}
	}
	return sole
}

// litClassSet[b] holds one bit per LitClass that byte b belongs to, so the
// executor answers "is this byte in that class" with a load and a mask rather
// than a switch. The table is 256 bytes and is built once at init.
var litClassSet = buildLitClassSet()

// One bit per class in a uint8, so the enum may not outgrow a byte. A wider
// entry would need a wider table, and this fails the build rather than
// silently dropping the classes past the eighth.
var _ [8 - int(numLitClasses)]struct{}

// buildLitClassSet spells out which bytes each class holds.
//
// Every set is a superset of what detect.Scan can assign to the matching
// CharClass, and has to stay one: a literal that refuses a byte Scan classified
// refuses an input detection accepted. Scan reads three of its classes from
// context ('T' between digits, 'Z' at the end, '-' after a time) and these sets
// are context-free, so they hold those bytes under both readings.
func buildLitClassSet() [256]uint8 {
	var t [256]uint8
	for i := range t {
		c := byte(i)
		digit := c >= '0' && c <= '9'
		add := func(k LitClass) { t[i] |= uint8(1) << k }

		if !digit {
			add(ClassAny)
		}
		switch c {
		case '-', '/', '.':
			add(ClassSep)
		case ' ', '\t':
			add(ClassSpace)
		case ':':
			add(ClassColon)
		}
		switch c {
		case 'T', 'Z', '-', '+', ',':
			add(ClassSpecial)
		}
		// Scan's letter arm and its default arm between them take every byte
		// that is not a digit and not one of the eight it names, which includes
		// the non-ASCII bytes the default arm calls a letter.
		switch c {
		case '-', '/', '.', ' ', '\t', ':', '+', ',':
		default:
			if !digit {
				add(ClassLetter)
			}
		}
	}
	return t
}

// litAccepts reports whether c satisfies an Aux code, under the three readings
// above. Kept minimal for inlining: it sits inside numericW, which runs on
// every fixed-width numeric field of every parse.
//
// Zero means "any byte that is not a digit" here, which is what OpLiteral wants
// from it. A numeric op reads zero as "no separator" and never calls this.
//
// The class arm indexes the table with c, which is a byte, so a hand-built
// Program carrying a nonsense Aux reads no further than any other and refuses
// on a mask nothing matches.
func litAccepts(aux uint16, c byte) bool {
	if aux == 0 {
		return c < '0' || c > '9'
	}
	if aux < auxClassBase {
		return c == byte(aux)
	}
	return litClassSet[c]&uint8(aux) != 0
}

// fusesSeparator reports whether a field of kind k can carry a fused separator,
// and how many bytes it reads itself.
//
// Only the fixed-width numeric kinds qualify. Their width is known here, their
// Aux is unused, and their arm in the executor sets w to a constant. The
// variable-width kinds are excluded deliberately: their width comes from the
// input and interacts with the executor's delta, and no format in the tree needs
// them fused, so they keep their separator as its own instruction.
//
// The width is int32 because its only caller adds it to a Field.Offset.
func fusesSeparator(k FieldKind) (int32, bool) {
	switch k {
	case FYear4:
		return 4, true
	case FYear2, FMonth2, FDay2, FHour24, FHour12, FMinute2, FSecond2, FISOWeek,
		FDaySpacePad:
		return 2, true
	}
	return 0, false
}
