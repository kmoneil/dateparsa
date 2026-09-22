# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

Nothing yet.

## [0.2.0] - 2026-09-22

Almost all of this release is correctness: dates that parsed to the wrong time
with no error, found by the fuzzers and by the sweeps written after them. Every
entry under Fixed changes what some input parses to, or whether it parses at
all, and names the inputs that move. That is why this is 0.2.0 and not 0.1.1:
before 1.0 a breaking change is a minor bump, and this project counts a changed
parse result as breaking even when no signature moves, because the caller's
stored data is what records the difference. No exported name was added or
removed.

### Fixed

- **A reused layout read a zone offset as a year.** A skipped run accepted
  bytes it had not matched, so the layout detected from `MAY1 00:00 1000` read
  `MAY1 00:00+0000` as 0000-05-01, and the layout from `May 1 10:30:00, 2024`
  read `May 1 10:30:00 -0700` as the year 700 in UTC. `Parser` and the
  `flextime` caches gave the same answers. A skipped run is now checked byte by
  byte against the run it was detected from: inside a run that starts with a
  letter, letters, spaces and accented characters stand in for each other, so a
  weekday name still reuses for the next day's, and every other byte has to be
  the byte that was there. The same rule stops a layout from
  `MAY1 10:00  PM` reading `MAY1 10:00\t\tPM` as 22:00, which detection reads
  as 10:00. Rows punctuated differently from the first row, such as `, `
  against `; ` or spaces against tabs, no longer share a layout, and `Parser`
  re-detects them. `Parse` also refuses free text dense enough with punctuation
  that describing it takes more than 24 instructions, such as
  `a,b,c,d,e,f,g,h,i,j,k,l March 15, 2024`
- **A reused layout read its fields out of order.** The layout from `\x00MAY1`
  answered 2026-05-10 for `1MAY10`, which detection reads as 2010-05-01, and
  now refuses it. In the other direction, the layout from `1 May 2024` refused
  `12 May 2024` and now reads it as 2024-05-12, as `May 1, 2024` already did
  for `May 12, 2024`
- **A reused layout accepted month names from locales nobody configured.** A
  layout detected with no locale read `März 15, 2024` as the fifteenth of
  March, which `Parse` refuses. It accepts the month names of the locales it was
  detected with now, and a layout from `Compile` accepts English, as
  `time.Parse` does
- **A word that decides the day was skipped.** `first monday of march 2024` was
  2024-03-01, `last day of february 2024` was 2024-02-01 and
  `end of march 2024` was 2024-03-01. An input whose unread words include a
  selector, an ordinal, a boundary, a relative word or a unit name returns
  `ErrNoMatch` now, and a configured locale adds its own words to the set. That
  includes free text that happened to parse correctly, such as
  `Last modified: March 15, 2024`. Weekday names, month names, "of", "at" and
  words the library does not recognise still parse
- **A number before a colon was read as a year.** `May 2024 15:04:05` was
  2024-05-15 04:05:00 and is 2024-05-01 15:04:05, and `MAY70 12:00:00` was
  1970-05-12 00:00:00 and is 1970-05-01 12:00:00. An input whose first time
  number is neither an hour nor a four-digit year, such as `MAY1 24:00:00` or
  `MAY1 70:00:00`, is refused, as is `2024 MAY 15:04:05`. `Mar 2024 15:04`
  parses now, to 2024-03-01 15:04:00
- **Two different month names were settled by table order.** `mar 15 mag 2024`
  with the Italian locale was the fifteenth of March. It means Tuesday the
  fifteenth of May, which this library cannot read because it does not read
  weekday names, so it is refused now. The same month named twice, as in
  `mar, 15 mar 2024` in Spanish, still parses
- **A month or year shift overflowed the target month.** `1 month ago` from
  2024-03-31 was 2024-03-02 and is 2024-02-29, and `in 1 year` from 2024-02-29
  was 2025-03-01 and is 2025-02-28
