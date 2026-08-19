package detect

import (
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kmoneil/dateparsa/internal/compile"
	"github.com/kmoneil/dateparsa/internal/locale"
)

// globalTrie is built once at init time.
var globalTrie *trie

func init() {
	globalTrie = buildTrie()
}

// Result holds the outcome of format detection.
type Result struct {
	Def *compile.FormatDef

	// Ambig reports that THIS input needed a guess: "01/02/2024" could be
	// either reading and the configured preference picked one.
	Ambig bool

	// AmbigProne reports that the FORMAT can need a guess, whether or not this
	// input did. "25/12/2024" is unambiguous because 25 is not a month, but it
	// is the same DD/DD/DDDD shape as "01/02/2024" and resolves by value.
	//
	// The distinction exists because Parser caches a layout and reuses it
	// without re-detecting. Ambiguity belongs to the input, so a layout cannot
	// answer it for the next value; what a layout can carry is whether the
	// question arises at all. Strict mode uses this to decline the cache.
	AmbigProne bool

	// AmbigKind says which question the input left open. Meaningful only when
	// Ambig is true, and it is what a caller building the alternative reading
	// has to branch on.
	AmbigKind AmbigKind
}

// AmbigKind names the question an ambiguous input left open.
//
// It exists because the two questions take different answers. The field-order
// one is answered by detecting the input a second time with the preference
// flipped, which produces a different format; the day-or-year one by reading
// one field of the format already detected as the other kind, because flipping
// a preference that does not participate produces the same format and the same
// instant.
//
// A caller that treats every ambiguity as the field-order kind hands the second
// kind back two copies of one reading under labels naming a format the input is
// not, which is what strict mode did for every textual month until C21.
type AmbigKind uint8

const (
	// AmbigNone is an input that needed no guess.
	AmbigNone AmbigKind = iota

	// AmbigFieldOrder is "01/02/2024": two numeric parts that are each a valid
	// month and a valid day, so the configured preference chose between them.
	AmbigFieldOrder

	// AmbigDayOrYear is "March 15": a bare two-digit number beside a month
	// name, read as the fifteenth because it is not over 31, and equally the
	// year 2015.
	AmbigDayOrYear

	// AmbigYearPosition is "01/02/03" read year-first, which is what
	// WithPreferYearFirst asks for. Three small parts and no way to tell from
	// the input which one is the year, so the reading the preference picked is
	// one of three rather than one of two: the year leads, or it trails and the
	// month and the day are still to be told apart.
	AmbigYearPosition
)

// Reading is one way to read an ambiguous input: the format that reads it that
// way, and a label naming the reading.
//
// The label is not Def.Name. classifyTextualPattern names "Mar 15 10:30" a
// MONTH_DAY_YEAR because it counts the time's numbers, and a label has to say
// which reading of the ambiguous number the caller is being offered.
type Reading struct {
	Def   *compile.FormatDef
	Label string
}

// Readings returns every way this input can be read: the one Detect chose,
// first, and the ones it did not.
//
// All of them are built by re-kinding the fields of the format already
// detected, not by detecting again. Nothing about the input moves between them:
// the same bytes are read at the same offsets, and the programs differ in the
// one to three instructions that decide which slot a number lands in.
//
// A second detection cannot produce the set, and that is the bug this replaces.
// The caller used to run Detect twice with PreferDayFirst flipped, which is a
// preference two of the three heuristics ahead of it can overrule:
//
//   - detectTextualMonth does not consult it at all, so "March 15" came back the
//     same both times and strict mode handed over two copies of one instant
//     under the labels MM/DD/YYYY and DD/MM/YYYY, for an input that is not a
//     numeric date.
//   - resolveAmbiguousFields overrides it for a dot separator, so "03.02.2024"
//     came back day-first both times and the copy labelled MM/DD/YYYY carried
//     the third of February. That label reads as the second of March.
//
// It returns nil when the input needed no guess, and when the fields do not
// hold what the kind says they should, which is a detector bug rather than an
// input the caller can be asked about.
func (r Result) Readings() []Reading {
	if r.Def == nil {
		return nil
	}
	switch r.AmbigKind {
	case AmbigDayOrYear:
		return r.dayOrYearReadings()
	case AmbigFieldOrder, AmbigYearPosition:
		return r.fieldOrderReadings()
	}
	return nil
}

// dayOrYearReadings reads the bare number beside a month name as the day of the
// month, which is what Detect chose, and as a two-digit year, which is what it
// did not: "March 15" is the fifteenth of March or March 2015.
func (r Result) dayOrYearReadings() []Reading {
	dayAt, monthAt := -1, -1
	for i := range r.Def.Fields {
		switch r.Def.Fields[i].Kind {
		case compile.FDay2:
			dayAt = i
		case compile.FMonthName:
			monthAt = i
		}
	}
	if dayAt < 0 || monthAt < 0 {
		return nil
	}

	// Which side of the month name the number sits on is what the labels differ
	// by, and it is read off the offsets rather than off the format name for the
	// reason on Reading.
	dayLabel, yearLabel := "MONTH_DAY", "MONTH_YEAR"
	if r.Def.Fields[dayAt].Offset < r.Def.Fields[monthAt].Offset {
		dayLabel, yearLabel = "DAY_MONTH", "YEAR_MONTH"
	}

	fields := copyFields(r.Def.Fields)
	fields[dayAt].Kind = compile.FYear2

	return []Reading{
		{Def: r.Def, Label: dayLabel},
		{Def: &compile.FormatDef{Name: yearLabel, Fields: fields}, Label: yearLabel},
	}
}

// fieldOrderReadings reads the three numeric parts in the order Detect chose,
// and in the other orders that describe the same bytes: "01/02/2024" is the
// second of January or the first of February, and "01/02/03" is either of those
// in 2003 or, for a caller who says their data can be year-first, the third of
// February 2001.
//
// The parts that swap are 12 or under, because that is what made the input
// ambiguous, so each is a valid month and a valid day and no swap can produce a
// date that does not exist. The year-first reading is only offered when Detect
// took it, which is when the caller asked for it and the values allowed it;
// yearFirstParts is where that is decided, and the arithmetic is there.
func (r Result) fieldOrderReadings() []Reading {
	monthAt, dayAt, yearAt := -1, -1, -1
	for i := range r.Def.Fields {
		switch r.Def.Fields[i].Kind {
		case compile.FMonth2, compile.FMonth1or2:
			monthAt = i
		case compile.FDay2, compile.FDay1or2:
			dayAt = i
		case compile.FYear4, compile.FYear2:
			yearAt = i
		}
	}
	if monthAt < 0 || dayAt < 0 || yearAt < 0 {
		return nil
	}

	out := []Reading{{Def: r.Def, Label: numericLabel(r.Def.Fields, monthAt, dayAt, yearAt)}}

	if r.AmbigKind != AmbigYearPosition {
		// The other order of the two parts Detect chose between, with the year
		// where it is.
		if alt, ok := r.rekinded(numericLabel(r.Def.Fields, dayAt, monthAt, yearAt),
			roleAt{yearAt, 'Y'}, roleAt{dayAt, 'M'}, roleAt{monthAt, 'D'}); ok {
			out = append(out, alt)
		}
		return out
	}

	// Year-first leaves no month-versus-day question to offer: a format that
	// writes the year first writes ISO order after it, and YY/DD/MM is not a
	// format anybody writes. So the alternatives are the two year-last
	// readings, and both are new: the year moves out of the leading part and
	// the other two move up.

	// Written as the position each role takes rather than as a pair of swaps,
	// because three roles over three positions is a permutation and naming it
	// that way is what stops the second one being the first one with a typo in
	// it. The chosen def is year, month, day at the three positions in input
	// order, which yearFirstParts guarantees, so position 0 is yearAt,
	// position 1 is monthAt and position 2 is dayAt.
	for _, perm := range [2]struct {
		roles [3]byte
	}{
		{[3]byte{'M', 'D', 'Y'}},
		{[3]byte{'D', 'M', 'Y'}},
	} {
		at := [3]int{yearAt, monthAt, dayAt}
		var m, d, y int
		for i, role := range perm.roles {
			switch role {
			case 'M':
				m = at[i]
			case 'D':
				d = at[i]
			case 'Y':
				y = at[i]
			}
		}
		if rd, ok := r.rekinded(numericLabel(r.Def.Fields, m, d, y),
			roleAt{y, 'Y'}, roleAt{m, 'M'}, roleAt{d, 'D'}); ok {
			out = append(out, rd)
		}
	}
	return out
}

// roleAt says which of year, month and day a field holds in some reading.
type roleAt struct {
	at   int
	role byte
}

// rekinded builds one reading by giving the named fields the kinds their roles
// need, at the widths they already have.
//
// It reports false when a role cannot take the width it is being given, which
// is how a reading that does not exist is left out rather than compiled into a
// field that reads two bytes and declares one. A year is the case: it is
// written with exactly two digits or exactly four, so a one-digit part can be a
// month or a day and never a year.
func (r Result) rekinded(label string, roles ...roleAt) (Reading, bool) {
	fields := copyFields(r.Def.Fields)
	for _, ra := range roles {
		kind, ok := kindForRole(ra.role, fields[ra.at].Len)
		if !ok {
			return Reading{}, false
		}
		fields[ra.at].Kind = kind
	}
	return Reading{Def: &compile.FormatDef{Name: label, Fields: fields}, Label: label}, true
}

// kindForRole is the field kind that reads a part of the given width as a year,
// a month or a day.
func kindForRole(role byte, width int32) (compile.FieldKind, bool) {
	switch role {
	case 'Y':
		switch width {
		case 2:
			return compile.FYear2, true
		case 4:
			return compile.FYear4, true
		}
	case 'M':
		switch width {
		case 1:
			return compile.FMonth1or2, true
		case 2:
			return compile.FMonth2, true
		}
	case 'D':
		switch width {
		case 1:
			return compile.FDay1or2, true
		case 2:
			return compile.FDay2, true
		}
	}
	return 0, false
}

// numericLabel names a reading of a three-part numeric date by the order its
// parts sit in, with the caller saying which index holds which role.
//
// The separator is always a slash, whatever the input uses. "MM/DD/YYYY" is the
// name of an ordering rather than a description of the bytes, and it is the
// string this library has always returned for the reading.
func numericLabel(fields []compile.Field, monthAt, dayAt, yearAt int) string {
	// From the width of the part rather than from its kind in the chosen
	// reading, because in an alternative reading the year sits where the
	// chosen one put a day, and asking that field what kind it is answers
	// about the wrong reading. It said YYYY for the year-last readings of
	// "01/02/03", which has no four-digit year in it anywhere.
	year := "YYYY"
	if fields[yearAt].Len == 2 {
		year = "YY"
	}
	parts := [3]struct {
		offset int32
		name   string
	}{
		{fields[monthAt].Offset, "MM"},
		{fields[dayAt].Offset, "DD"},
		{fields[yearAt].Offset, year},
	}
	for i := 1; i < len(parts); i++ {
		for j := i; j > 0 && parts[j].offset < parts[j-1].offset; j-- {
			parts[j], parts[j-1] = parts[j-1], parts[j]
		}
	}
	return parts[0].name + "/" + parts[1].name + "/" + parts[2].name
}

// copyFields is the alternative reading's own field slice. It runs once per
// input refused in strict mode, so it is off the path the rest of this package
// is written for.
func copyFields(src []compile.Field) []compile.Field {
	dst := make([]compile.Field, len(src))
	copy(dst, src)
	return dst
}

