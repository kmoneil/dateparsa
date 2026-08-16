# dateparsa

High-performance date parsing for Go. Detect the format once, parse millions of rows at native speed.

```go
result, err := dateparsa.Parse("2024-03-15T10:30:00Z")
fmt.Println(result.Time)   // 2024-03-15 10:30:00 +0000 UTC

// Reuse the detected layout — zero allocations, faster than stdlib
t, err := result.Layout.Parse("2025-01-01T00:00:00Z")
```

## Why dateparsa

**When you already know the format**, use `time.Parse`. It's about 27 ns, zero allocs, stdlib. Nothing should replace it.

**When you don't know the format** — CSV imports, log ingestion, API responses from third parties, user-submitted data, multi-source pipelines — that's where dateparsa comes in.

One `Parse()` call handles ISO 8601, RFC 3339, RFC 2822, RFC 850, ANSIC, SQL timestamps, syslog, Common Log Format, spreadsheet dates, compact formats, Unix timestamps, partial dates, and natural language expressions like "3 days ago" or "next friday at 2pm". In 20 languages.

The key insight: **detect the format once, parse millions of rows at native speed.** The first call returns both the parsed time and a compiled `Layout`. Reusing that `Layout` bypasses all detection and runs at about 25 ns with zero allocations, against 28 ns for `time.Parse` on the same format. A shorter format is further ahead: a compact date is 17 ns.

### When to use dateparsa

- You're ingesting dates and don't control the format
- You have millions of rows in the same unknown format (detect once, parse at stdlib speed)
- You need structured dates AND natural language ("yesterday", "in 3 hours") through one API
- You need localized month/day names (20 languages built in, no runtime file loading)
- You want ambiguity handled explicitly (DD/MM vs MM/DD with value-range checking + strict mode)

## Install

```
go get github.com/kmoneil/dateparsa
```

Requires Go 1.26+. Zero runtime dependencies.

## Usage

### Auto-detect and parse

```go
result, err := dateparsa.Parse("March 15, 2024")
if err != nil {
    log.Fatal(err)
}
fmt.Println(result.Time)      // 2024-03-15 00:00:00 +0000 UTC
fmt.Println(result.Layout)    // MONTH_DAY_YEAR
fmt.Println(result.Ambiguous) // false
```

### Reuse the detected layout

```go
// Detect once
result, _ := dateparsa.Parse("2024-03-15T10:30:00Z")
layout := result.Layout

// Parse millions — zero alloc, ~25 ns/op for this format
for _, row := range rows {
    t, err := layout.Parse(row)
    // ...
}
```

### Batch parsing

```go
p := dateparsa.NewParser()

// Detects format on first row, reuses for the rest.
// Falls back to re-detection if the format changes, and re-detects
// every row of an ambiguous format such as DD/MM vs MM/DD, because
// which part is the month is decided per value and not per format.
times, errs := p.ParseColumn([]string{
    "2024-01-01",
    "2024-06-15",
    "2024-12-31",
})
```

### Natural language dates

```go
base := time.Date(2024, 3, 15, 12, 0, 0, 0, time.UTC)
opts := dateparsa.WithBaseTime(base)

result, _ := dateparsa.ParseWith("3 days ago", opts)
fmt.Println(result.Time)  // 2024-03-12 12:00:00 +0000 UTC
fmt.Println(result.Kind)  // relative

result, _ = dateparsa.ParseWith("yesterday at 5pm", opts)
fmt.Println(result.Time)  // 2024-03-14 17:00:00 +0000 UTC

result, _ = dateparsa.ParseWith("next friday at 14:00", opts)
fmt.Println(result.Time)  // 2024-03-22 14:00:00 +0000 UTC

result, _ = dateparsa.ParseWith("beginning of month", opts)
fmt.Println(result.Time)  // 2024-03-01 00:00:00 +0000 UTC
```

### Locale support

```go
// Parse French dates
result, _ := dateparsa.ParseWith("15 mars 2024", dateparsa.WithLocales(dateparsa.FR))
fmt.Println(result.Time)  // 2024-03-15 00:00:00 +0000 UTC

// French natural language
result, _ = dateparsa.ParseWith("hier", dateparsa.WithLocales(dateparsa.FR),
    dateparsa.WithBaseTime(time.Now()))
fmt.Println(result.Time)  // yesterday

// Russian, German, Spanish, etc.
dateparsa.ParseWith("15 марта 2024", dateparsa.WithLocales(dateparsa.RU))
dateparsa.ParseWith("gestern", dateparsa.WithLocales(dateparsa.DE),
    dateparsa.WithBaseTime(time.Now()))

// Lookup by BCP 47 tag
fr, _ := dateparsa.LookupLocale("fr")
dateparsa.ParseWith("demain", dateparsa.WithLocales(fr),
    dateparsa.WithBaseTime(time.Now()))
```

