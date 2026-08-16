package compile

import (
	"testing"
	"time"
)

// interpreted returns a copy of p that Execute will run through executeInner
// rather than executeFast.
//
// It works because planFast leaves Insts[:N] exactly as Compile emitted it and
// writes the slots past the end, so clearing the widths does not restore an
// approximation of the program: it restores the program. That is the whole
// reason the slot region sits where it does.
func interpreted(p Program) Program {
	p.Width, p.WidthAlt = 0, 0
	return p
}

// agree reports whether two parses of the same input landed on the same instant
// in the same zone.
//
// It compares the zone by name and offset and not by pointer, which == would.
// parseTZOffset answers a 15-minute offset from a pre-built table and anything
// else from time.FixedZone, so "+00:09" hands each caller a fresh Location: the
// two executors return equal zones through different pointers, and that is the
// existing behaviour of the slow path rather than a difference between them.
// What has to match is what a caller can observe.
func agree(a, b time.Time) bool {
	if !a.Equal(b) {
		return false
	}
	an, ao := a.Zone()
	bn, bo := b.Zone()
	return an == bn && ao == bo
}

// fastLayouts are Go reference layouts whose programs planFast accepts, one per
// shape of slot it can fill.
var fastLayouts = []string{
	"2006-01-02",
	"2006/01/02",
	"20060102",
	"01/02/2006",
	"02-01-2006",
	"06-01-02",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
	"2006-01-02T15:04:05Z07:00",
	"2006-01-02T15:04:05Z0700",
	"2006-01-02 15:04:05.000",
	"2006-01-02 15:04:05.000000",
	"2006-01-02T15:04:05.000000000Z07:00",
	"2006-01-02 15:04:05 -07:00",
	"2006-01-02 15:04:05 MST",
	"15:04:05",
	"15:04",
	"03:04:05PM",
	"2006-01-_2 15:04:05",
	"01/02/06 03:04PM",
}

// inputsFor renders one valid input per layout, plus the malformed neighbours
// that decide whether the two executors agree about refusal as well as about
// value. A fast program answers a wrong length with one compare where the
// interpreter counts bytes as it goes, and those are the same answer or this
// change is wrong.
func inputsFor(layout string) []string {
	base := time.Date(2024, 3, 15, 22, 30, 45, 123456789, time.UTC)
	valid := base.Format(layout)
	out := []string{
		valid,
		"",
		valid + "x",          // one byte too many
		valid + "0",          // a digit too many
		valid[:len(valid)-1], // one byte short
	}
	// Every single-byte mutation, which is what reaches the range checks and
	// the fused separators.
	for i := range valid {
		for _, c := range []byte{'0', '9', 'x', '-', ':', ' ', 'Z', '+', 0} {
			if valid[i] == c {
				continue
			}
			b := []byte(valid)
			b[i] = c
			out = append(out, string(b))
		}
	}
	return out
}

// TestFastAgreesWithInterpreter is the cross-check the fast path rests on.
//
// executeFast reimplements every extraction executeInner already had right, and
// nothing about the two being separate functions makes them agree. This runs the
// same program both ways over valid input and over every single-byte mutation of
// it, and requires the same instant and the same accept-or-refuse decision from
// both.
func TestFastAgreesWithInterpreter(t *testing.T) {
	for _, layout := range fastLayouts {
		t.Run(layout, func(t *testing.T) {
			def, err := ParseGoLayout(layout)
			if err != nil {
				t.Fatalf("ParseGoLayout: %v", err)
			}
			fast, _, err := Compile(def, time.UTC)
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			if !fast.isFast() {
				t.Fatalf("layout %q did not take the fast path; if that is "+
					"intended, move it out of fastLayouts", layout)
			}
			slow := interpreted(fast)

			for _, in := range inputsFor(layout) {
				gotT, gotErr := fast.Execute(in)
				wantT, wantErr := slow.Execute(in)

				if (gotErr == nil) != (wantErr == nil) {
					t.Errorf("Execute(%q): fast err=%v, interpreted err=%v",
						in, gotErr, wantErr)
					continue
				}
				if gotErr == nil && !agree(gotT, wantT) {
					t.Errorf("Execute(%q): fast %v, interpreted %v", in, gotT, wantT)
				}
			}
		})
	}
}

