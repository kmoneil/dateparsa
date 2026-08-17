# Security

dateparsa exists to parse strings its caller did not write. Log lines, CSV
cells, API responses, form fields, filenames. Hostile input is not an edge case
here; it is the normal case, and it is the whole job.

Two things follow, and this document is about both. The library must never be
the reason a process crashes or stalls. And it must never return a time that is
confidently wrong, because a date decides an expiry, a retention window, a
billing period, and an audit boundary. A wrong date that parses cleanly is a
security problem wearing a correctness costume.

## Reporting a vulnerability

Email **kevin@oneil.xyz**. Please do not open a public issue for a
vulnerability.

You will get an acknowledgement within 3 working days. Fixed issues are
disclosed publicly within **90 days** of the report, sooner if a fix ships
earlier, later only by agreement with the reporter.

Include the exact input string, as bytes if it is not printable, and the options
passed to `Parse`, `ParseWith`, or `Compile`. An input that reproduces is worth
more than a description of one. If a fuzzer found it, the corpus entry is the
report.

## Supported versions

Pre-1.0. Until the v1.0 tag, only the tip of `main` receives fixes.

## Threat model

**Assumed hostile: every byte of the input string.** Any length, any encoding,
any content. Invalid UTF-8, embedded NUL bytes, unpaired surrogates encoded as
WTF-8, a megabyte of digits, a string that is 64 separators and nothing else.
The library either parses it, or returns a `*ParseError`. Those are the only two
outcomes. It does not panic, it does not read out of bounds, and it does not
return a time it cannot justify.

**Assumed trusted: the caller's options.** `WithTimezone`, `WithBaseTime`,
`WithLocales`, `WithPreferDayFirst`, and the rest are the program's own
instructions, and `MustCompile` panics on a bad layout by design because a
layout string is source code, not input. If a hostile party controls your
options, they control your program and dateparsa is not the boundary.

**Not in scope: what the caller does with a valid time.** A correctly parsed
date in the year 1900 is a correct answer. Range and plausibility checks belong
to the application, which is the only layer that knows what a plausible date is.

## The `flextime` boundary takes values that were never a string

This section exists because the threat model above says "every byte of the input
string" and `flextime` is where something other than a string arrives.

`flextime.FlexTime` implements `sql.Scanner` and `json.Unmarshaler`, so it is
handed whatever a driver or a JSON document produced: a `time.Time`, a `string`,
a `[]byte`, an `int64`, a `float64`, or nil. The string and byte-slice forms go
through `Parse` and everything above applies to them unchanged. The numeric forms
do not, and they are their own surface.

**A numeric timestamp is read at the precision its digit count names.** Ten to
twelve digits are seconds, 13 are milliseconds, 16 are microseconds, 19 are
nanoseconds, and a digit count naming none of those is refused. That is the same
table `Detect` applies to a string, deliberately: until 2026-08-17 the numeric
paths read every value as seconds whatever its magnitude, so `1710500000000` was
2024-03-15 as a string and **year 56173** as a JSON number, with no error. A
millisecond epoch is what `Date.now()` and `System.currentTimeMillis()` produce,
so this was not an edge case, and `README.md` advertised the struct field shape
that hit it.

**Every numeric value is range-checked against the same bound as a string.** About
3168 years either side of the epoch. `1e300` used to produce year 292277026596,
`-1e300` the same *positive* year, and `NaN` came back as 1970-01-01 with no error.

**NaN and the infinities are refused rather than converted, and this was
architecture-dependent.** Go leaves the result of an out-of-range float-to-int
conversion implementation-defined: arm64 saturates, amd64 yields the most negative
`int64`. So the same JSON document parsed to two different instants on two
different machines. The range is now checked against the float, before any
conversion, so no out-of-range conversion happens at all.

**A number with fewer than ten digits is read as seconds, where a string of the
same digits is refused.** The one deliberate divergence. A short string is far more
likely to be a compact date or a bare year, and reading `"20240315"` as a
timestamp is the mistake the range bound was introduced to prevent; a value a
schema has already typed as a number has no date reading to protect.

**Ambiguity is reported here too, and can be refused.** `FlexTime.Ambiguous()`
carries the flag `dateparsa.Parse` returns beside the time, on every path that
parses a string: `Scan`, `UnmarshalJSON`, `UnmarshalText` and `Scanner.Scan`. It is
false for a value that arrived already typed, and it is cleared on reuse, which
matters because `database/sql` hands the same `*FlexTime` to every row of a result
set.

