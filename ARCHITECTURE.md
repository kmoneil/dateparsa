# dateparsa Architecture

This document is for engineers working on or reviewing the dateparsa codebase. It covers every package, every data flow, every performance decision, and every gotcha.

---

## Overview

dateparsa is a date parsing library built around one insight: **format detection and parsing are separate problems, and the detection result should be reusable.**

The library detects date formats via a trie-based character-class scanner, compiles the detected format into a sequence of byte-extraction instructions, and hands the caller a `Layout` object that can re-parse the same format at near-stdlib speed with zero allocations.

```
Input string
    │
    ▼
┌──────────────┐     ┌────────────┐     ┌──────────────┐
│   Detect     │────▶│   Compile  │────▶│   Execute    │──▶ time.Time
│  (one-time)  │     │  (one-time)│     │ (every call) │
└──────────────┘     └────────────┘     └──────────────┘
                                              ▲
                                              │
                                 Layout.Parse(s) skips
                                 detect + compile entirely
```

---

## Package Map

```
dateparsa/                  Public API
├── flextime/               sql.Scanner / driver.Valuer / JSON integration
├── internal/compile/       Instruction set, compiler, executor
├── internal/detect/        Signature scanner, trie, format database
├── internal/epoch/         Unix timestamp detection
├── internal/natural/       English + localized NL parser
└── internal/locale/        Locale registry and data
    └── data/               20 compiled locale files (init-registered)
```

**Dependency graph** (no cycles):

```
flextime ────── dateparsa (one-way; dateparsa must never import flextime)
dateparsa ──┬── compile
            ├── detect ──── compile, locale
            ├── epoch
            ├── natural ─── locale
            └── locale
                └── data (init-only)
```

---

## The Detection Pipeline

`detect.Detect(s, cfg)` is the entry point. It runs a cascade of detectors, each more expensive than the last. The first match wins.

| Step | Detector                 | What it catches                                     | Cost                                 |
| ---- | ------------------------ | --------------------------------------------------- | ------------------------------------ |
| 1    | `detectISOWeekOrOrdinal` | `2024-W11-5`, `2024-074`                            | Byte checks at fixed offsets         |
| 2    | `detectCJKDate`          | `2014年04月08日`                                    | Scan for 年/月/日 bytes              |
| 3    | `detectTextualMonth`     | `March 15, 2024`, `Fri, 15 Mar 2024 10:30:00 +0000` | Case-insensitive word scan           |
| 4    | **Trie lookup**          | 50+ fixed-signature formats                         | O(n) signature scan + O(n) trie walk |
| 5    | `detectISO8601Frac`      | `2024-03-15T10:30:00.123Z` (any frac length)        | Fixed-offset checks                  |
| 6    | `detectVariableNumeric`  | `3/15/2024`, `3/15/24`, `3/15/2024 10:30 AM`        | Split-and-parse                      |
| 7    | `detectGoTimeString`     | `2012-08-03 18:31:59.257000000 +0000 UTC`           | Fixed-offset checks                  |
| 8    | `detectDatePlusTZ`       | `2020-07-20+08:00`                                  | Length + byte checks                 |

If all structured detection fails, the caller (`Parse`/`ParseWith`) tries epoch timestamps, then natural language.

### The Trie

The trie is the fast path for common formats. Every supported structured format has a **character-class signature** — a sequence of 6 possible classes:

| Class     | Symbol | Matches                                        |
| --------- | ------ | ---------------------------------------------- |
| Digit     | `D`    | `0-9`                                          |
| Letter    | `L`    | `a-zA-Z` (except T/Z in ISO positions)         |
| Separator | `S`    | `- / .`                                        |
| Space     | `W`    | space, tab                                     |
| Colon     | `C`    | `:`                                            |
| Special   | `X`    | `T` (between digits), `Z` (terminal), `+`, `,` |

Example: `"2024-03-15T10:30:00Z"` → `DDDDSDDSDDXDDCDDCDDX`

The trie is a 6-ary tree indexed by character class. At init time, `buildTrie()` inserts all format signatures from `phase1Formats()` and `phase2Formats()`. At runtime, `Scan()` maps the input to a signature, then `trie.lookup()` walks it in O(n).

**Key optimization**: non-ambiguous trie entries carry a pre-built `*compile.FormatDef` pointer, set at init time. This means `Detect` returns without allocating a FormatDef for common formats.

### Signature Scanner Gotchas

The scanner has special-case logic for characters that are ambiguous:

