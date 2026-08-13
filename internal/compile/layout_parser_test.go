package compile

import "testing"

func TestParseGoLayout(t *testing.T) {
	tests := []struct {
		name       string
		layout     string
		wantFields []FieldKind
		wantErr    bool
	}{
		{
			name:   "ISO date",
			layout: "2006-01-02",
			wantFields: []FieldKind{
				FYear4, FLiteral, FMonth2, FLiteral, FDay2,
			},
		},
		{
			name:   "ISO datetime",
			layout: "2006-01-02T15:04:05",
			wantFields: []FieldKind{
				FYear4, FLiteral, FMonth2, FLiteral, FDay2, FLiteral,
				FHour24, FLiteral, FMinute2, FLiteral, FSecond2,
			},
		},
		{
			name:   "US date",
			layout: "01/02/2006",
			wantFields: []FieldKind{
				FMonth2, FLiteral, FDay2, FLiteral, FYear4,
			},
		},
		{
			name:   "datetime with milliseconds",
			layout: "2006-01-02 15:04:05.000",
			wantFields: []FieldKind{
				FYear4, FLiteral, FMonth2, FLiteral, FDay2, FLiteral,
				FHour24, FLiteral, FMinute2, FLiteral, FSecond2, FLiteral,
				FFracSec,
			},
		},
		{
			name:   "datetime with microseconds",
			layout: "2006-01-02 15:04:05.000000",
			wantFields: []FieldKind{
				FYear4, FLiteral, FMonth2, FLiteral, FDay2, FLiteral,
				FHour24, FLiteral, FMinute2, FLiteral, FSecond2, FLiteral,
				FFracSec,
			},
		},
		{
			name:   "datetime with nanoseconds",
			layout: "2006-01-02 15:04:05.000000000",
			wantFields: []FieldKind{
				FYear4, FLiteral, FMonth2, FLiteral, FDay2, FLiteral,
				FHour24, FLiteral, FMinute2, FLiteral, FSecond2, FLiteral,
				FFracSec,
			},
		},
		{
			name:   "non-padded month and day",
			layout: "2006-1-2",
			wantFields: []FieldKind{
				FYear4, FLiteral, FMonth1or2, FLiteral, FDay1or2,
			},
		},
		{
			name:   "2-digit year",
			layout: "01/02/06",
			wantFields: []FieldKind{
				FMonth2, FLiteral, FDay2, FLiteral, FYear2,
			},
		},
		{
			name:   "12-hour with AM/PM uppercase",
			layout: "03:04PM",
			wantFields: []FieldKind{
				FHour12, FLiteral, FMinute2, FAMPM,
			},
		},
		{
			name:   "12-hour with am/pm lowercase",
			layout: "03:04pm",
			wantFields: []FieldKind{
				FHour12, FLiteral, FMinute2, FAMPM,
			},
		},
		{
			name:   "RFC3339 with Z07:00",
			layout: "2006-01-02T15:04:05Z07:00",
			wantFields: []FieldKind{
				FYear4, FLiteral, FMonth2, FLiteral, FDay2, FLiteral,
				FHour24, FLiteral, FMinute2, FLiteral, FSecond2,
				FTZZOrOffset,
			},
		},
		{
			name:   "compact Z0700",
			layout: "2006-01-02T15:04:05Z0700",
			wantFields: []FieldKind{
				FYear4, FLiteral, FMonth2, FLiteral, FDay2, FLiteral,
				FHour24, FLiteral, FMinute2, FLiteral, FSecond2,
				FTZZOrOffset,
			},
		},
		{
			name:   "timezone offset -07:00",
			layout: "2006-01-02T15:04:05-07:00",
			wantFields: []FieldKind{
				FYear4, FLiteral, FMonth2, FLiteral, FDay2, FLiteral,
				FHour24, FLiteral, FMinute2, FLiteral, FSecond2,
				FTZOffset,
			},
		},
		{
			name:   "compact timezone offset -0700",
			layout: "2006-01-02T15:04:05-0700",
			wantFields: []FieldKind{
				FYear4, FLiteral, FMonth2, FLiteral, FDay2, FLiteral,
				FHour24, FLiteral, FMinute2, FLiteral, FSecond2,
				FTZOffset,
			},
		},
		{
			name:   "hours-only offset -07",
			layout: "2006-01-02T15:04:05-07",
			wantFields: []FieldKind{
				FYear4, FLiteral, FMonth2, FLiteral, FDay2, FLiteral,
				FHour24, FLiteral, FMinute2, FLiteral, FSecond2,
				FTZOffset,
			},
		},
		{
			name:   "timezone name MST",
			layout: "2006-01-02 15:04:05 MST",
			wantFields: []FieldKind{
				FYear4, FLiteral, FMonth2, FLiteral, FDay2, FLiteral,
				FHour24, FLiteral, FMinute2, FLiteral, FSecond2, FLiteral,
				FTZName,
			},
		},
		{
			name:   "space-padded day",
			layout: "2006-01-_2",
			wantFields: []FieldKind{
				FYear4, FLiteral, FMonth2, FLiteral, FDay1or2,
			},
		},
		{
			name:   "non-padded 12-hour",
			layout: "3:04PM",
			wantFields: []FieldKind{
				FHour1or2, FLiteral, FMinute2, FAMPM,
			},
		},
		{
			name:   "time only",
			layout: "15:04:05",
			wantFields: []FieldKind{
				FHour24, FLiteral, FMinute2, FLiteral, FSecond2,
			},
		},
		{
			name:   "offset only -0700",
			layout: "-0700",
			wantFields: []FieldKind{
				FTZOffset,
			},
		},
		{
			name:   "offset only -07:00",
			layout: "-07:00",
			wantFields: []FieldKind{
				FTZOffset,
			},
		},
		{
			name:   "optional fractional .999",
			layout: "2006-01-02 15:04:05.999",
			wantFields: []FieldKind{
				FYear4, FLiteral, FMonth2, FLiteral, FDay2, FLiteral,
				FHour24, FLiteral, FMinute2, FLiteral, FSecond2, FLiteral,
				FFracSec,
			},
		},
		{
			name:   "optional fractional .999999",
			layout: "2006-01-02 15:04:05.999999",
			wantFields: []FieldKind{
				FYear4, FLiteral, FMonth2, FLiteral, FDay2, FLiteral,
				FHour24, FLiteral, FMinute2, FLiteral, FSecond2, FLiteral,
				FFracSec,
			},
		},
		{
			name:   "optional fractional .999999999",
			layout: "2006-01-02 15:04:05.999999999",
			wantFields: []FieldKind{
				FYear4, FLiteral, FMonth2, FLiteral, FDay2, FLiteral,
				FHour24, FLiteral, FMinute2, FLiteral, FSecond2, FLiteral,
				FFracSec,
			},
		},

		// Error cases
		{
			name:    "empty layout",
			layout:  "",
			wantErr: true,
		},
		{
			name:    "abbreviated month name unsupported",
			layout:  "Jan 2, 2006",
			wantErr: true,
		},
		{
			name:    "full month name unsupported",
			layout:  "January 2, 2006",
			wantErr: true,
		},
		{
			name:    "day name unsupported",
			layout:  "Mon, 02 Jan 2006",
			wantErr: true,
		},
		{
			name:    "no date fields detected",
			layout:  "hello world",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def, err := ParseGoLayout(tt.layout)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(def.Fields) != len(tt.wantFields) {
				t.Fatalf("got %d fields, want %d\nfields: %+v",
					len(def.Fields), len(tt.wantFields), def.Fields)
			}
			for i, wk := range tt.wantFields {
				if def.Fields[i].Kind != wk {
					t.Errorf("field[%d].Kind = %d (%s), want %d (%s)",
						i, def.Fields[i].Kind, fieldKindName(def.Fields[i].Kind),
						wk, fieldKindName(wk))
				}
			}
		})
	}
}

