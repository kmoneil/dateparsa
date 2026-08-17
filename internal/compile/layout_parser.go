package compile

import (
	"errors"
	"fmt"
	"strings"
)

// goToken maps a Go reference-time token to its FieldKind and the width
// it consumes in the input string.
type goToken struct {
	token    string
	kind     FieldKind
	inputLen int    // expected width in the input string (0 = variable)
	aux      uint16 // auxiliary data (e.g., literal byte value)
}

// longestGoToken is the widest token below, and maxLayoutLen is how long a
// layout string can be and still have any chance of compiling.
//
// Every token this file recognises produces at least one Field, and so does
// every byte it does not recognise, which becomes a literal. So a layout of L
// bytes produces at least L/longestGoToken fields, and Compile refuses a def
// with more than MaxInstructions of them. A layout past that product cannot
// compile whatever it says, and the loop below used to find that out one Field
// at a time: 88 bytes of Fields per layout byte, so a megabyte of prose with a
// date token in it allocated 88MB and then returned an error.
//
// A layout string is source code rather than input, and SECURITY.md says so, so
// this is not a hole. It becomes one the day a caller reads layouts from a
// configuration file, which is a plausible thing for a tool to do.
//
// The product is derived rather than written down: 24 by 10 is 240 today, and a
// token wider than ten bytes moves the bound with it instead of silently making
// this refuse a layout that would have compiled. It is deliberately loose. The
// longest layout that compiles in practice is nearer 144 bytes, because the
// widest token produces two fields and the best ratio is six layout bytes per
// field, and a bound that is provably safe beats one that is tight.
var longestGoToken = func() int {
	n := 1
	for _, t := range goTokens {
		if len(t.token) > n {
			n = len(t.token)
		}
	}
	return n
}()

var maxLayoutLen = MaxInstructions * longestGoToken

// tokens is ordered longest-first for greedy matching.
// Unsupported tokens (month/day names) are listed with a sentinel kind
// so we can produce clear error messages.
var goTokens = []goToken{
	// Fractional seconds (longest first)
	{".000000000", FFracSec, 9, 0},
	{".000000", FFracSec, 6, 0},
	{".000", FFracSec, 3, 0},
	{".999999999", FFracSec, 9, 0},
	{".999999", FFracSec, 6, 0},
	{".999", FFracSec, 3, 0},

	// Conditional timezone: Z or offset
	{"Z07:00", FTZZOrOffset, 6, 0}, // Len=6 for the offset part (+HH:MM)
	{"Z0700", FTZZOrOffset, 5, 0},  // Len=5 for the offset part (+HHMM)

	// Timezone offsets
	{"-07:00", FTZOffset, 6, 0},
	{"-0700", FTZOffset, 5, 0},
	{"-07", FTZOffset, 3, 0},

	// Unsupported: month/day names (must be before shorter tokens)
	{"January", fieldKindUnsupported, 0, 0},
	{"Monday", fieldKindUnsupported, 0, 0},

	// Multi-character tokens
	{"2006", FYear4, 4, 0},
	{"Mon", fieldKindUnsupported, 0, 0},
	{"Jan", fieldKindUnsupported, 0, 0},
	{"MST", FTZName, 3, 0},

	// Two-character tokens
	{"PM", FAMPM, 2, 0},
	{"pm", FAMPM, 2, 0},
	{"15", FHour24, 2, 0},
	{"06", FYear2, 2, 0},
	{"05", FSecond2, 2, 0},
	{"04", FMinute2, 2, 0},
	{"03", FHour12, 2, 0},
	{"02", FDay2, 2, 0},
	{"01", FMonth2, 2, 0},
	{"_2", FDaySpacePad, 2, 0}, // space-padded day: two layout bytes, two input bytes

	// Single-character tokens
	{"3", FHour1or2, 0, 0},  // variable width
	{"2", FDay1or2, 0, 0},   // variable width
	{"1", FMonth1or2, 0, 0}, // variable width
}

const fieldKindUnsupported FieldKind = 255