20 built-in locales: `EN`, `ES`, `FR`, `DE`, `IT`, `PT`, `NL`, `RU`, `ZH`, `JA`, `KO`, `AR`, `HI`, `PL`, `SV`, `DA`, `NO`, `FI`, `TR`, `UK`.

Supported patterns: `now`, `today`, `yesterday`, `tomorrow`, `N units ago`,
`N units from now`, `in N units`, `last/next/this <weekday>`,
`last/next/this <month>`, `last/next week/month/year`,
`beginning/end of day/week/month/year`, plus `at <time>` suffixes.

### Handle ambiguous dates

```go
// 01/02/2024 — is it January 2 or February 1?

// Default: MM/DD/YYYY (US convention)
result, _ := dateparsa.Parse("01/02/2024")
fmt.Println(result.Time)      // 2024-01-02
fmt.Println(result.Ambiguous) // true

// Prefer day-first (European convention)
result, _ = dateparsa.ParseWith("01/02/2024",
    dateparsa.WithPreferDayFirst(true))
fmt.Println(result.Time) // 2024-02-01

// Strict mode — reject ambiguous dates entirely
_, err := dateparsa.ParseWith("01/02/2024",
    dateparsa.WithStrictMode(true))
// err is *dateparsa.AmbiguousDateError with both interpretations
```

### Database and JSON integration

The `flextime` subpackage provides a `FlexTime` type that works as a drop-in replacement for `time.Time` in database models and JSON APIs. It implements `sql.Scanner`, `driver.Valuer`, `json.Marshaler`, `json.Unmarshaler`, and the `encoding.Text*` interfaces.

```go
import "github.com/kmoneil/dateparsa/flextime"

// Use directly in database models — handles time.Time, string, []byte,
// int64, float64, and NULL from any driver (PostgreSQL, MySQL, SQLite).
type User struct {
    CreatedAt flextime.FlexTime
    DeletedAt flextime.FlexTime // NULL-safe
}

var u User
db.QueryRow("SELECT created_at, deleted_at FROM users WHERE id = $1", id).
    Scan(&u.CreatedAt, &u.DeletedAt)

fmt.Println(u.CreatedAt.Time())  // 2024-03-15 10:30:00 +0000 UTC
fmt.Println(u.DeletedAt.Valid()) // false (was NULL)

// Works in JSON APIs with mixed date formats
type APIResponse struct {
    Created flextime.FlexTime `json:"created"` // "2024-03-15T10:30:00Z"
    Epoch   flextime.FlexTime `json:"epoch"`   // 1710505800
    Deleted flextime.FlexTime `json:"deleted"` // null
}

// Pre-configured scanner for non-default options
scanner := flextime.NewScanner(flextime.WithPreferDayFirst(true))
var ft flextime.FlexTime
scanner.Scan(&ft, "01/02/2024") // February 1, not January 2
```

### Options

```go
dateparsa.ParseWith(s,
    dateparsa.WithBaseTime(t),          // Reference for relative dates
    dateparsa.WithTimezone(loc),        // Default timezone when none in input
    dateparsa.WithPreferDayFirst(true), // DD/MM/YYYY for ambiguous dates
    dateparsa.WithPreferYearFirst(true),// YYYY/MM/DD for ambiguous dates
    dateparsa.WithPreferFuture(true),   // "Tuesday" = next Tuesday
    dateparsa.WithStrictMode(true),         // Reject ambiguous dates
)
```

## Supported Formats

### Structured Dates

| Category          | Examples                                                                                  |
| ----------------- | ----------------------------------------------------------------------------------------- |
| ISO 8601          | `2024-03-15`, `2024-03-15T10:30:00`, `2024-03-15T10:30:00Z`, `2024-03-15T10:30:00+05:30`  |
| RFC 3339          | `2024-03-15T10:30:00Z`, `2024-03-15T10:30:00.123456789Z`, `2024-03-15T10:30:00.123+05:30` |
| RFC 2822          | `Fri, 15 Mar 2024 10:30:00 +0000`                                                         |
| RFC 850           | `Friday, 15-Mar-24 10:30:00 UTC`                                                          |
| RFC 822 / 1123    | `15 Mar 24 10:30 UTC`, `Fri, 15 Mar 2024 10:30:00 UTC`                                    |
| ANSIC / Unix      | `Fri Mar 15 10:30:00 2024`, `Fri Mar 15 10:30:00 UTC 2024`                                |
| SQL / Database    | `2024-03-15 10:30:00`, `2024-03-15 10:30:00.000`, `2024-03-15 10:30:00.000000`            |
| SQL + timezone    | `2024-03-15 10:30:00+00`, `2024-03-15 10:30:00+05:30`                                     |
| US numeric        | `03/15/2024`, `3/15/24`, `3/15/2024`                                                      |
| European numeric  | `15.03.2024`, `15/03/2024`                                                                |
| Asian numeric     | `2024/03/15`, `2024.03.15`                                                                |
| Textual month     | `March 15, 2024`, `15 Mar 2024`, `Mar 15, 2024`, `15-Mar-2024`                            |
| Compact           | `20240315`, `20240315T103000`, `20240315103000`, `20240315T103000Z`                       |
| ISO week          | `2024-W11-5`, `2024-W11`                                                                  |
| ISO ordinal       | `2024-074`                                                                                |
| Common Log Format | `15/Mar/2024:10:30:00 +0000`                                                              |
| Syslog            | `Mar 15 10:30:00`                                                                         |
| Spreadsheet       | `3/15/2024 10:30:00 AM`, `15-Mar-2024 10:30`                                              |
| Time only         | `10:30`, `10:30:00`, `10:30 PM`, `10:30:00 PM`, `10:30:00.123`, `10:30:00.123456`         |
| Partial dates     | `Mar 15`, `15 Mar`, `March 2024`                                                          |
| Unix timestamps   | `1710500000` (sec), `1710500000000` (ms), `1710500000000000` (us), `1710500000.123`       |

