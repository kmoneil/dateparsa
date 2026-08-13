package compile

import "time"

// MaxInstructions is the maximum number of instructions in a program.
// Covers the most complex date formats with room to spare.
const MaxInstructions = 24

// Program is a compiled sequence of parse instructions.
// It is a value type (no pointers) so it can be embedded directly in Layout
// without indirection or heap allocation.
type Program struct {
	Insts [MaxInstructions]Inst
	N     int            // Number of valid instructions
	Tz    *time.Location // Default timezone for this program (set at compile time)
}

// Execute runs the program against a string input and returns the parsed time.
func (p *Program) Execute(s string) (time.Time, error) {
	return p.executeInner(s, len(s))
}

// ExecuteBytes runs the program against a byte slice input.
// Note: currently converts to string internally. A future optimization
// could use unsafe.String or refactor executeInner to operate on []byte
// natively to avoid the copy.
func (p *Program) ExecuteBytes(b []byte) (time.Time, error) {
	return p.executeInner(string(b), len(b))
}
