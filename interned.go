package dateparsa

import (
	"time"

	"github.com/kmoneil/dateparsa/internal/compile"
	"github.com/kmoneil/dateparsa/internal/detect"
)

// internedLayouts holds one shared *Layout per prebuilt trie format, built at
// init and keyed on the *compile.FormatDef a detection carries.
//
// The pointer is the key because the pointer is the guarantee. A trie hit whose
// input spells the entry's literals canonically hands back the entry's own def,
// the same address every time; anything else builds a fresh one, which is every
// respelled separator and every fallback detector. So identity answers "is this
// the format I prebuilt" exactly, with nothing to keep in step.
//
// A map rather than an array indexed by a small id. The id was tried: it needs
// a field on detect.Result and one on the trie entry, and adding them cost
// Parse_TextualMonth +8.86%, Parse_TimeAMPM +6.41% and Parse_UnixTimestamp
// +5.48%, all at p<0.05, against a root package whose source had not changed.
// Result stayed 16 bytes and formatEntry 112, moving the field to the end of
// the struct did not help, and no inline decision moved, so the cost was the
// binary's layout rather than anything the source says. A map lookup is a few
// nanoseconds against the ninety this saves, and it leaves detect's structures
// byte for byte what they were.
//
// It exists because the Program a trie hit compiles to is a pure function of
// the format, the timezone and the base year, and detection was recomputing it
// on every call. Measured on linux/arm64 before this existed, on the
// ISO8601_DATETIME_Z def:
//
//	Compile(def, tz)         52.08 ns/op   ran on every Parse
//	  planFast alone         26.71 ns/op
//	&Program on the heap     37.85 ns/op   the &Layout{} Detect built
//
// which was 45% of a 199 ns Detect_Only between them.
//
// Sharing one Layout across every caller in the process is sound only because
// Layout is immutable: it has no exported fields, nothing writes to one after
// construction, and Layout.Parse reads. That is already documented as a
// promise and already relied on by Parser's atomic.Pointer cache. This makes it
// load-bearing in a second way, which TestInternedLayoutHasNoWritableSurface
// states outright rather than leaving to the type.
//
// An entry is nil where the format cannot be prebuilt, which is a format
// carrying no year field: its BaseYear comes from the clock or from the
// caller's base time, so the program differs per call and there is nothing to
// share. Those compile per call exactly as they did before.
//
// The cost is a fixed 25 Layouts, 208 bytes each, live for the life of the
// process whether or not anything parses. That is about 5 KB of heap and about
// 1.5 microseconds of init, against a binary-size budget of 10 MB and 3 MB
// used. It buys back one allocation per parse for the formats a column is
// actually made of.
var internedLayouts = buildInternedLayouts()

func buildInternedLayouts() map[*compile.FormatDef]*Layout {
	defs := detect.PrebuiltDefs()
	out := make(map[*compile.FormatDef]*Layout, len(defs))
	for _, def := range defs {
		program, needsBaseYear, err := compile.Compile(def, time.UTC)
		if err != nil || needsBaseYear {
			continue
		}
		out[def] = &Layout{
			program:  program,
			goLayout: def.GoLayout,
			label:    def.Name,

			// A detected layout describes a value, so it trims the next row's
			// padding. Every layout on this path came from detection, so this
			// is the same true that parseWithConfig and Detect write when they
			// build one themselves.
			trimsPadding: true,
		}
	}
	return out
}

// internedLayout returns the shared Layout for a detection result, or nil when
// this result is not one the table can answer.
//
// The gate is narrow on purpose, and each clause is a way the compiled program
// or the layout around it would differ from the prebuilt one:
//
//   - A Def that is not one of the prebuilt pointers is a def some detector
//     built for this input, so its fields are not the shared fields. The map
//     misses and says so.
//   - A timezone other than UTC compiles into Program.Tz, so the program is not
//     the prebuilt one.
//   - Ambig and AmbigProne travel onto the Layout, and the prebuilt one carries
//     false for both. Neither can be true of a canonical trie hit today, since
//     an ambiguous entry is resolved before that line is reached; the check is
//     what keeps that true if the path ever widens.
//
// The timezone test comes first because it is a compare against a register and
// the map lookup is not, and a caller who has configured a timezone pays only
// the compare.
//
// What the rest pay is one map miss, and it is not free: a format that reaches
// here and is not prebuilt does the lookup, finds nothing, and compiles as
// before. Parse_TextualMonth measures +4.47% (p=0.005) for it, against -47% to
// -60% on the six formats that do hit. There is no cheaper discriminator: the
// alternative was a small id on detect.Result naming the def, and carrying that
// id cost more than the miss does. See the P16 card for the three-way
// measurement that settled it.
func internedLayout(result detect.Result, cfg config) *Layout {
	if cfg.timezone != time.UTC || result.Ambig || result.AmbigProne {
		return nil
	}
	return internedLayouts[result.Def]
}
