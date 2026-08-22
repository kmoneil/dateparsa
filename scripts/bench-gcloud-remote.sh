#!/usr/bin/env bash
#
# The half of the cloud benchmark that runs on the VM. scripts/bench-gcloud.sh
# uploads this, runs it with --detach, and then polls, so that an ssh connection
# dropping halfway through a twenty-five minute run costs nothing.
#
# Everything it does before measuring is about making the machine boring:
# stop the timers that wake up and apt-get things, install exactly what is
# needed, pin the toolchain, and run once and throw it away so that the
# measured run is not also measuring a cold page cache and an empty build cache.
#
#   --detach   fork into the background, write /tmp/bench-out/state, return
#   --worker   the body, which is what --detach re-execs
#   (nothing)  the body, in the foreground
#
# Inputs arrive as environment variables. They have defaults so this can be run
# by hand on a VM that is already up.

set -euo pipefail
set -o errtrace

OUT=/tmp/bench-out
WORK=/tmp/dateparsa
SRC=/tmp/dateparsa-src.tar.gz

BENCH_GOTOOLCHAIN="${BENCH_GOTOOLCHAIN:-go1.26.6}"
BENCH_COUNT="${BENCH_COUNT:-10}"
BENCH_TIMEOUT="${BENCH_TIMEOUT:-3600s}"
BENCH_CPUS="${BENCH_CPUS:-1-3}"
BENCH_GOMAXPROCS="${BENCH_GOMAXPROCS:-3}"
BENCH_COMMIT="${BENCH_COMMIT:-unknown}"
BENCH_SOURCE_DESC="${BENCH_SOURCE_DESC:-unknown}"
BENCH_TURBO="${BENCH_TURBO:-unset}"
BENCH_THREADS_PER_CORE="${BENCH_THREADS_PER_CORE:-unset}"

say() { printf '[%s] %s\n' "$(date -u +%H:%M:%S)" "$*"; }

# ── Detach ───────────────────────────────────────────────────────────────────

if [ "${1:-}" = "--detach" ]; then
	mkdir -p "$OUT"
	: > "$OUT/run.log"
	printf 'running\n' > "$OUT/state"
	setsid nohup "$0" --worker >> "$OUT/run.log" 2>&1 < /dev/null &
	printf 'detached as pid %s, log at %s/run.log\n' "$!" "$OUT"
	exit 0
fi

mkdir -p "$OUT"
trap 'printf "failed\n" > "$OUT/state"' ERR

# ── Quiet the machine ────────────────────────────────────────────────────────
#
# A Debian cloud image wakes up on a timer to refresh package lists and to
# rebuild the man page index. Either one landing in the middle of a benchmark
# moves a number by more than the changes this library measures.

quiesce() {
	say "stopping the periodic jobs"
	local unit
	for unit in apt-daily.timer apt-daily-upgrade.timer man-db.timer \
		unattended-upgrades.service google-osconfig-agent.service \
		systemd-tmpfiles-clean.timer fstrim.timer; do
		sudo systemctl stop "$unit" >/dev/null 2>&1 || true
		sudo systemctl disable "$unit" >/dev/null 2>&1 || true
	done

	if command -v cloud-init >/dev/null 2>&1; then
		say "waiting for cloud-init"
		sudo cloud-init status --wait >/dev/null 2>&1 || true
	fi
}

install_packages() {
	say "installing git, make, curl and a bootstrap Go"
	export DEBIAN_FRONTEND=noninteractive
	# The lock timeout is the boot-time apt run that has not finished yet. It
	# waits rather than failing, which is what a fresh VM needs.
	sudo -E apt-get -o DPkg::Lock::Timeout=600 -qq update
	sudo -E apt-get -o DPkg::Lock::Timeout=600 -qq install -y \
		--no-install-recommends git make curl golang-go
}

# ── Toolchain ────────────────────────────────────────────────────────────────
#
# apt's Go is only a bootstrap. GOTOOLCHAIN names the version that does the
# measuring, and the bootstrap fetches it through the module proxy and verifies
# it against sum.golang.org, so the pin is cryptographic and not a URL.

