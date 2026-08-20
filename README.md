<p align="center">
  <img src="docs/assets/dateparsa.png" alt="dateparsa" width="440">
</p>

<p align="center">
  <b>Date parsing for Go, for input whose format you do not control.</b>
</p>

<p align="center">
  <a href="https://github.com/kmoneil/dateparsa/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/kmoneil/dateparsa/actions/workflows/ci.yml/badge.svg"></a>
  <a href="LICENSE"><img alt="Licence" src="https://img.shields.io/badge/licence-Apache--2.0-blue"></a>
  <a href="go.mod"><img alt="Go" src="https://img.shields.io/badge/go-1.26%2B-00ADD8"></a>
  <a href="go.mod"><img alt="Dependencies" src="https://img.shields.io/badge/dependencies-0-brightgreen"></a>
  <a href="#performance"><img alt="Allocations" src="https://img.shields.io/badge/Layout.Parse-0%20allocs-brightgreen"></a>
</p>

```go
result, err := dateparsa.Parse("2024-03-15T10:30:00Z")
fmt.Println(result.Time)   // 2024-03-15 10:30:00 +0000 UTC

// Reuse the detected layout: zero allocations, faster than stdlib
t, err := result.Layout.Parse("2025-01-01T00:00:00Z")
```

Detection and parsing are separate problems, and the detection result is
reusable. `Parse` hands back the time **and** a compiled `Layout`, and that
layout re-parses the same format with zero allocations, at 24.6 ns for an ISO
date. The cost of not knowing the format is paid once per column, not once per
row.

### What you get

|  | |
| --- | --- |
| **Detect once, reuse forever** | The first call returns a `Layout`. Every row after it skips detection entirely and allocates nothing. |
| **Faster than `time.Parse`** | 2.9x to 4.0x on the same format, both sides told the layout, zero allocations on either. |
| **One API for two problems** | ISO, RFC, SQL, syslog, epochs and compact formats, plus "3 days ago" and "next friday at 2pm". |
| **20 languages, compiled in** | Month and day names for 20 locales, registered at init. No file to ship, no path to configure. |
| **Ambiguity is reported, never hidden** | `DD/MM` against `MM/DD` is a guess, and `ParseResult.Ambiguous` says it was one. Strict mode returns both readings instead. |
| **Zero dependencies** | `go.mod` has no `require` block. Nothing here reaches the network or the filesystem. |