// Config passes user preferences into the detection layer.
type Config struct {
	PreferDayFirst  bool
	PreferYearFirst bool
	Timezone        *time.Location
	Locales         []*locale.Data // Locale data for month/day name lookup
}

// maxDetectFields is how many fields a Scratch holds for one format.
//
// compile.MaxInstructions is the number Compile will accept, so a detector
// producing more is producing a format that is about to be refused. Overflowing
// the array is still correct rather than merely unlikely: the append that
// overflows moves to the heap and the Scratch's array goes unused, which costs
// an allocation on an input that was going to error anyway.
const maxDetectFields = compile.MaxInstructions

// scratchFields is how many fields the heap object holds inline.
//
// It is sized to the work rather than to the limit. The widest format any
// detector in this file produces is 15 fields, which
// TestFallbackFieldCountsFitTheScratch measures rather than assumes; at
// MaxInstructions the object was 656 bytes where the separate allocations it
// replaced came to 448, which is a poor trade for a count that was already
// down from seven to two.
//
// Overflowing it is correct and not merely unlikely: the append in newResult
// moves to the heap, the inline array goes unused, and the format costs one
// more allocation than it needed to. That is a worse number, never a wrong
// answer, which is why this is tuned to the measurement and not to the bound.
const scratchFields = 16

// smallScratchFields is the other size. Most fallback formats are a date and
// nothing else and compile to five or six fields, and giving those the
// sixteen-field object cost more than the separate allocations it replaced:
// 528 bytes a parse against 432, and "March 15, 2024" measured slower for it
// even while allocating one time fewer. Two sizes cover the measured
// distribution, which is 3 to 6 fields for a bare date and 10 to 15 once a time
// or a weekday is in the input, with nothing in between.
const smallScratchFields = 6

// maxTimeFields is the same for a time component on its own, which is an hour,
// a minute, a second, a fraction, an am/pm and a zone, plus the literals
// between them.
const maxTimeFields = 12

// scratch is one heap object holding everything a successful fallback detection
// has to hand back: the FormatDef that Result points at, and the fields it
// describes.
//
// The fallback detectors used to allocate those separately, and a helper each
// for the sub-slices they built on the way, so a textual date cost four
// allocations and "03/15/2024 10:30:00" cost ten. All of them were dead the
// moment Compile ran, since Compile copies every field into the Program by
// value and keeps no reference, and the only other things read off a Def are
// two strings. That is why those formats lost to araddon on a cold parse while
// the trie formats beat it.
//
// It cannot be the caller's, which was the first attempt. A slice taken from an
// array reachable through a pointer parameter and then grown with append is
// marked as escaping whatever the caller does with it, so a caller-owned buffer
// bought one heap object for the buffer in place of the ones it removed, and
// cost the trie formats an allocation they did not have. One object per
// successful detection is what escape analysis will actually give.
//
// It is built at the point a detector knows it has an answer. A detector that
// gives up allocates nothing, which matters because several of them run and
// fail on every input that reaches the fallbacks at all.
type scratch struct {
	def    compile.FormatDef
	fields [scratchFields]compile.Field
}

// smallScratch is the same thing for a format that does not need the room.
type smallScratch struct {
	def    compile.FormatDef
	fields [smallScratchFields]compile.Field
}

// newResult copies fields into one heap object alongside the def that describes
// them, and is how every fallback detector returns.
//
// fields is copied rather than retained, so the caller can build it in a local
// array that never leaves its frame.
// The ambiguity arrives as a kind rather than as a bool so that no detector can
// report that this input needed a guess without saying which guess it was. Ambig
// is derived here and read everywhere, because "did this need a guess" is the
// question almost every caller asks.
func newResult(name, goLayout string, fields []compile.Field, ambig AmbigKind, prone bool) Result {
	if len(fields) <= smallScratchFields {
		sc := new(smallScratch)
		sc.def = compile.FormatDef{
			Name:     name,
			GoLayout: goLayout,
			Fields:   append(sc.fields[:0], fields...),
		}
		return Result{Def: &sc.def, Ambig: ambig != AmbigNone, AmbigProne: prone, AmbigKind: ambig}
	}
	sc := new(scratch)
	sc.def = compile.FormatDef{
		Name:     name,
		GoLayout: goLayout,
		Fields:   append(sc.fields[:0], fields...),
	}
	return Result{Def: &sc.def, Ambig: ambig != AmbigNone, AmbigProne: prone, AmbigKind: ambig}
}

// Detect analyzes a date string and returns the matching FormatDef.
// Returns ok=false if no structured format matches.
//
// The detectors are in detectFormat; what this wrapper adds is the last check,
// which every detector needs and none of them can do on its own: a format whose
// skipped runs hold a word that decides the answer has not described the input,
// it has discarded part of it. See skipwords.go, and C26 for the four English
// phrases that were answered with the wrong day until this ran.
func Detect(s string, cfg Config) (Result, bool) {
	r, ok := detectFormat(s, cfg)
	if !ok {
		return Result{}, false
	}
	if skipRunCarriesMeaning(s, r.Def, cfg.Locales) {
		return Result{}, false
	}
	return r, true
}

func detectFormat(s string, cfg Config) (Result, bool) {
	// Every position below becomes a compile.Field.Offset or Len, which are
	// int32. Nothing here can describe an input longer than that, and Compile
	// refuses any field past byte 255 in any case, so an input this long has no
	// correct answer. Refusing at the door is what makes the int32 conversions
	// in this file safe: the alternative is a position wrapping into a small
	// positive offset that looks valid and reads the wrong bytes, which is the
	// C9 failure shape.
	//
	// This is the only entry point to the package, so one check covers every
	// detector.
	if len(s) > math.MaxInt32 {
		return Result{}, false
	}

	// And nothing this package can describe is longer than two field widths,
	// which is compile.MaxDescribableLen. The check above is about the int32
	// conversions; this one is about the work: the trie reads the first 64
	// bytes and stops, but the fallback detectors behind it are linear in the
	// input and run once per configured locale, so a megabyte of prose cost
	// 1.27 seconds of CPU with twenty locales and could not have produced an
	// answer at the end of it.
	//
	// It refuses nothing that used to parse. A field starts at byte 255 at the
	// latest and runs 255 bytes at the most, so a program cannot cover an input
	// longer than their sum, and the executor requires the whole input to be
	// covered. The one field whose width was unbounded was OpTail, which W16
	// bounded at 64; until then this bound was not provable.
	if len(s) > compile.MaxDescribableLen {
		return Result{}, false
	}

	// Step 1: Try special formats that contain letters but aren't textual months.
	if r, ok := detectISOWeekOrOrdinal(s); ok {
		return r, true
	}

	// Step 1b: Try CJK ideographic dates before textual month (年月日).
	if r, ok := detectCJKDate(s); ok {
		return r, true
	}

	// Step 2: Compute signature and walk the trie. The signature scan also
	// detects whether the input contains letters (sig.HasLetter), eliminating
	// the need for a separate hasLetter pass over the input.
	sig := Scan(s)
	entry := globalTrie.lookup(&sig)

	// Step 2b: If the trie missed and the string contains letters, try textual month.
	// This is ordered after the trie so that formats with timezone names (e.g.
	// "2024-03-15 10:30:00 UTC") are matched by the trie first.
	//
	// sig.HasLetter alone was the condition, and it is computed from a pass that
	// stops at maxSigLen, so it means "has a letter in the first 64 bytes". A
	// date behind 64 bytes of anything else never reached this detector at all:
	// Parse(strings.Repeat(" ", 64) + "March 15, 2024") fell through to natural
	// language, which read "March 15" and answered with the base year rather
	// than the 2024 in front of it. See hasLetterPastSignature for why the
	// second look lives here and not in Scan.
	if entry == nil && (sig.HasLetter || hasLetterPastSignature(s)) {
		if result, ok := detectTextualMonth(s, cfg); ok {
			return result, true
		}
	}

	if entry == nil {
		// Step 3b: Try ISO 8601 with variable-length fractional seconds.
		if r, ok := detectISO8601Frac(s); ok {
			return r, true
		}
		// Step 3c: Try variable-width numeric formats (e.g. 3/15/2024, 3/15/24).
		if r, ok := detectVariableNumeric(s, cfg); ok {
			return r, true
		}
		// Step 3d: Try Go time.String() output and other complex formats.
		if r, ok := detectGoTimeString(s); ok {
			return r, true
		}
		// Step 3e: Try date + tz offset without time (e.g. 2020-07-20+08:00).
		if r, ok := detectDatePlusTZ(s); ok {
			return r, true
		}
		return Result{}, false
	}

	// Step 4: Handle ambiguous formats.
	if entry.ambig {
		return resolveAmbiguous(s, cfg)
	}

	// Step 5: Return the pre-built FormatDef (zero allocation for trie-matched formats).
	//
	// The pre-built def carries the entry's own goLayout, which describes the
	// input only when the input spells its literals the way the layout does. A
	// signature is a sequence of character classes, so one entry matches
	// "2024-03-15", "2024/03/15" and "2024.03.15", and the layout names one of
	// the three. goLayoutFor answers that question in a couple of byte compares
	// for the canonical spelling, which is the one nearly every input uses, and
	// builds a corrected def for the others.
	if entry.def != nil {
		gl := goLayoutFor(entry, s)
		if gl == entry.goLayout {
			return Result{Def: entry.def}, true
		}
		return newResult(entry.name, gl, entry.fields, AmbigNone, false), true
	}
	// Fallback for entries without pre-built defs.
	return newResult(entry.name, goLayoutFor(entry, s), entry.fields, AmbigNone, false), true
}

// withTimeSuffix names the format that resolveAmbiguous's date and a trailing
// time make together.
//
// It was Name + "_TIME", which allocates a string on every parse of every
// "03/15/2024 10:30:00" in a column, for one of three answers known when this
// file was written. resolveAmbiguous produces exactly the three names below and
// TestWithTimeSuffixCoversResolveAmbiguous holds it to them, so an unnamed
// fourth falls back to the concatenation rather than to a wrong label.
func withTimeSuffix(name string) string {
	switch name {
	case "NUMERIC_MDY":
		return "NUMERIC_MDY_TIME"
	case "NUMERIC_DMY":
		return "NUMERIC_DMY_TIME"
	case "NUMERIC_AMBIG":
		return "NUMERIC_AMBIG_TIME"
	}
	return name + "_TIME"
}

// lit returns a one-byte literal field carrying the byte the detector checked,
// so the executor can check it too. A separator is what tells two formats
// apart: ISO_ORDINAL described its year and its day-of-year and nothing at all
// for the '-' between them, so a layout built from "0000-001" accepted the
// compact date "00000101" and read its "0101" as day-of-year 101.
func lit(s string, off int) compile.Field {
	return compile.Field{Kind: compile.FLiteral, Offset: int32(off), Len: 1, Aux: uint16(s[off])}
}

