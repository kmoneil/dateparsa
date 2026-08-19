# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

Nothing yet.

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

[Unreleased]: https://github.com/kmoneil/dateparsa/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/kmoneil/dateparsa/releases/tag/v0.1.0