func FuzzFastAgreesWithInterpreter(f *testing.F) {
	for i, layout := range fastLayouts {
		f.Add(i, time.Date(2024, 3, 15, 22, 30, 45, 0, time.UTC).Format(layout))
	}
	f.Add(0, "")
	f.Add(8, "2024-03-15T10:30:00Z")
	f.Add(8, "2024-03-15T10:30:00+05:30")
	f.Add(8, "2024-03-15T10:30:00Z+05:3")

	f.Fuzz(func(t *testing.T, idx int, in string) {
		if idx < 0 {
			t.Skip()
		}
		layout := fastLayouts[idx%len(fastLayouts)]
		def, err := ParseGoLayout(layout)
		if err != nil {
			t.Skip()
		}
		fast, _, err := Compile(def, time.UTC)
		if err != nil || !fast.isFast() {
			t.Skip()
		}
		slow := interpreted(fast)

		gotT, gotErr := fast.Execute(in)
		wantT, wantErr := slow.Execute(in)

		if (gotErr == nil) != (wantErr == nil) {
			t.Fatalf("layout %q Execute(%q): fast err=%v, interpreted err=%v",
				layout, in, gotErr, wantErr)
		}
		if gotErr == nil && !agree(gotT, wantT) {
			t.Fatalf("layout %q Execute(%q): fast %v, interpreted %v",
				layout, in, gotT, wantT)
		}
	})
}

// TestFastWidthsMatchTheExecutor pins the widths in fastOps against what the
// executor actually consumes.
//
// planFast proves the fields tile the input using the widths this table claims.
// If a width here is larger than the bytes the executor reads, the tiling proof
// passes over a byte nothing examines, and the layout accepts an input it should
// refuse. The interpreter's own coverage check is the reference: it accepts a
// one-instruction program only when the instruction covers the whole input, so
// an input of exactly the claimed width has to be accepted and one byte more
// has to be refused.
func TestFastWidthsMatchTheExecutor(t *testing.T) {
	cases := []struct {
		name  string
		inst  Inst
		input string
	}{
		{"Year4", Inst{Op: OpYear4, Len: 4}, "2024"},
		{"Year4+sep", Inst{Op: OpYear4, Len: 4, Aux: uint16('-')}, "2024-"},
		{"Year2", Inst{Op: OpYear2, Len: 2}, "24"},
		{"Year2+sep", Inst{Op: OpYear2, Len: 2, Aux: uint16('/')}, "24/"},
		{"Month2", Inst{Op: OpMonth2, Len: 2}, "03"},
		{"Month2+sep", Inst{Op: OpMonth2, Len: 2, Aux: AuxClass(ClassSep)}, "03-"},
		{"Day2", Inst{Op: OpDay2, Len: 2}, "15"},
		{"Day2+sep", Inst{Op: OpDay2, Len: 2, Aux: uint16('T')}, "15T"},
		{"DaySpacePad", Inst{Op: OpDaySpacePad, Len: 2}, " 5"},
		{"DaySpacePad+sep", Inst{Op: OpDaySpacePad, Len: 2, Aux: uint16(' ')}, "15 "},
		{"Hour24", Inst{Op: OpHour24, Len: 2}, "22"},
		{"Hour24+sep", Inst{Op: OpHour24, Len: 2, Aux: uint16(':')}, "22:"},
		{"Hour12", Inst{Op: OpHour12, Len: 2}, "11"},
		{"Minute2", Inst{Op: OpMinute2, Len: 2}, "30"},
		{"Minute2+sep", Inst{Op: OpMinute2, Len: 2, Aux: uint16(':')}, "30:"},
		{"Second2", Inst{Op: OpSecond2, Len: 2}, "45"},
		{"Second2+sep", Inst{Op: OpSecond2, Len: 2, Aux: uint16('.')}, "45."},
		{"FracSec3", Inst{Op: OpFracSec, Len: 3}, "123"},
		{"FracSec9", Inst{Op: OpFracSec, Len: 9}, "123456789"},
		{"AMPM", Inst{Op: OpAMPM, Len: 2}, "pm"},
		{"TZZ", Inst{Op: OpTZZ, Len: 1}, "Z"},
		{"TZOffset6", Inst{Op: OpTZOffset, Len: 6}, "+05:30"},
		{"TZOffset5", Inst{Op: OpTZOffset, Len: 5}, "-0800"},
		{"TZName", Inst{Op: OpTZName, Len: 3}, "UTC"},
		{"TZZOrOffset/offset", Inst{Op: OpTZZOrOffset, Len: 6}, "+05:30"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, width, ok := fastSlot(c.inst)
			if !ok {
				t.Fatalf("fastSlot refused %v, which this table says it takes", c.inst.Op)
			}
			if width != len(c.input) {
				t.Fatalf("fastSlot claims width %d, the sample %q is %d bytes",
					width, c.input, len(c.input))
			}

			// The interpreter accepts a one-instruction program only when the
			// instruction covers the input exactly, so this is its opinion of
			// the same width.
			var p Program
			p.Tz = time.UTC
			p.Insts[0] = c.inst
			p.N = 1
			if _, err := p.Execute(c.input); err != nil {
				t.Errorf("interpreter refused %q at the claimed width: %v", c.input, err)
			}
			if _, err := p.Execute(c.input + "x"); err == nil {
				t.Errorf("interpreter accepted %q, one byte past the claimed width",
					c.input+"x")
			}
		})
	}
}