// coverGaps appends a skip for every run of bytes no field reads, so the
// program accounts for the whole input.
//
// The textual formats locate their fields by scanning, so what sits between
// them is punctuation, a weekday name, or an " at ", and none of it is read.
// Width is still worth fixing: a run of a different length shifts every field
// after it, which is how a layout built from "Fri Jul 03 2015" would land its
// day field in the middle of "Wednesday". Content is not worth checking here,
// unlike the separators above, because nothing else parses these bytes
// differently: "March 15; 2024" has only one reading whatever the punctuation.
func coverGaps(fields []compile.Field, s string) []compile.Field {
	// One bit per input byte rather than one bool, so the common case needs no
	// heap at all: coverWords covers 256 bytes, which is every input a program
	// can address, since Compile refuses a field past byte 255. A longer input
	// still gets the exact same answer from a slice, because it is the answer
	// that decides what the program describes and it may not depend on how the
	// bookkeeping was stored.
	//
	// This allocated a []bool of len(s) on every call, and it is reached by
	// every textual format and every variable-width numeric one, which is
	// exactly the set that was losing to araddon on a cold parse.
	var stack [coverWords]uint64
	covered := stack[:]
	if words := (len(s) + 63) / 64; words > coverWords {
		covered = make([]uint64, words)
	}

	// The bit twiddling is written out rather than wrapped in a set/isSet pair.
	// Closures over covered were the first version and they kept the array on
	// the heap anyway: a captured local escapes unless every closure over it
	// inlines, so the allocation this function exists to remove came back
	// through the helpers meant to make it readable.
	for _, f := range fields {
		off, w := int(f.Offset), int(f.Len)
		if fw, fixed := compile.FixedWidth(f.Kind); fixed {
			w = fw
		}
		for i := off; i < off+w && i < len(s); i++ {
			covered[i>>6] |= 1 << uint(i&63)
		}
	}
	for i := 0; i < len(s); {
		if covered[i>>6]&(1<<uint(i&63)) != 0 {
			i++
			continue
		}
		start := i
		for i < len(s) && covered[i>>6]&(1<<uint(i&63)) == 0 {
			i++
		}
		fields = append(fields, skip(start, i-start))
	}
	return fields
}

// coverWords is how many uint64 of coverage sit on the stack: 256 bits, which
// is every byte a compiled program can address.
const coverWords = 4

// skip covers a run the format does not read and does not constrain, such as
// the weekday name of an RFC 2822 date. It fixes the run's width without
// looking at it, which is what stops a wider one shifting every field after it.
func skip(off, length int) compile.Field {
	return compile.Field{Kind: compile.FSkip, Offset: int32(off), Len: int32(length)}
}

// detectISO8601Frac handles ISO 8601/RFC 3339 with variable-length fractional seconds:
// "2024-03-15T10:30:00.123Z", "2024-03-15T10:30:00.123456+05:30",
// "2024-03-15 10:30:00.123456789Z", etc.
// Matches: YYYY-MM-DD[T ]HH:MM:SS.{1-9 digits}[Z|±HH:MM|±HHMM]
func detectISO8601Frac(s string) (Result, bool) {
	n := len(s)
	// Minimum: "YYYY-MM-DDTHH:MM:SS.f" = 21 chars, and the zone is optional.
	//
	// This was 22, counting a trailing zone byte the format does not require, so a
	// one-digit fraction with no zone was one byte short and refused:
	// "2024-03-15 10:30:00.9" did not parse while "2024-03-15 10:30:00.99" did.
	// Nothing else in the cascade covers it, because the trie carries the
	// three-digit and six-digit fractions and not the one-digit one.
	//
	// Found by TestParseAgreesWithTimeParseOnFractions, which asserts that a
	// fraction the stdlib reads exactly is one this library does not refuse. That
	// assertion is about C17 and this is a second, older defect underneath it.
	if n < 21 {
		return Result{}, false
	}
	// Check the fixed prefix: YYYY-MM-DD[T ]HH:MM:SS.
	if !(isDigit(s[0]) && s[4] == '-' && s[7] == '-' &&
		(s[10] == 'T' || s[10] == ' ') &&
		s[13] == ':' && s[16] == ':' && s[19] == '.') {
		return Result{}, false
	}

	// Count fractional digits after the dot.
	//
	// The upper bound is what the other two producers of this field were missing.
	// A nanosecond holds nine digits, so a wider run has no representation, and
	// this detector has always refused one. detectGoTimeString and
	// parseTimeComponent accepted one and computed the wrong instant from it, so
	// three detectors disagreed about the same input in the worst direction. They
	// agree now, and this is the arm that was already right.
	fracStart := 20
	fracEnd := fracStart
	for fracEnd < n && isDigit(s[fracEnd]) {
		fracEnd++
	}
	fracLen := fracEnd - fracStart
	if fracLen < 1 || fracLen > compile.MaxFracDigits {
		return Result{}, false
	}

	var buf [maxDetectFields]compile.Field
	fields := append(buf[:0],
		compile.Field{Kind: compile.FYear4, Offset: 0, Len: 4},
		lit(s, 4),
		compile.Field{Kind: compile.FMonth2, Offset: 5, Len: 2},
		lit(s, 7),
		compile.Field{Kind: compile.FDay2, Offset: 8, Len: 2},
		lit(s, 10),
		compile.Field{Kind: compile.FHour24, Offset: 11, Len: 2},
		lit(s, 13),
		compile.Field{Kind: compile.FMinute2, Offset: 14, Len: 2},
		lit(s, 16),
		compile.Field{Kind: compile.FSecond2, Offset: 17, Len: 2},
		lit(s, 19),
		compile.Field{Kind: compile.FFracSec, Offset: int32(fracStart), Len: int32(fracLen)},
	)

	// Parse timezone immediately after fractional seconds (no space).
	// If there's a space, bail — let detectGoTimeString handle it.
	pos := fracEnd
	if pos < n {
		if s[pos] == 'Z' && (pos+1 == n) {
			fields = append(fields, compile.Field{Kind: compile.FTZZ, Offset: int32(pos), Len: 1})
		} else if s[pos] == '+' || s[pos] == '-' {
			tzLen := n - pos
			if tzLen == 5 || tzLen == 6 {
				fields = append(fields, compile.Field{Kind: compile.FTZOffset, Offset: int32(pos), Len: int32(tzLen)})
			} else {
				return Result{}, false // complex tz — let other handlers deal with it
			}
		} else {
			// Anything else after the fraction, a space included, is not this
			// format. A space means a Go time.String or a SQL value with a
			// separated zone, and detectGoTimeString reads the offset those
			// carry. Returning a def covering only the prefix here read
			// "2012-08-03 18:31:59.257000000 +0300 MSK" as UTC, three hours
			// off, and did it only when a fraction was present: the same value
			// without one reached detectGoTimeString and came out right.
			return Result{}, false
		}
	}

	return newResult("ISO8601_FRAC", "", fields, AmbigNone, false), true
}

// detectGoTimeString handles Go's time.Time.String() output:
// "2012-08-03 18:31:59.257000000 +0000 UTC"
// "2015-02-08 03:02:00 +0300 MSK"
// Pattern: YYYY-MM-DD HH:MM:SS[.nnnnnnnnn] +HHMM [TZName]
func detectGoTimeString(s string) (Result, bool) {
	n := len(s)
	// Minimum: "YYYY-MM-DD HH:MM:SS +HHMM" = 25 chars
	if n < 25 {
		return Result{}, false
	}
	// Must start with YYYY-MM-DD HH:MM:SS pattern.
	if !(isDigit(s[0]) && s[4] == '-' && s[7] == '-' && s[10] == ' ' &&
		s[13] == ':' && s[16] == ':') {
		return Result{}, false
	}

	fields := make([]compile.Field, 0, 10)
	fields = append(fields,
		compile.Field{Kind: compile.FYear4, Offset: 0, Len: 4},
		lit(s, 4),
		compile.Field{Kind: compile.FMonth2, Offset: 5, Len: 2},
		lit(s, 7),
		compile.Field{Kind: compile.FDay2, Offset: 8, Len: 2},
		lit(s, 10),
		compile.Field{Kind: compile.FHour24, Offset: 11, Len: 2},
		lit(s, 13),
		compile.Field{Kind: compile.FMinute2, Offset: 14, Len: 2},
		lit(s, 16),
		compile.Field{Kind: compile.FSecond2, Offset: 17, Len: 2},
	)

	pos := 19
	// Optional fractional seconds.
	//
	// Bounded at compile.MaxFracDigits, which it was not. A nanosecond holds nine digits
	// and this emitted a field as wide as the run, so
	// "2024-03-15 10:30:00.99999999999999999999999 +0000 UTC" produced a field of
	// 23 and came back as 2030-07-21 with a nil error, where time.Parse reads
	// 2024-03-15. detectISO8601Frac has carried this bound all along; the two
	// detectors that did not are what C17 was.
	if pos < n && s[pos] == '.' {
		fracStart := pos + 1
		fracEnd := fracStart
		for fracEnd < n && isDigit(s[fracEnd]) {
			fracEnd++
		}
		if fracEnd-fracStart > compile.MaxFracDigits {
			return Result{}, false
		}
		if fracEnd > fracStart {
			fields = append(fields, lit(s, pos),
				compile.Field{Kind: compile.FFracSec, Offset: int32(fracStart), Len: int32(fracEnd - fracStart)})
			pos = fracEnd
		}
	}

	// Skip space.
	if pos < n && s[pos] == ' ' {
		fields = append(fields, lit(s, pos))
		pos++
	}

	// Timezone offset: +HHMM or +HH:MM
	if pos < n && (s[pos] == '+' || s[pos] == '-') {
		tzStart := pos
		pos++
		// Count digits and optional colon.
		digits := 0
		for pos < n && (isDigit(s[pos]) || s[pos] == ':') {
			pos++
			digits++
		}
		if digits >= 4 {
			fields = append(fields, compile.Field{Kind: compile.FTZOffset, Offset: int32(tzStart), Len: int32(pos - tzStart)})
		}
	}

	// The timezone name and any trailing content ("m=+0.000000001") are
	// ignored: the offset above already fixes the instant. FTail records that
	// as a decision the program carries, rather than leaving it to the
	// executor not to check.
	//
	// Described by compile.ValidTail, which it was not. Everything else this
	// library detects is bounded by what a program can describe, because a
	// field cannot start past byte 255 or run longer than 255; the tail was the
	// exception, so "2024-03-15 10:30:00 +0000 UTC" followed by a megabyte of
	// anything was a date. Refused here as well as in the executor so that
	// Detect and Layout.Parse agree about what the format is.
	if pos < n {
		if !compile.ValidTail(s[pos:]) {
			return Result{}, false
		}
		fields = append(fields, compile.Field{Kind: compile.FTail, Offset: int32(pos)})
	}

	return newResult("GO_TIME_STRING", "", fields, AmbigNone, false), true
}

// detectDatePlusTZ handles date + timezone offset without time:
// "2020-07-20+08:00"
func detectDatePlusTZ(s string) (Result, bool) {
	n := len(s)
	if n < 15 || n > 16 {
		return Result{}, false
	}
	// Must be YYYY-MM-DD±HH:MM
	if !(isDigit(s[0]) && s[4] == '-' && isDigit(s[5]) && s[7] == '-' && isDigit(s[8])) {
		return Result{}, false
	}
	if s[10] != '+' && s[10] != '-' {
		return Result{}, false
	}

	tzLen := n - 10
	fields := []compile.Field{
		{Kind: compile.FYear4, Offset: 0, Len: 4},
		lit(s, 4),
		{Kind: compile.FMonth2, Offset: 5, Len: 2},
		lit(s, 7),
		{Kind: compile.FDay2, Offset: 8, Len: 2},
		{Kind: compile.FTZOffset, Offset: 10, Len: int32(tzLen)},
	}
	return newResult("ISO_DATE_TZ", "", fields, AmbigNone, false), true
}