Every one of those paths dropped the flag until 2026-08-17. A row from a database
or a field from a JSON body carried a guessed day with `Valid()` true and no way at
all to find out, which made the promise under *Ambiguity is reported, never hidden*
below true of the root package and false of the type this library recommends for
database models and JSON APIs.

**Refusing needs configuration, and configuration reaches a `Scanner` only.**
`flextime.NewScanner(flextime.WithStrictMode(true))` returns dateparsa's
`*AmbiguousDateError` for a value with more than one honest reading, and
`errors.Is` and `errors.As` both work through the wrapping. A `FlexTime` reached
through `encoding/json` or `database/sql` is constructed by the decoder, so there
is no moment at which a caller could attach an option to it; at that boundary,
checking `Ambiguous()` after the fact is the equivalent, and it is the reason the
accessor exists rather than only the option.

`ParseOption` and `MarshalOption` are separate types for the same reason. There
used to be one, and three of its four constructors silently did nothing on the
value they were most likely passed to:
`NewWithOptions(t, WithPreferDayFirst(true))` compiled, ran, and read US order on
every row. It no longer compiles.

## What is not possible, by construction

These are properties of the code, not of the test suite. Each one is checkable
by reading imports or a single constant.

**No regular expressions.** Nothing in the library imports `regexp`. Detection
is a single forward scan that maps bytes to one of six character classes,
followed by a walk of a 6-ary trie. There is no backtracking anywhere in the
structured path, so there is no catastrophic backtracking to trigger. This is
the difference that matters most against regex-driven date parsers, where a
crafted input is a stall.

**No network.** Nothing imports `net`, `net/http`, or anything that dials.
Parsing a date never leaves the process.

**No filesystem.** Nothing imports `os` or `io`, and nothing reads a file.
Timezone resolution used to be an exception and is not any more; see below.

**No `unsafe`, no `reflect`, no `os/exec`.** The extraction path is ordinary Go
that the bounds checker can see. `Program.ExecuteBytes` converts to a string and
copies rather than aliasing the caller's slice through `unsafe.String`, which
costs an allocation on that path and is why the byte slice a caller passes can
never be mutated through a returned value.

**No dependencies.** `go.mod` has no `require` block, direct or indirect.
Nothing but the Go standard library reaches your binary through this library, so
there is no transitive dependency to audit and nothing here for a compromised
upstream package to ride in on.

**No global mutable state a caller can observe.** Locale data registers itself
through `init()` and is read-only from then on.

Three caches are built lazily beside it, because deriving their contents at
`init()` would pay for twenty locales a caller may never configure: the month
spelling tables in `internal/detect`, the phrase tables in `internal/natural`,
and the month index in `internal/locale`. Each is written once per key behind a
`sync.Map` or a `sync.Once`, holds nothing but data derived from the compiled-in
locale tables, and is never modified after it is stored. Two goroutines racing
to fill the same key build equal values and one of them wins, so what a reader
gets does not depend on who got there first. No input a caller passes reaches
any of them as a key.

`Layout` holds a value-type `Program`
and no pointers into caller memory, which is why it is safe to share across
goroutines. `Parser` holds one piece of mutable state, the cached layout, in an
`atomic.Pointer[Layout]`; the layout it points at is immutable, so a reader is
never handed a half-written value and sharing a `Parser` across goroutines is
safe. It used to be a plain field and a documented constraint on the caller.

## Bounds

Work is bounded, and the bounds are constants you can read.

**The signature buffer is 64 bytes.** `detect.Scan` classifies at most
`maxSigLen = 64` bytes into a stack-allocated array. Input longer than that is
truncated for signature purposes. The signature scan and the trie lookup on it
therefore do a fixed amount of work regardless of input size, and allocate
nothing.

The fallback detectors behind the trie are linear in input length, and always
were: `detectTextualMonth` scans the whole string for numeric tokens. Since
`91e9f7a`'s successor, one byte scan past the signature buffer decides whether
that detector is offered the input at all, on the path where the trie already
missed. Everything on that path was linear before and still is, and a program still
cannot address past byte 255, so a long input is refused rather than parsed slowly.