- **Guesses that were not reported as guesses.** No instant returned outside
  strict mode moved. What changed is `ParseResult.Ambiguous`, and what
  `WithStrictMode` returns:
  - two two-digit numbers around a month name report a guess: `01MAY10`,
    `01 MAY 10`, `May 10, 24`, and RFC 822 and RFC 850 dates such as
    `15 Mar 24 10:30 UTC`, because a two-digit year is what both of them write
  - a short date whose parts are all small and whose first part has two digits
    reports its year-first reading: `31/12/24`, `15/06/09`, `17-1-01`
  - a first part over 31 settles the order and no longer reports a guess:
    `70/01/02`
  - `*AmbiguousDateError` carries the year-first reading of an all-small
    three-part date whether or not `WithPreferYearFirst` is set, so `01/02/03`
    in strict mode has three interpretations where it had two

### Changed

- **Surrounding whitespace is padding.** An input that differs from a supported
  format only by leading or trailing ASCII whitespace parses where it returned
  `ErrNoMatch`, such as ` 2024-03-15`, `2024-03-15T10:30:00Z\n` and
  ` 1710504800`. A layout from detection accepts the same padding, so a column
  that pads inconsistently reuses one layout. U+00A0, an interior double space
  and a layout from `Compile` are unchanged
- **`Layout.Reusable()` answers whether to keep a layout for a column**, and is
  false for an ambiguity-prone layout where it was true: the numeric slash, dot
  and dash formats whatever value detected them, `13/01/2024` included, and the
  textual formats whose number could be a two-digit year, such as `MAY70`,
  `March 32` and RFC 822. `Layout.Parse` on such a layout still succeeds, and
  `Parser` re-detects every row of those formats
- **A fixed-width format's layout is built once and shared.** For a format the
  signature trie recognises, that carries a year, parsed in UTC, `Parse` returns
  the same `*Layout` for every value, so a cold parse of those formats allocates
  nothing and takes about 45 percent less time, measured on the machine README's
  Performance section names. `Layout` is immutable, so the sharing is safe; do
  not read two equal pointers as "detected from the same input"
- **A timestamp with a zone costs what a UTC one costs.** `Layout.Parse`
  computes the instant for UTC, every numeric offset and the sixteen zone
  abbreviations it knows instead of calling `time.Date`. A `time.Location` from
  `WithTimezone` still goes through `time.Date`, because its offset depends on
  the instant
- **Testing**: the parse results of 1,377 inputs pinned in
  `testdata/corpus/golden.txt`; 28 fuzz targets across six packages, up from
  23; sweeps asserting that a cached layout agrees with detection over
  generated rows; bounds checks, inlining and escapes gated against a golden
  file; and a benchmark baseline measured on a rented Compute Engine
  `c4-standard-8`, which `benchmarks/baseline.env` records

### Known issues

Both were in 0.1.0 too, the second in a wider form.

- A zone written after a meridiem is dropped: `March 15, 2024 10:30 pm EST`
  parses to 22:30 UTC. A meridiem written after a tab or a comma is dropped the
  same way, so `March 15, 2024 10:30, PM` is 10:30
- Words that decide the day are refused at detection only. A `Parser` that has
  already cached the layout from `every monday of march 2024` answers
  `first monday of march 2024` as 2024-03-01, which is the answer 0.1.0 gave
  `Parse` for that input on its own

## [0.1.0] - 2026-08-19

The first release anybody can fetch. This file previously recorded `0.1.0` and
`0.2.0`, both dated 2026-04-04, and linked each to a GitHub release. Neither was
ever published: there was no remote, the two local tags were never pushed, and
both were deleted on 2026-08-13. Everything built so far is collected here.

`v0.0.1-rc.1` was published ahead of this one and is deliberately not recorded
as a version. It existed to run `.github/workflows/release.yml` against a real
tag once, because that workflow publishes through a third-party action that had
never executed and a tag cannot be moved afterwards. It carries no content this
release does not, and it is marked a prerelease, so it is not offered as the
version to use.

### Added

- **Auto-detection parsing**: `Parse()` and `ParseWith()` detect and parse a
  date without a layout string, across ISO 8601, RFC 2822, RFC 3339, RFC 850,
  ANSIC, SQL datetime, syslog, Common Log Format, US and European numeric,
  CJK, compact, week and ordinal dates, and Go's own time string format
- **`ParseTime()`**: returns `time.Time` directly when format metadata is not
  needed
- **`Detect()`**: identifies the format of a date string and returns a reusable
  `*Layout` without parsing