// detectCJKDate handles dates with CJK ideographic characters:
// "2014年04月08日" (year年month月day日)
func detectCJKDate(s string) (Result, bool) {
	// Look for 年 (U+5E74), 月 (U+6708), 日 (U+65E5)
	// In UTF-8: 年=E5B9B4, 月=E69C88, 日=E697A5
	yearIdx := strings.Index(s, "年")
	monthIdx := strings.Index(s, "月")
	dayIdx := strings.Index(s, "日")

	if yearIdx < 0 || monthIdx < 0 || dayIdx < 0 {
		return Result{}, false
	}
	if yearIdx >= monthIdx || monthIdx >= dayIdx {
		return Result{}, false
	}

	// Extract year (digits before 年).
	yearStr := s[:yearIdx]
	if len(yearStr) != 4 {
		return Result{}, false
	}

	// Extract month (digits between 年 and 月).
	monStr := s[yearIdx+len("年") : monthIdx]
	// Extract day (digits between 月 and 日).
	dayStr := s[monthIdx+len("月") : dayIdx]

	// Validate they're all digits.
	if !allDigits(yearStr) || !allDigits(monStr) || !allDigits(dayStr) {
		return Result{}, false
	}

	// Build fields using byte offsets.
	fields := []compile.Field{
		{Kind: compile.FYear4, Offset: 0, Len: int32(len(yearStr))},
		skip(yearIdx, len("年")),
		{Kind: compile.FMonth1or2, Offset: int32(yearIdx + len("年")), Len: int32(len(monStr))},
		skip(monthIdx, len("月")),
		{Kind: compile.FDay1or2, Offset: int32(monthIdx + len("月")), Len: int32(len(dayStr))},
		skip(dayIdx, len("日")),
	}
	return newResult("CJK_DATE", "", fields, AmbigNone, false), true
}

// detectVariableNumeric handles numeric dates with variable-width fields:
// "3/15/2024", "3/15/24", "3/15/2024 10:30:00 AM", etc.
// Recognizes patterns of 2-3 numeric parts separated by / or -.
func detectVariableNumeric(s string, cfg Config) (Result, bool) {
	if len(s) < 3 {
		return Result{}, false
	}

	// Find the separator (must be / or -).
	sepIdx := -1
	for i := 0; i < len(s) && i < 4; i++ {
		if s[i] == '/' || s[i] == '-' {
			sepIdx = i
			break
		}
	}
	if sepIdx < 1 {
		return Result{}, false
	}
	sep := s[sepIdx]

	// Split the date portion (stop at space for compound formats).
	dateEnd := len(s)
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' || s[i] == '\t' {
			dateEnd = i
			break
		}
	}
	datePart := s[:dateEnd]

	var partsBuf [maxDateParts]string
	parts := splitOnSep(partsBuf[:0], datePart, sep)
	if len(parts) < 2 || len(parts) > 3 {
		return Result{}, false
	}

	// Validate all parts are numeric and reasonable length.
	for _, p := range parts {
		if len(p) == 0 || len(p) > 4 {
			return Result{}, false
		}
		for j := 0; j < len(p); j++ {
			if p[j] < '0' || p[j] > '9' {
				return Result{}, false
			}
		}
	}

	// For 2-part dates, not a full date. Skip.
	if len(parts) < 3 {
		return Result{}, false
	}

	// Use resolveAmbiguous logic on the date portion.
	// But first, verify this looks like a numeric date, not an ISO date
	// (ISO dates are 4-digit year first with - separator, already handled by trie).
	if sep == '-' && len(parts[0]) == 4 {
		return Result{}, false // YYYY-MM-DD handled by trie
	}

	// Dot separator with 4-digit last part also handled by trie.
	if sep == '.' {
		return Result{}, false
	}

	// Resolve the date into a local buffer rather than through
	// resolveAmbiguous, so that a trailing time can be appended to it before
	// anything is put on the heap.
	var buf [maxDetectFields]compile.Field
	name, fields, ambig, ok := resolveAmbiguousFields(buf[:0], datePart, cfg)
	if !ok {
		return Result{}, false
	}

	// If there's a time component after the date, parse it.
	if dateEnd < len(s) {
		var tbuf [maxTimeFields]compile.Field
		timeFields := parseTimeComponent(tbuf[:0], s, dateEnd)
		if len(timeFields) > 0 {
			name = withTimeSuffix(name)
			fields = coverGaps(append(fields, timeFields...), s)
		}
	}

	return newResult(name, "", fields, ambig, true), true
}

// datePart holds a parsed component of an ambiguous date string with its position.
type datePart struct {
	value  int
	offset int
	length int
}

// resolveAmbiguous handles DD/DD/DDDD type signatures where the format
// could be MM/DD/YYYY or DD/MM/YYYY.
func resolveAmbiguous(s string, cfg Config) (Result, bool) {
	var buf [maxDetectFields]compile.Field
	name, fields, ambig, ok := resolveAmbiguousFields(buf[:0], s, cfg)
	if !ok {
		return Result{}, false
	}
	return newResult(name, "", fields, ambig, true), true
}

// resolveAmbiguousFields is resolveAmbiguous without the heap object, for a
// caller that has more to add before it wants one.
//
// detectVariableNumeric is that caller: it resolves the date and then appends a
// time to it. Going through resolveAmbiguous made it build one object for the
// date, overflow it with the time fields, and build a second, so
// "03/15/2024 10:30:00" cost two allocations where it needed one and the first
// was thrown away.
func resolveAmbiguousFields(dst []compile.Field, s string, cfg Config) (name string, fields []compile.Field, ambig AmbigKind, ok bool) {
	sep := findSep(s)
	if sep < 0 {
		return "", nil, AmbigNone, false
	}
	sepChar := s[sep]

	// Dot separator strongly implies European DMY convention.
	if sepChar == '.' {
		cfg.PreferDayFirst = true
	}

	var partsBuf [maxDateParts]string
	parts := splitOnSep(partsBuf[:0], s, sepChar)
	if len(parts) < 3 {
		return "", nil, AmbigNone, false
	}

	first := parseSmallInt(parts[0])
	second := parseSmallInt(parts[1])
	third := parseSmallInt(parts[2])
	if first < 0 || second < 0 || third < 0 {
		return "", nil, AmbigNone, false
	}

	monthPart, dayPart, ambig, ok := resolveYearMonthDay(parts, first, second, third, cfg)
	if !ok {
		return "", nil, AmbigNone, false
	}

	fields, ok = buildDatePartFields(dst, parts, sepChar, monthPart, dayPart)
	if !ok {
		return "", nil, AmbigNone, false
	}

	name = "NUMERIC_AMBIG"
	if ambig == AmbigNone {
		if cfg.PreferDayFirst {
			name = "NUMERIC_DMY"
		} else {
			name = "NUMERIC_MDY"
		}
	}
	return name, fields, ambig, true
}

// resolveYearMonthDay determines which parts are year, month, and day,
// and whether the result is ambiguous.
// resolveYearMonthDay decides which of three numeric parts is the month and
// which is the day, given where the year sits.
//
// It used to return the year as well, and the only caller wrote
// "_ = year // year value used for validation only" and dropped it. Nothing
// validated it, here or anywhere: the fields the caller builds read the year
// out of the input at run time, which is the whole design. The year is still
// identified, because that is what leaves two parts to choose between, but it
// is identified rather than computed.
func resolveYearMonthDay(parts []string, first, second, third int, cfg Config) (month, day datePart, ambig AmbigKind, ok bool) {
	// Step 1: Identify year position.
	var v1, v2 int
	var v1Offset, v2Offset int

	if third > 31 || len(parts[2]) == 4 {
		// Year is last: ??/??/YYYY
		v1, v2 = first, second
		v1Offset = 0
		v2Offset = len(parts[0]) + 1
	} else if first > 31 || len(parts[0]) == 4 {
		// Year is first: YYYY/??/??
		v1, v2 = second, third
		v1Offset = len(parts[0]) + 1
		v2Offset = len(parts[0]) + 1 + len(parts[1]) + 1
	} else if m, d, ok := yearFirstParts(parts, second, third, cfg); ok {
		// All three parts are small, and the caller said their data can be
		// year-first, so "01/02/03" is 2001-02-03 rather than 2003-01-02.
		//
		// Year-first means year, then month, then day, in that order and no
		// other: every format that writes the year first writes ISO order
		// after it, and nothing writes YY/DD/MM. So this arm decides both
		// questions at once and step 2 does not run for it, where the other
		// two arms leave the month and the day to be told apart by value.
		//
		// It is still ambiguous, and more so than the arm below: the input has
		// a year-last reading as well, which is what the caller is refusing to
		// choose between under strict mode.
		return m, d, AmbigYearPosition, true
	} else {
		// All small numbers, truly ambiguous with 2-digit year last.
		v1, v2 = first, second
		v1Offset = 0
		v2Offset = len(parts[0]) + 1
	}

	// Step 2: Resolve month vs day from the two non-year parts.
	p1 := datePart{v1, v1Offset, len(parts[partIndex(v1Offset, parts)])}
	p2 := datePart{v2, v2Offset, len(parts[partIndex(v2Offset, parts)])}

	if v1 > 12 {
		// First must be day.
		day, month = p1, p2
	} else if v2 > 12 {
		// Second must be day.
		month, day = p1, p2
	} else {
		// Genuinely ambiguous — both could be month or day.
		ambig = AmbigFieldOrder
		if cfg.PreferDayFirst {
			day, month = p1, p2
		} else {
			month, day = p1, p2
		}
	}

	if month.value < 1 || month.value > 12 || day.value < 1 || day.value > 31 {
		return datePart{}, datePart{}, AmbigNone, false
	}

	// A day or a month is written with one digit or two, never three. The
	// field kinds below can only express those two widths, so a wider part
	// produced a field whose declared width and read width disagreed:
	// buildDatePartFields gave "020" an FDay2 with Len 3, OpDay2 read the
	// first two bytes, and Parse("020/01/2024") came back as the second of
	// January having validated the twentieth.
	if month.length > 2 || day.length > 2 {
		return datePart{}, datePart{}, AmbigNone, false
	}
	return month, day, ambig, true
}

// yearFirstParts reads an all-small three-part date as year, month, day, and
// reports whether that reading exists at all.
//
// It is what WithPreferYearFirst does, and until now the option did nothing:
// detect.Config declared PreferYearFirst, four call sites set it, and no
// detector read it, so "01/02/03" was the second of January 2003 with the
// option on and with it off. README documented it as a preference rule in two
// places for the whole of that time.
//
// The reading has to be structurally possible before the preference can pick
// it, and it is not always:
//
//	"01/02/03"  year 01, month 02, day 03    available
//	"01/13/03"  month 13 does not exist      not available, so year-last stands
//	"1/02/03"   a year field is two bytes    not available
//
// Falling back rather than refusing is the point. A caller who sets the option
// because some of their rows are year-first still has to parse the rows that
// are not, and a row the reading cannot describe is not an error, it is a row
// the reading does not apply to.
func yearFirstParts(parts []string, second, third int, cfg Config) (month, day datePart, ok bool) {
	if !cfg.PreferYearFirst {
		return datePart{}, datePart{}, false
	}
	// A year field reads exactly two bytes or exactly four, and four would have
	// been taken by the arm above, so a one-digit leading part cannot be one.
	if len(parts[0]) != 2 {
		return datePart{}, datePart{}, false
	}
	if second < 1 || second > 12 || third < 1 || third > 31 {
		return datePart{}, datePart{}, false
	}
	if len(parts[1]) > 2 || len(parts[2]) > 2 {
		return datePart{}, datePart{}, false
	}
	monthOffset := len(parts[0]) + 1
	dayOffset := monthOffset + len(parts[1]) + 1
	return datePart{second, monthOffset, len(parts[1])},
		datePart{third, dayOffset, len(parts[2])},
		true
}