// TestPlanFastRefusesWhatItCannotRun records the shapes that keep the
// interpreter. Each is a program the slot layout cannot describe, and accepting
// one would mean executeFast reading a field that is not where it thinks.
func TestPlanFastRefusesWhatItCannotRun(t *testing.T) {
	mk := func(insts ...Inst) Program {
		var p Program
		p.Tz = time.UTC
		copy(p.Insts[:], insts)
		p.N = len(insts)
		return p
	}

	cases := []struct {
		name string
		p    Program
	}{
		{"empty", mk()},
		{"variable-width day", mk(
			Inst{Op: OpDay1or2, Offset: 0, Len: 1},
		)},
		{"variable-width month moves what follows", mk(
			Inst{Op: OpMonth1or2, Offset: 0, Len: 1},
			Inst{Op: OpLiteral, Offset: 1, Len: 1, Aux: uint16('/')},
			Inst{Op: OpYear4, Offset: 2, Len: 4},
		)},
		{"month name has no slot", mk(
			Inst{Op: OpMonthName, Offset: 0, Len: 3, Aux: 3},
		)},
		{"skip has no slot", mk(
			Inst{Op: OpYear4, Offset: 0, Len: 4},
			Inst{Op: OpSkip, Offset: 4, Len: 2},
		)},
		{"tail has no slot", mk(
			Inst{Op: OpYear4, Offset: 0, Len: 4},
			Inst{Op: OpTail, Offset: 4},
		)},
		{"iso week needs the post-processing", mk(
			Inst{Op: OpYear4, Offset: 0, Len: 4},
			Inst{Op: OpISOWeek, Offset: 4, Len: 2},
		)},
		{"ordinal day needs the post-processing", mk(
			Inst{Op: OpYear4, Offset: 0, Len: 4},
			Inst{Op: OpOrdinalDay, Offset: 4, Len: 3},
		)},
		{"two fields want the same slot", mk(
			Inst{Op: OpYear4, Offset: 0, Len: 4},
			Inst{Op: OpYear4, Offset: 4, Len: 4},
		)},
		{"a gap between fields", mk(
			Inst{Op: OpYear4, Offset: 0, Len: 4},
			Inst{Op: OpMonth2, Offset: 5, Len: 2},
		)},
		{"does not start at zero", mk(
			Inst{Op: OpYear4, Offset: 1, Len: 4},
		)},
		{"fields overlap", mk(
			Inst{Op: OpYear4, Offset: 0, Len: 4},
			Inst{Op: OpMonth2, Offset: 3, Len: 2},
		)},
		{"a gap and an overlap that cancel in a sum", mk(
			Inst{Op: OpYear4, Offset: 0, Len: 4},
			Inst{Op: OpMonth2, Offset: 3, Len: 2}, // overlaps by one
			Inst{Op: OpDay2, Offset: 6, Len: 2},   // leaves a gap of one
		)},
		{"a conditional zone with a field after it", mk(
			Inst{Op: OpTZZOrOffset, Offset: 0, Len: 6},
			Inst{Op: OpYear4, Offset: 6, Len: 4},
		)},
		{"wider than the tiling proof can hold", mk(
			Inst{Op: OpYear4, Offset: 0, Len: 4},
			Inst{Op: OpFracSec, Offset: 4, Len: 90},
		)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := c.p
			if planFast(&p) {
				t.Errorf("planFast accepted a program it cannot run: Width=%d", p.Width)
			}
			if p.isFast() {
				t.Errorf("program reports fast after a refusal")
			}
		})
	}
}