- **`T`**: Special only between digits (`2024-03-15T10:30`). Otherwise it's a letter (`Tuesday`).
- **`Z`**: Special only at end of string or before `+`/`-`. Otherwise it's a letter (`Zurich`).
- **`-`**: Separator usually, but Special when it's a timezone sign (after a colon pattern like `HH:MM:SS-`).

The `hasLetter()` function that gates textual-month detection also skips `T` and `Z` in ISO positions. This prevents `"2024-03-15T10:30:00"` from entering the textual month path (which would waste ~500ns scanning for month names).

### Ambiguity Resolution

When the trie matches a signature marked `ambig: true` (like `DD/DD/DDDD`), `resolveAmbiguous` kicks in:

1. Parse the three numeric parts
2. If any part > 12 → it must be the day (not month). Unambiguous.
3. If any part is 4 digits → it's the year. Narrows to 2 possibilities.
4. Separator heuristic: `.` implies European (DD.MM.YYYY)
5. Apply `PreferDayFirst` / `PreferYearFirst` preferences
6. Set `Ambig: true` on the result so the caller knows it was a guess

In strict mode, both interpretations are computed and returned in an `AmbiguousDateError`.

---

## The Compile/Execute Engine

### Instructions

Each `Inst` is 8 bytes:

```go
type Inst struct {
    Op     OpCode  // What to extract
    Offset byte    // Where in the input string
    Len    byte    // How many bytes to read
    Aux    uint16  // Pre-resolved value (e.g., month number for OpMonthName)
}
```

A `Program` holds up to 24 instructions in a fixed-size array. **This is a value type**, not a pointer — it embeds directly in the `Layout` struct without heap allocation.

### Execution

`executeInner` is the hot path. It's a single loop over instructions, extracting fields at known byte offsets into stack-allocated variables:

```
year=0, month=January, day=1, hour=0, min=0, sec=0, nsec=0, loc=UTC
    │
    ▼
for each instruction:
    read field at offset → validate → store in variable
    │
    ▼
Apply AM/PM, ISO week, ordinal day conversions
    │
    ▼
time.Date(year, month, day, hour, min, sec, nsec, loc)
```

**Why it's fast**: no string splitting, no regex, no map lookups, no allocations. Just byte reads at computed offsets. The only allocation is the `time.Time` return value itself (which is unavoidable).

### The parsing primitives

These are the workhorses:

- `parse2Digits(s, off)` — Reads exactly 2 ASCII digits. Branchless: `(s[off]-'0')*10 + (s[off+1]-'0')`.
- `parse4Digits(s, off)` — Same for 4 digits.
- `parse1or2Digits(s, off, slen)` — Variable width. Tries 2, falls back to 1.
- `parseFracSec(s, off, len)` — Reads N digits, scales to nanoseconds.
- `parseTZOffset(s, off, len)` — Handles `+HH`, `+HHMM`, `+HH:MM`.

Every function checks bounds before reading. Every function returns `(value, ok)`.

---

## Natural Language Parser

### Architecture

```
"3 days ago at 5pm"
    │
    ▼
Scanner ─────────▶ [NUM(3), UNIT(day), DIR(ago), AT, TIME(17:00)]
    │
    ▼
Evaluator ───────▶ base.AddDate(0, 0, -3) then set hour to 17
    │
    ▼
Result{Time: ..., Kind: KindRelative}
```

### Scanner

`Scan(s)` tokenizes English. `ScanLocale(s, locale)` tokenizes using locale-specific keywords.

Key design decisions:

- "a" and "an" produce `TokNumber{IntVal: 1}` so "a week ago" = "1 week ago"
- Written-out numbers (one through thirty, plus "few"=3) produce TokNumber
- "half" produces TokHalf, handled specially in evaluation
- Ordinal suffixes (st, nd, rd, th) are stripped during number parsing
- "from now" is recognized as a two-word phrase (DirFromNow)
- Inline time like "5pm" or "14:00" produces TokTime with hour/minute already resolved

### Evaluator

`Eval` tries 12 pattern matchers in priority order. Each either matches and returns a Result, or returns nil.

The pattern priority matters. `evalRelWord` (yesterday, today) runs before `evalNAgo` (3 days ago) because "yesterday at 5pm" should match as a relative word + time suffix, not fail as "N units ago" (no unit).

**Compound durations**: "1 hour and 3 minutes ago" is handled by `evalCompoundNAgo`, which accumulates multiple (N, unit) pairs separated by "and", then applies the total offset with the direction.