// partIndex returns which index (0, 1, or 2) a given byte offset corresponds to
// in a 3-part date string split by separators.
func partIndex(offset int, parts []string) int {
	pos := 0
	for i, p := range parts {
		if pos == offset {
			return i
		}
		pos += len(p) + 1 // +1 for separator
	}
	return 0
}

// buildDatePartFields constructs compile.Fields for a 3-part date, sorting by
// input position and inserting literal separator fields between them.
func buildDatePartFields(dst []compile.Field, parts []string, sepChar byte, month, day datePart) ([]compile.Field, bool) {
	// Determine year part by elimination: whichever offset is not month or day.
	// Both are assigned on every path below, so neither carries an initializer.
	var yearOffset, yearLen int
	p1End := len(parts[0]) + 1
	p2End := p1End + len(parts[1]) + 1
	if month.offset == 0 || day.offset == 0 {
		if month.offset != 0 && day.offset != 0 {
			yearOffset = 0
			yearLen = len(parts[0])
		} else if month.offset == 0 {
			if day.offset == p1End {
				yearOffset = p2End
				yearLen = len(parts[2])
			} else {
				yearOffset = p1End
				yearLen = len(parts[1])
			}
		} else {
			if month.offset == p1End {
				yearOffset = p2End
				yearLen = len(parts[2])
			} else {
				yearOffset = p1End
				yearLen = len(parts[1])
			}
		}
	} else {
		yearOffset = 0
		yearLen = len(parts[0])
	}

	// A year here is written with exactly two digits or exactly four, because
	// those are the only widths FYear2 and FYear4 read. Anything else produced
	// a field whose declared width and read width disagreed, which is the same
	// defect as the over-wide day and month parts refused above. Those two
	// happened to refuse anyway, by running off the end of the input, which is
	// luck rather than a check.
	var yearKind compile.FieldKind
	switch yearLen {
	case 4:
		yearKind = compile.FYear4
	case 2:
		yearKind = compile.FYear2
	default:
		return nil, false
	}
	monthKind := compile.FMonth2
	if month.length == 1 {
		monthKind = compile.FMonth1or2
	}
	dayKind := compile.FDay2
	if day.length == 1 {
		dayKind = compile.FDay1or2
	}

	type posField struct {
		offset int
		field  compile.Field
	}
	infos := [3]posField{
		{yearOffset, compile.Field{Kind: yearKind, Offset: int32(yearOffset), Len: int32(yearLen)}},
		{month.offset, compile.Field{Kind: monthKind, Offset: int32(month.offset), Len: int32(month.length)}},
		{day.offset, compile.Field{Kind: dayKind, Offset: int32(day.offset), Len: int32(day.length)}},
	}

	// Sort 3 elements by offset (simple conditional swaps).
	if infos[0].offset > infos[1].offset {
		infos[0], infos[1] = infos[1], infos[0]
	}
	if infos[1].offset > infos[2].offset {
		infos[1], infos[2] = infos[2], infos[1]
	}
	if infos[0].offset > infos[1].offset {
		infos[0], infos[1] = infos[1], infos[0]
	}

	// Build fields with literal separators between them.
	//
	// The run between two parts is the byte the caller split on, and findSep
	// only ever picks '-', '/' or '.', so it is a CSep whatever the input was.
	// Saying so is what stops the layout reading another format of the same
	// shape: while this literal asked only that the byte was not a digit, one
	// built from "20-1-00" accepted "10:01:00" and answered the tenth of
	// January 2000 for an input that means one minute past ten.
	//
	// The class rather than sepChar itself, for the reason a trie literal takes
	// a class: a column written with one separator and read with another is a
	// format this detector resolves the same way both times.
	sepAux := compile.AuxFor(compile.ClassSep)
	if sepChar != '-' && sepChar != '/' && sepChar != '.' {
		sepAux = 0 // not a class this knows; fall back to "not a digit"
	}
	fields := dst
	prevEnd := 0
	for _, info := range infos {
		if info.offset > prevEnd {
			aux := sepAux
			if info.offset-prevEnd != 1 {
				aux = 0 // a wider run needs a class per byte, and Aux holds one
			}
			fields = append(fields, compile.Field{
				Kind: compile.FLiteral, Offset: int32(prevEnd), Len: int32(info.offset - prevEnd), Aux: aux,
			})
		}
		fields = append(fields, info.field)
		prevEnd = info.offset + int(info.field.Len)
	}
	return fields, true
}

// detectISOWeekOrOrdinal detects ISO week dates (2024-W11-5) and ordinal dates (2024-074).
func detectISOWeekOrOrdinal(s string) (Result, bool) {
	n := len(s)

	// ISO week date: YYYY-Www-D (10 chars) or YYYY-Www (8 chars)
	// Pattern: 4 digits, '-', 'W', 2 digits, optionally '-', 1 digit
	if n >= 8 && s[4] == '-' && (s[5] == 'W' || s[5] == 'w') {
		if n == 8 && isDigit(s[6]) && isDigit(s[7]) {
			// YYYY-Www (week only, assume day 1=Monday)
			var buf [maxDetectFields]compile.Field
			return newResult("ISO_WEEK", "", append(buf[:0],
				compile.Field{Kind: compile.FYear4, Offset: 0, Len: 4},
				lit(s, 4), lit(s, 5),
				compile.Field{Kind: compile.FISOWeek, Offset: 6, Len: 2},
			), AmbigNone, false), true
		}
		if n == 10 && isDigit(s[6]) && isDigit(s[7]) && s[8] == '-' && isDigit(s[9]) {
			// YYYY-Www-D
			var buf [maxDetectFields]compile.Field
			return newResult("ISO_WEEK_DATE", "", append(buf[:0],
				compile.Field{Kind: compile.FYear4, Offset: 0, Len: 4},
				lit(s, 4), lit(s, 5),
				compile.Field{Kind: compile.FISOWeek, Offset: 6, Len: 2},
				lit(s, 8),
				compile.Field{Kind: compile.FISOWeekDay, Offset: 9, Len: 1},
			), AmbigNone, false), true
		}
	}

	// ISO ordinal date: YYYY-DDD (8 chars)
	// Pattern: 4 digits, '-', 3 digits
	if n == 8 && s[4] == '-' && isDigit(s[0]) && isDigit(s[5]) && isDigit(s[6]) && isDigit(s[7]) {
		// Verify it's not a regular date (month would be 01-12, not 1xx-3xx).
		// The key: ordinal day is 3 digits (001-366).
		d0 := s[5] - '0'
		d1 := s[6] - '0'
		d2 := s[7] - '0'
		if d0 <= 9 && d1 <= 9 && d2 <= 9 {
			val := int(d0)*100 + int(d1)*10 + int(d2)
			if val >= 1 && val <= 366 {
				var buf [maxDetectFields]compile.Field
				return newResult("ISO_ORDINAL", "", append(buf[:0],
					compile.Field{Kind: compile.FYear4, Offset: 0, Len: 4},
					lit(s, 4),
					compile.Field{Kind: compile.FOrdinalDay, Offset: 5, Len: 3},
				), AmbigNone, false), true
			}
		}
	}

	return Result{}, false
}

// indexFoldASCII returns the byte index of the first occurrence of sub in s,
// folding ASCII case, or -1 if sub is not present.
//
// It exists because an index taken from a strings.ToLower copy is not valid in
// the original. Lowering can change a string's byte length: every byte of
// invalid UTF-8 lowers to a three-byte U+FFFD, and a few real runes grow too
// (U+0130 becomes two runes). trimAtSuffix used to search the lowered copy and
// slice the original with the result, which panicked with a slice bounds error
// on "dEC0000A\xbe\xc2\xd0 At 0" and on any other input carrying an invalid
// byte before an " at ". Both needles here are ASCII, so folding ASCII on the
// input is exactly equivalent to the search it replaces, minus the copy.
func indexFoldASCII(s, sub string) int {
	if len(sub) == 0 {
		return 0
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if equalsFoldASCII(s[i:i+len(sub)], sub) {
			return i
		}
	}
	return -1
}

// trimAtSuffix strips the " at ..." portion from a string when it would
// cause a bare time number to be misidentified as a year.
// e.g. "25th at 5pm" → "25th" (no year present, trim to avoid confusion).
// But "17, 2012 at 10:09am" → unchanged (4-digit year present, keep for parsing).
func trimAtSuffix(s string) string {
	atIdx := indexFoldASCII(s, " at ")
	if atIdx < 0 {
		return s
	}
	textBeforeAt := s[:atIdx]
	numsBeforeAt, _ := countNumbers(textBeforeAt)
	if numsBeforeAt <= 1 || !hasFourDigitYear(textBeforeAt) {
		return textBeforeAt
	}
	return s
}

// detectTextualMonth handles formats with named months:
// "March 15, 2024", "15 Mar 2024", "Mar 15, 2024",
// "Fri, 15 Mar 2024 10:30:00 +0000" (RFC 2822), "March 2024", "15 March".
func detectTextualMonth(s string, cfg Config) (Result, bool) {
	monthNum, monthStart, monthEnd := findMonthNameCI(s, cfg.Locales)
	if monthNum == 0 {
		return Result{}, false
	}

	// If the post-month text contains " at " without a 4-digit year,
	// this is a NL expression like "december 25th at 5pm" — bail so the
	// NL parser handles it.
	after := strings.TrimSpace(s[monthEnd:])
	if indexFoldASCII(after, " at ") >= 0 && !hasFourDigitYear(after) {
		return Result{}, false
	}

	// Classify the surrounding structure to determine the format name.
	name := classifyTextualPattern(s, monthStart, monthEnd)
	if name == "" {
		return Result{}, false
	}

	// Build field list from actual byte positions.
	var buf [maxDetectFields]compile.Field
	fields := coverGaps(buildTextualFields(buf[:0], s, monthNum, monthStart, monthEnd), s)
	prone, guess := textualDayIsAGuess(s, fields)
	ambig := AmbigNone
	if guess {
		ambig = AmbigDayOrYear
	}
	return newResult(name, "", fields, ambig, prone), true
}

