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
	OpNop                       // Reads nothing and covers nothing

	numOpCodes // sentinel, must be last
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
//
// The last two are not Scan's and no literal carries one. They describe a
// skipped run, and SkipRun has the reason they are narrower than ClassLetter:
// a skip that matched a weekday name may take another weekday name, but not an
// "AM" where it matched a NUL or an accent.
type LitClass uint16

const (
	ClassAny     LitClass = iota // any byte that is not a digit
	ClassSep                     // - / .
	ClassSpace                   // space, tab
	ClassColon                   // :
	ClassSpecial                 // T Z - + ,
	ClassLetter                  // anything Scan would call a letter
	ClassAlpha                   // A-Z a-z, and only in a skipped run
	ClassOpaque                  // a control byte other than tab, DEL, 0x80 and up; skips only

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

// SkipRun describes a skipped run one instruction at a time. It returns how
// many bytes from off, and short of end, a single skip can describe, and the
// Aux that skip carries; the caller covers the rest of the run with further
// calls. The executor holds every byte of a skip to its Aux, so the Aux is the
// whole of what a reused layout knows about the bytes detection scanned past.
//
// C28 is why a skip carries anything. "MAY1 00:00 1000" skips the space at
// offset 10 and reads a year at 11, and applied to "MAY1 00:00+0000" the skip
// swallowed the '+' and the year read "0000", 2026 years from what detection
// answers for those bytes, which it reads as a zone offset.
//
// C34 is why one Aux may not describe a run whose bytes differ. C28 gave the
// run the classes its bytes had in common, which is exact for a run of spaces
// and empty for "! ": '!' is ClassLetter, ' ' is ClassSpace, and a run whose
// bytes shared nothing narrower fell back to 0, which is any byte that is not
// a digit. So a layout from "MAY1 00:00! 1000" accepted "MAY1 00:00A+0000" and
// read year 0000, where detection reads the 'A' as a zone name and "+0000" as
// its offset. ", " is the same run in real input: a layout from
// "May 1 10:30:00, 2024" read "May 1 10:30:00 -0700" as the year 700 in UTC. A
// run whose bytes did share a class was loose too. Two spaces carried
// ClassSpace and took two tabs, and detection reads an AM/PM behind spaces and
// skips the same two letters behind tabs, so the layout answered 22:00 for a
// row detection reads as 10:00.
//
// So a run is cut into pieces, each its own skip, and what a piece is depends
// on the byte it starts with:
//
//	a letter        it and the letters, spaces,   "Mon ", "th", "Miércoles",
//	                tabs and opaque bytes after   "Central European Time"
//	                it: ClassAlpha, ClassSpace
//	                and ClassOpaque
//	an opaque byte  it and the opaque bytes       "年", "é", "\x00"
//	                after it: ClassOpaque
//	a digit         0, which refuses it
//	anything else   that byte, repeated           " ", "  ", ",", "--"
//
// Words are the one thing a piece gives anything up for, and it has to: the
// weekday a column's first row skipped is not the weekday of its second row,
// and a JavaScript date's "(Central European Summer Time)" is not the same
// words on every row. What makes that safe is where a word can sit. Detection
// reads letters as a token of their own in one place, behind a time and the
// spaces after it, where a lone 'Z' is UTC, "am" or "pm" is a meridiem, and any
// other letters are a zone name; and it tells a space from a tab in the same
// place, since it steps over one on its way there and not the other. Letters
// there are always a field, or the name in front of an offset, which
// detect.appendZoneNameSkip describes and never through here. So a piece that
// starts with a letter never starts where a letter means something, and inside
// one nothing detection reads changes when a letter, a space, a tab or an
// accented byte stands in for another.
//
// An opaque piece can start there, because isLetter is ASCII:
// "MAY1 10:00é 1000" skips the 'é' straight after the time. So an opaque piece
// takes no letters and no spaces, or "PM" would stand in for the 'é' and the
// layout would answer 10:00 for a row detection reads as 22:00. NUL is opaque,
// which matters: it is the one byte the exact form cannot carry, because an Aux
// of 0 already means any byte that is not a digit.
//
// Everything else is carried byte for byte, which is C28's one-byte rule
// applied to every length. ClassSpecial holds ',' and '+' together, and a
// space and a tab are different bytes wherever a piece of them can start, so
// no class narrower than the byte is safe.
//
// The cost is instructions, against a budget of MaxInstructions for the whole
// format. "Mon, " is three skips where it was one, because the comma and the
// space are each themselves. A run of words is one skip whatever its length:
// cutting at every space put a JavaScript date with its zone spelled out in
// brackets at 26 instructions, and Compile refused a format that parses on
// main.
func SkipRun(s string, off, end int) (n int, aux uint16) {
	if off < 0 || end > len(s) || off >= end {
		return 0, 0
	}
	// Resliced so that every index below is provably in range.
	run := s[off:end]
	c := run[0]
	n = 1
	switch k := litClassSet[c]; {
	case c >= '0' && c <= '9':
		// No detector leaves a digit unread, and TestNoUnreadRunCoversADigit
		// holds them to that. If one did, the program refuses the input it was
		// detected from rather than describing a number as noise.
		for n < len(run) && run[n] >= '0' && run[n] <= '9' {
			n++
		}
		return n, 0
	case k&alphaBit != 0:
		for n < len(run) && litClassSet[run[n]]&wordsMask != 0 {
			n++
		}
		return n, auxClassBase | uint16(wordsMask)
	case k&opaqueBit != 0:
		for n < len(run) && litClassSet[run[n]]&opaqueBit != 0 {
			n++
		}
		return n, AuxClass(ClassOpaque)
	}
	for n < len(run) && run[n] == c {
		n++
	}
	return n, uint16(c)
}

// alphaBit and opaqueBit select the two skip classes in a litClassSet entry,
// and wordsMask is what a piece of words may hold: those two and a space or a
// tab.
const (
	alphaBit  = uint8(1) << ClassAlpha
	opaqueBit = uint8(1) << ClassOpaque
	wordsMask = alphaBit | opaqueBit | uint8(1)<<ClassSpace
)

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
// silently dropping the classes past the eighth. The two skip classes took the
// last two bits, so a ninth class is a change to the encoding and not an entry.
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
		// The skip classes, which are not supersets of anything Scan assigns
		// and do not need to be, since no trie literal carries one. ClassAlpha
		// is what detection's isLetter accepts, the bytes an AM/PM, a zone name
		// or an English month is spelled with. ClassOpaque is the rest of what
		// is not printable ASCII, less the tab, which parseTimeComponent steps
		// over on its way to a time and so is not inert.
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			add(ClassAlpha)
		}
		if (c < ' ' && c != '\t') || c >= 0x7f {
			add(ClassOpaque)
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