This used to add "nothing on it allocates per byte", which is not true.
`buildTextualFields` grows a slice of 24-byte tokens, one per run of digits, so an
input of alternating digits and spaces allocates about twelve bytes per input byte on
this path. It is linear and it is bounded by nothing here: the refusal comes later,
when a field lands past byte 255. That is a smaller number than the one the natural
language paragraph below records and it is the same shape, and it is not fixed.

**A program is at most 24 instructions.** `compile.MaxInstructions` is 24 and
the compiler **refuses** a format needing more, rather than stopping there.
Execution is a single pass over that fixed array with no loop whose trip count
depends on the input.

The refusal is the part that changed. The compiler used to stop filling and
return the truncated program with no error, so
`Compile("The current date and time: 2006-01-02")` returned a layout that
answered year zero for every input: `ParseGoLayout` emits one instruction per
unrecognised layout byte and 27 of them exhausted the count before the first
field. Detection is unaffected, and never emits more than 16 fields.

A format whose fields all sit at fixed offsets is executed without the loop at
all. `planFast` proves at compile time that the fields tile the input exactly,
using a 64-bit map with one bit per input byte, and records the single length
the program describes; `executeFast` then reads the fields at constant indices
in a straight line. The bound that comes with it is `fastMaxWidth`, which is 64:
a format describing more than 64 bytes keeps the interpreter, which has no such
bound. No input is refused because of it, and the two paths are held to the same
answer by `TestFastAgreesWithInterpreter` and
`FuzzFastAgreesWithInterpreter`.

Both paths still prove the program covers the whole input, and the proof moved
rather than weakened. The interpreter sums the widths it reads and compares the
sum and the maximum against the input length, which a gap and an overlap of the
same size defeat between them because they cancel in a sum. The bitmap cannot
cancel, so a fast program is checked more strictly, once, when it is built.

**A program addresses at most 256 bytes of input.** `Inst.Offset` and `Inst.Len`
are single bytes, and the compiler refuses a format with a field past what they
hold rather than narrowing it. Before that check, a field at offset 260 read
byte 4.

**The natural language path refuses an input over 512 bytes.**
`natural.MaxInputLen` is checked before anything is lowered or tokenised, so an
over-long input costs one comparison. It runs only after structured detection and
epoch detection have both failed, so no date format is affected: 512 bytes admits a
compound relative expression of about fifty terms, which is far past anything anybody
writes, and no shorter input is refused that used to parse.

**This paragraph used to say the path had "no amplification" and that "a 10MB string
of words costs roughly 10MB of work", and both were wrong by two orders of
magnitude.** It is worth saying what was measured, because that sentence was what a
reader would have sized their own cap from. `natural.Token` is 104 bytes and the
scanner appended one per whitespace-separated word, and Go grows a large slice by
about 1.25x, so the intermediate arrays came to roughly five times the final one. On
linux/arm64, Go 1.26.4:

    1 MiB of words        135 MB peak heap, 281 MB allocated, 190 ms
    4 MiB of words        665 MB peak heap, 1.35 GB allocated
    16 x 1 MiB, at once   1.94 GiB peak heap
    100 KB, 20 locales    348 MB allocated

The last row is the per-locale cost: `ScanLocale` lowered its own copy of the input
and built its own token slice once per configured locale. With the bound in place all
four are a comparison and a return, and a 1 MiB input allocates 48 bytes.

**The CPU cost of a long input is unchanged and is not this bound's to fix.** A
megabyte that reaches the fallback detectors still takes about 35 ms, because
`detect` scans it: `hasLetterPastSignature` walks the tail, and `findMonthNameCI`
tries 24 English spellings and each is linear. That is the "linear in input length"
property above, with a constant of roughly 35 ns a byte, and it was measured at 51 ns
a byte on 2026-08-14 before two performance changes moved it.

**Bound the input length yourself anyway.** The 512-byte bound is on one path, after
two others have already done work. If you are parsing strings that arrive over a
network, cap them before calling `Parse`. No real date is longer than about 64 bytes.

## The failure that matters: a confident wrong answer

A panic is loud and gets fixed. A date that parses to the wrong day is silent
and gets stored.