// textualDayIsAGuess reports whether the number this format read as a day could
// equally have been read as a two-digit year.
//
// A bare month and number is classified by value: over 31 it is a year, at or
// under 31 it is a day. So "MAY70" is May 1970 and "MAY10" is the tenth of May,
// and nothing said the second was a choice. A caller who cached the layout from
// the first row of a column then got 2010-05-01 out of "MAY10" while Parse gave
// 2026-05-10, with Ambiguous false on both.
//
// The width of the number is what separates a guess from a certainty:
//
//	"March 5"      one digit, and no year is written with one    day
//	"March 15"     could be the fifteenth, could be 2015         GUESS
//	"March 32"     over 31, so not a day                         year
//	"March 2015"   four digits                                   year
//
// Two digits is the whole of the ambiguous set, because NormalizeTwoDigitYear
// maps every two-digit value to some year, so "could be a year" excludes
// nothing that "could be a day" admits.
//
// An ordinal suffix takes the number back out of that set, which is why the
// input is a parameter and not just its fields. "March 15th" is the fifteenth
// and nothing else: no year is written "15th", so there is no second reading to
// choose between and the alternative offered for one would have to be invented.
// It is not the input that is exempt but the shape, because the suffix is part
// of what the program accepts, and every input the layout takes carries it too.
//
// It returns prone, meaning the day-or-year question arises for this shape at
// all, and guess, meaning this input actually needed it answered.
func textualDayIsAGuess(s string, fields []compile.Field) (prone, guess bool) {
	for i := range fields {
		switch fields[i].Kind {
		case compile.FYear2:
			// A year is written out, so the other number is not one, and this
			// input needed no guess. The format still did: buildTextualFields
			// calls a bare number over 31 a year and one at or under 31 a day,
			// and both emit a two-byte field at the same offset. The programs
			// are the same shape, so a layout built under one reading accepts
			// input that wanted the other and answers with the wrong instant.
			//
			// "MAY70" builds a year field and reads "MAY10" as 2010-05-01,
			// where detection reads the tenth of May. "March 32" reads
			// "March 31" as 2031-03-01 against the thirty-first of March. Both
			// used to come back with Ambiguous false and no error, because
			// prone was false here and Parser reuses a layout it is not told to
			// re-detect.
			return true, false
		case compile.FYear4:
			// Four digits cannot be a day, so nothing about this shape is
			// decided by value and no later row can flip it.
			return false, false
		case compile.FDay1or2, compile.FDay2:
			if ordinalSuffixLen(s, int(fields[i].Offset+fields[i].Len)) > 0 {
				continue
			}
			prone = true
			if fields[i].Kind == compile.FDay2 && fields[i].Len == 2 {
				guess = true
			}
		}
	}
	return prone, guess
}

// classifyTextualPattern determines the format name based on how numbers
// are arranged around the month name. This decides whether the format is
// "MONTH_DAY_YEAR", "DAY_MONTH_YEAR", "MONTH_YEAR", "MONTH_DAY", or "DAY_MONTH".
//
// Returns "" if the surrounding structure doesn't match any known pattern.
func classifyTextualPattern(s string, monthStart, monthEnd int) string {
	before := strings.TrimSpace(s[:monthStart])
	after := strings.TrimSpace(s[monthEnd:])

	nBefore, _ := countNumbers(before)
	afterStr := strings.TrimLeft(after, ", ")
	nAfter, firstAfter := countNumbers(trimAtSuffix(afterStr))

	switch {
	case nBefore == 0 && nAfter >= 2:
		// "March 15, 2024"
		return "MONTH_DAY_YEAR"

	case nBefore >= 1 && nAfter >= 1:
		// "15 Mar 2024" or "Fri, 15 Mar 2024"
		return "DAY_MONTH_YEAR"

	case nBefore == 0 && nAfter == 1:
		// "March 2024" (value > 31 → year) or "March 15" (value ≤ 31 → day)
		if firstAfter > 31 {
			return "MONTH_YEAR"
		}
		return "MONTH_DAY"

	case nBefore == 1 && nAfter == 0:
		// "15 March"
		return "DAY_MONTH"

	default:
		return ""
	}
}

// buildTextualFields constructs compile.Fields for a textual-month date string
// by scanning the actual byte positions.
func buildTextualFields(dst []compile.Field, s string, monthNum int, monthStart, monthEnd int) []compile.Field {
	// Use stack-allocated fixed-size arrays to avoid heap allocations.
	fields := dst
	var tbuf [maxTimeFields]compile.Field

	// Scan for numeric tokens in the string, skipping the month name region.
	nums := make([]numToken, 0, 6)

	i := 0
	for i < len(s) {
		if i >= monthStart && i < monthEnd {
			i = monthEnd
			continue
		}
		if s[i] >= '0' && s[i] <= '9' {
			start := i
			val := 0
			for i < len(s) && s[i] >= '0' && s[i] <= '9' {
				val = val*10 + int(s[i]-'0')
				i++
			}
			nums = append(nums, numToken{val, start, i})
		} else {
			i++
		}
	}

	// Add the month name field.
	fields = append(fields, compile.Field{
		Kind:   compile.FMonthName,
		Offset: int32(monthStart),
		Len:    int32(monthEnd - monthStart),
		Aux:    uint16(monthNum),
	})

	// Identify day and year from the numeric tokens.
	switch len(nums) {
	case 2:
		// Two numbers + month name. Smaller is day, larger is year (usually).
		n0, n1 := nums[0], nums[1]
		if n0.value > 31 || (n1.value <= 31 && n0.start > monthEnd) {
			// n0 is year, n1 is day — unlikely but handle it
			fields = append(fields, yearField(n0))
			fields = appendDay(fields, s, n1)
		} else if n1.value > 31 {
			fields = appendDay(fields, s, n0)
			fields = append(fields, yearField(n1))
		} else {
			// Both small, and the first number is the day wherever it sits.
			//
			// This was an if/else on n0.start < monthStart with two identical
			// arms, under a comment promising "or vice versa". Both arms are
			// reachable: "15 MAY 20" takes the first and "MAY15 20" the second,
			// the difference being whether a number precedes the name or abuts
			// it. Neither wants the year first, because the branch above this
			// one has already claimed every case where n0 sits past the end of
			// the month name, which is the only way a year leads two small
			// numbers here. So the position does not decide anything and the
			// vice versa never happens.
			fields = appendDay(fields, s, n0)
			fields = append(fields, yearField(n1))
		}

		// Check for time component after the date.
		afterLast := nums[len(nums)-1].end
		timeFields := parseTimeComponent(tbuf[:0], s, afterLast)
		fields = append(fields, timeFields...)

	case 1:
		n := nums[0]
		if n.value > 31 {
			fields = append(fields, yearField(n))
		} else {
			fields = appendDay(fields, s, n)
		}
		// Check for time component after the number.
		timeFields := parseTimeComponent(tbuf[:0], s, n.end)
		fields = append(fields, timeFields...)

	case 0:
		// Month name only — unusual but valid.

	default:
		fields = appendMultiNumFields(tbuf[:0], s, nums, fields)
	}

	return fields
}

// appendMultiNumFields handles the 3+ numeric tokens case in buildTextualFields.
// Patterns: "day time [year]" (e.g., "Mar 15 10:30:00 2024") or "day year time".
func appendMultiNumFields(tbuf []compile.Field, s string, nums []numToken, fields []compile.Field) []compile.Field {
	n0, n1 := nums[0], nums[1]

	// Detect if nums[1:] form a time pattern (HH:MM or HH:MM:SS).
	isTimeAtN1 := n1.end < len(s) && s[n1.end] == ':' && len(nums) >= 3

	if isTimeAtN1 && n0.value <= 31 && n1.value <= 23 {
		// Pattern: "day time [year]", e.g. "Mar 15 10:30:00 2024"
		fields = appendDay(fields, s, n0)
		timeFields := parseTimeComponent(tbuf[:0], s, n0.end)
		fields = append(fields, timeFields...)
		if yr := findTrailingYear(nums, timeFields, n0.end); yr != nil {
			fields = append(fields, yearField(*yr))
		}
		return fields
	}

	// Pattern: "day year time" or "year day time".
	if n1.value > 31 {
		fields = appendDay(fields, s, n0)
		fields = append(fields, yearField(n1))
	} else if n0.value > 31 {
		fields = append(fields, yearField(n0))
		fields = appendDay(fields, s, n1)
	} else {
		fields = appendDay(fields, s, n0)
		fields = append(fields, yearField(n1))
	}
	timeFields := parseTimeComponent(tbuf[:0], s, nums[1].end)
	fields = append(fields, timeFields...)
	return fields
}

// findTrailingYear scans for a 4-digit year among nums that is not already
// claimed by timeFields (i.e., not at the same offset as a time field).
func findTrailingYear(nums []numToken, timeFields []compile.Field, afterOffset int) *numToken {
	for i := range nums {
		num := &nums[i]
		if num.start <= afterOffset || num.value < 1000 || num.value > 9999 {
			continue
		}
		isTimeField := false
		for _, tf := range timeFields {
			if int(tf.Offset) == num.start {
				isTimeField = true
				break
			}
		}
		if !isTimeField {
			return num
		}
	}
	return nil
}

func yearField(n numToken) compile.Field {
	kind := compile.FYear4
	if n.end-n.start <= 2 {
		kind = compile.FYear2
	}
	return compile.Field{Kind: kind, Offset: int32(n.start), Len: int32(n.end - n.start)}
}

// ordinalSuffixLen returns 2 when an English ordinal suffix follows a day
// number at position at, and 0 otherwise. The suffix carries no value, but it
// is part of the input and a program has to account for every byte it accepts.
func ordinalSuffixLen(s string, at int) int {
	if at+2 > len(s) {
		return 0
	}
	c0 := s[at] | 0x20
	c1 := s[at+1] | 0x20
	if c1 != 't' && c1 != 'd' && c1 != 'h' {
		return 0
	}
	switch {
	case c0 == 's' && c1 == 't', // 1st
		c0 == 'n' && c1 == 'd', // 2nd
		c0 == 'r' && c1 == 'd', // 3rd
		c0 == 't' && c1 == 'h': // 4th
		// Only when the suffix is not the start of a longer word.
		return boundaryAfter(s, at+2)
	}
	return 0
}

func boundaryAfter(s string, at int) int {
	if at == len(s) || !isLetter(s[at]) {
		return 2
	}
	return 0
}

// appendDay appends the day field for n, plus a skip covering any ordinal
// suffix that follows it.
func appendDay(fields []compile.Field, s string, n numToken) []compile.Field {
	fields = append(fields, dayField(n))
	if w := ordinalSuffixLen(s, n.end); w > 0 {
		fields = append(fields, compile.Field{Kind: compile.FSkip, Offset: int32(n.end), Len: int32(w)})
	}
	return fields
}

func dayField(n numToken) compile.Field {
	kind := compile.FDay2
	if n.end-n.start == 1 {
		kind = compile.FDay1or2
	}
	return compile.Field{Kind: kind, Offset: int32(n.start), Len: int32(n.end - n.start)}
}

// parseTimeComponent looks for HH:MM or HH:MM:SS or HH:MM:SS.fff patterns
// starting from offset `from` in the string, and returns nil where there is no
// time to describe.
//
// The slice is built after the HH:MM test and not before it. Most of what
// reaches here carries no time at all: "March 15, 2024" is the benchmarked
// textual input and every textual detection runs this once, so the eight fields
// it used to reserve up front were 256 bytes allocated, never written, and
// returned empty. Every caller either checks the length or appends the result,
// and appending a nil slice appends nothing.
func parseTimeComponent(dst []compile.Field, s string, from int) []compile.Field {
	// Skip whitespace, punctuation, and colons to find time start.
	// Colons appear as time separators in CLF: "2024:10:30:00".
	i := from
	for i < len(s) && (s[i] == ' ' || s[i] == ',' || s[i] == '\t' || s[i] == ':') {
		i++
	}

	// Look for "at " prefix.
	if i+3 <= len(s) && equalsFoldASCII(s[i:i+3], "at ") {
		i += 3
	}

	// Find HH:MM pattern.
	if i+5 <= len(s) && isDigit(s[i]) && isDigit(s[i+1]) && s[i+2] == ':' && isDigit(s[i+3]) && isDigit(s[i+4]) {
		fields := dst
		fields = append(fields, compile.Field{Kind: compile.FHour24, Offset: int32(i), Len: 2})
		fields = append(fields, compile.Field{Kind: compile.FMinute2, Offset: int32(i + 3), Len: 2})

		j := i + 5
		// Check for :SS
		if j+3 <= len(s) && s[j] == ':' && isDigit(s[j+1]) && isDigit(s[j+2]) {
			fields = append(fields, compile.Field{Kind: compile.FSecond2, Offset: int32(j + 1), Len: 2})
			j += 3

			// Check for .fractional
			//
			// Bounded at compile.MaxFracDigits, which it was not. See the same bound in
			// detectGoTimeString and detectISO8601Frac: this is the third producer
			// of the field and the second of the two that were missing it, which
			// is why "March 15, 2024 10:30:00." followed by 25 nines came back as
			// 2074-08-13. A run over the bound leaves this returning no time
			// fields at all, so the caller describes the date and the coverage
			// check refuses the input rather than reading part of it.
			if j+1 < len(s) && s[j] == '.' {
				fracStart := j + 1
				fracEnd := fracStart
				for fracEnd < len(s) && isDigit(s[fracEnd]) {
					fracEnd++
				}
				if fracEnd-fracStart > compile.MaxFracDigits {
					return nil
				}
				if fracEnd > fracStart {
					fields = append(fields, compile.Field{Kind: compile.FFracSec, Offset: int32(fracStart), Len: int32(fracEnd - fracStart)})
					j = fracEnd
				}
			}
		}

		// Skip trailing whitespace before AM/PM or timezone suffix.
		for j < len(s) && s[j] == ' ' {
			j++
		}
		return appendTimeSuffix(s, j, fields)
	}

	return nil
}

