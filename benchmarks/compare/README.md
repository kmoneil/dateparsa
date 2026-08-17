# dateparsa vs araddon/dateparse

`github.com/araddon/dateparse` is the other Go library that detects a date
format rather than being told one, so it is the only meaningful comparison for
what dateparsa does. This module measures both on the same inputs.

```
make bench-vs         # the benchmarks (downloads araddon; not part of make ci)
make bench-vs-check   # the correctness tests, including where the two disagree
```

## Read this before the numbers

**araddon/dateparse is unmaintained.** The version measured is
`v0.0.0-20210429162001-6b43995a97de`, April 2021, and the repository is
archived. Being faster than an abandoned library is a weak claim on its own,
and the interesting content here is the shape of the difference, not the
margin.

**This is one machine and one run.** `linux/arm64`, Go 1.26.4, 10 cores,
median of 6 runs at `-benchtime=300ms`. Every number below reproduces with
`make bench-vs`. They are not the M2 Max figures the root README quotes and
should not be read against them.

**dateparsa does not win everything.** It is behind on 7 of 16 formats when
each value is parsed cold, and behind on the two text-shaped misses. Those
sections say so and say why.

## Why this is a separate module

The library has zero module dependencies and `README.md` promises exactly that.
A `require` for another parser would land in every downstream binary and every
downstream vulnerability scan, so this lives in its own module, where:

- `go build ./...`, `go test ./...`, `go vet ./...` and `make ci` at the root do
  not reach it, because `./...` stops at a nested `go.mod`
- `go list -m all` at the root does not list araddon, so govulncheck's graph is
  unchanged
- the module proxy excludes it from the parent's zip, so `go get` on the library
  never fetches it

It is deliberately not part of `make ci`: it downloads a module, and a gate that
reaches the network fails when the network does.

## Three different questions

The two libraries have the same job and a different design, and which one is
faster depends entirely on which question is asked.

### One-shot: every value pays detection

The comparison that matches how araddon is used, and how dateparsa is used by a
caller holding a single date. `dateparsa.Parse` against `dateparse.ParseAny`.

| Format               | dateparsa | araddon |       | allocs d/a |
| -------------------- | --------- | ------- | ----- | ---------- |
| unix_seconds         | 87.2 ns   | 140.2   | 1.61x | 1 / 3      |
| ISO8601_date         | 157.7 ns  | 251.4   | 1.59x | 1 / 3      |
| RFC3339_nano         | 223.4 ns  | 342.6   | 1.53x | 1 / 3      |
| SQL_datetime         | 191.2 ns  | 279.4   | 1.46x | 1 / 3      |
| SQL_datetime_frac    | 203.1 ns  | 296.8   | 1.46x | 1 / 3      |
| ISO8601_datetime     | 196.0 ns  | 283.0   | 1.44x | 1 / 3      |
| ISO8601_datetime_Z   | 209.3 ns  | 278.2   | 1.33x | 1 / 3      |
| compact_date         | 145.9 ns  | 189.0   | 1.30x | 1 / 5      |
| RFC3339_offset       | 216.1 ns  | 387.9   | 1.80x | 1 / 6      |
| **textual_month**    | 387.1 ns  | 348.1   | 0.90x | 4 / 7      |
| **ANSIC**            | 660.3 ns  | 481.2   | 0.73x | 6 / 7      |
| **textual_month_abbr** | 390.7 ns | 277.4  | 0.71x | 4 / 4      |
| **US_slash**         | 263.6 ns  | 185.0   | 0.70x | 4 / 3      |
| **day_month_year_text** | 367.9 ns | 221.8 | 0.60x | 5 / 4      |
| **RFC1123**          | 744.5 ns  | 363.2   | 0.49x | 7 / 3      |
| **US_slash_time**    | 585.3 ns  | 264.4   | 0.45x | 10 / 3     |

Ahead on 9, behind on 7. The split is not random: everything dateparsa wins is
a format its trie matches on a character-class signature, which costs one scan
and one allocation. Everything it loses is a format that misses the trie and
falls through to a fallback detector, and those allocate a `[]Field` per call.
`US_slash_time` allocates ten times and is the worst row in the table.

Two of the losses are paid for something araddon does not do. `US_slash` and
`US_slash_time` are `DD/MM` against `MM/DD`, and dateparsa compiles both
readings so it can report which one it guessed; `ParseResult.Ambiguous` is that
answer. araddon defaults to month-first and moves on.