setup_toolchain() {
	local bootstrap
	bootstrap="$(go env GOVERSION 2>/dev/null || echo none)"
	say "bootstrap toolchain is $bootstrap"

	case "$bootstrap" in
	none) printf 'no go on PATH after installing golang-go\n' >&2; return 1 ;;
	go1.[0-9].*|go1.1[0-9].*|go1.20.*)
		printf 'bootstrap %s is older than go1.21 and cannot fetch a toolchain.\n' "$bootstrap" >&2
		printf 'Use a newer BENCH_IMAGE_FAMILY.\n' >&2
		return 1 ;;
	esac

	export GOTOOLCHAIN="$BENCH_GOTOOLCHAIN"
	export GOFLAGS=""
	export GOWORK=off

	say "fetching $BENCH_GOTOOLCHAIN"
	GO_ACTUAL="$(cd "$WORK" && go version)"
	say "$GO_ACTUAL"
}

unpack() {
	say "unpacking the source"
	rm -rf "$WORK"
	mkdir -p "$WORK"
	tar -xzf "$SRC" -C "$WORK"
	[ -f "$WORK/go.mod" ] || { printf 'no go.mod in the tarball\n' >&2; return 1; }
}

# ── Provenance ───────────────────────────────────────────────────────────────
#
# Written beside the numbers, never into them: benchstat reads `key: value`
# lines at the top of a benchmark file as configuration and will split the table
# on any key whose value differs between the two files it is comparing. A commit
# hash differing between baseline and current is the whole point of the
# comparison, so it goes in its own file.

metadata_value() {
	curl -sf -H 'Metadata-Flavor: Google' \
		"http://metadata.google.internal/computeMetadata/v1/$1" 2>/dev/null || echo unknown
}

write_env() {
	local phase="$1"
	{
		printf '# provenance for the benchmark numbers beside this file\n'
		printf '# written by scripts/bench-gcloud-remote.sh\n'
		printf 'run_state=%s\n' "$phase"
		printf 'measured_at=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
		printf 'commit=%s\n' "$BENCH_COMMIT"
		printf 'source=%s\n' "$BENCH_SOURCE_DESC"
		printf 'go_toolchain=%s\n' "$BENCH_GOTOOLCHAIN"
		printf 'go_version=%s\n' "${GO_ACTUAL:-unknown}"
		printf 'machine_type=%s\n' "$(metadata_value instance/machine-type | sed 's|.*/||')"
		printf 'cpu_platform=%s\n' "$(metadata_value instance/cpu-platform)"
		printf 'zone=%s\n' "$(metadata_value instance/zone | sed 's|.*/||')"
		printf 'image=%s\n' "$(metadata_value instance/image | sed 's|.*/||')"
		printf 'threads_per_core=%s\n' "$BENCH_THREADS_PER_CORE"
		printf 'turbo_mode=%s\n' "$BENCH_TURBO"
		printf 'bench_cpus=%s\n' "$BENCH_CPUS"
		printf 'gomaxprocs=%s\n' "$BENCH_GOMAXPROCS"
		printf 'bench_count=%s\n' "$BENCH_COUNT"
		printf 'nproc=%s\n' "$(nproc)"
		printf 'cpu_model=%s\n' "$(grep -m1 '^model name' /proc/cpuinfo 2>/dev/null | cut -d: -f2- | sed 's/^ *//' || echo unknown)"
		printf 'kernel=%s\n' "$(uname -sr)"
		printf 'os=%s\n' "$(. /etc/os-release 2>/dev/null && echo "$PRETTY_NAME" || echo unknown)"
		printf 'thp=%s\n' "$(cat /sys/kernel/mm/transparent_hugepage/enabled 2>/dev/null || echo unknown)"
		printf 'aslr=%s\n' "$(cat /proc/sys/kernel/randomize_va_space 2>/dev/null || echo unknown)"
		printf 'governor=%s\n' "$(cat /sys/devices/system/cpu/cpu0/cpufreq/scaling_governor 2>/dev/null || echo none)"
	} > "$OUT/bench.env"
}

# ── Measure ──────────────────────────────────────────────────────────────────

