#!/bin/sh
# bench-ab.sh measures the working tree against a base commit, by running two
# test binaries alternately on a quiet machine.
#
# It exists because the same harness was rebuilt by hand three times in one
# session, and the cost of rebuilding it is why a card's number gets inherited
# instead of re-taken. Three cards in that session were justified by a number
# measured against code that had since changed; see P17, P24 and P25.
#
# Two things it does that a plain `go test -bench` before-and-after does not,
# both of which produced wrong answers on this hardware before they were added.
#
# ALTERNATING. Running all of A and then all of B attributes any drift in the
# machine to the change. A run done that way reported Parse_TextualMonth +17.9%
# and Layout_Parse_ISODate +7.6%; interleaved, at the same commit, the second
# was p=0.42 and had never been real. Both binaries see the same thermal state,
# the same neighbours and the same page cache, because they see them in turn.
#
# WAITING FOR QUIET. Another project's fuzz sweep took this box to load 27 and
# produced base-side spreads of 193%, which benchstat will happily compare. The
# wait is five consecutive one-minute readings under LOADMAX.
#
# It also refuses to present an answer it does not believe: a row whose spread
# exceeds SPREADMAX on either side is reported as unusable rather than as a
# result, because a percentage difference between two noisy medians is not one.
#
# Usage:
#   make bench-ab                      the working tree against origin/main
#   make bench-ab BASE=abc1234         against any commit
#   make bench-ab BENCH='Detect|Parse_ISO'
#   make bench-ab COUNT=18 BENCHTIME=300ms
set -eu

BASE=${BASE:-origin/main}
BENCH=${BENCH:-.}
COUNT=${COUNT:-10}
BENCHTIME=${BENCHTIME:-350ms}
LOADMAX=${LOADMAX:-3}
SPREADMAX=${SPREADMAX:-12}
PKG=${PKG:-.}

command -v benchstat >/dev/null 2>&1 || {
	echo "bench-ab: benchstat is not on PATH" >&2
	echo "          go install golang.org/x/perf/cmd/benchstat@latest" >&2
	exit 2
}

work=$(mktemp -d)
tree="$work/base"
cleanup() {
	git worktree remove --force "$tree" >/dev/null 2>&1 || true
	rm -rf "$work"
}
trap cleanup EXIT INT TERM

baseref=$(git rev-parse --short "$BASE")
head=$(git rev-parse --short HEAD)
dirty=""
git diff --quiet HEAD 2>/dev/null || dirty=" plus uncommitted changes"

echo "==> base $baseref, head $head$dirty"
echo "==> $COUNT alternating runs of '$BENCH' at $BENCHTIME each"

git worktree add -q --detach "$tree" "$BASE"
go -C "$tree" test -c -o "$work/base.test" "$PKG"
go test -c -o "$work/head.test" "$PKG"

# Nothing to measure is a mistake, not an empty result.
if ! "$work/head.test" -test.run '^$' -test.bench "$BENCH" -test.benchtime=1x 2>&1 | grep -q '^Benchmark'; then
	echo "bench-ab: '$BENCH' matched no benchmark in $PKG" >&2
	exit 1
fi

printf '==> waiting for the machine to be quiet (load under %s)' "$LOADMAX"
q=0
while [ "$q" -lt 5 ]; do
	load=$(cut -d' ' -f1 /proc/loadavg 2>/dev/null | cut -d. -f1 || echo 0)
	if [ "$load" -lt "$LOADMAX" ]; then q=$((q + 1)); else q=0; printf '.'; fi
	[ "$q" -lt 5 ] && sleep 15
done
echo " ok, load $(cut -d' ' -f1-3 /proc/loadavg 2>/dev/null || echo unknown)"

i=0
while [ "$i" -lt "$COUNT" ]; do
	"$work/base.test" -test.run '^$' -test.bench "$BENCH" -test.benchtime="$BENCHTIME" -test.benchmem >> "$work/a.txt" 2>&1
	"$work/head.test" -test.run '^$' -test.bench "$BENCH" -test.benchtime="$BENCHTIME" -test.benchmem >> "$work/b.txt" 2>&1
	i=$((i + 1))
	printf '\r==> run %s of %s' "$i" "$COUNT"
done
echo ""
echo "==> load after: $(cut -d' ' -f1-3 /proc/loadavg 2>/dev/null || echo unknown)"
echo ""

benchstat "$work/a.txt" "$work/b.txt" | sed "s#$work/a.txt#$baseref#; s#$work/b.txt#working tree#"

# A row nobody should read as a result. benchstat prints the spread as ±N%
# beside each side, so this reads its own output rather than recomputing.
noisy=$(benchstat "$work/a.txt" "$work/b.txt" |
	awk -v max="$SPREADMAX" '
		/±/ {
			n = 0
			for (i = 1; i <= NF; i++) if ($i ~ /^±?[0-9]+%$/) { v = $i; gsub(/[±%]/, "", v); if (v + 0 > max) n = 1 }
			if (n) print "    " $1
		}' | sort -u)
if [ -n "$noisy" ]; then
	echo ""
	echo "==> these rows spread more than $SPREADMAX% and are not a result:"
	printf '%s\n' "$noisy"
	echo "    Raise COUNT, or wait for a quieter machine. A percentage between"
	echo "    two noisy medians is not a measurement."
fi
