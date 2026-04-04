# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-04-04

Initial release of dateparsa — a high-performance, zero-dependency date parsing
library for Go.

### Added

- **Auto-detection parsing**: `Parse()` and `ParseWith()` automatically detect
  and parse 60+ date/time formats without requiring a layout string
- **`ParseTime()` convenience function**: returns `time.Time` directly when
  format metadata is not needed
- **`Detect()` function**: identifies the format of a date string and returns a
  reusable `*Layout` for repeated parsing
- **Compiled layouts**: `Compile()`, `MustCompile()`, and `CompileWithTimezone()`
  parse Go time reference layout strings (e.g. `"2006-01-02"`) into a zero-alloc
  instruction-based executor
- **`Layout.Parse()` / `Layout.ParseBytes()`**: zero-allocation parsing once a
  format is known, via the compiled instruction executor
- **Trie-based format detection**: O(n) character-class signature matching for
  50+ fixed-signature formats
- **Structured format support**: ISO 8601 (dates, datetimes, fractional seconds,
  week dates, ordinal dates), RFC 2822/RFC 3339, US/European numeric formats,
  CJK dates, SQL datetime, Go time string format, and more
- **Epoch timestamp support**: Unix seconds, milliseconds, microseconds, and
  nanoseconds with automatic range detection
- **Natural language date parsing**: relative expressions ("3 days ago",
  "next friday", "in 2 weeks"), written-out numbers, half-unit expressions
- **Locale support**: 10 locales (EN, ES, FR, DE, IT, PT, NL, RU, JA, ZH) with
  localized month/day names and natural language keywords
- **Parsing options**: `WithBaseTime`, `WithTimezone`, `WithPreferDayFirst`,
  `WithPreferYearFirst`, `WithPreferFuture`, `WithLocales`
- **`AmbiguousDateError`**: returns both interpretations for genuinely ambiguous
  dates (e.g. "01/02/2024") instead of silently guessing
- **`ParseResult` with `Kind`**: categorizes results as `KindDate`, `KindDateTime`,
  `KindRelative`, or `KindEpoch`
- **Architecture documentation** (`ARCHITECTURE.md`): comprehensive guide to
  the codebase structure and design decisions
- **CI/CD**: GitHub Actions workflow, pre-commit hooks, linting, Makefile
- **Testing**: unit tests, integration tests, format coverage tests, gap coverage
  tests, benchmarks, panic fuzz targets (`FuzzParse`, `FuzzDetect`), and a
  semantic round-trip fuzzer (29 formats x 1000 random dates)

[0.1.0]: https://github.com/kmoneil/dateparsa/releases/tag/v0.1.0