run_bench() {
	cd "$WORK"

	# BENCH_CPUS is written for the default machine type. Overriding the
	# machine to something smaller and leaving the core list alone is the
	# obvious way to get here, and taskset's own error does not say why.
	if ! taskset -c "$BENCH_CPUS" true 2>/dev/null; then
		printf 'cores %s do not exist on this VM, which has %s\n' "$BENCH_CPUS" "$(nproc)" >&2
		printf 'Set BENCH_CPUS and BENCH_GOMAXPROCS to match BENCH_MACHINE.\n' >&2
		return 1
	fi

	# Thrown away on purpose. It compiles every test binary, fills the page
	# cache, and gets the cores out of whatever idle state they booted into, so
	# the run that counts is not measuring any of that.
	say "warm-up run, discarded"
	taskset -c "$BENCH_CPUS" env GOMAXPROCS="$BENCH_GOMAXPROCS" \
		go test -run='^$' -bench=. -benchmem -count=1 -timeout="$BENCH_TIMEOUT" ./... \
		> /tmp/warmup.txt 2>&1 || {
			say "the warm-up run failed"
			tail -40 /tmp/warmup.txt >&2
			return 1
		}
	say "warm-up done, $(grep -c '^Benchmark' /tmp/warmup.txt || true) results discarded"

	# Two trees measured against each other, alternating, on this one machine.
	#
	# The alternation is the whole point and is not a detail. Two runs taken
	# separately, even on the same machine type in the same zone, differ by more
	# than the change being measured often enough to be useless: a hot path
	# measured 9 to 11 percent apart between two baselines three days apart, and
	# neither number said whether that was the code or the host. Alternating
	# gives both trees the same boot, the same thermal state and the same
	# neighbours, because they get them in turn.
	#
	# One count per pass rather than BENCH_COUNT in one go, so the samples
	# interleave rather than forming two blocks.
	if [ -n "${BENCH_AGAINST:-}" ]; then
		local basedir="$WORK-base"
		mkdir -p "$basedir"
		tar -xzf /tmp/dateparsa-src-base.tar.gz -C "$basedir"
		say "A/B: $BENCH_COUNT alternating passes"
		: > "$OUT/a.txt"; : > "$OUT/b.txt"
		local pass=1
		while [ "$pass" -le "$BENCH_COUNT" ]; do
			(cd "$basedir" && taskset -c "$BENCH_CPUS" env GOMAXPROCS="$BENCH_GOMAXPROCS" \
				make bench BENCH_COUNT=1 BENCH_TIMEOUT="$BENCH_TIMEOUT" >/dev/null 2>&1) || true
			cat "$basedir/benchmarks/current.txt" >> "$OUT/a.txt" 2>/dev/null || true
			(cd "$WORK" && taskset -c "$BENCH_CPUS" env GOMAXPROCS="$BENCH_GOMAXPROCS" \
				make bench BENCH_COUNT=1 BENCH_TIMEOUT="$BENCH_TIMEOUT" >/dev/null 2>&1) || true
			cat "$WORK/benchmarks/current.txt" >> "$OUT/b.txt" 2>/dev/null || true
			say "pass $pass of $BENCH_COUNT done"
			pass=$((pass + 1))
		done
		if [ ! -s "$OUT/a.txt" ] || [ ! -s "$OUT/b.txt" ]; then
			say "one side produced nothing"
			return 1
		fi
		say "$(grep -c '^Benchmark' "$OUT/a.txt" || true) base lines, $(grep -c '^Benchmark' "$OUT/b.txt" || true) head lines"
		return 0
	fi

	# The measured run goes through `make bench` so that the flags are the ones
	# the Makefile defines and not a second copy of them that drifts.
	say "measured run: -count=$BENCH_COUNT on cores $BENCH_CPUS, GOMAXPROCS=$BENCH_GOMAXPROCS"
	taskset -c "$BENCH_CPUS" env GOMAXPROCS="$BENCH_GOMAXPROCS" \
		make bench BENCH_COUNT="$BENCH_COUNT" BENCH_TIMEOUT="$BENCH_TIMEOUT" \
		> /tmp/bench-stdout.txt 2>&1 || true

	if [ ! -s "$WORK/benchmarks/current.txt" ]; then
		say "make bench produced nothing"
		tail -40 /tmp/bench-stdout.txt >&2
		return 1
	fi

	cp "$WORK/benchmarks/current.txt" "$OUT/bench.txt"
	say "$(grep -c '^Benchmark' "$OUT/bench.txt" || true) benchmark result lines"

	if grep -q '^FAIL' "$OUT/bench.txt"; then
		say "the suite reported FAIL, which the local side will refuse to promote"
	fi
}

# ── Body ─────────────────────────────────────────────────────────────────────

say "starting"
quiesce
install_packages
unpack
setup_toolchain
write_env "running"
run_bench
write_env "done"

printf 'done\n' > "$OUT/state"
say "finished"