- **Compiled layouts**: `Compile()`, `MustCompile()`, and
  `CompileWithTimezone()` turn a Go reference layout such as `"2006-01-02"`
  into the same instruction-based executor detection produces
- **`Layout.Parse()` and `Layout.ParseBytes()`**: zero-allocation parsing once
  the format is known, at 33 ns/op against 44 ns/op for `time.Parse` with a
  known layout, and 23 ns/op for a compact date. Measured on the machine
  README's Performance section names, which is the one `benchmarks/baseline.env`
  records
- **Trie-based detection**: O(n) character-class signature matching with no
  backtracking, over the fixed-width members of the thirty-one supported
  formats, plus a cascade of special-case detectors for the variable-width and
  textual forms a fixed signature cannot describe
- **Epoch timestamps**: Unix seconds, milliseconds, microseconds, and
  nanoseconds, distinguished by range
- **Natural language**: relative expressions ("3 days ago", "next friday",
  "in 2 weeks"), written-out numbers, and half-unit expressions
- **Locale support**: 20 locales (EN, ES, FR, DE, IT, PT, NL, RU, ZH, JA, KO,
  AR, HI, PL, SV, DA, NO, FI, TR, UK) with localized month and weekday names
  and relative-date keywords, compiled into the binary with no runtime file
  loading
- **Parsing options**: `WithBaseTime`, `WithTimezone`, `WithPreferDayFirst`,
  `WithPreferYearFirst`, `WithPreferFuture`, `WithStrictMode`, `WithLocales`
- **`AmbiguousDateError`**: under `WithStrictMode`, returns every interpretation
  of a genuinely ambiguous date such as "01/02/2024" instead of guessing.
  Outside strict mode the guess is still reported, through
  `ParseResult.Ambiguous`
- **`ParseResult.Kind`**: categorizes a result as `KindAbsolute`,
  `KindRelative`, or `KindNow`
- **`Parser`**: a stateful parser that caches the last successful layout, for
  parsing many dates of the same shape
- **`dateparsa/flextime` subpackage**: `FlexTime`, a `time.Time` wrapper with
  automatic format detection for database and JSON integration
  - `sql.Scanner`: accepts `time.Time`, `string`, `[]byte`, `int64` (Unix
    seconds), `float64` (Unix seconds with a fraction), and `nil` for SQL NULL
  - `driver.Valuer`: returns `time.Time` natively, `nil` for SQL NULL
  - `json.Marshaler` and `json.Unmarshaler`: encodes RFC3339Nano, decodes
    quoted strings in any detectable format, numeric Unix timestamps, and
    JSON `null`
  - `encoding.TextMarshaler` and `encoding.TextUnmarshaler`
  - `flextime.Scanner`: a pre-configured scanner taking `WithPreferDayFirst`,
    `WithTimezone`, and `WithJSONFormat`
- **Zero dependencies**: `go.mod` declares no requirements, direct or indirect
- **Testing**: unit, integration, and format-coverage tests; benchmarks; 23
  fuzz targets across five packages, swept on every merge and nightly; a
  semantic round-trip generator running 31 formats at 1000 random dates each,
  which is what catches a parse that succeeds and returns the wrong time; an
  oracle asserting agreement with `time.Parse` in both directions; and a
  zero-allocation gate on `Layout.Parse` that runs on every commit

### Changed

- **Licensed under Apache 2.0.** The project had no licence file at all; the
  README said the word MIT and nothing else. Locale data is derived from CLDR,
  and `NOTICE` carries the Unicode licence that has to travel with it
- **Requires Go 1.26.** `go.mod` had declared 1.26.1 while the README promised
  1.23 and CI claimed to test 1.23 and 1.24, which no build could satisfy
- **`Compile` is documented as stricter than `time.Parse`.** A compiled layout
  reads fixed byte offsets and range-checks each field, so it refuses a
  single-digit hour where the layout declares two, and a `+24:00` zone offset
  that RFC 3339 does not permit. `time.Parse` accepts both by falling back to
  its general layout parser. Use `Parse` to have the width detected rather than
  declared

[Unreleased]: https://github.com/kmoneil/dateparsa/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/kmoneil/dateparsa/releases/tag/v0.2.0
[0.1.0]: https://github.com/kmoneil/dateparsa/releases/tag/v0.1.0