// ParseGoLayout parses a Go time reference layout string into a FormatDef.
// Returns an error if the layout contains unsupported tokens (month/day names)
// or if no date/time fields are detected.
func ParseGoLayout(layout string) (*FormatDef, error) {
	if layout == "" {
		return nil, errors.New("compile: empty layout string")
	}
	if len(layout) > maxLayoutLen {
		return nil, fmt.Errorf("compile: layout is %d bytes and nothing over %d can compile:"+
			" %d instructions of at most %d layout bytes each is the whole budget",
			len(layout), maxLayoutLen, MaxInstructions, longestGoToken)
	}

	var fields []Field
	pos := 0    // position in the layout string
	offset := 0 // position in the hypothetical input string
	hasDateTimeField := false

	// Both counters stay int and convert where a Field is built. Field.Offset
	// and Field.Len are int32, and the conversion cannot truncate: offset sums
	// the widths of the tokens in one layout string, and Compile refuses any
	// field past maxFieldByte, which is 255, long before either counter could
	// reach the int32 range.

	for pos < len(layout) {
		matched := false
		for _, tok := range goTokens {
			if !strings.HasPrefix(layout[pos:], tok.token) {
				continue
			}

			if tok.kind == fieldKindUnsupported {
				return nil, fmt.Errorf("compile: unsupported token %q in layout %q"+
					" (month/day names require auto-detection; use Parse instead)", tok.token, layout)
			}

			matched = true

			// Fractional seconds: the layout token includes the dot prefix,
			// but the dot is a literal in the input. Emit a literal for the dot,
			// then the FFracSec field for the digits.
			if tok.kind == FFracSec {
				fields = append(fields, Field{
					Kind:   FLiteral,
					Offset: int32(offset),
					Len:    1,
					Aux:    uint16('.'),
				})
				offset++
				fields = append(fields, Field{
					Kind:   FFracSec,
					Offset: int32(offset),
					Len:    int32(tok.inputLen),
				})
				offset += tok.inputLen
				pos += len(tok.token)
				hasDateTimeField = true
				break
			}

			// Conditional timezone (Z07:00 / Z0700): single field.
			//
			// offset advances by the offset form's width, which is the wider
			// of the two the field can take. When the input carries a 'Z'
			// instead, the executor's delta shifts everything after it left by
			// the difference, the same machinery the 1-or-2 ops use.
			//
			// This did not advance offset at all, on the reasoning that "for
			// fields after this, we won't have more tokens (TZ is always
			// last)". That holds for the layouts this library ships. It does
			// not hold for a layout a caller writes, which is the entire point
			// of a public Compile: every field after the zone was assigned the
			// zone's own offset, so "15:04:05Z07:00 2006" read its year out of
			// the middle of the zone.
			if tok.kind == FTZZOrOffset {
				fields = append(fields, Field{
					Kind:   FTZZOrOffset,
					Offset: int32(offset),
					Len:    int32(tok.inputLen), // length of the offset form (+HH:MM or +HHMM)
				})
				offset += tok.inputLen
				pos += len(tok.token)
				hasDateTimeField = true
				break
			}

			// Variable-width fields: 1 or 2 input bytes
			inputWidth := tok.inputLen
			if inputWidth == 0 {
				// Variable width tokens (1, 2, 3) consume 1-2 bytes.
				// For offset tracking of subsequent fields, we assume minimum width.
				// The executor handles variable width at runtime.
				inputWidth = 1 // minimum; could be 2
			}

			// Space-padded day (_2): consumes 2 layout chars but 1-2 input bytes
			if tok.token == "_2" {
				inputWidth = 2 // space + digit or two digits
			}

			f := Field{
				Kind:   tok.kind,
				Offset: int32(offset),
				Len:    int32(inputWidth),
				Aux:    tok.aux,
			}

			// Track whether we have real date/time fields (not just literals/TZ)
			if tok.kind != FLiteral && tok.kind != FSkip {
				hasDateTimeField = true
			}

			fields = append(fields, f)
			offset += inputWidth
			pos += len(tok.token)
			break
		}

		if !matched {
			// Not a reference token — treat as a literal character.
			fields = append(fields, Field{
				Kind:   FLiteral,
				Offset: int32(offset),
				Len:    1,
				Aux:    uint16(layout[pos]),
			})
			offset++
			pos++
		}
	}

	if !hasDateTimeField {
		return nil, fmt.Errorf("compile: layout %q contains no date/time fields", layout)
	}

	return &FormatDef{
		Name:     "COMPILED",
		GoLayout: layout,
		Fields:   fields,
	}, nil
}
