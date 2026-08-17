package compile

import "time"

// MaxInstructions is the maximum number of instructions in a program, and the
// bound on the work one Execute can do.
//
// Detection never comes close: the widest format the detectors emit is 16
// fields, which TestEveryInputByteIsDescribedExactlyOnce checks. What spends it
// is the public Compile, where ParseGoLayout emits one instruction per
// unrecognised layout byte, so "Generated on 2006-01-02 at 15:04" is 25
// instructions and is refused.
//
// Raising it is not free, which is worth writing down because it looks free.
// Compile returns a Program by value and Parse builds one on every call, so the
// array is copied per parse and not per Layout. Taking this to 64 measured
// +37% on Parse_ISO8601 and +45% on Detect_Only, against Layout_Parse_ISODate
// and a stdlib control both flat. The way to buy the headroom is to spend fewer
// instructions on literal text, not to hold more of them.
const MaxInstructions = 24

// maxFieldByte is the largest value Inst.Offset and Inst.Len can hold. A
// program cannot address a byte past this, so Compile refuses a def that tries
// rather than wrapping into the middle of the input.
const maxFieldByte = 255

// MaxDescribableLen is the longest input any program this package compiles can
// describe, and therefore the longest one worth detecting.
//
// A field starts at maxFieldByte at the latest and runs maxFieldByte bytes at
// the most, so the last byte any field can address is the sum of the two. An
// input longer than that cannot be covered, whatever it says, and the executor
// requires the program to cover the whole input.
//
// The tail was the exception until W16 and is the reason this constant could
// not be relied on before: OpTail's width was whatever was left. It is bounded
// at MaxTailLen now, and a tail begins at maxFieldByte at the latest, so the
// widest input a tail-bearing format can describe is well under this.
//
// Exported for detect, which refuses a longer input rather than running its
// fallback detectors over it. A megabyte of prose cost 1.27 seconds of CPU with
// twenty locales configured, and no answer was available at the end of it.
const MaxDescribableLen = 2 * maxFieldByte

// Slot numbers for a fast program. See planFast.
//
// A fast program stores each of its fields at the matching constant index in
// Insts, with OpNop in the slots the format does not use. That is what lets
// executeFast read them without a loop, without a switch, and without a bounds
// check: every index below is a constant, so the compiler resolves the address
// at compile time and the only branch per slot is "is this slot used", which is
// a test against program data that never changes for a given Layout.
//
// The slots sit at the END of Insts, at slotBase and up, so that Insts[:N] keeps
// the instructions the format compiled to, in the order it compiled them. The
// two representations coexist rather than replacing one another: N still counts
// what fusion produced, the interpreter still runs the original program
// unchanged, and TestFastAgreesWithInterpreter can therefore compare the fast
// path against the real thing instead of against a rewrite of itself.
//
// The regions cannot collide. planFast accepts a program only when every one of
// its instructions claims a distinct slot, so a fast program has at most
// numSlots instructions, and numSlots is at most slotBase.
const (
	SlotYear = iota
	SlotMonth
	SlotDay
	SlotHour
	SlotMinute
	SlotSecond
	SlotFrac
	SlotAMPM
	SlotZone

	numSlots // sentinel, must be last
)

// slotBase is where the slot region starts in Insts.
const slotBase = MaxInstructions - numSlots

// The slot region may not reach back into the instructions a fast program can
// hold, which is at most numSlots of them. This fails the build rather than
// letting the two regions overlap, which would corrupt whichever was written
// second and be visible only as a wrong date.
var _ [slotBase - numSlots]struct{}

// Program is a compiled sequence of parse instructions.
// It is a value type (no pointers) so it can be embedded directly in Layout
// without indirection or heap allocation.
type Program struct {
	Insts [MaxInstructions]Inst
	N     int            // Number of valid instructions
	Tz    *time.Location // Default timezone for this program (set at compile time)

	// BaseYear is substituted when the format carries no year field at all,
	// as in "10:30:00" or "March 15". Zero means leave the year unset, which
	// is what time.Parse does and what the public Compile wants.
	//
	// It is fixed at compile time rather than read from the clock on each
	// call, for the same reason LayoutNaturalLanguage refuses to re-parse: a
	// value the caller believes is a compiled layout must not return a
	// different instant depending on when it runs. It also keeps Execute off
	// the clock, which is what makes the hot path what it is.
	//
	// int32 rather than int so that Width and WidthAlt below fit beside it in
	// the eight bytes this field used to have to itself. See the note on
	// Program's size under Width.
	BaseYear int32

	// Width and WidthAlt are the input lengths a fast program describes, and
	// are both zero for a program planFast refused.
	//
	// The interpreter proves a program covered its input by summing widths as
	// it goes and comparing at the end, which costs an add, a compare and a
	// branch on every instruction. For a program whose fields all sit at fixed
	// offsets the answer is known when the program is built, so planFast
	// computes it once and executeFast settles the whole question with one
	// compare before it reads anything.
	//
	// WidthAlt is the second acceptable length, and equals Width unless the
	// zone slot holds OpTZZOrOffset, whose two forms differ in width: "Z" is
	// one byte where "+05:30" is six. It is why that op is only allowed in a
	// fast program when it sits last, since anything after it would move.
	//
	// uint16 rather than uint8 because a program may address 255 bytes of
	// input, so a field at 255 tiles to a width of 256 and would wrap.
	//
	// Program's size is not incidental. Parse compiles a fresh one on every
	// call and Layout embeds it by value, so a Layout is 208 bytes and lands in
	// the 208-byte size class exactly. Adding these two fields alongside a full
	// int BaseYear measured +25.8% on Parse_ISO8601 and +16 B/op, all of it the
	// jump to the next size class, against no change at all on Layout.Parse.
	// TestProgramFitsItsSizeClass is what stops that happening again.
	Width    uint16
	WidthAlt uint16
}

// isFast reports whether the slot region at the end of Insts describes this
// program, so executeFast can run it.
//
// Width doubles as the flag rather than a bool beside it, because a bool would
// cost this struct eight bytes and a size class. Every program planFast accepts
// covers at least one byte, and it writes Width nowhere else, so a zero Width
// and "not planned" are the same fact.
func (p *Program) isFast() bool { return p.Width != 0 }

// Execute runs the program against a string input and returns the parsed time.
func (p *Program) Execute(s string) (time.Time, error) {
	return p.executeInner(s)
}

// ExecuteBytes runs the program against a byte slice input.
//
// The conversion copies, and that is deliberate rather than pending.
// unsafe.String is the obvious way to avoid it and is closed off: this library
// imports no unsafe, which README promises and SECURITY.md lists among the
// things it does not do. The only honest alternative is an executor that reads
// []byte natively, which means a second copy of the opcode switch or a generic
// one, and nobody has costed that. Until somebody does, the copy stays and this
// comment says why instead of pointing at a door that is locked.
func (p *Program) ExecuteBytes(b []byte) (time.Time, error) {
	return p.executeInner(string(b))
}
