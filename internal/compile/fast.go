package compile

import (
	"fmt"
	"time"
)

// fastMaxWidth is the longest input a fast program may describe.
//
// It is 64 because the tiling proof in planFast is a uint64 with one bit per
// input byte, and it costs nothing real: the longest format in the trie is 35
// bytes ("2024-03-15T10:30:00.123456789+05:30") and anything longer carries a
// month name, a weekday, or a tail, none of which has a slot. A format over this
// keeps the interpreter, which has no such bound.
const fastMaxWidth = 64

// planFast rewrites p into slot form when every field it holds sits at a fixed
// offset, and reports whether it did.
//
// The interpreter in execute.go costs about 4 ns per instruction on top of the
// work the instruction does, in loop overhead, an indirect branch through the
// opcode switch, and the running coverage counters. Measured on linux/arm64 at
// e61660a, a 7-instruction RFC 3339 program spent 44 ns where the same
// extractions written straight-line, calling the same helpers, spent 15 ns.
// stdlib time.Parse spends 28 ns on that input, so the interpreter was losing to
// it on the one format stdlib hand-writes a parser for.
//
// The fix is to stop interpreting the formats that do not need it. A format
// whose fields are all fixed-width sits at offsets known when the program is
// built, so there is nothing to decide per call: the slot layout below puts each
// field at a constant index and executeFast reads them in a straight line. What
// that buys is not one trick but four at once. The loop is gone, so no counter
// and no reload. The switch is gone, so no indirect branch to mispredict. The
// coverage counters are gone, because Width answers the same question with one
// compare up front. And every Insts index is a constant, so the bounds checks go
// with them.
//
// Not every format qualifies, and the ones that do not keep the interpreter
// unchanged. A variable-width field (OpDay1or2 and its siblings) moves every
// field after it, which is the delta machinery the interpreter exists to run. A
// month name, a skip, or an unfused literal has no slot. Those formats are the
// minority and they are not the ones in a ten-million-row column.
//
// The slot form stays runnable by the interpreter, which is deliberate: the
// unused slots hold OpNop, which reads nothing and covers nothing, so a fast
// program executed either way must produce the same answer.
// TestFastAgreesWithInterpreter is what holds that to be true, and it is the
// check that makes this optimisation reviewable at all, since the fast path
// reimplements every extraction the interpreter already had right.
func planFast(p *Program) bool {
	// A fast program holds at most one instruction per slot, so anything longer
	// cannot qualify. Checking it here is not only an early out: it is what
	// makes writing into the slot region below safe, since it puts every real
	// instruction at an index under numSlots, and numSlots is at most slotBase.
	n := int(p.N)
	if n == 0 || n > numSlots {
		return false
	}

	var (
		used    uint32 // one bit per slot already claimed
		mask    uint64 // one bit per input byte some field claims
		width   int    // one past the last byte any field claims
		zoneOff = -1   // offset of a zone whose width the input decides
		zoneEnd = -1   // one past its wider form, to check nothing follows it
	)

	// Every slot starts empty, so the loop below writes only the ones the format
	// uses and nothing has to walk the rest afterwards.
	copy(p.Insts[slotBase:], emptySlots[:])

	for i := range n {
		in := p.Insts[i]
		slot, w, ok := fastSlot(in)
		if !ok || w < 1 || used&(1<<uint(slot)) != 0 {
			return false
		}

		off := int(in.Offset)
		if off+w > fastMaxWidth {
			return false
		}

		// The fields have to tile [0, width) with no gap and no overlap, and
		// this is the proof. The interpreter answers the same question per call
		// by summing widths and comparing the sum and the maximum against the
		// input length, which a gap and an overlap of equal size defeat between
		// them: they cancel in a sum. A bitmap cannot be fooled that way, and
		// asking it here means not asking anything at parse time.
		bits := (uint64(1)<<uint(w) - 1) << uint(off)
		if mask&bits != 0 {
			return false // two fields claim the same byte
		}
		mask |= bits
		used |= 1 << uint(slot)

		if in.Op == OpTZZOrOffset {
			// Its two forms differ in width, so everything after it would have
			// to move, which is exactly what a fast program cannot do. Allowed
			// only as the last field, where nothing follows it to be moved.
			zoneOff, zoneEnd = off, off+w
		}

		if off+w > width {
			width = off + w
		}

		// Write the slot as we go. n is under numSlots and the slot region
		// starts at slotBase, which is not, so this cannot reach an instruction.
		// A run that gives up after writing some of them leaves the region
		// inconsistent and that is fine: Width stays zero, so nothing reads it.
		p.Insts[slotBase+slot] = in
	}

	// Every byte from 0 to width claimed exactly once. width is at least 1, so
	// the shift below is defined at the full 64 as well.
	if mask != ^uint64(0)>>uint(64-width) {
		return false
	}

	widthAlt := width
	if zoneOff >= 0 {
		if zoneEnd != width {
			return false // something follows the zone, and would move
		}
		widthAlt = zoneOff + 1 // the 'Z' form
	}

	p.Width = uint16(width)
	p.WidthAlt = uint16(widthAlt)
	return true
}

