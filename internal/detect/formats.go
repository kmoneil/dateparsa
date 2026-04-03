package detect

import "github.com/kmoneil/dateparsa/internal/compile"

// formatEntry pairs a signature pattern with the format definition it matches.
// Signatures use the same CharClass values as the scanner.
type formatEntry struct {
	name     string
	goLayout string
	sig      []CharClass // Expected signature pattern
	fields   []compile.Field
	ambig    bool // True if this signature is ambiguous (DD/MM vs MM/DD)
}

// Phase 1 format definitions. Each entry maps a character-class signature
// to the fields that should be extracted.
//
// Naming convention: category_variant
// Signatures are written using: D=digit, L=letter, S=sep, W=space, C=colon, X=special

func phase1Formats() []formatEntry {
	return []formatEntry{
		// === ISO 8601 / RFC 3339 ===

		// 2024-03-15
		{
			name:     "ISO8601_DATE",
			goLayout: "2006-01-02",
			sig:      sig("DDDDSDDSDD"),
			fields: []compile.Field{
				{Kind: compile.FYear4, Offset: 0, Len: 4},
				{Kind: compile.FLiteral, Offset: 4, Len: 1},
				{Kind: compile.FMonth2, Offset: 5, Len: 2},
				{Kind: compile.FLiteral, Offset: 7, Len: 1},
				{Kind: compile.FDay2, Offset: 8, Len: 2},
			},
		},
		// 2024-03-15T10:30:00
		{
			name:     "ISO8601_DATETIME",
			goLayout: "2006-01-02T15:04:05",
			sig:      sig("DDDDSDDSDDXDDCDDCDD"),
			fields: []compile.Field{
				{Kind: compile.FYear4, Offset: 0, Len: 4},
				{Kind: compile.FLiteral, Offset: 4, Len: 1},
				{Kind: compile.FMonth2, Offset: 5, Len: 2},
				{Kind: compile.FLiteral, Offset: 7, Len: 1},
				{Kind: compile.FDay2, Offset: 8, Len: 2},
				{Kind: compile.FLiteral, Offset: 10, Len: 1}, // T
				{Kind: compile.FHour24, Offset: 11, Len: 2},
				{Kind: compile.FLiteral, Offset: 13, Len: 1}, // :
				{Kind: compile.FMinute2, Offset: 14, Len: 2},
				{Kind: compile.FLiteral, Offset: 16, Len: 1}, // :
				{Kind: compile.FSecond2, Offset: 17, Len: 2},
			},
		},
		// 2024-03-15T10:30:00Z
		{
			name:     "ISO8601_DATETIME_Z",
			goLayout: "2006-01-02T15:04:05Z",
			sig:      sig("DDDDSDDSDDXDDCDDCDDX"),
			fields: []compile.Field{
				{Kind: compile.FYear4, Offset: 0, Len: 4},
				{Kind: compile.FLiteral, Offset: 4, Len: 1},
				{Kind: compile.FMonth2, Offset: 5, Len: 2},
				{Kind: compile.FLiteral, Offset: 7, Len: 1},
				{Kind: compile.FDay2, Offset: 8, Len: 2},
				{Kind: compile.FLiteral, Offset: 10, Len: 1},
				{Kind: compile.FHour24, Offset: 11, Len: 2},
				{Kind: compile.FLiteral, Offset: 13, Len: 1},
				{Kind: compile.FMinute2, Offset: 14, Len: 2},
				{Kind: compile.FLiteral, Offset: 16, Len: 1},
				{Kind: compile.FSecond2, Offset: 17, Len: 2},
				{Kind: compile.FTZZ, Offset: 19, Len: 1},
			},
		},
		// 2024-03-15T10:30:00+05:30
		{
			name:     "RFC3339",
			goLayout: "2006-01-02T15:04:05-07:00",
			sig:      sig("DDDDSDDSDDXDDCDDCDDXDDCDD"),
			fields: []compile.Field{
				{Kind: compile.FYear4, Offset: 0, Len: 4},
				{Kind: compile.FLiteral, Offset: 4, Len: 1},
				{Kind: compile.FMonth2, Offset: 5, Len: 2},
				{Kind: compile.FLiteral, Offset: 7, Len: 1},
				{Kind: compile.FDay2, Offset: 8, Len: 2},
				{Kind: compile.FLiteral, Offset: 10, Len: 1},
				{Kind: compile.FHour24, Offset: 11, Len: 2},
				{Kind: compile.FLiteral, Offset: 13, Len: 1},
				{Kind: compile.FMinute2, Offset: 14, Len: 2},
				{Kind: compile.FLiteral, Offset: 16, Len: 1},
				{Kind: compile.FSecond2, Offset: 17, Len: 2},
				{Kind: compile.FTZOffset, Offset: 19, Len: 6},
			},
		},
		// 2024-03-15T10:30:00.123456789Z (RFC3339 with nanoseconds)
		{
			name:     "RFC3339_NANO",
			goLayout: "2006-01-02T15:04:05.999999999Z",
			sig:      sig("DDDDSDDSDDXDDCDDCDDSDDDDDDDDDX"),
			fields: []compile.Field{
				{Kind: compile.FYear4, Offset: 0, Len: 4},
				{Kind: compile.FLiteral, Offset: 4, Len: 1},
				{Kind: compile.FMonth2, Offset: 5, Len: 2},
				{Kind: compile.FLiteral, Offset: 7, Len: 1},
				{Kind: compile.FDay2, Offset: 8, Len: 2},
				{Kind: compile.FLiteral, Offset: 10, Len: 1},
				{Kind: compile.FHour24, Offset: 11, Len: 2},
				{Kind: compile.FLiteral, Offset: 13, Len: 1},
				{Kind: compile.FMinute2, Offset: 14, Len: 2},
				{Kind: compile.FLiteral, Offset: 16, Len: 1},
				{Kind: compile.FSecond2, Offset: 17, Len: 2},
				{Kind: compile.FLiteral, Offset: 19, Len: 1}, // .
				{Kind: compile.FFracSec, Offset: 20, Len: 9},
				{Kind: compile.FTZZ, Offset: 29, Len: 1},
			},
		},

		// === SQL / Database ===

		// 2024-03-15 10:30:00 (PostgreSQL / MySQL datetime)
		{
			name:     "SQL_DATETIME",
			goLayout: "2006-01-02 15:04:05",
			sig:      sig("DDDDSDDSDDWDDCDDCDD"),
			fields: []compile.Field{
				{Kind: compile.FYear4, Offset: 0, Len: 4},
				{Kind: compile.FLiteral, Offset: 4, Len: 1},
				{Kind: compile.FMonth2, Offset: 5, Len: 2},
				{Kind: compile.FLiteral, Offset: 7, Len: 1},
				{Kind: compile.FDay2, Offset: 8, Len: 2},
				{Kind: compile.FLiteral, Offset: 10, Len: 1},
				{Kind: compile.FHour24, Offset: 11, Len: 2},
				{Kind: compile.FLiteral, Offset: 13, Len: 1},
				{Kind: compile.FMinute2, Offset: 14, Len: 2},
				{Kind: compile.FLiteral, Offset: 16, Len: 1},
				{Kind: compile.FSecond2, Offset: 17, Len: 2},
			},
		},

		// === Numeric with separator (DD/DD/DDDD, DD.DD.DDDD, DD-DD-DDDD) ===
		// These are all ambiguous in the trie — the detect layer resolves
		// them by checking separator character and value ranges.
		{
			name:  "NUMERIC_SEP",
			ambig: true,
			sig:   sig("DDSDDSDDDD"),
			// Fields are built dynamically by resolveAmbiguous.
		},

		// === Time Only ===

		// 10:30
		{
			name:     "TIME_HM",
			goLayout: "15:04",
			sig:      sig("DDCDD"),
			fields: []compile.Field{
				{Kind: compile.FHour24, Offset: 0, Len: 2},
				{Kind: compile.FLiteral, Offset: 2, Len: 1},
				{Kind: compile.FMinute2, Offset: 3, Len: 2},
			},
		},
		// 10:30:00
		{
			name:     "TIME_HMS",
			goLayout: "15:04:05",
			sig:      sig("DDCDDCDD"),
			fields: []compile.Field{
				{Kind: compile.FHour24, Offset: 0, Len: 2},
				{Kind: compile.FLiteral, Offset: 2, Len: 1},
				{Kind: compile.FMinute2, Offset: 3, Len: 2},
				{Kind: compile.FLiteral, Offset: 5, Len: 1},
				{Kind: compile.FSecond2, Offset: 6, Len: 2},
			},
		},
	}
}