### Ambiguity Handling

When a date like `01/02/2024` could be MM/DD or DD/MM:

1. **Value-range check** — if one number exceeds 12, it must be the day
2. **Separator heuristic** — dot separator (`.`) implies European DD.MM.YYYY
3. **User preference** — `WithPreferDayFirst` / `WithPreferYearFirst`
4. **Ambiguity flag** — `result.Ambiguous` tells you when the choice was a guess
5. **Strict mode** — `WithStrictMode(true)` returns all interpretations as an error
6. **No reuse of a guess** — `Parser` re-detects every row of an ambiguous
   format instead of applying the previous row's reading to it. Steps 1 and 2
   answer per value, so a layout cannot carry the answer forward.

## Performance

**Two machines appear below and the sections say which.** "Hot path" and
"Against `time.Parse`" are `linux/arm64`, Go 1.26.4, 10 cores. "Full detection +
parse", "Bulk", and "Natural language" are Apple M2 Max, Go 1.26.1,
`darwin/arm64`, taken as the median of the three runs in
`benchmarks/baseline.txt`. Do not read a row from one against a row from
another.

The split is not tidiness, it is what could actually be measured. The change
that made the hot path what it is could not be run on the M2 Max, and
`benchmarks/baseline.txt` was deliberately **not** overwritten with
`linux/arm64` numbers, because that would silently retarget the committed
reference and make every later `make bench-compare` on the M2 Max print deltas
that are really just the difference between two machines. That is also the state
`make bench-compare` is in today until somebody re-runs the suite there. If you
change a number in a table, change it on the machine that table names, and say
what produced it.

**Allocs** is `linux/arm64` throughout, including in the M2 Max tables: the
whole column was re-measured at `e61660a`, because seven of the counts had
drifted from the baseline, and the natural-language rows again when `Option`
became a value form, which took one allocation off every call that passes
options. An allocation count does not depend on the machine, being a property of
the code, which is why it is the one column that can be shared. In the M2 Max
tables the ns column has not been re-run since, so it is stale by an unknown
amount on any row whose allocation count moved.

### Hot path (compiled Layout reuse)

`linux/arm64`, Go 1.26.4, 10 cores, benchstat over 12 runs, all within ±2%.

| Operation                       | ns/op | Allocs | vs `time.Parse` |
| ------------------------------- | ----- | ------ | --------------- |
| `Layout.Parse` (compact date)   | 17.2  | 0      | 0.6x            |
| `Layout.Parse` (ISO date)       | 17.7  | 0      | 0.6x            |
| `Layout.Parse` (ISO datetime+Z) | 24.7  | 0      | 0.9x            |
| `Parser` (cached layout)        | 26.9  | 0      | 1.0x            |
| `time.Parse` (stdlib baseline)  | 27.9  | 0      | 1.0x            |

### Against `time.Parse` on the same format

Both sides are given the format, so this is the fair comparison: `Compile` and
`time.Parse` each get a layout and parse the same string. `BenchmarkCompiledLayout_vs_Stdlib`
is the source, same machine and method as above.

| Format       | dateparsa | `time.Parse` |          |
| ------------ | --------- | ------------ | -------- |
| SQL datetime | 24.2 ns   | 89.4 ns      | **3.7x** |
| ISO date     | 17.9 ns   | 56.6 ns      | **3.2x** |
| US slash     | 18.0 ns   | 55.0 ns      | **3.1x** |
| RFC 3339     | 24.8 ns   | 27.9 ns      | **1.1x** |