**Locale NL**: French "il y a 2 heures" works because "il y a" is in the French locale's `Ago` keywords, and `ScanLocale` produces `[DIR(ago), NUM(2), UNIT(hour)]`. The evaluator's `evalPrefixAgo` pattern matches `DIR NUM UNIT` (prefix-ago, used by French/Spanish/German).

---

## Locale System

### How Locale Data Gets Loaded

Each locale file in `internal/locale/data/` has an `init()` function:

```go
func init() { locale.Register(&FR) }
```

The public package `locale.go` has a blank import:

```go
import _ "github.com/kmoneil/dateparsa/internal/locale/data"
```

This triggers all 20 `init()` functions at program start, populating the global registry. The pre-built locale vars (`dateparsa.FR`, `dateparsa.DE`, etc.) are initialized at package init time via `LookupLocale`.

### How Locales Affect Detection

When `WithLocales(FR)` is passed:

1. **Structured detection**: `findMonthNameCI()` searches English month names first (default), then each locale's `MonthsWide` and `MonthsAbbr` arrays. Case-insensitive matching uses `equalsFoldASCII` — no allocation.

2. **NL parsing**: `ScanLocale()` builds a word list from the locale's `RelativeKeywords`, weekdays, and months. It matches against the lowercased input with accent-folding for languages like Spanish (días → dias).

---

## The `Parse()` Fast Path

`Parse(s)` (no options) has a dedicated fast path that avoids allocating a `config` struct:

```go
func Parse(s string) (ParseResult, error) {
    dcfg := detect.Config{Timezone: time.UTC}  // stack-allocated
    result, ok := detect.Detect(s, dcfg)
    if !ok {
        // try epoch, then NL (with time.Now() only if needed)
    }
    program := compile.Compile(result.Def, time.UTC)
    t, _ := program.Execute(s)
    layout := &Layout{program: program, ...}  // 1 alloc: the Layout itself
    return ParseResult{Time: t, Layout: layout, ...}, nil
}
```

This produces **1 allocation** (the `Layout` pointer) for common formats. The `Layout` is the thing users keep for reuse, so this allocation is intentional and amortized over millions of subsequent zero-alloc `Layout.Parse` calls.

`ParseWith(s, opts...)` goes through `buildConfig`, which heap-allocates due to the `Option func(*config)` pattern taking a pointer. This adds 1-2 extra allocations.

---

## Performance Model

| Operation               | ns/op    | Allocs | What dominates                       |
| ----------------------- | -------- | ------ | ------------------------------------ |
| `Layout.Parse`          | 21-36    | 0      | Instruction loop + `time.Date`       |
| `Parse` (trie hit)      | 87-143   | 1      | Signature scan + trie walk + compile |
| `Parse` (RFC3339/+TZ)   | 170-180  | 1      | Same + pre-built TZ offset lookup    |
| `Parse` (textual month) | 430-490  | 6      | Month name search + field building   |
| `Parse` (NL)            | 490-900  | 3      | Tokenization + evaluation            |
| `Parse` (epoch)         | 55-70    | 1      | Digit scan + range check             |
| `ParseWith` (with opts) | +50-80   | +1     | Config allocation                    |
| `Parser.Parse` (cached) | 38-45    | 0      | Same as Layout.Parse                 |

### Where Allocations Come From

For `Parse("2024-03-15")` → 1 alloc:

1. `&Layout{...}` — the returned Layout pointer (**intentional**: user keeps this)

For `ParseWith("March 15, 2024")` → 6 allocs:

1. `config` struct (escapes due to option closures)
2. `Layout` pointer
3. `FormatDef` (textual month builds dynamically)
4. `[]compile.Field` in `buildTextualFields`
5. `[]numToken` in `buildTextualFields`
6. `[]compile.Field` in `parseTimeComponent`

The textual month path is inherently more expensive because the format isn't in the trie — it requires scanning for month names and dynamically building field definitions.

---

## Testing Architecture

### Test Layers

| Layer                | Files                                                | What it tests                                               |
| -------------------- | ---------------------------------------------------- | ----------------------------------------------------------- |
| Unit                 | `internal/*/..._test.go`                             | Individual components in isolation                          |
| Integration          | `dateparser_test.go`                                 | Full `Parse`/`ParseWith` pipeline                           |
| Format coverage      | `coverage_test.go`                                   | All 53+ format strings parse correctly                      |
| Gap coverage         | `gaps_format_test.go`, `gaps_nl_test.go`             | Every competitive gap is closed                             |
| Phase tests          | `phase2_test.go`, `phase3_test.go`, `phase4_test.go` | Feature-specific coverage                                   |
| Examples             | `example_test.go`                                    | README code compiles and runs                               |
| Benchmarks           | `bench_test.go`                                      | Performance regression tracking                             |
| Panic fuzzing        | `fuzz_test.go`                                       | `FuzzParse`, `FuzzDetect` — no panics on arbitrary input    |
| **Semantic fuzzing** | `roundtrip_test.go`                                  | **29 formats × 1000 random dates: format → parse → verify** |

