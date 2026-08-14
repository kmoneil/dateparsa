package compile

import (
	"testing"
	"time"
)

func TestExecute_BasicISO(t *testing.T) {
	// Simulate "2024-03-15" with manually constructed program.
	var p Program
	p.Tz = time.UTC
	p.Insts[0] = Inst{Op: OpYear4, Offset: 0, Len: 4}
	p.Insts[1] = Inst{Op: OpLiteral, Offset: 4, Len: 1}
	p.Insts[2] = Inst{Op: OpMonth2, Offset: 5, Len: 2}
	p.Insts[3] = Inst{Op: OpLiteral, Offset: 7, Len: 1}
	p.Insts[4] = Inst{Op: OpDay2, Offset: 8, Len: 2}
	p.N = 5

	got, err := p.Execute("2024-03-15")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestExecute_WithTime(t *testing.T) {
	var p Program
	p.Tz = time.UTC
	p.Insts[0] = Inst{Op: OpHour24, Offset: 0, Len: 2}
	p.Insts[1] = Inst{Op: OpLiteral, Offset: 2, Len: 1}
	p.Insts[2] = Inst{Op: OpMinute2, Offset: 3, Len: 2}
	p.Insts[3] = Inst{Op: OpLiteral, Offset: 5, Len: 1}
	p.Insts[4] = Inst{Op: OpSecond2, Offset: 6, Len: 2}
	p.N = 5

	got, err := p.Execute("10:30:45")
	if err != nil {
		t.Fatal(err)
	}
	if got.Hour() != 10 || got.Minute() != 30 || got.Second() != 45 {
		t.Errorf("got %v", got)
	}
}

func TestExecute_AMPM(t *testing.T) {
	var p Program
	p.Tz = time.UTC
	p.Insts[0] = Inst{Op: OpHour12, Offset: 0, Len: 2}
	p.Insts[1] = Inst{Op: OpLiteral, Offset: 2, Len: 1}
	p.Insts[2] = Inst{Op: OpMinute2, Offset: 3, Len: 2}
	p.Insts[3] = Inst{Op: OpLiteral, Offset: 5, Len: 1}
	p.Insts[4] = Inst{Op: OpAMPM, Offset: 6, Len: 2}
	p.N = 5

	got, err := p.Execute("03:30 PM")
	if err != nil {
		t.Fatal(err)
	}
	if got.Hour() != 15 || got.Minute() != 30 {
		t.Errorf("got hour=%d min=%d, want 15:30", got.Hour(), got.Minute())
	}
}

func TestExecute_FracSeconds(t *testing.T) {
	var p Program
	p.Tz = time.UTC
	p.Insts[0] = Inst{Op: OpHour24, Offset: 0, Len: 2}
	p.Insts[1] = Inst{Op: OpLiteral, Offset: 2, Len: 1}
	p.Insts[2] = Inst{Op: OpMinute2, Offset: 3, Len: 2}
	p.Insts[3] = Inst{Op: OpLiteral, Offset: 5, Len: 1}
	p.Insts[4] = Inst{Op: OpSecond2, Offset: 6, Len: 2}
	p.Insts[5] = Inst{Op: OpLiteral, Offset: 8, Len: 1}
	p.Insts[6] = Inst{Op: OpFracSec, Offset: 9, Len: 3}
	p.N = 7

	got, err := p.Execute("10:30:45.123")
	if err != nil {
		t.Fatal(err)
	}
	if got.Nanosecond() != 123000000 {
		t.Errorf("got nsec=%d, want 123000000", got.Nanosecond())
	}
}

func TestExecute_TZOffset(t *testing.T) {
	var p Program
	p.Tz = time.UTC
	p.Insts[0] = Inst{Op: OpHour24, Offset: 0, Len: 2}
	p.Insts[1] = Inst{Op: OpLiteral, Offset: 2, Len: 1}
	p.Insts[2] = Inst{Op: OpMinute2, Offset: 3, Len: 2}
	p.Insts[3] = Inst{Op: OpTZOffset, Offset: 5, Len: 6}
	p.N = 4

	got, err := p.Execute("10:30+05:30")
	if err != nil {
		t.Fatal(err)
	}
	_, offset := got.Zone()
	expected := 5*3600 + 30*60
	if offset != expected {
		t.Errorf("got offset=%d, want %d", offset, expected)
	}
}

func TestExecute_InvalidDigits(t *testing.T) {
	var p Program
	p.Tz = time.UTC
	p.Insts[0] = Inst{Op: OpYear4, Offset: 0, Len: 4}
	p.N = 1

	_, err := p.Execute("abcd")
	if err == nil {
		t.Error("expected error for non-digit input")
	}
}

func TestExecute_TZZOrOffset(t *testing.T) {
	// Build a program: HH:MM:SS then OpTZZOrOffset at offset 8
	base := [4]Inst{
		{Op: OpHour24, Offset: 0, Len: 2},
		{Op: OpLiteral, Offset: 2, Len: 1},
		{Op: OpMinute2, Offset: 3, Len: 2},
		{Op: OpLiteral, Offset: 5, Len: 1},
	}

	t.Run("Z_means_UTC", func(t *testing.T) {
		var p Program
		p.Tz = time.UTC
		copy(p.Insts[:], base[:])
		p.Insts[4] = Inst{Op: OpSecond2, Offset: 6, Len: 2}
		p.Insts[5] = Inst{Op: OpTZZOrOffset, Offset: 8, Len: 6}
		p.N = 6

		got, err := p.Execute("10:30:00Z")
		if err != nil {
			t.Fatal(err)
		}
		if got.Location() != time.UTC {
			t.Errorf("expected UTC, got %v", got.Location())
		}
	})

	t.Run("plus_offset", func(t *testing.T) {
		var p Program
		p.Tz = time.UTC
		copy(p.Insts[:], base[:])
		p.Insts[4] = Inst{Op: OpSecond2, Offset: 6, Len: 2}
		p.Insts[5] = Inst{Op: OpTZZOrOffset, Offset: 8, Len: 6}
		p.N = 6

		got, err := p.Execute("10:30:00+05:30")
		if err != nil {
			t.Fatal(err)
		}
		_, offset := got.Zone()
		if offset != 5*3600+30*60 {
			t.Errorf("got offset=%d, want %d", offset, 5*3600+30*60)
		}
	})

	t.Run("minus_offset", func(t *testing.T) {
		var p Program
		p.Tz = time.UTC
		copy(p.Insts[:], base[:])
		p.Insts[4] = Inst{Op: OpSecond2, Offset: 6, Len: 2}
		p.Insts[5] = Inst{Op: OpTZZOrOffset, Offset: 8, Len: 6}
		p.N = 6

		got, err := p.Execute("10:30:00-08:00")
		if err != nil {
			t.Fatal(err)
		}
		_, offset := got.Zone()
		if offset != -8*3600 {
			t.Errorf("got offset=%d, want %d", offset, -8*3600)
		}
	})

	t.Run("compact_plus_offset", func(t *testing.T) {
		var p Program
		p.Tz = time.UTC
		copy(p.Insts[:], base[:])
		p.Insts[4] = Inst{Op: OpSecond2, Offset: 6, Len: 2}
		p.Insts[5] = Inst{Op: OpTZZOrOffset, Offset: 8, Len: 5} // +HHMM
		p.N = 6

		got, err := p.Execute("10:30:00+0530")
		if err != nil {
			t.Fatal(err)
		}
		_, offset := got.Zone()
		if offset != 5*3600+30*60 {
			t.Errorf("got offset=%d, want %d", offset, 5*3600+30*60)
		}
	})
}

func TestCompile(t *testing.T) {
	def := &FormatDef{
		Name:     "TEST",
		GoLayout: "2006-01-02",
		Fields: []Field{
			{Kind: FYear4, Offset: 0, Len: 4},
			{Kind: FLiteral, Offset: 4, Len: 1},
			{Kind: FMonth2, Offset: 5, Len: 2},
			{Kind: FLiteral, Offset: 7, Len: 1},
			{Kind: FDay2, Offset: 8, Len: 2},
		},
	}

	prog, _, err := Compile(def, time.UTC)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if prog.N != 5 {
		t.Errorf("got %d instructions, want 5", prog.N)
	}

	got, err := prog.Execute("2024-03-15")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestCompileRefusesWhatItCannotAddress covers the other half of C4. Offset and
// Len are bytes on Inst, and Compile used to narrow them with a conversion, so
// a field at offset 260 addressed byte 4 instead. Detection reaches this with a
// long enough input, and restoring textual detection past byte 64 (C13) makes
// it routine, so the guard has to be here rather than in any one caller.
func TestCompileRefusesWhatItCannotAddress(t *testing.T) {
	for _, tt := range []struct {
		name  string
		field Field
	}{
		{"offset past a byte", Field{Kind: FYear4, Offset: 260, Len: 4}},
		{"offset exactly past", Field{Kind: FYear4, Offset: 256, Len: 4}},
		{"length past a byte", Field{Kind: FSkip, Offset: 0, Len: 300}},
	} {
		def := &FormatDef{Name: "PROBE", Fields: []Field{tt.field}}
		if _, _, err := Compile(def, time.UTC); err == nil {
			t.Errorf("%s: Compile accepted a field it cannot address", tt.name)
		}
	}

	// The largest addressable field still compiles, or the guard is off by one.
	def := &FormatDef{Name: "PROBE", Fields: []Field{{Kind: FSkip, Offset: 255, Len: 255}}}
	if _, _, err := Compile(def, time.UTC); err != nil {
		t.Errorf("Compile refused the largest addressable field: %v", err)
	}
}

// TestCompileRefusesTooManyInstructions is the instruction-count half.
func TestCompileRefusesTooManyInstructions(t *testing.T) {
	fields := make([]Field, MaxInstructions+1)
	for i := range fields {
		fields[i] = Field{Kind: FLiteral, Offset: i, Len: 1, Aux: 'x'}
	}
	if _, _, err := Compile(&FormatDef{Name: "PROBE", Fields: fields}, time.UTC); err == nil {
		t.Errorf("Compile accepted %d fields, the limit is %d", len(fields), MaxInstructions)
	}

	if _, _, err := Compile(&FormatDef{Name: "PROBE", Fields: fields[:MaxInstructions]}, time.UTC); err != nil {
		t.Errorf("Compile refused exactly %d fields: %v", MaxInstructions, err)
	}
}

// TestFieldKindToOpIsComplete guards the fieldKindToOp table.
//
// The FieldKind and OpCode enums are not parallel, and a reader who believes
// they are can replace the table with OpCode(f.Kind). That compiles, vets,
// lints, and turns every month into a day. A FieldKind added without an entry
// is worse: the array's zero value is OpYear4, so the field silently parses a
// year at its own offset. Both are invisible without this test.
func TestFieldKindToOpIsComplete(t *testing.T) {
	for k := FieldKind(0); k < numFieldKinds; k++ {
		if k == FYear4 {
			continue // the only kind whose op is legitimately the zero value
		}
		if fieldKindToOp[k] == OpYear4 {
			t.Errorf("FieldKind %d has no fieldKindToOp entry: it falls through to OpYear4", k)
		}
	}

	// The enums genuinely diverge. If this ever stops being true, the table is
	// still the contract, but the comment above it needs rewriting.
	if OpCode(FMonthName) == OpMonthName && OpCode(FDay2) == OpDay2 {
		t.Error("FieldKind and OpCode now agree at 4 and 5; re-check the comment on fieldKindToOp")
	}
}

// TestExecuteRefusesAMalformedProgram covers the two ways a Program can be
// out of shape without any input being at fault, both of which used to be an
// out-of-bounds read rather than an error.
//
// Neither is reachable through the public API today. Compile refuses a def over
// MaxInstructions before it fills anything, and the only op that drives delta
// negative is OpTZZOrOffset, whose 'Z' branch subtracts at most 5 while the
// field after it sits at least 6 bytes further along, so offsets stay positive.
// Fifteen adversarial layouts through the public Compile crossed with sixteen
// inputs produced no panic. That is an argument, not a proof, and Program is a
// plain struct with exported fields that anything in this module can build, so
// the guards are cheaper than having the argument again.
//
// "Parse never panics on any input" is the invariant these defend.
func TestExecuteRefusesAMalformedProgram(t *testing.T) {
	t.Run("negative offset", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panicked instead of erroring: %v", r)
			}
		}()
		// A conditional zone declared 6 wide at offset 0, then a year at 1.
		// "Z" takes the one-byte branch, so delta is 1-6 and the year would
		// read at offset -4.
		var p Program
		p.Insts[0] = Inst{Op: OpTZZOrOffset, Offset: 0, Len: 6}
		p.Insts[1] = Inst{Op: OpYear4, Offset: 1, Len: 4}
		p.N = 2
		if _, err := p.Execute("Z2024"); err == nil {
			t.Error("Execute accepted a program that reads before the start of the input")
		}
	})

	t.Run("N past the instruction array", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panicked instead of erroring: %v", r)
			}
		}()
		var p Program
		p.Insts[0] = Inst{Op: OpYear4, Offset: 0, Len: 4}
		p.N = MaxInstructions + 7
		if _, err := p.Execute("2024"); err == nil {
			t.Error("Execute accepted a program claiming more instructions than it holds")
		}
	})
}
