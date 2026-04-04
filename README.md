# dateparsa

High-performance date parsing for Go. Detect the format once, parse millions of rows at native speed.

```go
result, err := dateparsa.Parse("2024-03-15T10:30:00Z")
fmt.Println(result.Time)   // 2024-03-15 10:30:00 +0000 UTC

// Reuse the detected layout — zero allocations, near-stdlib speed
t, err := result.Layout.Parse("2025-01-01T00:00:00Z")
```

## Why dateparsa

**When you already know the format**, use `time.Parse`. It's 26 ns, zero allocs, stdlib. Nothing should replace it.

**When you don't know the format** — CSV imports, log ingestion, API responses from third parties, user-submitted data, multi-source pipelines — that's where dateparsa comes in.

One `Parse()` call handles ISO 8601, RFC 3339, RFC 2822, RFC 850, ANSIC, SQL timestamps, syslog, Common Log Format, spreadsheet dates, compact formats, Unix timestamps, partial dates, and natural language expressions like "3 days ago" or "next friday at 2pm". In 20 languages.

The key insight: **detect the format once, parse millions of rows at native speed.** The first call returns both the parsed time and a compiled `Layout`. Reusing that `Layout` bypasses all detection and runs at 36 ns with zero allocations — within 1.4x of `time.Parse` with a known format.

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

Requires Go 1.23+. Zero runtime dependencies.

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

// Parse millions — zero alloc, ~25 ns/op
for _, row := range rows {
    t, err := layout.Parse(row)
    // ...
}
```

### Batch parsing

```go
p := dateparsa.NewParser()

// Detects format on first row, reuses for the rest.
// Falls back to re-detection if the format changes.
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

## Performance

Measured on Apple M2 Max, Go 1.26.1, `darwin/arm64`.

### Hot path (compiled Layout reuse)

| Operation                       | ns/op | Allocs | vs `time.Parse` |
| ------------------------------- | ----- | ------ | --------------- |
| `Layout.Parse` (ISO date)       | 21    | 0      | 0.8x            |
| `Layout.Parse` (ISO datetime+Z) | 36    | 0      | 1.4x            |
| `Layout.Parse` (compact date)   | 19    | 0      | 0.7x            |
| `Parser` (cached layout)        | 38    | 0      | 1.5x            |
| `time.Parse` (stdlib baseline)  | 26    | 0      | 1.0x            |

### Full detection + parse (first call)

| Format               | ns/op | Allocs |
| -------------------- | ----- | ------ |
| ISO 8601 date        | 95    | 1      |
| ISO 8601 datetime    | 143   | 1      |
| SQL datetime         | 135   | 1      |
| Compact date         | 87    | 1      |
| SQL datetime + frac6 | 172   | 1      |
| Unix timestamp       | 55    | 1      |
| ISO week date        | 202   | 3      |
| ISO ordinal          | 109   | 3      |
| Textual month        | 603   | 6      |
| Ambiguous slash      | 212   | 4      |

### Bulk (10M rows, Apple M2 Max)

| Operation                     | Time  | Per row |
| ----------------------------- | ----- | ------- |
| `Layout.Parse` 10M rows       | 354ms | 35 ns   |
| `Parser.ParseColumn` 10M rows | 410ms | 41 ns   |

### Natural language

| Expression           | ns/op | Allocs |
| -------------------- | ----- | ------ |
| "yesterday"          | 634   | 3      |
| "3 days ago"         | 686   | 3      |
| "next friday"        | 769   | 3      |
| "in 10 minutes"      | 800   | 3      |
| "yesterday at 5pm"   | 1,005 | 3      |
| "beginning of month" | 1,015 | 3      |

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

The detected format is compiled into a `Layout` — a fixed sequence of extraction instructions (extract year at offset 0..4, month at 5..7, etc). Reusing the `Layout` bypasses all detection logic and extracts fields at known byte offsets. This is why it runs at near-`time.Parse` speed with zero allocations.

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

MIT