type numToken struct {
	value int
	start int
	end   int
}

// appendTimeSuffix checks for AM/PM or timezone suffix at position j in s,
// and appends the appropriate field(s) to fields. If AM/PM is found, the first
// field's Kind is changed from FHour24 to FHour12.
func appendTimeSuffix(s string, j int, fields []compile.Field) []compile.Field {
	if j >= len(s) {
		return fields
	}

	// Check AM/PM first — "AM"/"PM" would be misidentified as timezone abbreviations.
	if j+2 <= len(s) {
		c0 := s[j] | 0x20
		c1 := s[j+1] | 0x20
		if (c0 == 'a' || c0 == 'p') && c1 == 'm' && (j+2 == len(s) || !isLetter(s[j+2])) {
			if len(fields) > 0 && fields[0].Kind == compile.FHour24 {
				fields[0].Kind = compile.FHour12
			}
			return append(fields, compile.Field{Kind: compile.FAMPM, Offset: int32(j), Len: 2})
		}
	}

	// Timezone: Z, ±HHMM/±HH:MM, or abbreviation.
	if s[j] == 'Z' && (j+1 == len(s) || !isLetter(s[j+1])) {
		return append(fields, compile.Field{Kind: compile.FTZZ, Offset: int32(j), Len: 1})
	}
	if s[j] == '+' || s[j] == '-' {
		remaining := len(s) - j
		if remaining >= 5 {
			tzLen := 5
			if remaining >= 6 && s[j+3] == ':' {
				tzLen = 6
			}
			return append(fields, compile.Field{Kind: compile.FTZOffset, Offset: int32(j), Len: int32(tzLen)})
		}
	}
	if isLetter(s[j]) {
		tzEnd := j
		for tzEnd < len(s) && isLetter(s[tzEnd]) {
			tzEnd++
		}
		// "GMT+0100" (JS Date.toString) is the name and the offset together,
		// and the offset is what decides the instant. Reading only the name
		// gave UTC for a value an hour ahead of it.
		if rem := len(s) - tzEnd; rem >= 5 && (s[tzEnd] == '+' || s[tzEnd] == '-') {
			tzLen := 5
			if rem >= 6 && s[tzEnd+3] == ':' {
				tzLen = 6
			}
			fields = append(fields, compile.Field{Kind: compile.FSkip, Offset: int32(j), Len: int32(tzEnd - j)})
			return append(fields, compile.Field{Kind: compile.FTZOffset, Offset: int32(tzEnd), Len: int32(tzLen)})
		}
		return append(fields, compile.Field{Kind: compile.FTZName, Offset: int32(j), Len: int32(tzEnd - j)})
	}
	return fields
}

// monthEntry pairs a lowercase month name with its month number. It is what the
// English table is written as; init turns it into the same prepared form the
// locale tables use, so there is one lookup and not two.
//
// The lookup and the length are computed once in init rather than per call.
// These 24 spellings are tried on every call to findMonthNameCI, before any
// locale, so asking allWordChars there cost 12% on BenchmarkParse_Miss_Short: a
// three-byte cell is dismissed by the length guard almost immediately, and a
// per-spelling scan is then most of what is left.
//
// It is not a formality. 140 of the registered locale month names carry a byte
// that is not a word character, and they are not all trailing dots: the CJK
// locales spell months with digits.
type monthEntry struct {
	name string
	num  int
}

// defaultMonths is defaultMonthNames prepared, in the same order.
var defaultMonths []monthSpelling

func init() {
	defaultMonths = make([]monthSpelling, len(defaultMonthNames))
	for i, e := range defaultMonthNames {
		defaultMonths[i] = newMonthSpelling(e.name, e.num)
	}
}

// defaultMonthNames is sorted longest-first for greedy matching.
// Using a slice instead of a map gives deterministic iteration order
// and eliminates hash-table traversal overhead on the hot path.
var defaultMonthNames = []monthEntry{
	{name: "september", num: 9},
	{name: "february", num: 2},
	{name: "november", num: 11},
	{name: "december", num: 12},
	{name: "january", num: 1},
	{name: "october", num: 10},
	{name: "august", num: 8},
	{name: "march", num: 3},
	{name: "april", num: 4},
	{name: "june", num: 6},
	{name: "july", num: 7},
	{name: "sept", num: 9},
	{name: "jan", num: 1},
	{name: "feb", num: 2},
	{name: "mar", num: 3},
	{name: "apr", num: 4},
	{name: "may", num: 5},
	{name: "jun", num: 6},
	{name: "jul", num: 7},
	{name: "aug", num: 8},
	{name: "sep", num: 9},
	{name: "oct", num: 10},
	{name: "nov", num: 11},
	{name: "dec", num: 12},
}

// spellingLookup says how a month spelling has to be looked for. It is a
// property of the spelling, so it is computed when the locale's table is built
// and not on the way past it.
type spellingLookup uint8

const (
	// lookupWordList: every byte is a word character, so the spelling can only
	// match a whole word of its own length and the word list answers it.
	lookupWordList spellingLookup = iota
	// lookupDotted: word characters and one trailing dot, which the word list
	// answers too. See findSpelling.
	lookupDotted
	// lookupScan: anything else, which means the input is scanned. 60 of the
	// 584 spellings across the twenty locales land here, and they are all
	// Japanese, Korean or Chinese: those write a month as a digit and a
	// character, and a digit is not a word character.
	lookupScan
)

// monthSpelling is one spelling from a month table, prepared.
//
// lenBit is the length prefilter. A spelling answered from the word list can
// only match a whole word of one exact length, so it has nothing to do in an
// input holding no word of that length, and one AND against wordMatcher.lenMask
// says so before the lookup is called at all. A spelling that scans is not
// bound to a word length, so its lenBit is every bit and the filter never
// dismisses it: see wordMatcher.lenMask for why that is safe when the input has
// no words in it.
type monthSpelling struct {
	name   string
	num    int
	how    spellingLookup
	lenBit uint64
}

// newMonthSpelling prepares one spelling: which lookup answers it, and the
// length that lookup matches on.
func newMonthSpelling(name string, num int) monthSpelling {
	sp := monthSpelling{name: name, num: num, how: classifySpelling(name)}
	switch sp.how {
	case lookupScan:
		sp.lenBit = ^uint64(0)
	case lookupDotted:
		sp.lenBit = lenBitFor(len(name) - 1)
	default:
		sp.lenBit = lenBitFor(len(name))
	}
	return sp
}

// lenBitFor maps a word length to its bit. Lengths at or above 63 share the top
// bit, so a spelling that long is only dismissed when the input holds no word
// that long either. No month name comes close: the longest across the twenty
// locales is 21 bytes.
func lenBitFor(n int) uint64 {
	if n < 0 {
		return 0
	}
	if n >= 63 {
		n = 63
	}
	return 1 << uint(n)
}

// localeMonths holds a locale's month spellings in the order findMonthNameCI
// tries them, which is wide, abbreviated, then the abbreviation without its
// trailing dot, month by month.
//
// The order is load bearing and it is not input order: see findMonthNameCI.
type localeMonths struct {
	spellings []monthSpelling
}

// localeMonthCache caches the prepared table per locale Data pointer, built
// once on first use. sync.Map handles the concurrent access, and what it holds
// never changes after it is built.
var localeMonthCache sync.Map // map[*locale.Data]*localeMonths

func getLocaleMonths(loc *locale.Data) *localeMonths {
	if v, ok := localeMonthCache.Load(loc); ok {
		return v.(*localeMonths)
	}
	lm := buildLocaleMonths(loc)
	localeMonthCache.Store(loc, lm)
	return lm
}

func buildLocaleMonths(loc *locale.Data) *localeMonths {
	lm := &localeMonths{spellings: make([]monthSpelling, 0, 36)}
	add := func(name string, num int) {
		lm.spellings = append(lm.spellings, newMonthSpelling(name, num))
	}
	for i := range 12 {
		if name := loc.MonthsWide[i]; name != "" {
			add(name, i+1)
		}
		if name := loc.MonthsAbbr[i]; name != "" {
			add(name, i+1)
			if clean := strings.TrimRight(name, "."); clean != name {
				add(clean, i+1)
			}
		}
	}

	// A spelling is looked for anywhere in the string, so one that is a suffix
	// of another in the same locale is only ever reachable if it is tried
	// second. This list was built in month order, so "1월" came before "11월"
	// and Korean November read as January. A digit is what makes that reachable
	// here and not in English: IsWordChar counts letters and bytes over 0x7f,
	// so a digit is not a word character and the boundary check that stops
	// "mar" matching inside "march" does not stop "1월" matching inside "11월".
	// Every CJK locale spells its months with digits, so ja, ko and zh all had
	// months 11 and 12 wrong.
	//
	// Sorted longest-first only where such a pair exists, which is those three.
	// That is to keep the change of behaviour to the locales that need it, and
	// not for speed: sorting all twenty measured +5.47% on
	// BenchmarkParse_Locale_GermanMonth and +6.61% on Parse_Miss_Locales on a
	// development container, and both of those turned out to be noise. On the
	// machine benchmarks/baseline.env names, this narrowed version is
	// +0.78% geomean, German is 0.64% faster, and Parse_Miss_Locales is
	// unchanged, so the numbers that argued for narrowing did not survive being
	// measured properly. If somebody wants the simpler unconditional sort, the
	// measurement to make is that one on the pinned machine, not on a laptop.
	//
	// The check below is quadratic over at most 36 short strings, once per
	// locale, behind the same cache as the rest of this.
	if hasSuffixPair(lm.spellings) {
		slices.SortStableFunc(lm.spellings, func(a, b monthSpelling) int {
			return len(b.name) - len(a.name)
		})
	}
	return lm
}

// hasSuffixPair reports whether any spelling is a proper suffix of another,
// which is the only arrangement where the order they are tried in changes
// which month a string reads as.
func hasSuffixPair(sp []monthSpelling) bool {
	for i := range sp {
		for j := range sp {
			if i == j || len(sp[i].name) >= len(sp[j].name) {
				continue
			}
			if strings.HasSuffix(sp[j].name, sp[i].name) {
				return true
			}
		}
	}
	return false
}