### The Semantic Round-Trip Fuzzer

This is the most important test. For each supported format:

1. Generate a random `time.Time` (1970-2099, safe day range 1-28)
2. Format it using Go's `time.Format` or a custom renderer
3. Parse it back with dateparsa
4. Compare the result against the original

This catches **silent wrong answers** — the class of bug where `Parse` succeeds but returns the wrong time. Panic-only fuzz targets can't catch these.

The fuzzer runs 29,000 round-trips deterministically (seed 42) on every `go test`. The `FuzzRoundTrip_ISO` and `FuzzRoundTrip_SQL` targets provide additional coverage via `go test -fuzz`.

### Zero-Alloc Guarantee

`TestLayoutParseZeroAlloc` uses `testing.AllocsPerRun` to verify that `Layout.Parse` allocates exactly 0 times over 1000 calls. This runs in CI and in the pre-commit hook. If any code change adds an allocation to the hot path, the build breaks.

---

## CI Pipeline

```
Push/PR to main
    │
    ├── Test (Go 1.23/1.24/stable × Linux/macOS/Windows, race detector)
    ├── Lint (golangci-lint: errcheck, govet, ineffassign, unused)
    ├── Fuzz (FuzzParse 30s + FuzzDetect 30s)
    ├── Alloc (TestLayoutParseZeroAlloc)
    ├── Benchmark (full suite, key results printed)
    └── Binary Size (fail if > 10MB)
```

Tag `v*` triggers release workflow with benchmark results attached.

### Local Development

```bash
make check       # vet + test (fast, runs on every commit via hook)
make ci          # vet + lint + test + alloc + fuzz (full local CI)
make bench       # save benchmarks to benchmarks/current.txt
make bench-compare  # benchstat against baseline
```

Pre-commit hook runs automatically: `go vet` → `go test` → alloc check.

---

## Adding a New Format

1. **Compute the signature**: Run `detect.Scan("your format example")` and read the character classes
2. **Add a trie entry** in `formats.go` (`phase2Formats`) with the signature and field definitions
3. **If variable-width**: Add a special-case detector in `detect.go` (like `detectVariableNumeric`)
4. **Add a test case** to `coverage_test.go`
5. **Add a round-trip spec** to `roundtrip_test.go` with a format renderer and comparison function
6. **Run `make check`** to verify

If the signature collides with an existing entry, you'll need a fallback detector that disambiguates using the actual input characters (not just the character classes).

## Adding a New Locale

1. Create `internal/locale/data/xx.go` following the existing pattern
2. Include `init() { locale.Register(&XX) }`
3. Add the pre-built var in `locale.go`: `XX, _ = LookupLocale("xx")`
4. Add test cases in `phase4_test.go` for month names and NL expressions
5. The locale data should include: MonthsWide, MonthsAbbr, WeekdaysWide, WeekdaysAbbr, AM/PM, and RelativeKeywords

## Adding a New NL Pattern

1. If a new token type is needed, add it to `scanner.go` (`TokenKind` enum + `classifyWord`)
2. Add an `eval*` function in `eval.go` that matches the token pattern
3. Wire it into `Eval()` at the correct priority position
4. Add test cases in `gaps_nl_test.go` or `internal/natural/natural_test.go`
5. Consider: does this pattern conflict with existing patterns? Test with the full suite.

---

## Known Limitations

- **Timezone abbreviations are ambiguous**: "CST" = US Central, China Standard, or Cuba Standard. We default to US interpretations. The user can override with `WithTimezone`.
- **No calendar systems**: Hijri, Jalali, Hebrew, Buddhist, Japanese Imperial are not supported.
- **No date extraction from prose**: Input must be a date string, not "Meet me on March 15 at the office."
- **Textual month path allocates more**: Dynamic field building for month-name formats can't use the pre-built trie optimization.
- **Locale NL is basic**: Only relative words (yesterday, tomorrow, ago) are localized. Full NL pattern localization (compound expressions, ordinals) is English-only.