// Phase 2 format definitions.
func phase2Formats() []formatEntry {
	return []formatEntry{

		// === Compact Formats ===

		// 20240315 (YYYYMMDD — 8 digits)
		{
			name:     "COMPACT_DATE",
			goLayout: "20060102",
			sig:      sig("DDDDDDDD"),
			fields: []compile.Field{
				{Kind: compile.FYear4, Offset: 0, Len: 4},
				{Kind: compile.FMonth2, Offset: 4, Len: 2},
				{Kind: compile.FDay2, Offset: 6, Len: 2},
			},
		},
		// 20240315T103000 (YYYYMMDDTHHmmss — 15 chars)
		{
			name: "COMPACT_DATETIME",
			sig:  sig("DDDDDDDDXDDDDDD"),
			fields: []compile.Field{
				{Kind: compile.FYear4, Offset: 0, Len: 4},
				{Kind: compile.FMonth2, Offset: 4, Len: 2},
				{Kind: compile.FDay2, Offset: 6, Len: 2},
				{Kind: compile.FLiteral, Offset: 8, Len: 1},
				{Kind: compile.FHour24, Offset: 9, Len: 2},
				{Kind: compile.FMinute2, Offset: 11, Len: 2},
				{Kind: compile.FSecond2, Offset: 13, Len: 2},
			},
		},
		// 20240315103000 (YYYYMMDDHHmmss — 14 digits)
		{
			name: "COMPACT_DATETIME_NOSEP",
			sig:  sig("DDDDDDDDDDDDDD"),
			fields: []compile.Field{
				{Kind: compile.FYear4, Offset: 0, Len: 4},
				{Kind: compile.FMonth2, Offset: 4, Len: 2},
				{Kind: compile.FDay2, Offset: 6, Len: 2},
				{Kind: compile.FHour24, Offset: 8, Len: 2},
				{Kind: compile.FMinute2, Offset: 10, Len: 2},
				{Kind: compile.FSecond2, Offset: 12, Len: 2},
			},
		},
		// 20240315T103000Z (16 chars)
		{
			name: "COMPACT_DATETIME_Z",
			sig:  sig("DDDDDDDDXDDDDDDX"),
			fields: []compile.Field{
				{Kind: compile.FYear4, Offset: 0, Len: 4},
				{Kind: compile.FMonth2, Offset: 4, Len: 2},
				{Kind: compile.FDay2, Offset: 6, Len: 2},
				{Kind: compile.FLiteral, Offset: 8, Len: 1},
				{Kind: compile.FHour24, Offset: 9, Len: 2},
				{Kind: compile.FMinute2, Offset: 11, Len: 2},
				{Kind: compile.FSecond2, Offset: 13, Len: 2},
				{Kind: compile.FTZZ, Offset: 15, Len: 1},
			},
		},

		// === Time with AM/PM ===

		// 10:30 PM (HH:MM AM/PM)
		{
			name:     "TIME_HM_AMPM",
			goLayout: "03:04 PM",
			sig:      sig("DDCDDWLL"),
			fields: []compile.Field{
				{Kind: compile.FHour12, Offset: 0, Len: 2},
				{Kind: compile.FLiteral, Offset: 2, Len: 1},
				{Kind: compile.FMinute2, Offset: 3, Len: 2},
				{Kind: compile.FLiteral, Offset: 5, Len: 1},
				{Kind: compile.FAMPM, Offset: 6, Len: 2},
			},
		},
		// 10:30:00 PM (HH:MM:SS AM/PM)
		{
			name:     "TIME_HMS_AMPM",
			goLayout: "03:04:05 PM",
			sig:      sig("DDCDDCDDWLL"),
			fields: []compile.Field{
				{Kind: compile.FHour12, Offset: 0, Len: 2},
				{Kind: compile.FLiteral, Offset: 2, Len: 1},
				{Kind: compile.FMinute2, Offset: 3, Len: 2},
				{Kind: compile.FLiteral, Offset: 5, Len: 1},
				{Kind: compile.FSecond2, Offset: 6, Len: 2},
				{Kind: compile.FLiteral, Offset: 8, Len: 1},
				{Kind: compile.FAMPM, Offset: 9, Len: 2},
			},
		},

		// === Time with fractional seconds ===

		// 10:30:00.123 (HH:MM:SS.mmm — 12 chars)
		{
			name:     "TIME_HMS_FRAC3",
			goLayout: "15:04:05.000",
			sig:      sig("DDCDDCDDSDDD"),
			fields: []compile.Field{
				{Kind: compile.FHour24, Offset: 0, Len: 2},
				{Kind: compile.FLiteral, Offset: 2, Len: 1},
				{Kind: compile.FMinute2, Offset: 3, Len: 2},
				{Kind: compile.FLiteral, Offset: 5, Len: 1},
				{Kind: compile.FSecond2, Offset: 6, Len: 2},
				{Kind: compile.FLiteral, Offset: 8, Len: 1},
				{Kind: compile.FFracSec, Offset: 9, Len: 3},
			},
		},
		// 10:30:00.123456 (HH:MM:SS.ffffff — 15 chars)
		{
			name:     "TIME_HMS_FRAC6",
			goLayout: "15:04:05.000000",
			sig:      sig("DDCDDCDDSDDDDDD"),
			fields: []compile.Field{
				{Kind: compile.FHour24, Offset: 0, Len: 2},
				{Kind: compile.FLiteral, Offset: 2, Len: 1},
				{Kind: compile.FMinute2, Offset: 3, Len: 2},
				{Kind: compile.FLiteral, Offset: 5, Len: 1},
				{Kind: compile.FSecond2, Offset: 6, Len: 2},
				{Kind: compile.FLiteral, Offset: 8, Len: 1},
				{Kind: compile.FFracSec, Offset: 9, Len: 6},
			},
		},

		// === SQL datetime with fractional seconds ===

		// 2024-03-15 10:30:00.000000 (PostgreSQL with microseconds — 26 chars)
		// 2024-03-15 10:30:00.000000
		// DDDDSDDSDDWDDCDDCDDSDDDDDDD
		// D4(0-3) S(4) D2(5-6) S(7) D2(8-9) W(10) D2(11-12) C(13) D2(14-15) C(16) D2(17-18) S(19) D6(20-25) = 26 chars
		// 2024-03-15 10:30:00.000000 (26 chars)
		{
			name:     "SQL_DATETIME_FRAC6",
			goLayout: "2006-01-02 15:04:05.000000",
			sig:      sig("DDDDSDDSDDWDDCDDCDDSDDDDDD"),
			fields: []compile.Field{
				{Kind: compile.FYear4, Offset: 0, Len: 4},
				{Kind: compile.FLiteral, Offset: 4, Len: 1},
				{Kind: compile.FMonth2, Offset: 5, Len: 2},
				{Kind: compile.FLiteral, Offset: 7, Len: 1},
				{Kind: compile.FDay2, Offset: 8, Len: 2},
				{Kind: compile.FLiteral, Offset: 10, Len: 1},
				{Kind: compile.FHour24, Offset: 11, Len: 2},
				{Kind: compile.FLiteral, Offset: 13, Len: 1},
				{Kind: compile.FMinute2, Offset: 14, Len: 2},
				{Kind: compile.FLiteral, Offset: 16, Len: 1},
				{Kind: compile.FSecond2, Offset: 17, Len: 2},
				{Kind: compile.FLiteral, Offset: 19, Len: 1},
				{Kind: compile.FFracSec, Offset: 20, Len: 6},
			},
		},

		// 2024-03-15 10:30:00.000 (SQL with milliseconds — 23 chars)
		{
			name:     "SQL_DATETIME_FRAC3",
			goLayout: "2006-01-02 15:04:05.000",
			sig:      sig("DDDDSDDSDDWDDCDDCDDSDDD"),
			fields: []compile.Field{
				{Kind: compile.FYear4, Offset: 0, Len: 4},
				{Kind: compile.FLiteral, Offset: 4, Len: 1},
				{Kind: compile.FMonth2, Offset: 5, Len: 2},
				{Kind: compile.FLiteral, Offset: 7, Len: 1},
				{Kind: compile.FDay2, Offset: 8, Len: 2},
				{Kind: compile.FLiteral, Offset: 10, Len: 1},
				{Kind: compile.FHour24, Offset: 11, Len: 2},
				{Kind: compile.FLiteral, Offset: 13, Len: 1},
				{Kind: compile.FMinute2, Offset: 14, Len: 2},
				{Kind: compile.FLiteral, Offset: 16, Len: 1},
				{Kind: compile.FSecond2, Offset: 17, Len: 2},
				{Kind: compile.FLiteral, Offset: 19, Len: 1},
				{Kind: compile.FFracSec, Offset: 20, Len: 3},
			},
		},

		// === SQL datetime with timezone ===

		// 2024-03-15 10:30:00+00 (PostgreSQL short tz — 22 chars)
		{
			name:     "SQL_DATETIME_TZ_SHORT",
			goLayout: "2006-01-02 15:04:05-07",
			sig:      sig("DDDDSDDSDDWDDCDDCDDXDD"),
			fields: []compile.Field{
				{Kind: compile.FYear4, Offset: 0, Len: 4},
				{Kind: compile.FLiteral, Offset: 4, Len: 1},
				{Kind: compile.FMonth2, Offset: 5, Len: 2},
				{Kind: compile.FLiteral, Offset: 7, Len: 1},
				{Kind: compile.FDay2, Offset: 8, Len: 2},
				{Kind: compile.FLiteral, Offset: 10, Len: 1},
				{Kind: compile.FHour24, Offset: 11, Len: 2},
				{Kind: compile.FLiteral, Offset: 13, Len: 1},
				{Kind: compile.FMinute2, Offset: 14, Len: 2},
				{Kind: compile.FLiteral, Offset: 16, Len: 1},
				{Kind: compile.FSecond2, Offset: 17, Len: 2},
				{Kind: compile.FTZOffset, Offset: 19, Len: 3},
			},
		},

		// 2024-03-15 10:30:00+05:30 (SQL with full tz offset — 25 chars)
		{
			name:     "SQL_DATETIME_TZ",
			goLayout: "2006-01-02 15:04:05-07:00",
			sig:      sig("DDDDSDDSDDWDDCDDCDDXDDCDD"),
			fields: []compile.Field{
				{Kind: compile.FYear4, Offset: 0, Len: 4},
				{Kind: compile.FLiteral, Offset: 4, Len: 1},
				{Kind: compile.FMonth2, Offset: 5, Len: 2},
				{Kind: compile.FLiteral, Offset: 7, Len: 1},
				{Kind: compile.FDay2, Offset: 8, Len: 2},
				{Kind: compile.FLiteral, Offset: 10, Len: 1},
				{Kind: compile.FHour24, Offset: 11, Len: 2},
				{Kind: compile.FLiteral, Offset: 13, Len: 1},
				{Kind: compile.FMinute2, Offset: 14, Len: 2},
				{Kind: compile.FLiteral, Offset: 16, Len: 1},
				{Kind: compile.FSecond2, Offset: 17, Len: 2},
				{Kind: compile.FTZOffset, Offset: 19, Len: 6},
			},
		},

		// === RFC 3339 with fractional seconds + tz offset ===

		// 2024-03-15T10:30:00.123+05:30 (29 chars)
		{
			name:     "RFC3339_FRAC3_TZ",
			sig:      sig("DDDDSDDSDDXDDCDDCDDSDDDXDDCDD"),
			fields: []compile.Field{
				{Kind: compile.FYear4, Offset: 0, Len: 4},
				{Kind: compile.FLiteral, Offset: 4, Len: 1},
				{Kind: compile.FMonth2, Offset: 5, Len: 2},
				{Kind: compile.FLiteral, Offset: 7, Len: 1},
				{Kind: compile.FDay2, Offset: 8, Len: 2},
				{Kind: compile.FLiteral, Offset: 10, Len: 1},
				{Kind: compile.FHour24, Offset: 11, Len: 2},
				{Kind: compile.FLiteral, Offset: 13, Len: 1},
				{Kind: compile.FMinute2, Offset: 14, Len: 2},
				{Kind: compile.FLiteral, Offset: 16, Len: 1},
				{Kind: compile.FSecond2, Offset: 17, Len: 2},
				{Kind: compile.FLiteral, Offset: 19, Len: 1},
				{Kind: compile.FFracSec, Offset: 20, Len: 3},
				{Kind: compile.FTZOffset, Offset: 23, Len: 6},
			},
		},
	}
}

// sig converts a signature string (using D, L, S, W, C, X characters)
// into a CharClass slice.
func sig(s string) []CharClass {
	out := make([]CharClass, len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case 'D':
			out[i] = CDigit
		case 'L':
			out[i] = CLetter
		case 'S':
			out[i] = CSep
		case 'W':
			out[i] = CSpace
		case 'C':
			out[i] = CColon
		case 'X':
			out[i] = CSpecial
		}
	}
	return out
}
