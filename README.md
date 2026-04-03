# dateparsa

High-performance date parsing for Go. Detect the format once, parse millions of rows at native speed.

```go
result, err := dateparsa.Parse("2024-03-15T10:30:00Z")
fmt.Println(result.Time)   // 2024-03-15 10:30:00 +0000 UTC

// Reuse the detected layout — zero allocations, near-stdlib speed
t, err := result.Layout.Parse("2025-01-01T00:00:00Z")
```

## Why

Existing Go date parsing falls into two camps:

- **`time.Parse`** — fast, but you must already know the format
- **`araddon/dateparse`** — auto-detects, but tries 100+ patterns sequentially and throws away the result

Neither serves the primary real-world use case: ingesting a column of dates where every row has the same format but you don't know what it is until you see the first one.

**dateparsa** unifies format detection and parsing behind a trie-based scanner that identifies formats in O(n) — then hands you a compiled `Layout` you can reuse at near-`time.Parse` speed with zero allocations.

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
    dateparsa.WithStrictMode())
// err is *dateparsa.AmbiguousDateError with both interpretations
```

### Options

```go
dateparsa.ParseWith(s,
    dateparsa.WithBaseTime(t),          // Reference for relative dates
    dateparsa.WithTimezone(loc),        // Default timezone when none in input
    dateparsa.WithPreferDayFirst(true), // DD/MM/YYYY for ambiguous dates
    dateparsa.WithPreferYearFirst(true),// YYYY/MM/DD for ambiguous dates
    dateparsa.WithPreferFuture(true),   // "Tuesday" = next Tuesday
    dateparsa.WithStrictMode(),         // Reject ambiguous dates
)
```

## Supported Formats

### Structured Dates

| Category          | Examples                                                                         |
| ----------------- | -------------------------------------------------------------------------------- |
| ISO 8601          | `2024-03-15`, `2024-03-15T10:30:00Z`, `2024-03-15T10:30:00+05:30`                |
| RFC 3339          | `2024-03-15T10:30:00Z`, `2024-03-15T10:30:00.123456789Z`                         |
| RFC 2822          | `Fri, 15 Mar 2024 10:30:00 +0000`                                                |
| SQL / Database    | `2024-03-15 10:30:00`, `2024-03-15 10:30:00.000000`, `2024-03-15 10:30:00+05:30` |
| US numeric        | `03/15/2024`, `3/15/24`                                                          |
| European numeric  | `15.03.2024`, `15/03/2024`                                                       |
| Textual month     | `March 15, 2024`, `15 Mar 2024`, `Mar 15, 2024`                                  |
| Compact           | `20240315`, `20240315T103000`, `20240315T103000Z`                                |
| ISO week          | `2024-W11-5`, `2024-W11`                                                         |
| ISO ordinal       | `2024-074`                                                                       |
| Common Log Format | `15/Mar/2024:10:30:00 +0000`                                                     |
| Syslog            | `Mar 15 10:30:00`                                                                |
| Time only         | `10:30`, `10:30:00`, `10:30 PM`, `10:30:00.123456`                               |
| Partial dates     | `Mar 15`, `15 Mar`, `March 2024`                                                 |
| Unix timestamps   | `1710500000`, `1710500000000`, `1710500000.123`                                  |

### Ambiguity Handling

When a date like `01/02/2024` could be MM/DD or DD/MM:

1. **Value-range check** — if one number exceeds 12, it must be the day
2. **Separator heuristic** — dot separator (`.`) implies European DD.MM.YYYY
3. **User preference** — `WithPreferDayFirst` / `WithPreferYearFirst`
4. **Ambiguity flag** — `result.Ambiguous` tells you when the choice was a guess
5. **Strict mode** — `WithStrictMode()` returns all interpretations as an error

## Performance

Measured on Apple M2 Max, Go 1.26.1, `darwin/arm64`.

### Hot path (compiled Layout reuse)

| Operation                       | ns/op | Allocs | vs `time.Parse` |
| ------------------------------- | ----- | ------ | --------------- |
| `Layout.Parse` (ISO date)       | 25    | 0      | 0.96x           |
| `Layout.Parse` (ISO datetime+Z) | 41    | 0      | 1.6x            |
| `Layout.Parse` (compact date)   | 21    | 0      | 0.8x            |
| `Parser` (cached layout)        | 43    | 0      | 1.6x            |
| `time.Parse` (stdlib baseline)  | 26    | 0      | 1.0x            |

### Full detection + parse (first call)

| Format               | ns/op | Allocs |
| -------------------- | ----- | ------ |
| ISO 8601 date        | 190   | 4      |
| SQL datetime         | 234   | 4      |
| Compact date         | 177   | 4      |
| RFC 3339 + tz        | 598   | 8      |
| Textual month        | 459   | 10     |
| ISO ordinal          | 185   | 5      |
| ISO week date        | 269   | 5      |
| Unix timestamp       | 112   | 2      |
| SQL datetime + frac6 | 263   | 4      |
| Ambiguous slash      | 366   | 9      |

### Natural language

| Expression           | ns/op | Allocs |
| -------------------- | ----- | ------ |
| "3 days ago"         | 1,850 | 5      |
| "yesterday"          | 2,210 | 3      |
| "in 10 minutes"      | 2,400 | 5      |
| "next friday"        | 2,500 | 4      |
| "yesterday at 5pm"   | 3,320 | 5      |
| "beginning of month" | 3,980 | 5      |

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
- [ ] Phase 4: 200+ locale support via CLDR (French, German, Spanish, CJK, ...)
- [ ] Phase 5: Batch optimization and allocation elimination pass
- [ ] Phase 6: Documentation and v0.1.0 release

## License

MIT