Zero allocations on every row, both sides. RFC 3339 is close because it is the
one layout the standard library hand-writes a dedicated parser for; the other
three go through its general layout scanner, which re-reads the layout string on
every call. A compiled `Layout` never re-reads anything.

A format whose fields all sit at fixed offsets is executed as straight-line code
rather than interpreted, which is where most of that margin comes from. Twenty
of the thirty-one supported formats qualify, including every one above;
`TestFastPathCoverage` is the list. The rest are the ones carrying a month name,
a weekday, a variable-width number, or an ISO week, and they run the instruction
interpreter as before.

The trade is that `Detect` on its own got **16% slower** (137 ns to 160 ns): it
does the planning and never runs the program, so it pays and does not collect.
`Parse`, which does run it, is flat to slightly faster, and anything that reuses
the layout is a third to a half faster. For a library whose reason to exist is
parsing the second row through the ten-millionth with the format found on the
first, that is the right side of the trade.

### Full detection + parse (first call)

| Format               | ns/op | Allocs |
| -------------------- | ----- | ------ |
| Unix timestamp       | 97    | 1      |
| Compact date         | 101   | 1      |
| ISO ordinal          | 103   | 3      |
| ISO 8601 date        | 112   | 1      |
| SQL datetime         | 157   | 1      |
| ISO 8601 datetime    | 165   | 1      |
| SQL datetime + frac6 | 189   | 1      |
| ISO week date        | 195   | 3      |
| Ambiguous slash      | 238   | 4      |
| Textual month        | 679   | 4      |

### Bulk (10M rows, Apple M2 Max)


| Operation                     | Time  | Per row |
| ----------------------------- | ----- | ------- |
| `Layout.Parse` 10M rows       | 366ms | 36.6 ns |
| `Parser.ParseColumn` 10M rows | 403ms | 40.3 ns |

**Both rows are stale and high.** They are the per-row cost of `Layout.Parse`
and of the cached layout in `Parser`, and both of those got about 45% faster on
`linux/arm64` when fixed-offset formats stopped being interpreted. Neither
number has been re-run on the machine this table names. The shape of the claim
is unchanged: a column costs one detection and then a compiled parse per row.

### Natural language

| Expression           | ns/op | Allocs |
| -------------------- | ----- | ------ |
| "yesterday"          | 714   | 2      |
| "3 days ago"         | 803   | 2      |
| "next friday"        | 834   | 2      |
| "in 10 minutes"      | 936   | 2      |
| "beginning of month" | 1,101 | 2      |
| "yesterday at 5pm"   | 1,111 | 2      |

### Regression tracking

Benchmark baselines are checked into `benchmarks/baseline.txt`. To check for regressions:

```bash
go test -bench=. -benchmem -count=3 > benchmarks/current.txt
benchstat benchmarks/baseline.txt benchmarks/current.txt
```

## How It Works

### Trie-based format detection

Every structured date format maps to a **character-class signature**. `2024-03-15` becomes `DDDD-DD-DD`. `March 15, 2024` normalizes to a month-name token plus digits.

Instead of trying 100+ `time.Parse` calls, dateparsa:

1. Scans the input once, mapping each byte to a character class (digit, letter, separator, whitespace, colon, special)
2. Walks the signature through a trie of known formats
3. Returns the match in O(n) time — no backtracking

### Compiled Layout

The detected format is compiled into a `Layout` — a fixed sequence of extraction instructions (extract year at offset 0..4, month at 5..7, etc). Reusing the `Layout` bypasses all detection logic and extracts fields at known byte offsets. This is why it runs at `time.Parse` speed or better with zero allocations.

Where a format's fields are all fixed-width, the `Layout` skips the instruction interpreter too. Its fields are placed in fixed slots when the layout is compiled, along with the one input length they describe, so parsing is straight-line code with no loop, no opcode dispatch, and one length check instead of a running byte count. That is worth about a third to a half of the parse.

```
First parse:    input → signature → trie → compile → execute → time.Time + Layout
Subsequent:     input → execute (same instructions) → time.Time
```

## Roadmap

- [x] Phase 1: Core detection and parsing (ISO, RFC, SQL, textual months)
- [x] Phase 2: Extended formats (compact, timestamps, week dates, syslog, AM/PM)
- [x] Phase 3: English natural language ("3 days ago", "next friday", "yesterday at 5pm")
- [x] Phase 4: 20 locale support (French, German, Spanish, Russian, CJK, Arabic, ...)
- [x] Phase 5: Batch optimization and allocation elimination pass
- [x] Phase 6: `flextime` subpackage — `sql.Scanner`/`driver.Valuer`/JSON integration
- [ ] Phase 7: Documentation and v0.1.0 release

## License

Apache License 2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).

Locale data is derived from the Unicode Common Locale Data Repository (CLDR) and
carries the Unicode License v3, reproduced in `NOTICE`.