// fastOp describes how one opcode fits the slot layout.
//
// noSlot in slot means the opcode keeps the interpreter: an OpLiteral that did
// not fuse, an OpSkip, an OpMonthName, an OpTail, the 1-or-2 widths, and the ISO
// week and ordinal ops, which need the post-processing executeInner does after
// its loop.
type fastOp struct {
	slot    int8 // which slot the opcode claims, or noSlot
	width   int8 // input bytes it reads, when that is fixed
	fromLen bool // the width is Inst.Len instead of the field above
	fusible bool // a separator in Aux adds one byte to the width
}

const noSlot int8 = -1

// fastOps is a table rather than a switch because planFast runs on every Parse,
// and a switch over this many opcodes compiles to the same indirect branch that
// made the executor slow. Indexing an array costs a load and no prediction.
//
// The widths repeat what the arms in executeInner assume, and they have to: a
// width claimed here that the executor does not read is a byte belonging to
// nothing, which is the failure planFast's tiling check exists to catch and
// cannot catch if this table is the thing that is wrong.
// TestFastWidthsMatchTheExecutor holds the two together.
var fastOps = buildFastOps()

func buildFastOps() [numOpCodes]fastOp {
	var t [numOpCodes]fastOp
	for i := range t {
		t[i] = fastOp{slot: noSlot}
	}
	num := func(op OpCode, slot int8, w int8) {
		t[op] = fastOp{slot: slot, width: w, fusible: true}
	}
	num(OpYear4, SlotYear, 4)
	num(OpYear2, SlotYear, 2)
	num(OpMonth2, SlotMonth, 2)
	num(OpDay2, SlotDay, 2)
	num(OpDaySpacePad, SlotDay, 2)
	num(OpHour24, SlotHour, 2)
	num(OpHour12, SlotHour, 2)
	num(OpMinute2, SlotMinute, 2)
	num(OpSecond2, SlotSecond, 2)

	// OpAMPM ignores Aux; its arm in the executor sets w to 2 outright.
	t[OpAMPM] = fastOp{slot: SlotAMPM, width: 2}
	t[OpTZZ] = fastOp{slot: SlotZone, width: 1}
	t[OpFracSec] = fastOp{slot: SlotFrac, fromLen: true}
	t[OpTZOffset] = fastOp{slot: SlotZone, fromLen: true}
	t[OpTZName] = fastOp{slot: SlotZone, fromLen: true}
	t[OpTZZOrOffset] = fastOp{slot: SlotZone, fromLen: true}
	return t
}

// emptySlots is copied over the slot region before planning fills it, so the
// unused slots need no loop of their own.
var emptySlots = func() (e [numSlots]Inst) {
	for i := range e {
		e[i] = Inst{Op: OpNop}
	}
	return e
}()

// fastSlot maps an instruction to its slot and reports how many input bytes it
// accounts for, including a separator fused onto it.
func fastSlot(in Inst) (slot, width int, ok bool) {
	if int(in.Op) >= len(fastOps) {
		return 0, 0, false // a hand-built Program carrying a nonsense opcode
	}
	fo := fastOps[in.Op]
	if fo.slot == noSlot {
		return 0, 0, false
	}
	w := int(fo.width)
	if fo.fromLen {
		w = int(in.Len)
	}
	if fo.fusible && in.Aux != sepNone {
		w++
	}
	return int(fo.slot), w, true
}

