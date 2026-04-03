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

	prog := Compile(def, time.UTC)
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