**Jump to:** [Install](#install) &nbsp;·&nbsp; [Usage](#usage) &nbsp;·&nbsp;
[Supported formats](#supported-formats) &nbsp;·&nbsp;
[Ambiguity](#ambiguity-handling) &nbsp;·&nbsp;
[Database and JSON](#database-and-json-integration) &nbsp;·&nbsp;
[Performance](#performance) &nbsp;·&nbsp;
[How it works](#how-it-works)

## Why dateparsa

**When you already know the format**, use `time.Parse`. It's 44 ns on the machine the Performance section names, zero allocs, stdlib. Nothing should replace it.

**When you don't know the format** — CSV imports, log ingestion, API responses from third parties, user-submitted data, multi-source pipelines — that's where dateparsa comes in.

One `Parse()` call handles ISO 8601, RFC 3339, RFC 2822, RFC 850, ANSIC, SQL timestamps, syslog, Common Log Format, spreadsheet dates, compact formats, Unix timestamps, partial dates, and natural language expressions like "3 days ago" or "next friday at 2pm". In 20 languages.

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

// Parse millions, zero alloc, 33 ns/op for this format
for _, row := range rows {
    t, err := layout.Parse(row)
    // ...
}
```

Not every result carries a layout you can reuse. Ask before keeping one:

```go
if result.Layout.Reusable() {
    // safe for the rest of the column
}
```

It is false for two different reasons, and both mean do not keep this.

A Unix timestamp has no format to reuse, and `3 days ago` resolves against the
time it was parsed at, so both come back as sentinels that refuse to re-parse
rather than answering a different day later.

The second reason does not refuse, which is why asking matters. Where detection
chose between readings by looking at the values, both readings compile to the
same program, so the layout accepts the next row and reads it the first row's
way: the layout from `70MAY1` reads `01MAY10` as 2001-05-10 where detection
reads 2010-05-01, and the layout from `25/12/2024` reads `01/02/2024` as the
first of February where detection reads the second of January. `Reusable()` is
false for those formats whatever value detected them, `13/01/2024` included:
which part is the month is decided per row, so no layout can carry the answer.

Use `Parser` for such a column and it is handled for you: it declines its own
cache on exactly those formats and re-detects per row. `Layout.Parse` still
works on one, because a caller who knows their column is uniform is entitled to
the fast path, and `Reusable()` is how you find out that you need to know.

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

A month or year shift clamps to the end of the target month rather than
overflowing it: one month before `2024-03-31` is `2024-02-29`, and one year
after `2024-02-29` is `2025-02-28`. Go's `time.AddDate` normalises instead,
which answers `2024-03-02` for the first of those.

Two bounds apply on this path and nowhere else. The expression is at most 512
bytes, which admits about fifty terms, and `N` is at most six digits. Both refuse
rather than truncate, and neither is reachable by a structured format: this path
runs only after format and timestamp detection have both failed.

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

`Scan`, `UnmarshalText` and `UnmarshalJSON` each keep the layout the last value
they saw was detected with, one cache per entry point, and every caller in the
binary shares them, so a document or a column of one format detects once rather
than once per value. **They cannot change the instant a value parses to.** A
layout that does not fit fails and detection runs again, and a format whose
reading is a guess is never reused, so the answer always comes from parsing that
value. **What they can change is whether a value parses at all**: a layout that
fits accepts bytes detection would refuse, so `"2024-03-15 10:30:00 "` with its
trailing space is an error against a cold cache and 2024-03-15 10:30:00 against
one primed by a Go time string, which is the instant detection would have
returned had it accepted the value. Call `dateparsa.Parse` directly if
acceptance has to be a property of the value alone. `SECURITY.md` says this in
its own terms, and the allocation table under
[Performance](#performance) has what the paths cost.

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

A string column costs nothing per row after the first, because the entry point
keeps the layout it last detected, and the numeric arms allocate nothing at any
point.

### Options

```go
dateparsa.ParseWith(s,
    dateparsa.WithBaseTime(t),          // Reference for relative dates
    dateparsa.WithTimezone(loc),        // Default timezone when none in input
    dateparsa.WithPreferDayFirst(true), // DD/MM/YYYY for ambiguous dates
    dateparsa.WithPreferYearFirst(true),// YY/MM/DD for ambiguous dates
    dateparsa.WithPreferFuture(true),   // "Tuesday" = next Tuesday
    dateparsa.WithStrictMode(true),     // Reject ambiguous dates
)
```

`WithPreferYearFirst` applies where the input leaves the year's position open,
which is a date whose three parts are all small: `01/02/03` is 2001-02-03 with
it and 2003-01-02 without. A written four-digit year wins over it, and so does a
value the reading cannot use, so `01/13/03` keeps its year last rather than
being refused for having no month 13.

## Supported Formats

### Structured Dates

The two-digit-year forms of RFC 822 and RFC 850 parse, and they report
`Ambiguous`: nothing in `15 Mar 24 10:30 UTC` says it is not a `YY Mon DD`
column, so strict mode refuses it. See *Ambiguity Handling* below.

| Category          | Examples                                                                                  |
| ----------------- | ----------------------------------------------------------------------------------------- |
| ISO 8601          | `2024-03-15`, `2024-03-15T10:30:00`, `2024-03-15T10:30:00Z`, `2024-03-15T10:30:00+05:30`  |
| RFC 3339          | `2024-03-15T10:30:00Z`, `2024-03-15T10:30:00.123456789Z`, `2024-03-15T10:30:00.123+05:30` |
| RFC 2822          | `Fri, 15 Mar 2024 10:30:00 +0000`                                                         |
| RFC 850           | `Friday, 15-Mar-24 10:30:00 UTC` (two-digit year, see below)                              |
| RFC 822 / 1123    | `15 Mar 24 10:30 UTC` (two-digit year, see below), `Fri, 15 Mar 2024 10:30:00 UTC`        |
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

### Words around a date

A textual format finds its fields by scanning, so words it does not read can sit
between them. `Fri, 15 Mar 2024 10:30:00 +0000` has a weekday name, `the 15th of
March 2024` has an article and an `of`, and `March 15, 2024 at 10:30` has an
`at`. All three parse, and so does `invoice 15 March 2024 paid`.

**A word that changes what the date means is refused instead.** Selectors
(`last`, `next`, `this`), ordinals (`first` through `fifth`), boundaries
(`beginning`, `start`, `end`), the relative words and the unit names all decide
a date, and a format that dropped them would answer a different day than the one
written:

```go
_, err := dateparsa.Parse("first monday of march 2024")
// *ParseError wrapping ErrNoMatch: the first Monday is the 4th, and
// MONTH_YEAR would have answered the 1st
```

An error is the answer, not a guess: `last day of february 2024`,
`end of march 2024` and `third thursday of november 2024` are all refused. The
cost is that free text carrying one of those words is refused too, so
`Last modified: March 15, 2024` no longer parses. Nothing in the sentence tells
that case apart from the ones above.

The check reads whole words, so it never fires on a language whose date
separator is a unit name: `2024年3月15日` is unaffected.

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
   `Layout.Reusable()` says the same thing to a caller holding the layout
   themselves, and says it for the format rather than for the row that produced
   it: `13/01/2024` needed no guess and its layout is still the one that meets
   `01/02/2024` two rows later.

Each interpretation is labelled with the reading it carries, and the labels name
the ordering rather than the separator: `MM/DD/YYYY`, `DD/MM/YYYY`, and
`MM/DD/YY` where the year is written with two digits.

Most ambiguous inputs have two readings. One has three: with
`WithPreferYearFirst`, a date whose three parts are all small leaves the year's
position open as well as the month's, so `01/02/03` is `YY/MM/DD` 2001-02-03,
`MM/DD/YY` 2003-01-02, or `DD/MM/YY` 2003-02-01. Strict mode returns all three,
with the one the preferences chose first.

A month name and a bare number is the other ambiguous shape, and it is reported
the same way. `March 15` is the fifteenth of March or March 2015, because
`March 32` can only be a year and nothing about `15` says which was meant:

```go
r, _ := dateparsa.Parse("March 15")
fmt.Println(r.Ambiguous) // true — read as the day, and that was a choice

_, err := dateparsa.ParseWith("March 15", dateparsa.WithStrictMode(true))
// *AmbiguousDateError: MONTH_DAY 2026-03-15 and MONTH_YEAR 2015-03-01
```

An ordinal suffix settles it and reports nothing: `March 15th` is a day, since
no year is written `15th`.

Two numbers beside a month name ask the same question, since whichever one is
the year the other is the day, and both readings are real dates:

```go
r, _ := dateparsa.Parse("01MAY10")
fmt.Println(r.Time)      // 2010-05-01, the first of May 2010
fmt.Println(r.Ambiguous) // true: 2001-05-10 reads the same bytes

_, err := dateparsa.ParseWith("01MAY10", dateparsa.WithStrictMode(true))
// *AmbiguousDateError: DAY_MONTH_YEAR 2010-05-01 and YEAR_MONTH_DAY 2001-05-10
```

A number over 31 is not a day, so it settles both slots and reports nothing:
`70MAY10` is the tenth of May 1970 and nothing else. A four-digit year settles
them for any value, which is why `01 May 2010` and every RFC 2822 date are
unaffected.

**RFC 822 and RFC 850 write a two-digit year, so they report it.**
`15 Mar 24 10:30 UTC` is the fifteenth of March 2024 or the twenty-fourth of
March 2015, and nothing in the bytes says which. The lenient path still reads it
day-first; strict mode refuses it. The weekday in the RFC 850 form would settle
it, and weekday names are skipped without being read.

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

**One machine, and it is rented.** Every number below is a Compute Engine
`c4-standard-8`, `linux/amd64`, an Intel Xeon Platinum 8581C (Emerald Rapids) at
2.30GHz, Debian 13, Go 1.26.6. Simultaneous multithreading is off, so the eight
vCPUs are four physical cores with no sibling sharing one. The turbo clock is
held at its all-core value rather than boosting with however many cores happen
to be busy. The measurement runs on cores 1 to 3 at `GOMAXPROCS=3`, with core 0
left to the kernel and the interrupts. Every figure is the median of the ten
runs in `benchmarks/baseline.txt`, and `benchmarks/baseline.env` records the
instance that produced them, down to the zone, the image and the kernel.
`make bench-cloud` rents that machine again and measures the same way;
**Regression tracking** below says what it costs and how it is torn down.

Spread across the ten runs has a median of 1.2%. Six of the 66 benchmarks exceed
4% and the worst is 6.5%, so a difference smaller than a few percent is the
machine and not the code. Those are the numbers to hold a claimed improvement
against, and `make bench-cloud` prints benchstat's own confidence intervals.

It was an Apple M2 Max until 2026-08-19. The reason for moving is the history of
this section: the tables were once split across two machines and labelled as
such, `benchmarks/baseline.txt` was deliberately not overwritten with the second
machine's numbers because retargeting the committed reference would have made
every later comparison print deltas that were really the difference between two
machines, and the split was then closed by re-running everything on the laptop.
That holds until the laptop is busy, or warm, or replaced. A rented machine is
the same machine on every run and is nobody's laptop.

None of the move is a claim about the code. Every ns/op below is larger than the
M2 Max figure it replaces, by between a quarter and a half, because a 2.3GHz
Xeon is not an M2 Max. The exception is the `time.Parse` baseline, which is 57%
larger, and that is worth knowing because it is why two ratio columns moved in
this library's favour without a line of code changing.

One thing did change on its merits. The `flextime` table is measured in the same
run as everything else now, and its allocation counts agree with what that table
already claimed. The previous `benchmarks/baseline.txt` disagreed with it, and
the baseline was the wrong one: it predated the commit that fixed those
benchmarks, which had been reporting the cost of boxing a `time.Time` into an
`any` rather than the cost of the method they name.

**Allocs** is the one column that does not depend on the machine, an allocation
count being a property of the code. It comes from the same run as everything
else, the `flextime` table included.

The zero in it has two exceptions. A timezone offset is answered from a table of
`*time.Location` built at init, at 15-minute granularity out to 14 hours, which
covers every offset in use today. An offset off that grid, `+05:53` for Bombay
before 1955 or `-00:44` for Monrovia before 1972, is built on first sight and
cached: three allocations for the first row of such a column and none for the
rest.

The second is `Layout.ParseBytes`, which is not a row in the table. It copies its
argument to a string, and the runtime answers that out of a stack buffer for 32
bytes or less and out of the heap above it: one allocation a row for a format
wider than 32 bytes, and none for the rest, which is most of them.
`Layout.Parse` on a string you already hold allocates nothing at any length.

### Hot path (compiled Layout reuse)

Median of the ten runs in `benchmarks/baseline.txt`, on the machine named above.
These five spread at most 4.2% across those ten runs, which is the figure a
claimed improvement has to clear before it is an improvement.

| Operation                       | ns/op | Allocs | vs `time.Parse` |
| ------------------------------- | ----- | ------ | --------------- |
| `Layout.Parse` (compact date)   | 23.1  | 0      | 0.5x            |
| `Layout.Parse` (ISO date)       | 24.6  | 0      | 0.6x            |
| `Layout.Parse` (ISO datetime+Z) | 33.3  | 0      | 0.8x            |
| `Parser` (cached layout)        | 35.6  | 0      | 0.8x            |
| `time.Parse` (stdlib baseline)  | 43.9  | 0      | 1.0x            |

<details>
<summary><b>Against time.Parse on the same format, and what the fast path costs Detect</b></summary>

Both sides are given the format, so this is the fair comparison: `Compile` and
`time.Parse` each get a layout and parse the same string. `BenchmarkCompiledLayout_vs_Stdlib`
is the source, same machine and method as above.

| Format       | dateparsa | `time.Parse` |          |
| ------------ | --------- | ------------ | -------- |
| SQL datetime | 31.6 ns   | 125 ns       | **4.0x** |
| ISO date     | 24.4 ns   | 75.7 ns      | **3.1x** |
| US slash     | 24.9 ns   | 72.2 ns      | **2.9x** |
| RFC 3339     | 32.2 ns   | 42.3 ns      | **1.3x** |

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
and never runs the program, so it pays and does not collect. That percentage is
an A/B across two commits, measured on `linux/arm64` when the change landed
(137 ns to 160 ns), and it is not re-measurable here because only one of the two
commits is checked out. `Detect` measures 221 ns on the machine this section
names. `Parse`, which does run the program, is flat to slightly faster, and
anything that reuses the layout is a third to a half faster. For a library whose
reason to exist is parsing the second row through the ten-millionth with the
format found on the first, that is the right side of the trade.

</details>

<details>
<summary><b>Cold path: first call, bulk columns, and natural language</b></summary>

| Format               | ns/op | Allocs |
| -------------------- | ----- | ------ |
| Unix timestamp       | 110   | 1      |
| Compact date         | 174   | 1      |
| ISO ordinal          | 185   | 2      |
| ISO 8601 date        | 187   | 1      |
| SQL datetime         | 248   | 1      |
| ISO 8601 datetime    | 260   | 1      |
| ISO week date        | 302   | 2      |
| SQL datetime + frac6 | 307   | 1      |
| Ambiguous slash      | 318   | 2      |
| Textual month        | 494   | 2      |

One allocation is the `Layout` the call returns. A format the trie matches needs
nothing else, because its fields were built at init. A format that falls through
to a detector needs one more, holding the fields it worked out and the
definition that describes them, and that is the whole of the second column
above: it was four for a textual month and ten for `03/15/2024 10:30:00`, in
scratch slices that were dead before `Parse` returned.

These rows are the expensive path and are meant to be. Detection describes every
byte of its input and refuses a numeric part wider than the field that reads it,
which is more work than a detector that does neither, and it is what stops
`2024-02-30` becoming the first of March. It is paid once per column, and the
hot path table is what every row after the first costs.

#### Bulk (10M rows)

| Operation                     | Time  | Per row |
| ----------------------------- | ----- | ------- |
| `Layout.Parse` 10M rows       | 327ms | 32.7 ns |
| `Parser.ParseColumn` 10M rows | 459ms | 45.9 ns |

A column costs one detection and then a compiled parse per row. The difference
between the two rows is the `[]time.Time` that `ParseColumn` fills and returns;
`Layout.Parse` hands back one value and allocates nothing.

#### Natural language

| Expression           | ns/op | Allocs |
| -------------------- | ----- | ------ |
| "yesterday"          | 381   | 2      |
| "in 10 minutes"      | 447   | 2      |
| "next friday"        | 470   | 2      |
| "beginning of month" | 479   | 2      |
| "yesterday at 5pm"   | 508   | 2      |
| "3 days ago"         | 567   | 2      |

</details>

<details>
<summary><b>Database and JSON (flextime): allocations and what they cost</b></summary>

Measured in the same run as everything above, which it was not before: the
nanoseconds used to be omitted here because this table was counted on a
different machine from the rest of the section and would not have been
comparable. Every row is a benchmark in `flextime/bench_test.go`:

```bash
go test -run '^$' -bench . -benchmem ./flextime/
```

| Operation                               | ns/op | Allocs |
| --------------------------------------- | ----- | ------ |
| `FlexTime.Value`                        | 0.65  | 0      |
| `UnmarshalJSON`, `null`                 | 1.9   | 0      |
| `FlexTime.Scan`, `time.Time`            | 2.3   | 0      |
| `FlexTime.Scan`, `float64`              | 8.0   | 0      |
| `FlexTime.Scan`, `int64`                | 11.6  | 0      |
| `Scanner.Scan`, string                  | 38.3  | 0      |
| `FlexTime.Scan`, string                 | 38.5  | 0      |
| `UnmarshalJSON`, integer number         | 43.6  | 0      |
| `FlexTime.Scan`, `[]byte`               | 52.3  | 1      |
| `UnmarshalText`                         | 53.0  | 1      |
| `UnmarshalJSON`, string                 | 67.5  | 1      |
| `MarshalJSON`                           | 167   | 3      |
| `UnmarshalJSON`, number with a fraction | 241   | 2      |

Every row that parses is steady state, the second value of a format onward. The
first value of a format costs one more, the `Layout` its detection returns, and
that layout is what the values after it reuse.

A string column costs nothing per row. The string arrives as a string, so there
is nothing to copy, and the layout its first row was detected with is what the
rest are parsed by. `[]byte` costs one, the conversion to a string that Go
requires, and a text column arrives as `[]byte` from most drivers; a `Scanner`
given `[]byte` pays that same copy.

`Scanner` is not faster than a bare `FlexTime` field any more, and it is not
supposed to be. It is the only place parse options can be passed, because
`database/sql`, `encoding/json` and the text decoders all construct the value
themselves. Reach for it when a column is day-first or needs strict mode, not
for speed.

The numeric arms allocate nothing at all. An `int64` or `float64` goes straight
to the epoch reading, and a JSON number written without a fraction or an
exponent is read from its bytes. A JSON number carrying either is decoded
through `encoding/json` into a `float64` instead, which is the 2.

`UnmarshalJSON` on a string is one: the string the quoted body is copied into.
It was four. Two went when the body stopped being decoded through
`encoding/json`, which has nothing to do for a body with no escape in it: a
timestamp is printable ASCII, so the bytes between the quotes already are the
string, and a body carrying a backslash, an embedded quote, a control character
or any byte over 0x7f is handed to the decoder unchanged. The third was the
`Layout`, and it went when the JSON path started keeping one, the same as the
other two.

`MarshalJSON` is three: the formatted string, boxing it for `json.Marshal`, and
the buffer `json.Marshal` returns.

</details>

<details>
<summary><b>Against araddon/dateparse, the other detecting library</b></summary>

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

</details>

<details>
<summary><b>Reproducing these numbers, and the machine that produced them</b></summary>

The baseline is checked into `benchmarks/baseline.txt`. To check for
regressions on the machine in front of you:

```bash
make bench-compare
```

That runs every benchmark in the tree, root package and `flextime` both, into
`benchmarks/current.txt` and benchstats it against the baseline. It is the fast
check, and it is only as good as the machine: a laptop throttles, has a browser
open, and is not the machine it was three months ago, so a small delta from it
is as likely to be the machine as the change.

#### The repeatable one

```bash
make bench-cloud          # measure, print the delta, delete the VM
make bench-cloud-update   # the same, and promote the result to the baseline
make bench-cloud-reap     # delete anything a crashed run left behind
```

`scripts/bench-gcloud.sh` rents one Compute Engine VM, measures on it, and gives
it back. The point is that the machine is the same one every time:
`c4-standard-8`, which is a single CPU platform rather than whichever generation
the zone happens to have; SMT off, so no hyperthread sibling shares a core with
the benchmark; the turbo clock held at its all-core value instead of boosting
with whatever else is running; the measurement pinned to cores 1 to 3 with core
0 left to the kernel; `GOMAXPROCS` set explicitly, because Go writes it into
every benchmark name; and an exact Go toolchain, fetched through the module
proxy and verified against `sum.golang.org`. It measures a commit, not a working
tree, unless you pass `--dirty`. It runs `-count=10`, which on this
machine puts the median run-to-run spread at 1.2% and the worst at 6.5%.

The VM is deleted three ways, because one way is how you pay for a VM you
forgot: a trap covering Ctrl-C and any failure, `--max-run-duration` on the
instance so Compute Engine deletes it whatever happens to your shell, and
`make bench-cloud-reap` for anything that still slipped through. A run costs
roughly $0.30 and takes about twenty minutes, most of it the measurement. The
script's header comment says what each pin is for, and every knob is an
environment variable.

Two of those knobs exist because of what the first live run hit. A zone runs out
of a machine type, so `BENCH_ZONE_FALLBACKS` is a list and the run walks it: all
four `us-central1` zones refused a `c4-standard-8` within a minute of each other
on the day this landed. And a network that reaches `googleapis.com` does not
necessarily reach a VM's own address on port 22, so `BENCH_SSH_TRANSPORT`
defaults to `auto` and switches to `--tunnel-through-iap` after a minute of
silence, which needs no inbound path of its own. Set it to `iap` outright to
skip the wait. Neither knob changes what is measured: the CPU platform follows
from the machine type in every zone, and how the file gets copied back is not
part of the number.

`benchmarks/baseline.env` records the machine that produced
`benchmarks/baseline.txt`: instance type, CPU platform, kernel, Go version,
commit, and the date. It sits beside the numbers rather than inside them,
because benchstat reads `key: value` lines at the top of a benchmark file as
configuration and would split the table on a commit hash that differs between
the two files, which is the one thing that always differs. Once it exists,
`make bench-update` refuses to promote a local run over the baseline and says
so, since retargeting the committed reference to a different machine is the
mistake described at the top of this section. `make bench-compare` still runs
from a laptop and still warns, reading the `goos`/`goarch`/`cpu` header the two
files carry: read its allocs/op column, which is a property of the code, and
ignore its nanoseconds, which are a property of whichever machine you are on.

Promote a run in a commit that says what moved and why.

</details>

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

## How this was built

Built with AI assistance. I directed, reviewed, and accepted every part of it,
and I'm responsible for the result. Noted for transparency, not as a selling
point or an excuse.

## License

Apache License 2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).

Locale data is derived from the Unicode Common Locale Data Repository (CLDR) and
carries the Unicode License v3, reproduced in `NOTICE`.
