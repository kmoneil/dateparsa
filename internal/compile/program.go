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
	BaseYear int
}

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
