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
	{"_2", FDay1or2, 2, 0}, // space-padded day — consumes 2 layout bytes, 1-2 input bytes

	// Single-character tokens
	{"3", FHour1or2, 0, 0}, // variable width
	{"2", FDay1or2, 0, 0},  // variable width
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

	var fields []Field
	pos := 0    // position in the layout string
	offset := 0 // position in the hypothetical input string
	hasDateTimeField := false

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
					Offset: offset,
					Len:    1,
					Aux:    uint16('.'),
				})
				offset++
				fields = append(fields, Field{
					Kind:   FFracSec,
					Offset: offset,
					Len:    tok.inputLen,
				})
				offset += tok.inputLen
				pos += len(tok.token)
				hasDateTimeField = true
				break
			}

			// Conditional timezone (Z07:00 / Z0700): single field
			if tok.kind == FTZZOrOffset {
				fields = append(fields, Field{
					Kind:   FTZZOrOffset,
					Offset: offset,
					Len:    tok.inputLen, // length of the offset form (+HH:MM or +HHMM)
				})
				// Input width is variable: 1 byte for 'Z', or inputLen bytes for offset.
				// We don't advance offset by a fixed amount since the executor
				// handles the variable width. But for fields after this, we won't
				// have more tokens (TZ is always last), so it doesn't matter.
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
				Offset: offset,
				Len:    inputWidth,
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
				Offset: offset,
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