// TestPlanFastNeverReachesTheInstructions is the bound the slot region depends
// on. planFast writes past slotBase, and a program long enough to reach that far
// would have its own instructions overwritten, which would be visible only as a
// wrong date.
func TestPlanFastNeverReachesTheInstructions(t *testing.T) {
	if numSlots > slotBase {
		t.Fatalf("the slot region at %d overlaps a program of %d instructions",
			slotBase, numSlots)
	}

	// A program of exactly numSlots instructions is the longest planFast can
	// accept, and its last instruction sits at numSlots-1, below slotBase.
	var p Program
	p.Tz = time.UTC
	for i := range numSlots {
		p.Insts[i] = Inst{Op: OpLiteral, Offset: byte(i), Len: 1, Aux: uint16('x')}
	}
	p.N = numSlots
	if planFast(&p) {
		t.Fatal("planFast accepted a program of literals")
	}
	for i := range numSlots {
		if p.Insts[i].Op != OpLiteral {
			t.Errorf("instruction %d was overwritten by the slot region", i)
		}
	}
}

// programSink keeps the allocation below from being optimised away.
var programSink *Program

// TestProgramFitsItsSizeClass pins the struct's size, which is not incidental.
//
// Parse compiles a fresh Program on every call and Layout embeds it by value, so
// Layout lands in the 208-byte size class exactly. Adding two uint16 to Program
// beside a full int BaseYear pushed it to 224 and cost +25.8% on Parse_ISO8601
// with no change at all on Layout.Parse, which is the wrong trade in both
// directions at once.
//
// The size is measured rather than declared because unsafe.Sizeof and reflect
// are both closed off here, and an allocation reports the size class the struct
// falls in, which is the number that actually costs anything.
func TestProgramFitsItsSizeClass(t *testing.T) {
	const want = 176 // the class holding 168 bytes

	r := testing.Benchmark(func(b *testing.B) {
		for b.Loop() {
			programSink = new(Program)
		}
	})
	if got := r.AllocedBytesPerOp(); got > want {
		t.Errorf("a Program allocates %d bytes, over the %d this pins. "+
			"Something was added to the struct; put it in the space "+
			"BaseYear, Width and WidthAlt share, or argue for the class.",
			got, want)
	}
}