**Ambiguity is reported, never hidden.** `01/02/03` has more than one honest
reading. `ParseResult.Ambiguous` is `true` when the answer came from a
preference rule rather than from the input. `WithStrictMode(true)` refuses to
guess at all and returns an `*AmbiguousDateError` carrying every interpretation,
so the application decides. **If you are parsing dates that cross a trust
boundary and a wrong day has consequences, use strict mode.** The default exists
for convenience, not for safety.

**Every interpretation names the reading it carries.** That is the half of the
promise above that was not kept until 2026-08-17. The error was built by
detecting the input a second time with the day-first preference flipped, and two
of the three heuristics ahead of that preference can overrule it, so for two
whole shapes both detections came back the same and the caller was handed two
copies of one reading:

- A month name and a number. `ParseWith("March 15", WithStrictMode(true))`
  returned the fifteenth of March twice, labelled `MM/DD/YYYY` and `DD/MM/YYYY`
  for an input that is not a numeric date, and the reading it was choosing
  between, March 2015, was not in the error at all.
- A dot-separated date, where the separator forces day-first.
  `ParseWith("03.02.2024", WithStrictMode(true))` returned the third of February
  twice, and one copy was labelled `MM/DD/YYYY`, which reads as the second of
  March. A caller filtering the interpretations by label got a European reading
  under an American label.

A caller who wrote the obvious guard, that two interpretations agreeing means
the guess was safe to take, was worse served than one who ignored the error.
Both readings are now built from the format that was detected, by re-reading the
one or two fields the question is about, so a label cannot disagree with the
instant beside it. An input whose second reading does not parse, such as
`March 00`, is refused rather than returned as a single interpretation: strict
mode may refuse more than the lenient path and never accepts more.

**A word can be ambiguous as well as a number.** Hindi writes both yesterday and
tomorrow as `कल` and tells them apart with the verb, which a date string does not
have. That word now reports `Ambiguous` and refuses under strict mode with both
days attached, the same as `01/02/03`. It did neither until 2026-08-17: the
natural-language path built its result without ever setting `Ambiguous`, and the
strict-mode check sat after that path returned, so no natural-language parse was
ever tested against it. `कल` came back as tomorrow, alone, with no error.

That answer was not chosen either. Five phrases across the twenty locales are
spelled the same as another phrase in the same locale, and which one won was
decided by `sort.Slice`, which is not stable, so a toolchain upgrade could move a
date with no line changing. Four of the five are two different kinds of token and
the grammar tells them apart; those are settled by an explicit rank now. The
fifth is `कल`, where both readings are the same kind and nothing can tell them
apart, so both are carried and the caller is told.

**A guess is never reused across rows.** `Parser` caches the layout it detected
and skips detection for later values, which is the whole reason it exists. It
does not do that for a format detection resolved by looking at the values, and
the reason is that the readings it chose between compile to the same program:
the same fields, the same widths, at the same offsets. A layout built from
`25/12/2024` is day-first because 25 cannot be a month, and it parses
`01/02/2024` perfectly well as the first of February where the format's own
preference rule reads the second of January. A layout built from `MAY70` reads
the 70 as a year, and it parses `MAY10` as 2010 where detection reads the tenth
of May. Both answers are wrong days returned with no error and no flag, so any
layout detection marks ambiguity-prone is re-detected per value rather than
reused. It costs the cache on those formats and on no others.

**Correctness is fuzzed semantically, not only for panics.** `FuzzParse` and
`FuzzDetect` prove the library does not crash. They cannot prove it returns the
right time, because a wrong answer is a successful parse. `roundtrip_test.go`
covers that gap: it generates random times, renders them in each supported
format, parses them back, and compares against the original, 29,000 times
deterministically on every `go test`. A format that enters the trie without a
round-trip spec is a format nothing checks for correctness, and that is treated
as a defect.

**Every extraction primitive checks bounds and returns `(value, ok)`.**
`parse2Digits`, `parse4Digits`, `parse1or2Digits`, `parseFracSec`, and
`parseTZOffset` each validate before reading. None of them assume a caller
checked the length, because the compiled offsets come from a format definition
and the input does not have to agree with it.

## Timezone resolution reads nothing

`lookupTZAbbr` resolves a closed set of fifteen abbreviations from pre-built
fixed-offset locations: `UTC`, `UCT`, `GMT`, `EST`, `EDT`, `CST`, `CDT`, `MST`,
`MDT`, `PST`, `PDT`, `HST`, `CET`, `EET`, `MET`, `WET`. No allocation, no I/O,
and no name outside that list is accepted.