func classifySpelling(name string) spellingLookup {
	if allWordChars(name) {
		return lookupWordList
	}
	if len(name) >= 2 && name[len(name)-1] == '.' && allWordChars(name[:len(name)-1]) {
		return lookupDotted
	}
	return lookupScan
}

// wordSpan is one maximal run of word characters in the input.
type wordSpan struct{ start, end int32 }

// monthWordCap bounds the word list a matcher keeps on the stack. A date has
// well under a dozen words; the cap is what stops a long input turning a stack
// array into a reason to allocate.
const monthWordCap = 48

// wordMatcher finds a whole-word occurrence of a name in s.
//
// A month spelling can only ever match a maximal run of word characters. Every
// byte of a spelling is a word character, and matchWordCI requires a non-word
// character on each side, so a match is exactly one whole word of the same byte
// length. Listing the words once and then asking each spelling only about words
// of its own length is therefore the same question as scanning the whole input
// once per spelling, and it is why this type exists: findMonthNameCI tries 24
// English spellings before it reaches any locale, and each one walked every
// position in the input. That was 47% of a failed parse.
//
// words is nil for an input holding more words than the cap, and the lookup
// falls back to scanning. Correct either way, and it keeps the cost linear in
// the input length rather than putting a slice on the heap for a path that is
// about to fail anyway.
//
// lenMask has bit k set when the input holds a word of k bytes, saturating at
// 63, and bit 0 set always. Bit 0 is the one no word can claim, so a spelling
// that must not be filtered carries every bit and meets it there.
//
// That sentinel is for the input with no word characters at all, "2024-03-15"
// among them, whose mask would otherwise be zero and dismiss every spelling
// including the ones that scan rather than match a word. Nothing in the locale
// data notices today, because every scanning spelling holds a character that is
// a word character, "1月" its own 月, so an input with no words cannot contain
// one anyway. The sentinel is what keeps that from being load bearing.
//
// The mask is a filter and never an answer. A set bit means a word of that
// length exists, not that it is the spelling being looked for.
type wordMatcher struct {
	s       string
	words   []wordSpan
	lenMask uint64
}

func newWordMatcher(s string, buf []wordSpan) wordMatcher {
	n := 0
	mask := uint64(1)
	for i := 0; i < len(s); {
		if !isWordChar(s[i]) {
			i++
			continue
		}
		start := i
		for i < len(s) && isWordChar(s[i]) {
			i++
		}
		if n == len(buf) {
			// Over the cap: no word list, so every spelling scans, and the
			// mask has to let all of them through. A zero mask here dismissed
			// every spelling instead, and an input of fifty words with a month
			// name in it stopped being a date. TestWordMatcherAgreesWithScanning
			// found that on the first run.
			return wordMatcher{s: s, lenMask: ^uint64(0)}
		}
		buf[n] = wordSpan{int32(start), int32(i)}
		mask |= lenBitFor(i - start)
		n++
	}
	return wordMatcher{s: s, words: buf[:n], lenMask: mask}
}

// allWordChars reports whether every byte of word is a word character, which is
// the condition that makes a whole-word match and a maximal word run the same
// thing.
//
// Not every spelling qualifies. A locale abbreviation can carry a trailing dot,
// and "sept." matches "sept. 1, 2020" at offset 0 where the maximal run is
// "sept" and stops at the dot, so the word list would miss it. Those spellings
// scan. TestWordMatcherAgreesWithScanning is what found this, on the one input
// in the corpus that has a dotted abbreviation in it.
func allWordChars(word string) bool {
	for i := 0; i < len(word); i++ {
		if !isWordChar(word[i]) {
			return false
		}
	}
	return true
}

// findSpelling returns the first whole-word occurrence of a prepared spelling
// in input order, which is the answer matchWordCI gives for the same spelling.
//
// It is one function and not a dispatcher over three, because it is called once
// per spelling and neither it nor anything it called would inline: two calls per
// spelling and 248 calls per parse put 9.4% of a failed parse in function
// prologues, which was more than either the loop or the comparisons underneath
// them. The receiver is a pointer for the same reason, since wordMatcher is 48
// bytes and was being copied at every one of those calls.
//
// A spelling that is word characters followed by one dot is answered from the
// word list too. Such a name can only match where its dotless part is a whole
// word: every byte before the dot is a word character, and a dot is not, so the
// word run a match starts at ends exactly where the dot begins. What the list
// does not hold is the dot or what follows it, so both are checked here.
//
// Two ways to get that wrong, both found by testing it against the scan rather
// than by reading it. The dot is not the end of the match, so the byte after it
// still has to be a boundary: "sept." does not occur in "x sept.y", and a
// version that stopped at the dot said it did. And a failed dot check is not a
// failed search: "sept." occurs in "sept sept." at offset 5, so a version that
// gave up on the first word spelled "sept" missed it.
func (m *wordMatcher) findSpelling(sp *monthSpelling) (int, int, bool) {
	if m.words == nil || sp.how == lookupScan {
		return matchWordCI(m.s, sp.name)
	}
	// The length guard is what makes a short input cheap, and it is the one
	// matchWordCI opens with for the same reason. A spelling wider than the
	// whole input cannot occur in it, and against "N/A" that dismisses 12 of
	// the 24 English spellings before anything looks at a byte.
	if len(sp.name) == 0 || len(sp.name) > len(m.s) {
		return 0, 0, false
	}
	wlen := len(sp.name)
	dotted := sp.how == lookupDotted
	if dotted {
		wlen--
	}
	for _, w := range m.words {
		if int(w.end-w.start) != wlen {
			continue
		}
		end := int(w.end)
		if dotted {
			if end >= len(m.s) || m.s[end] != '.' {
				continue
			}
			if end+1 < len(m.s) && isWordChar(m.s[end+1]) {
				continue
			}
			end++
		}
		if equalsFoldASCII(m.s[w.start:w.end], sp.name[:wlen]) {
			return int(w.start), end, true
		}
	}
	return 0, 0, false
}

// findMonthNameCI finds the first month name in the string using
// case-insensitive matching directly on the input (no lowered copy).
// Returns (month number 1-12, start index, end index) or (0, 0, 0) if not found.
//
// The spelling order is load bearing and it is not the input order. Names are
// tried longest first and each is looked for anywhere in the string, so a longer
// name later beats a shorter name earlier: "mar 1 september 2024" is the first
// of September, because "september" is tried before "mar". Restructuring this
// into one pass over the input reverses that and answers March. The word list
// changes how each spelling is looked for, never the order they are tried in.
//
// The lenMask test in both loops is the same kind of thing: it decides whether a
// spelling can match, never which one wins, so it dismisses without disturbing
// the order. It sits here rather than inside findSpelling because a spelling it
// dismisses should cost no call either, and the call was the larger half.
func findMonthNameCI(s string, locales []*locale.Data) (int, int, int) {
	var buf [monthWordCap]wordSpan
	m := newWordMatcher(s, buf[:])

	// Search English names (case-insensitive), longest first.
	for i := range defaultMonths {
		sp := &defaultMonths[i]
		if sp.lenBit&m.lenMask == 0 {
			continue
		}
		if idx, end, ok := m.findSpelling(sp); ok {
			return sp.num, idx, end
		}
	}
	// Search locale-specific names. The spelling list is the same one the loop
	// here used to build per call, wide then abbreviated then the abbreviation
	// without its dot, in the same order, prepared once per locale.
	for _, loc := range locales {
		lm := getLocaleMonths(loc)
		for i := range lm.spellings {
			sp := &lm.spellings[i]
			if sp.lenBit&m.lenMask == 0 {
				continue
			}
			if idx, end, ok := m.findSpelling(sp); ok {
				return sp.num, idx, end
			}
		}
	}
	return 0, 0, 0
}

// matchWordCI finds `word` as a whole word in `s`, case-insensitive.
// Returns (start, end, true) on match, or (0, 0, false).
func matchWordCI(s, word string) (int, int, bool) {
	wlen := len(word)
	if wlen == 0 || wlen > len(s) {
		return 0, 0, false
	}
	for i := 0; i <= len(s)-wlen; i++ {
		if !equalsFoldASCII(s[i:i+wlen], word) {
			continue
		}
		// Check word boundaries.
		if (i == 0 || !isWordChar(s[i-1])) &&
			(i+wlen == len(s) || !isWordChar(s[i+wlen])) {
			return i, i + wlen, true
		}
	}
	return 0, 0, false
}

// equalsFoldASCII compares two strings case-insensitively for ASCII.
// For non-ASCII bytes, compares exact. Both strings must be same length.
func equalsFoldASCII(a, b string) bool {
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca == cb {
			continue
		}
		// ASCII case fold.
		if ca >= 'A' && ca <= 'Z' {
			ca += 0x20
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 0x20
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// isWordChar returns true for characters that can be part of a word
// (letters, including non-ASCII bytes which may be part of UTF-8 sequences).
//
// One definition, in compile, because the executor verifies as a whole word
// what this package finds as one and the two disagreeing is C24. See
// compile.IsWordChar.
func isWordChar(c byte) bool {
	return compile.IsWordChar(c)
}

func isLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// extractNumbers extracts all decimal numbers from a string.
// countNumbers reports how many runs of digits s holds and the value of the
// first, which is all any caller ever wanted.
//
// It returned a []int, built with append from nil, and every caller then read
// its length and at most its first element. classifyTextualPattern calls it
// twice, so a textual date paid two heap allocations to count to three. The
// values past the first were never read by anything.
func countNumbers(s string) (n, first int) {
	i := 0
	for i < len(s) {
		if s[i] >= '0' && s[i] <= '9' {
			val := 0
			for i < len(s) && s[i] >= '0' && s[i] <= '9' {
				val = val*10 + int(s[i]-'0')
				i++
			}
			if n == 0 {
				first = val
			}
			n++
		} else {
			i++
		}
	}
	return n, first
}

// hasFourDigitYear checks if a string contains a 4-digit number (likely a year).
func hasFourDigitYear(s string) bool {
	i := 0
	for i < len(s) {
		if s[i] >= '0' && s[i] <= '9' {
			start := i
			for i < len(s) && s[i] >= '0' && s[i] <= '9' {
				i++
			}
			if i-start == 4 {
				return true
			}
		} else {
			i++
		}
	}
	return false
}

func findSep(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '/' || s[i] == '-' || s[i] == '.' {
			return i
		}
	}
	return -1
}

// splitOnSep appends the sep-separated parts of s to dst and returns it.
//
// dst is caller-owned so the parts can live in the caller's frame. It built its
// own slice with append, which grew from nil and cost two allocations on every
// numeric date that reached it, for a result never longer than the four parts
// maxDateParts allows and discarded before the function returned. A caller that
// hands in a short array gets the same answer with no heap.
//
// Nothing is dropped when there are more parts than dst holds: append grows it,
// on the heap, exactly as before. The array is a fast path and not a limit.
func splitOnSep(dst []string, s string, sep byte) []string {
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			dst = append(dst, s[start:i])
			start = i + 1
		}
	}
	return append(dst, s[start:])
}

// maxDateParts is the array a caller of splitOnSep puts on its stack. Three is
// what a date has; the fourth is headroom so a malformed input with an extra
// separator does not reach the heap either.
const maxDateParts = 4

func parseSmallInt(s string) int {
	if len(s) == 0 || len(s) > 4 {
		return -1
	}
	val := 0
	for i := 0; i < len(s); i++ {
		d := s[i] - '0'
		if d > 9 {
			return -1
		}
		val = val*10 + int(d)
	}
	return val
}