// sepOK checks a separator fused onto a fixed-width numeric field at off+w.
//
// The bound is what the fused byte needs and the field itself did not: a field
// ending exactly at the end of the input has nowhere to hold a separator. It
// reads len(s) rather than a passed-in length for the reason parse2Bounded does,
// which is that an opaque int bounds nothing the compiler can use.
func sepOK(s string, at int, aux uint16) bool {
	if aux == sepNone {
		return true
	}
	return at < len(s) && litAccepts(aux, s[at])
}

// executeFast runs a program in slot form. See planFast for why it exists.
//
// Every Insts index here is a constant and every branch tests program data,
// which does not change for the life of a Layout. A column of same-format values
// therefore takes the same path through this function on every row, and the
// branch predictor learns it once.
func (p *Program) executeFast(s string) (time.Time, error) {
	slen := len(s)
	if slen != int(p.Width) && slen != int(p.WidthAlt) {
		return time.Time{}, lengthError(slen, int(p.Width))
	}

	year, yearSet := 0, false
	month := time.January
	day := 1
	var hour, minute, second, nsec int
	var ampm int8

	loc := p.Tz
	if loc == nil {
		loc = time.UTC
	}

	if in := p.Insts[slotBase+SlotYear]; in.Op != OpNop {
		off := int(in.Offset)
		if in.Op == OpYear4 {
			if off+4 > slen {
				return time.Time{}, fieldError("year", off, slen)
			}
			y, ok := parse4Digits(s, off)
			if !ok || !sepOK(s, off+4, in.Aux) {
				return time.Time{}, fieldError("year", off, slen)
			}
			year = y
		} else {
			v, ok := parse2Bounded(s, off, 0, 99)
			if !ok || !sepOK(s, off+2, in.Aux) {
				return time.Time{}, fieldError("year", off, slen)
			}
			year = NormalizeTwoDigitYear(v)
		}
		yearSet = true
	}

	if in := p.Insts[slotBase+SlotMonth]; in.Op != OpNop {
		off := int(in.Offset)
		v, ok := parse2Bounded(s, off, 1, 12)
		if !ok || !sepOK(s, off+2, in.Aux) {
			return time.Time{}, fieldError("month", off, slen)
		}
		month = time.Month(v)
	}

	if in := p.Insts[slotBase+SlotDay]; in.Op != OpNop {
		off := int(in.Offset)
		v, ok := fastDay(s, off, in.Op)
		if !ok || !sepOK(s, off+2, in.Aux) {
			return time.Time{}, fieldError("day", off, slen)
		}
		day = v
	}

	if in := p.Insts[slotBase+SlotHour]; in.Op != OpNop {
		off := int(in.Offset)
		lo, hi := 0, 23
		if in.Op == OpHour12 {
			lo, hi = 1, 12
		}
		v, ok := parse2Bounded(s, off, lo, hi)
		if !ok || !sepOK(s, off+2, in.Aux) {
			return time.Time{}, fieldError("hour", off, slen)
		}
		hour = v
	}

	if in := p.Insts[slotBase+SlotMinute]; in.Op != OpNop {
		off := int(in.Offset)
		// 0 to 59. See the OpSecond2 arm in the interpreter for why the leap
		// second is refused rather than normalised into the next day.
		v, ok := parse2Bounded(s, off, 0, 59)
		if !ok || !sepOK(s, off+2, in.Aux) {
			return time.Time{}, fieldError("minute", off, slen)
		}
		minute = v
	}

	if in := p.Insts[slotBase+SlotSecond]; in.Op != OpNop {
		off := int(in.Offset)
		v, ok := parse2Bounded(s, off, 0, 59)
		if !ok || !sepOK(s, off+2, in.Aux) {
			return time.Time{}, fieldError("second", off, slen)
		}
		second = v
	}

	if in := p.Insts[slotBase+SlotFrac]; in.Op != OpNop {
		off, length := int(in.Offset), int(in.Len)
		if off+length > slen {
			return time.Time{}, fieldError("fractional second", off, slen)
		}
		ns, ok := parseFracSec(s, off, length)
		if !ok {
			return time.Time{}, fieldError("fractional second", off, slen)
		}
		nsec = ns
	}

	if in := p.Insts[slotBase+SlotAMPM]; in.Op != OpNop {
		off := int(in.Offset)
		if off+2 > slen {
			return time.Time{}, fieldError("am/pm", off, slen)
		}
		c0 := s[off] | 0x20 // lowercase
		c1 := s[off+1] | 0x20
		switch {
		case c0 == 'a' && c1 == 'm':
			ampm = 1
		case c0 == 'p' && c1 == 'm':
			ampm = -1
		default:
			return time.Time{}, fieldError("am/pm", off, slen)
		}
	}

	if in := p.Insts[slotBase+SlotZone]; in.Op != OpNop {
		// 'Z' is what both of the common zone ops carry, and reading it here
		// rather than in fastZone is what keeps an RFC 3339 parse a straight
		// line: the call was the one indirect branch left in this function.
		//
		// Requiring the 'Z' to end the input is what stops "…00Z+0530" being
		// read as UTC with a tail nobody described. For OpTZZ that is already
		// true of every input Width admits; for OpTZZOrOffset it is what
		// WidthAlt allows and therefore what has to be checked.
		off := int(in.Offset)
		if (in.Op == OpTZZ || in.Op == OpTZZOrOffset) && off < slen && s[off] == 'Z' {
			if off+1 != slen {
				return time.Time{}, fieldError("timezone", off, slen)
			}
			loc = time.UTC
		} else {
			l, err := fastZone(s, in)
			if err != nil {
				return time.Time{}, err
			}
			loc = l
		}
	}

	if !yearSet && p.BaseYear != 0 {
		year = int(p.BaseYear)
	}
	// The same refusal the interpreter makes, for the same reason, and the two
	// paths have to make it on the same inputs: TestFastAgreesWithInterpreter
	// and FuzzFastAgreesWithInterpreter are what hold them together, and a
	// check in one and not the other is a disagreement they would report.
	if !dayExists(year, int(month), day) {
		return time.Time{}, fmt.Errorf(
			"dateparsa: %d %s does not exist in %d",
			day, month, year)
	}
	return makeTime(year, month, day, applyAMPM(hour, ampm), minute, second, nsec, loc), nil
}