The rest, the textual and RFC1123 rows, are not paying for anything. They are
the fallback detectors allocating, and they are where this library has work
left.

### Reuse: the format is already known

Per row, once detection has happened. Each library gets its own mechanism:
dateparsa re-runs the compiled `Layout` it returned, araddon gets the Go layout
string from `ParseFormat` and runs `time.Parse`.

| Format               | dateparsa | araddon |       | allocs d/a |
| -------------------- | --------- | ------- | ----- | ---------- |
| RFC3339_offset       | 35.9 ns   | 183.1   | 5.10x | 0 / 3      |
| SQL_datetime_frac    | 28.5 ns   | 112.2   | 3.94x | 0 / 0      |
| RFC3339_nano         | 30.3 ns   | 117.7   | 3.89x | 0 / 0      |
| ISO8601_datetime_Z   | 24.6 ns   | 89.3    | 3.64x | 0 / 0      |
| SQL_datetime         | 24.4 ns   | 87.7    | 3.60x | 0 / 0      |
| ISO8601_datetime     | 24.3 ns   | 87.0    | 3.58x | 0 / 0      |
| ISO8601_date         | 17.9 ns   | 53.5    | 2.98x | 0 / 0      |
| US_slash             | 18.2 ns   | 53.2    | 2.92x | 0 / 0      |
| compact_date         | 17.7 ns   | 46.7    | 2.64x | 0 / 0      |
| US_slash_time        | 43.3 ns   | 87.1    | 2.01x | 0 / 0      |
| textual_month        | 31.8 ns   | 62.0    | 1.95x | 0 / 0      |
| day_month_year_text  | 31.0 ns   | 59.1    | 1.90x | 0 / 0      |
| RFC1123              | 70.5 ns   | 124.6   | 1.77x | 0 / 0      |
| textual_month_abbr   | 35.5 ns   | 60.3    | 1.70x | 0 / 0      |
| ANSIC                | 62.2 ns   | —       | —     | 0 / —      |
| unix_seconds         | —         | —       | —     | — / —      |

Ahead on all 14 that both can do, 1.7x to 5.1x, and zero allocations on every
row. This is the whole design showing up in one table: a compiled `Layout` is a
fixed sequence of byte offsets, while `time.Parse` re-reads its layout string on
every call.

The two gaps are araddon's, and `TestAraddonReuseGaps` pins them:

- **ANSIC**: `ParseFormat("Mon Jan  2 15:04:05 2006")` returns
  `"Jan  2 15:04:05 2006"`, dropping the weekday, so the layout it hands back
  does not re-parse the input it came from.
- **unix_seconds**: `ParseFormat("1710498600")` returns `"1710498600"`, the
  digits themselves, and `time.Parse` reads them as a reference time with a
  month of 17.

dateparsa answers the first with an ordinary reusable layout and the second with
`LayoutEpoch`, a sentinel that refuses to re-parse because an epoch has no
format to reuse. Refusing and returning something that does not work are
different answers.

### Column: the actual job

10,000 values in one unknown format, parsed end to end, detection included and
paid once by both. The destination slice is allocated outside the timer for the
two hand-rolled loops.

| Format               | dateparsa (Layout) | araddon  |       |
| -------------------- | ------------------ | -------- | ----- |
| ANSIC                | 618.3 µs           | 5030.8   | 8.14x |
| RFC3339_offset       | 361.8 µs           | 1985.7   | 5.49x |
| SQL_datetime_frac    | 287.6 µs           | 1142.4   | 3.97x |
| RFC3339_nano         | 305.2 µs           | 1180.8   | 3.87x |
| ISO8601_datetime_Z   | 251.7 µs           | 903.9    | 3.59x |
| SQL_datetime         | 247.8 µs           | 881.1    | 3.56x |
| ISO8601_datetime     | 249.1 µs           | 869.2    | 3.49x |
| ISO8601_date         | 184.4 µs           | 546.8    | 2.97x |
| US_slash             | 190.9 µs           | 533.4    | 2.79x |
| compact_date         | 177.3 µs           | 466.6    | 2.63x |
| textual_month        | 325.6 µs           | 618.6    | 1.90x |
| day_month_year_text  | 321.2 µs           | 592.4    | 1.84x |
| US_slash_time        | 505.5 µs           | 881.2    | 1.74x |
| RFC1123              | 712.9 µs           | 1234.7   | 1.73x |
| textual_month_abbr   | 367.6 µs           | 614.6    | 1.67x |
| unix_seconds         | 1468.5 µs          | 1517.0   | 1.03x |

