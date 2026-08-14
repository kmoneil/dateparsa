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

**No filesystem, with one documented exception.** Nothing imports `os` or `io`.
The exception is timezone resolution, described below.

**No `unsafe`, no `reflect`, no `os/exec`.** The extraction path is ordinary Go
that the bounds checker can see. `Program.ExecuteBytes` converts to a string and
copies rather than aliasing the caller's slice through `unsafe.String`, which
costs an allocation on that path and is why the byte slice a caller passes can
never be mutated through a returned value.

**No dependencies.** `go.mod` has no `require` block, direct or indirect.
Nothing but the Go standard library reaches your binary through this library, so
there is no transitive dependency to audit and nothing here for a compromised
upstream package to ride in on.

**No global mutable state after init.** Locale data registers itself through
`init()` and is read-only from then on. `Layout` holds a value-type `Program`
and no pointers into caller memory, which is why it is safe to share across
goroutines. `Parser` caches a layout on itself and is documented as not safe for
concurrent use; that is a correctness constraint on the caller and not a
defended boundary.

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
missed. Everything on that path was linear before and still is. Nothing on it
allocates per byte, and a program still cannot address past byte 255, so a long
input is refused rather than parsed slowly.

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

**A program addresses at most 256 bytes of input.** `Inst.Offset` and `Inst.Len`
are single bytes, and the compiler refuses a format with a field past what they
hold rather than narrowing it. Before that check, a field at offset 260 read
byte 4.

**The natural language path is linear in input length and has no cap.** It runs
only after structured detection and epoch detection have both failed. When it
runs, the scanner allocates a lowercased copy of the input and grows a token
slice. There is no quadratic behavior and no amplification, but there is also no
internal limit: a 10MB string of words costs roughly 10MB of work.

**Bound the input length yourself.** If you are parsing strings that arrive over
a network, cap them before calling `Parse`. No real date is longer than about
64 bytes, and the library will not stop you from handing it a megabyte.

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

## The one filesystem read

`lookupTZAbbr` resolves ten common abbreviations (`UTC`, `GMT`, `EST`, `EDT`,
`CST`, `CDT`, `MST`, `MDT`, `PST`, `PDT`) from pre-built locations with no
allocation and no I/O. Anything else falls through to `time.LoadLocation`, which
reads the system timezone database.

What this means in practice:

- **Path traversal is not reachable.** `time.LoadLocation` rejects any name
  containing `..` or beginning with a slash or backslash before it touches the
  filesystem. The guard is the standard library's, not this library's, which is
  the reason the call goes through it rather than reading a path directly.
- **The results are not cached.** Go re-reads and re-parses the zone file on
  every `LoadLocation` call. A stream of inputs carrying varied timezone
  abbreviations therefore turns into a stream of file reads. This is the one
  place where hostile input converts into I/O, and it is the first thing to look
  at if parsing throughput collapses on adversarial data.
- **It reads whatever `ZONEINFO` points at.** That environment variable is part
  of the standard library's contract, and on a host where an attacker sets
  environment variables you have already lost the process.

An application that never wants this can avoid it entirely by parsing once and
reusing the returned `Layout`, which resolves the timezone at compile time.

## Timezone abbreviations are ambiguous, and the resolution is a policy

`CST` is US Central Standard Time, China Standard Time, and Cuba Standard Time.
dateparsa picks the US reading. This is documented in `ARCHITECTURE.md` under
known limitations, and it is repeated here because it is a correctness decision
that looks like a parsing detail: input from a non-US source can parse to a time
14 hours from what its author meant, with no error and no ambiguity flag.

Use `WithTimezone` to state the assumption your data actually carries.

## What this library does not defend against

Said plainly, so it is not discovered later.

- **Unbounded input.** See above. Cap it at the caller.
- **A wrong but well-formed date.** `2024-02-30` is rejected, but `1970-01-01`
  in a field that should hold a recent timestamp parses fine. Range checks are
  the application's.
- **Concurrent misuse of `Parser`.** Sharing one across goroutines is a data
  race. `Layout` is the concurrent-safe type. `go test -race` runs in CI for
  this library's own tests, not for yours.
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