// fastDay reads the day slot, which holds either OpDay2 or Go's space-padded
// "_2". Split out so the common op stays a straight parse2Bounded and the padded
// form does not widen it.
func fastDay(s string, off int, op OpCode) (int, bool) {
	if op == OpDay2 {
		return parse2Bounded(s, off, 1, 31)
	}
	if off+2 > len(s) {
		return 0, false
	}
	if s[off] == ' ' {
		d := s[off+1] - '0'
		if d < 1 || d > 9 {
			return 0, false
		}
		return int(d), true
	}
	return parse2Bounded(s, off, 1, 31)
}

// fastZone reads the zone slot. Split out to keep executeFast's straight line
// straight: four ops share this slot and only one of them is common.
func fastZone(s string, in Inst) (*time.Location, error) {
	off, length := int(in.Offset), int(in.Len)
	slen := len(s)

	switch in.Op {
	case OpTZZ:
		if off >= slen || s[off] != 'Z' {
			return nil, fieldError("timezone", off, slen)
		}
		return time.UTC, nil

	case OpTZZOrOffset:
		if off >= slen {
			return nil, fieldError("timezone", off, slen)
		}
		if s[off] == 'Z' {
			// planFast only accepts this op as the last field, and WidthAlt is
			// what allows the shorter input. Requiring the 'Z' to end the input
			// is what stops "…00Z+0530" being read as UTC with a tail nobody
			// described.
			if off+1 != slen {
				return nil, fieldError("timezone", off, slen)
			}
			return time.UTC, nil
		}
		if off+length > slen {
			return nil, fieldError("timezone offset", off, slen)
		}
		loc, ok := parseTZOffset(s, off, length)
		if !ok {
			return nil, fieldError("timezone offset", off, slen)
		}
		return loc, nil

	case OpTZOffset:
		if off+length > slen {
			return nil, fieldError("timezone offset", off, slen)
		}
		loc, ok := parseTZOffset(s, off, length)
		if !ok {
			return nil, fieldError("timezone offset", off, slen)
		}
		return loc, nil

	default: // OpTZName
		if off+length > slen {
			return nil, fieldError("timezone name", off, slen)
		}
		loc, ok := lookupTZAbbr(s[off : off+length])
		if !ok {
			return nil, fieldError("timezone name", off, slen)
		}
		return loc, nil
	}
}