func TestParseGoLayout_Offsets(t *testing.T) {
	def, err := ParseGoLayout("2006-01-02")
	if err != nil {
		t.Fatal(err)
	}
	// Expected: Year4@0(4), Lit@4(1), Month2@5(2), Lit@7(1), Day2@8(2)
	expected := []struct {
		kind   FieldKind
		offset int
		length int
	}{
		{FYear4, 0, 4},
		{FLiteral, 4, 1},
		{FMonth2, 5, 2},
		{FLiteral, 7, 1},
		{FDay2, 8, 2},
	}
	if len(def.Fields) != len(expected) {
		t.Fatalf("got %d fields, want %d", len(def.Fields), len(expected))
	}
	for i, e := range expected {
		f := def.Fields[i]
		if f.Kind != e.kind || f.Offset != e.offset || f.Len != e.length {
			t.Errorf("field[%d] = {Kind:%d, Offset:%d, Len:%d}, want {Kind:%d, Offset:%d, Len:%d}",
				i, f.Kind, f.Offset, f.Len, e.kind, e.offset, e.length)
		}
	}
}

func TestParseGoLayout_FracSecLength(t *testing.T) {
	tests := []struct {
		layout  string
		fracLen int
	}{
		{"15:04:05.000", 3},
		{"15:04:05.000000", 6},
		{"15:04:05.000000000", 9},
		{"15:04:05.999", 3},
		{"15:04:05.999999", 6},
		{"15:04:05.999999999", 9},
	}
	for _, tt := range tests {
		t.Run(tt.layout, func(t *testing.T) {
			def, err := ParseGoLayout(tt.layout)
			if err != nil {
				t.Fatal(err)
			}
			// Find the FFracSec field
			var found bool
			for _, f := range def.Fields {
				if f.Kind == FFracSec {
					found = true
					if f.Len != tt.fracLen {
						t.Errorf("FFracSec.Len = %d, want %d", f.Len, tt.fracLen)
					}
				}
			}
			if !found {
				t.Error("no FFracSec field found")
			}
		})
	}
}