Ahead on all 16, because one detection amortised over ten thousand rows is the
case the design is for, and the per-row cost is the reuse table above. The
formats dateparsa loses cold, it wins by the second row.

`unix_seconds` is the exception and is 1.03x, which is a tie. An epoch has no
reusable format, so `LayoutEpoch` refuses and every row pays full detection.
That is deliberate: re-running "1710498600" through a cached layout would be
re-running a number.

## Parser versus holding the Layout

`BenchmarkColumn` measures dateparsa two ways, and on the slash formats they are
not close:

| Format        | `Parser.ParseColumn` | holding the `Layout` |      |
| ------------- | -------------------- | -------------------- | ---- |
| US_slash      | 3528.7 µs            | 190.9 µs             | 18x  |
| US_slash_time | 7352.9 µs            | 505.5 µs             | 15x  |
| everything else | within 1.1–1.4x    |                      |      |

**The fast one is the less safe one, and that is why `Parser` is slower.**
`Parser.Parse` declines its cache for a format whose reading depends on the
value rather than the shape, and re-detects every row instead. The comment in
`parser.go` lists what that fixed: a `Parser` seeded with `"03/15/2024"`, which
is unambiguous because 15 cannot be a month, read `"01/02/2024"` through the
cached layout and reported `Ambiguous: false` where `ParseWith` reports true.
Worse, one seeded with `"MAY70"` read `"MAY10"` as 2010-05-01 where detection
reads the tenth of May.

So the 18x is the price of not silently returning the wrong day on an ambiguous
column, and a caller who holds the `Layout` by hand across `DD/MM` data is
opting out of that check. On every format whose fields are fixed by its shape,
which is every other row in the table, `Parser` costs nothing extra.

## Misses: values in a date column that are not dates

Every real import has empty cells, `N/A`, and free text somebody typed into the
wrong field. Neither library's own benchmarks measure them.

| Input                 | dateparsa | araddon |       |
| --------------------- | --------- | ------- | ----- |
| `""`                  | 112.4 ns  | 137.8   | 1.23x |
| `"12345678901234567890"` | 348.8 ns | 357.7  | 1.03x |
| `"N/A"`               | 379.5 ns  | 181.2   | 0.48x |
| `"not a date at all"` | 554.1 ns  | 231.4   | 0.42x |

dateparsa is roughly twice as slow on the two text-shaped misses, and the reason
is a feature: after structured detection and epoch detection both fail, it tries
natural language, which lowercases the input and builds a token slice. araddon
has no natural-language path to try, so it gives up sooner. A caller who does
not want `"3 days ago"` parsed is paying for a cascade they will not use, on
every miss.

## Agreement

`TestAgreement` runs every corpus entry through both and compares the instant.

**Zero disagreements on all 16 formats.** The two libraries return the same
time for every input measured here, which is the more useful result than any of
the timings: nothing in this corpus is a migration hazard in either direction.

That is not a general guarantee. The corpus is 16 formats chosen because both
libraries accept them, and the two resolve `DD/MM` against `MM/DD` by different
rules, so a corpus built to probe ambiguity would find differences by
construction. dateparsa reports the guess in `ParseResult.Ambiguous` and can
refuse it outright with `WithStrictMode`; araddon has `ErrAmbiguousMMDD` and
`ParseStrict`.

## What the corpus is, and is not

`Corpus` in `corpus.go` is 16 formats, fixed rather than generated so a number
reproduces. `TestBothParseTheCorpus` asserts both libraries accept every entry,
and `TestBothRefuseTheMisses` asserts both refuse every miss. Without those, a
format one library refused would quietly become a benchmark of one library
against the other's error path, which is cheaper than a parse in one and dearer
in the other, and a comparison of nothing.

It is therefore **not** a capability comparison. Formats only one side supports
are excluded by construction, and that is the axis where the two differ most:
dateparsa also does natural language, 20 locales, ISO week dates and ordinal
dates, none of which araddon attempts, and none of which appear above.
