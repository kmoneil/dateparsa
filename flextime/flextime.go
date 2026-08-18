// Package flextime provides a FlexTime type that wraps time.Time with
// automatic format detection. It implements sql.Scanner, driver.Valuer,
// json.Marshaler, json.Unmarshaler, encoding.TextMarshaler, and
// encoding.TextUnmarshaler.
//
// FlexTime lives in a subpackage to avoid pulling database/sql into the
// dependency tree for users who do not need it.
//
// Zero value is valid Go and represents an unset time (SQL NULL equivalent).
package flextime

import (
	"fmt"
	"time"

	"github.com/kmoneil/dateparsa"
)

// FlexTime wraps a time.Time with automatic format detection.
// Zero value is valid and represents the zero time.Time.
//
// A value that was parsed from a string may have needed a guess, and Ambiguous
// reports that. Refusing to guess needs configuration, which a value reached
// through encoding/json or database/sql cannot carry, so it lives on Scanner: see
// WithStrictMode. At the JSON boundary, where there is no Scanner to configure,
// checking Ambiguous after unmarshalling is the equivalent.
type FlexTime struct {
	t         time.Time
	valid     bool
	ambiguous bool
	opts      *marshalConfig // nil for default behavior; set via NewWithOptions
}

// set writes the three fields that every parse path has to write together.
//
// It exists so that no path can set two of them and leave the third holding a
// previous row's value. database/sql reuses one *FlexTime across every row of a
// result set, so a value scanned after an ambiguous one would otherwise still
// report Ambiguous true.
func (ft *FlexTime) set(t time.Time, valid, ambiguous bool) {
	ft.t = t
	ft.valid = valid
	ft.ambiguous = ambiguous
}

// New creates a FlexTime from an existing time.Time.
func New(t time.Time) FlexTime {
	return FlexTime{t: t, valid: true}
}

// Now returns a FlexTime set to the current time.
func Now() FlexTime {
	return FlexTime{t: time.Now(), valid: true}
}

// Time returns the underlying time.Time.
// Returns the zero time if FlexTime has not been set.
func (ft FlexTime) Time() time.Time {
	return ft.t
}

// Valid reports whether the FlexTime holds a successfully parsed time.
// A zero-value FlexTime or one that scanned a NULL is not valid.
func (ft FlexTime) Valid() bool {
	return ft.valid
}

// Ambiguous reports whether the time came from a guess.
//
// "01/02/2024" is either the second of January or the first of February, and
// nothing in the string says which, so a preference rule picked one. This says it
// picked. It is false for a value that arrived already typed, and false for a
// format whose fields are fixed by its shape.
//
// It exists because this type is the boundary. Every path here dropped the flag
// dateparsa returns beside the time, so a row from a database or a field from a
// JSON body carried a guessed day with Valid true and no way at all to find out.
// SECURITY.md and README both promise "ambiguity is reported, never hidden", and
// both were true of the root package and false of the type recommended for
// database models and JSON APIs.
//
// Reporting is not refusing. A Scanner built WithStrictMode refuses instead, which
// is the stronger control and needs configuration this value cannot carry. The
// division matches the root package deliberately: dateparsa.ParseResult.Ambiguous
// reports and dateparsa.WithStrictMode refuses, and a caller who learned that model
// from README meets the same one here.
func (ft FlexTime) Ambiguous() bool {
	return ft.ambiguous
}

// IsZero reports whether the underlying time is the zero value.
func (ft FlexTime) IsZero() bool {
	return ft.t.IsZero()
}

// Equal reports whether ft and other represent the same time instant.
func (ft FlexTime) Equal(other FlexTime) bool {
	if ft.valid != other.valid {
		return false
	}
	if !ft.valid {
		return true // two NULLs are the same value here, unlike in SQL
	}
	return ft.t.Equal(other.t)
}

// String returns the time formatted as RFC3339Nano.
// Returns "<nil>" if not valid.
func (ft FlexTime) String() string {
	if !ft.valid {
		return "<nil>"
	}
	return ft.t.Format(time.RFC3339Nano)
}

// MarshalText implements encoding.TextMarshaler.
// Encodes as RFC3339Nano. Returns empty bytes if not valid.
func (ft FlexTime) MarshalText() ([]byte, error) {
	if !ft.valid {
		return []byte{}, nil
	}
	return []byte(ft.t.Format(time.RFC3339Nano)), nil
}

// Each of the three parse entry points on a FlexTime holds the layout the last
// value it saw was detected with, so a column or a document in one format
// detects once rather than once per value.
//
// This is package-level state and that is the whole of what is unusual about it.
// A FlexTime is constructed by database/sql, by encoding/json or by whatever
// reads the text form, never by the caller, so there is no receiver to hang a
// cache off and no options to configure. Scanner exists for the configured case
// and is the only place a caller can pass options; these are the unconfigured
// paths, which is every path a drop-in replacement for time.Time actually takes.
//
// What it costs is that one value decides which layout the next value is tried
// against, across goroutines and across unrelated callers in the same binary. A
// Scanner's cache reaches only the values its owner feeds it; these are shared.
// It cannot change the instant a value parses to: a layout that does not fit an
// input fails and detection runs again, and Parser.Parse refuses to reuse an
// ambiguity-prone layout at all, so the formats where reuse could return the
// other reading never take the fast path. It can change whether a value parses
// at all, because a layout that fits accepts bytes detection would refuse.
// SECURITY.md describes both halves and FuzzScanAcrossFormats,
// FuzzUnmarshalJSONAcrossFormats and FuzzParserAgreesWithParse hold them down.
//
// One per entry point rather than one shared between them. A process reading a
// database column in one format and unmarshalling JSON in another would evict
// each cache with the other's layout on every value and pay full detection on
// both, which is what this is here to stop.
//
// Default configuration, which is what these paths had before: none of them can
// be passed an option, so there is nothing here a caller could have configured
// differently. See C19 in _plans for why a package-level *default* was refused
// where these caches were not. A default changes what an input parses to; a
// cache changes how long the same answer takes, and, as above, whether an input
// detection refuses is accepted.
var (
	scanParser = dateparsa.NewParser()
	jsonParser = dateparsa.NewParser()
	textParser = dateparsa.NewParser()
)

// UnmarshalText implements encoding.TextUnmarshaler.
// Parses using dateparsa auto-detection.
func (ft *FlexTime) UnmarshalText(data []byte) error {
	s := string(data)
	if s == "" {
		// The inverse of MarshalText, which writes no bytes for an invalid
		// value. It used to be an error, so an invalid FlexTime did not
		// survive a text round trip: marshalling gave "" and unmarshalling
		// that "" refused it.
		ft.set(time.Time{}, false, false)
		return nil
	}
	result, err := textParser.Parse(s)
	if err != nil {
		return fmt.Errorf("flextime: %w", err)
	}
	ft.set(result.Time, true, result.Ambiguous)
	return nil
}
