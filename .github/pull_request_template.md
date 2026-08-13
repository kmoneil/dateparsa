<!--
The subject of the squashed commit comes from this PR's title, so write it as a
Conventional Commit: `<type>(<scope>): <subject>`, imperative, under 72
characters, no trailing period.

The body of the squashed commit comes from the commit messages on the branch,
not from this description, so write those with the same care: say what was
wrong and why the fix is shaped the way it is. This page is for a reviewer who
has the diff in front of them. The commit message is for somebody reading
`git log` in two years who does not.
-->

## What was wrong

## Why the fix is shaped this way

## Checks

<!--
Delete the rows that do not apply. The ones left should be true, and CI checks
most of them. This list is for the two or three it cannot.
-->

- [ ] `make ci` passes locally, or CI is green.
- [ ] **Zero-alloc held.** If this touched the detect, compile, or execute path,
      `make alloc` passes. An escape analysis change three packages away can
      break it.
- [ ] **A new format carries a round-trip spec.** A trie entry added without one
      in `roundtrip_test.go` is a format nothing checks for correctness. Panic
      fuzzing cannot catch a wrong answer, because a wrong answer is a
      successful parse.
- [ ] **A parse-result change is breaking.** If any input now parses to a
      different time, the commit is marked `!` with a `BREAKING CHANGE:` footer
      naming which inputs moved, even when no signature changed. The caller's
      database is what records the difference.
- [ ] **Performance claims carry numbers.** Before and after benchstat output in
      the commit body, from a real machine and not a shared runner. "No
      regression" with nothing under it is not reviewable.
- [ ] `make fuzz` run, if this touched anything that scans, classifies, or
      extracts, and any crasher Go wrote to `testdata/fuzz` is committed with
      the fix **and** added as an `f.Add` seed.
- [ ] **A new invariant has a test.** If this fixes a bug a gate should have
      caught, the gate is extended in the same change.
- [ ] `SECURITY.md` updated, if this added an import that touches the
      filesystem, the network, or the environment, added a dependency, changed
      a bound (`maxSigLen`, `MaxInstructions`), or changed how ambiguity is
      resolved.