func TestParseGoLayout_TZZOrOffsetLen(t *testing.T) {
	tests := []struct {
		layout string
		auxLen int // expected Len on the FTZZOrOffset field (offset part length)
	}{
		{"2006-01-02T15:04:05Z07:00", 6}, // +HH:MM = 6 bytes
		{"2006-01-02T15:04:05Z0700", 5},  // +HHMM = 5 bytes
	}
	for _, tt := range tests {
		t.Run(tt.layout, func(t *testing.T) {
			def, err := ParseGoLayout(tt.layout)
			if err != nil {
				t.Fatal(err)
			}
			var found bool
			for _, f := range def.Fields {
				if f.Kind == FTZZOrOffset {
					found = true
					if f.Len != tt.auxLen {
						t.Errorf("FTZZOrOffset.Len = %d, want %d", f.Len, tt.auxLen)
					}
				}
			}
			if !found {
				t.Error("no FTZZOrOffset field found")
			}
		})
	}
}

func TestParseGoLayout_GoLayoutPreserved(t *testing.T) {
	layouts := []string{
		"2006-01-02",
		"2006-01-02T15:04:05Z07:00",
		"01/02/2006",
		"15:04:05",
		"03:04PM",
	}
	for _, layout := range layouts {
		t.Run(layout, func(t *testing.T) {
			def, err := ParseGoLayout(layout)
			if err != nil {
				t.Fatal(err)
			}
			if def.GoLayout != layout {
				t.Errorf("GoLayout = %q, want %q", def.GoLayout, layout)
			}
		})
	}
}

// fieldKindName returns a human-readable name for debugging.
func fieldKindName(k FieldKind) string {
	names := map[FieldKind]string{
		FYear4:       "FYear4",
		FYear2:       "FYear2",
		FMonth2:      "FMonth2",
		FMonth1or2:   "FMonth1or2",
		FMonthName:   "FMonthName",
		FDay2:        "FDay2",
		FDay1or2:     "FDay1or2",
		FHour24:      "FHour24",
		FHour12:      "FHour12",
		FHour1or2:    "FHour1or2",
		FMinute2:     "FMinute2",
		FSecond2:     "FSecond2",
		FFracSec:     "FFracSec",
		FAMPM:        "FAMPM",
		FTZZ:         "FTZZ",
		FTZOffset:    "FTZOffset",
		FTZName:      "FTZName",
		FLiteral:     "FLiteral",
		FSkip:        "FSkip",
		FTZZOrOffset: "FTZZOrOffset",
	}
	if n, ok := names[k]; ok {
		return n
	}
	return "unknown"
}