**This section used to describe a filesystem read, and it is worth saying what
changed.** Anything not in the pre-built table fell through to
`time.LoadLocation`, which reads the system timezone database, does not cache,
and reads whatever `ZONEINFO` points at. A stream of inputs carrying varied
abbreviations turned into a stream of file reads, and `Layout.Parse` allocated
24 times per call on `CET`, `EET`, `MET` and `WET` while the README promised
zero.

The fallback reached exactly nine names beyond the pre-built ten, which was
established by asking `LoadLocation` about all 17576 three-letter strings rather
than by reasoning about tzdata. Six of them are listed above now. The other
three were `PRC`, `ROC` and `ROK`, tzdata aliases for countries rather than
timezone abbreviations, and they are refused.

This removes the only place where hostile input converted into I/O, and the only
reason this library ever touched the filesystem.

## Timezone abbreviations are ambiguous, and the resolution is a policy

`CST` is US Central Standard Time, China Standard Time, and Cuba Standard Time.
dateparsa picks the US reading. This is documented in `ARCHITECTURE.md` under
known limitations, and it is repeated here because it is a correctness decision
that looks like a parsing detail: input from a non-US source can parse to a time
14 hours from what its author meant, with no error and no ambiguity flag.

Every abbreviation is a **fixed offset, taken as written**, and daylight saving
is never applied on the caller's behalf. `CET` is +01:00 in July as well as in
January; a caller who means the summer offset writes `CEST`. Four names did not
follow that rule until the pre-built table absorbed them, because they are
tzdata zone names as well as abbreviations: `"2024-07-15 10:30:00 CET"` resolved
through the zone's daylight rules and came back as +02:00 CEST, an hour from
what the input said, while `EST` in the same position stayed -05:00.

Use `WithTimezone` to state the assumption your data actually carries.

## What this library does not defend against

Said plainly, so it is not discovered later.

- **Unbounded input.** See above. Cap it at the caller.
- **A wrong but well-formed date.** `2024-02-30` is rejected, but `1970-01-01`
  in a field that should hold a recent timestamp parses fine. Range checks are
  the application's.
- **Concurrent use of `Parser` costs cache hits, not correctness.** Sharing one
  across goroutines is safe: the cached layout is an atomic pointer to an
  immutable `Layout`. What sharing costs is hits, because goroutines parsing
  different formats evict each other, and every miss is a full detection. A
  `Parser` per column is still the arrangement that parses fastest, and
  `Layout.Parse` is still the floor. `go test -race` runs in CI for this
  library's own tests, not for yours.
- **Locale data correctness.** The month and weekday names in
  `internal/locale/data/` are derived from CLDR and are not independently
  verified per language. A wrong abbreviation in a locale means a failed parse
  or, in the worse case, a wrong month.
- **The system clock.** Relative expressions resolve against `time.Now()` unless
  `WithBaseTime` is given. Parsing "yesterday" on a host with a wrong clock
  returns a wrong date, correctly.

## Supply chain

Zero dependencies is the whole strategy, and it is why the `go.mod` invariant is
enforced rather than encouraged. There is no transitive tree to audit, no
proxy-fetched module to verify beyond the toolchain itself, and no upstream
maintainer whose account compromise becomes this library's problem.

CI runs the test matrix across three Go versions and three operating systems,
runs both fuzz targets for 30 seconds each, and fails a build whose linked
binary exceeds 10MB. The size budget is a supply chain control as much as a
performance one: a sudden jump means something got linked in that nobody
intended.

The locale data files are generated and carry a `DO NOT EDIT` header, and the
generator is not in this repository. Until it is committed, the data cannot be
independently regenerated and compared against upstream CLDR, which is a real
gap in the chain of custody for the only third-party material here. It is
recorded in `_CLAUDE_.md` as work to be done.

## Keeping this current

Update this document in the same change whenever you:

- import a package from the standard library that touches the filesystem, the
  network, the environment, or `unsafe`
- add a dependency to `go.mod`
- change `maxSigLen`, `MaxInstructions`, or any other bound named above
- add a code path whose cost is not linear in input length
- change how ambiguity is detected, reported, or resolved
- change the timezone abbreviation table or the fallback in `lookupTZAbbr`
- change what an exported type promises about concurrent use
