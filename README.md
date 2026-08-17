# dateparsa

High-performance date parsing for Go. Detect the format once, parse millions of rows at native speed.

```go
result, err := dateparsa.Parse("2024-03-15T10:30:00Z")
fmt.Println(result.Time)   // 2024-03-15 10:30:00 +0000 UTC

// Reuse the detected layout — zero allocations, faster than stdlib
t, err := result.Layout.Parse("2025-01-01T00:00:00Z")
```

## Why dateparsa

**When you already know the format**, use `time.Parse`. It's about 28 ns, zero allocs, stdlib. Nothing should replace it.

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
    Epoch   flextime.FlexTime `json:"epoch"`   // 1710505800, or 1710505800000
    Deleted flextime.FlexTime `json:"deleted"` // null
}

// A guessed day is reported here too, the same as result.Ambiguous is above
fmt.Println(u.CreatedAt.Ambiguous()) // true if the day came from a preference

// Configured parsing goes through a Scanner, which is the only place options
// can reach: encoding/json and database/sql construct the value themselves.
scanner := flextime.NewScanner(flextime.WithPreferDayFirst(true))
var ft flextime.FlexTime
scanner.Scan(&ft, "01/02/2024") // February 1, not January 2

// Or refuse to guess at all
strict := flextime.NewScanner(flextime.WithStrictMode(true))
err := strict.Scan(&ft, "01/02/2024")
// err is *dateparsa.AmbiguousDateError with both interpretations
```

`ParseOption` and `MarshalOption` are separate types so that a parse option cannot
be handed to a value that will never parse with it. `WithPreferDayFirst`,
`WithTimezone` and `WithStrictMode` go to `NewScanner`; `WithJSONFormat` goes to
`NewWithOptions`. Mixing them does not compile.

At the JSON boundary there is no `Scanner` to configure, because `encoding/json`
constructs the value. `Ambiguous()` after unmarshalling is the equivalent: it is set
on every path that parses a string.

A numeric column or a bare JSON number takes its precision from how many digits it
is written with, the same reading a string of those digits gets: 10 to 12 digits are
seconds, 13 are milliseconds, 16 are microseconds, and 19 are nanoseconds. So
`1710505800000` is March 2024 and not the year 56173. A digit count that names none
of those is refused, as is a value landing more than about 3168 years either side of
the epoch. A number written with a fraction or an exponent is seconds, because that
is the only thing a fractional timestamp can mean.

Fewer than ten digits is the one place a number and a string differ. `86400` from an
INTEGER column is the second of January 1970, while `"86400"` as a string is refused,
because a short string is far more likely to be a compact date or a bare year than a
timestamp and there is no such ambiguity once a schema has typed it as a number.

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

A word can be ambiguous too, and is reported the same way. Hindi writes both
yesterday and tomorrow as `कल`, choosing between them with the verb, which a
date string does not have:

```go
r, _ := dateparsa.ParseWith("कल", dateparsa.WithLocales(dateparsa.HI),
    dateparsa.WithBaseTime(base))
fmt.Println(r.Ambiguous) // true — it means either, and this is one of them

_, err := dateparsa.ParseWith("कल", dateparsa.WithLocales(dateparsa.HI),
    dateparsa.WithBaseTime(base), dateparsa.WithStrictMode(true))
// *AmbiguousDateError carrying both days
```

Writing the qualifier resolves it, and those forms report nothing: `बीता कल` is
yesterday and `आने वाला कल` is tomorrow.

## Performance

**One machine, and it is named here.** Every number below is an Apple M2 Max,
`darwin/arm64`, 12 cores, Go 1.26.6. "Hot path" and "Against `time.Parse`" are
benchstat over 12 runs; the rest are the median of the three runs in
`benchmarks/baseline.txt`. Where the two methods cover the same benchmark they
agree to within 3%.

The tables used to be split across two machines, and were labelled as such. The
change that made the hot path what it is was measured on a `linux/arm64` box,
and `benchmarks/baseline.txt` was deliberately not overwritten with its numbers,
because retargeting the committed reference would have made every later
`make bench-compare` on the M2 Max print deltas that were really the difference
between two machines. The suite has since been re-run here and the baseline
regenerated from it, so the split is gone and `make bench-compare` measures a
change again rather than a machine. If you change a number in a table, change it
on the machine the section names, and say what produced it.

**Allocs** is the one column that does not depend on the machine, an allocation
count being a property of the code. It comes from the same run as everything
else.

### Hot path (compiled Layout reuse)

Apple M2 Max, Go 1.26.6, 12 cores, benchstat over 12 runs, all within ±2%.

| Operation                       | ns/op | Allocs | vs `time.Parse` |
| ------------------------------- | ----- | ------ | --------------- |
| `Layout.Parse` (compact date)   | 16.8  | 0      | 0.6x            |
| `Layout.Parse` (ISO date)       | 17.9  | 0      | 0.6x            |
| `Layout.Parse` (ISO datetime+Z) | 24.4  | 0      | 0.9x            |
| `Parser` (cached layout)        | 26.6  | 0      | 1.0x            |
| `time.Parse` (stdlib baseline)  | 27.9  | 0      | 1.0x            |

### Against `time.Parse` on the same format

Both sides are given the format, so this is the fair comparison: `Compile` and
`time.Parse` each get a layout and parse the same string. `BenchmarkCompiledLayout_vs_Stdlib`
is the source, same machine and method as above.

| Format       | dateparsa | `time.Parse` |          |
| ------------ | --------- | ------------ | -------- |
| SQL datetime | 24.2 ns   | 91.9 ns      | **3.8x** |
| ISO date     | 18.0 ns   | 57.3 ns      | **3.2x** |
| US slash     | 18.2 ns   | 53.4 ns      | **2.9x** |
| RFC 3339     | 24.7 ns   | 27.7 ns      | **1.1x** |

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

The trade is that `Detect` on its own got **16% slower**: it does the planning
and never runs the program, so it pays and does not collect. That figure is the
one measurement in this section still carrying `linux/arm64` numbers (137 ns to
160 ns), because it is an A/B across two commits and only one of them is checked
out here; `Detect` measures 153 ns on the M2 Max today. `Parse`, which does run
the program, is flat to slightly faster, and anything that reuses the layout is
a third to a half faster. For a library whose reason to exist is parsing the
second row through the ten-millionth with the format found on the first, that is
the right side of the trade.

### Full detection + parse (first call)

| Format               | ns/op | Allocs |
| -------------------- | ----- | ------ |
| Unix timestamp       | 80    | 1      |
| ISO ordinal          | 122   | 2      |
| Compact date         | 130   | 1      |
| ISO 8601 date        | 133   | 1      |
| SQL datetime         | 174   | 1      |
| ISO 8601 datetime    | 177   | 1      |
| SQL datetime + frac6 | 198   | 1      |
| ISO week date        | 233   | 2      |
| Ambiguous slash      | 245   | 2      |
| Textual month        | 345   | 2      |

One allocation is the `Layout` the call returns. A format the trie matches needs
nothing else, because its fields were built at init. A format that falls through
to a detector needs one more, holding the fields it worked out and the
definition that describes them, and that is the whole of the second column
above: it was four for a textual month and ten for `03/15/2024 10:30:00`, in
scratch slices that were dead before `Parse` returned. The `ns/op` column
predates that change and is high on the four rows whose allocation count moved,
by 7% to 34% measured on `linux/arm64`.

Eight of these ten are slower than in the baseline this replaces, by 3% to 29%.
The cause is the range and separator checking the correctness fixes added: a
detection that describes every byte of its input and refuses a numeric part
wider than the field that reads it does more work than one that does neither.
It is paid once per column. The two rows that moved the other way moved
further. A textual month is half what it was, because a month name is now
matched against words rather than against every position, and a Unix timestamp
is 17% quicker.

### Bulk (10M rows)

| Operation                     | Time  | Per row |
| ----------------------------- | ----- | ------- |
| `Layout.Parse` 10M rows       | 248ms | 24.8 ns |
| `Parser.ParseColumn` 10M rows | 296ms | 29.6 ns |

A column costs one detection and then a compiled parse per row. The difference
between the two rows is the `[]time.Time` that `ParseColumn` fills and returns;
`Layout.Parse` hands back one value and allocates nothing.

### Natural language

| Expression           | ns/op | Allocs |
| -------------------- | ----- | ------ |
| "yesterday"          | 302   | 2      |
| "in 10 minutes"      | 332   | 2      |
| "next friday"        | 352   | 2      |
| "beginning of month" | 357   | 2      |
| "yesterday at 5pm"   | 404   | 2      |
| "3 days ago"         | 413   | 2      |

### Against araddon/dateparse

`github.com/araddon/dateparse` is the other Go library that detects a format
rather than being told one. `benchmarks/compare/` measures both on the same 16
formats and `benchmarks/compare/README.md` has the full tables; `make bench-vs`
reproduces them. It is a separate module so that the zero-dependency promise
above stays true, and it is not part of `make ci`.

Ratios rather than nanoseconds here, because that run is `linux/arm64` and this
section is not. araddon is also unmaintained, last released in April 2021, so
the shape of the difference is worth more than the margin.

| Question                                      | dateparsa vs araddon         |
| --------------------------------------------- | ---------------------------- |
| A column of 10k rows, one unknown format      | **1.03x to 8.1x**, ahead on 16 of 16 |
| Per row once the format is known              | **1.7x to 5.1x**, ahead on 14 of 14, zero allocs |
| One value parsed cold, no reuse               | 1.3x to 1.8x ahead on 9, **0.45x to 0.90x behind on 7** |
| A value that is not a date                    | 1.0x to 1.2x ahead on 2, **0.42x to 0.48x behind on 2** |

The two places it loses are worth stating plainly. Cold, it is behind on the
formats that miss the trie and fall through to a fallback detector, which
allocates: `03/15/2024 10:30:00` costs ten allocations and is 2.2x slower than
araddon. On a value that is not a date, it is about twice as slow on free text,
because after structured and epoch detection both fail it still tries natural
language, which a caller who never parses "3 days ago" is paying for and will
not use.

Both are recovered by the second row of a column, which is the case the library
is for, and neither is recovered by a caller parsing one date at a time.

On correctness the two agree on every one of the 16 formats, returning the same
instant for the same input. In the other direction, araddon's `ParseFormat`,
which is its analogue of a reusable `Layout`, returns a layout that does not
re-parse its own input for 2 of the 16.

### Regression tracking

The baseline is checked into `benchmarks/baseline.txt`. To check for
regressions:

```bash
make bench-compare
```

That runs every benchmark in the tree, root package and `flextime` both, into
`benchmarks/current.txt` and benchstats it against the baseline. Promote a run
to the baseline with `make bench-update`, on the machine the baseline names, in
a commit that says what moved and why.

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
