#!/usr/bin/env bash
#
# Run the benchmark suite on a Compute Engine VM that is the same machine every
# time, then delete the VM.
#
# A benchmark number is only comparable to another number measured the same way.
# A laptop is not that: it throttles, it has a browser open, and the machine it
# was three months ago is not the machine it is today. This script rents one
# fixed machine, measures on it, and gives it back.
#
# What is pinned, and why each one matters:
#
#   machine type      c4-standard-8. The C4 family is a single CPU platform
#                     (Intel Emerald Rapids), so the same machine type is the
#                     same silicon on every run. N1 and N2 are not: they span
#                     several platforms and you get whichever the zone has.
#   SMT off           --threads-per-core=1, so the 8 vCPUs become 4 physical
#                     cores with no hyperthread sibling. A sibling sharing your
#                     core is the largest source of run-to-run noise there is.
#   turbo pinned      --turbo-mode=ALL_CORE_MAX holds the all-core turbo clock
#                     instead of letting single-core boost float with how many
#                     cores happen to be busy.
#   core affinity     the benchmark runs under taskset on cores 1..3 and leaves
#                     core 0 to the kernel, the guest agent, and interrupts.
#   GOMAXPROCS        set explicitly, because Go writes it into every benchmark
#                     name and benchstat compares by name.
#   Go toolchain      GOTOOLCHAIN names an exact version, which Go fetches
#                     through the module proxy and verifies against
#                     sum.golang.org. The apt Go is only the bootstrap.
#   source            git archive of a commit, so the tree measured is a tree
#                     that exists in the history. --dirty overrides this.
#
# Two things are deliberately not pinned, because pinning them would only mean
# failing more often without making a number more comparable:
#
#   zone              a zone runs out of a machine type. BENCH_ZONE_FALLBACKS is
#                     walked in order and the one used is recorded. C4 is
#                     Emerald Rapids in every zone, so the silicon does not move.
#   ssh transport     direct to port 22, or tunnelled through IAP inside HTTPS.
#                     Tried in that order, because a network that reaches
#                     googleapis.com does not necessarily reach a VM address.
#                     How the result file travels is not part of the result.
#
# The VM is deleted three ways, because one way is how you end up paying for a
# VM you forgot:
#
#   1. a trap on EXIT, INT, TERM and HUP, which covers Ctrl-C and a failure
#      anywhere in the script
#   2. --max-run-duration with --instance-termination-action=DELETE, which is
#      Compute Engine deleting it without this script's help, and covers the
#      laptop closing, the network dropping, and kill -9
#   3. scripts/bench-gcloud.sh reap, which deletes anything left over that
#      carries the purpose=dateparsa-bench label
#
# Usage:
#   scripts/bench-gcloud.sh run [--update] [--dirty] [--keep]
#   scripts/bench-gcloud.sh list
#   scripts/bench-gcloud.sh reap [--yes]

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# ── Configuration ────────────────────────────────────────────────────────────
#
# Every one of these is an override, and every default is a decision. Changing
# one changes what the numbers mean, so a run that overrides anything writes the
# override into benchmarks/current.env beside the numbers.

BENCH_PROJECT="${BENCH_PROJECT:-}"
BENCH_ZONE="${BENCH_ZONE:-us-central1-a}"

# A zone runs out of a machine type, and a benchmark that cannot start is not
# more repeatable than one that started somewhere else. The pin that matters is
# the machine type, because the CPU platform follows from it in every zone; the
# zone only decides which building it is in. The one actually used is recorded
# in benchmarks/*.env.
#
# Ordered, not shuffled, so a run prefers the same zone as the last one when
# that zone has capacity. It crosses regions because a stockout is regional:
# all four us-central1 zones refused a c4-standard-8 within one minute of each
# other on 2026-08-19, which is what this list was widened for.
BENCH_ZONE_FALLBACKS="${BENCH_ZONE_FALLBACKS:-us-central1-c,us-central1-b,us-central1-f,us-east1-b,us-east1-c,us-east4-a,us-east4-b,us-east5-a,us-west1-a,us-west1-b,us-south1-a}"
BENCH_MACHINE="${BENCH_MACHINE:-c4-standard-8}"
BENCH_IMAGE_FAMILY="${BENCH_IMAGE_FAMILY:-debian-13}"
BENCH_IMAGE_PROJECT="${BENCH_IMAGE_PROJECT:-debian-cloud}"
BENCH_BOOT_DISK_TYPE="${BENCH_BOOT_DISK_TYPE:-hyperdisk-balanced}"
BENCH_BOOT_DISK_SIZE="${BENCH_BOOT_DISK_SIZE:-20GB}"

# SMT off and turbo held. Set BENCH_TURBO to the empty string for a machine
# family that does not accept --turbo-mode; the script also drops the flag by
# itself if Compute Engine rejects it.
BENCH_THREADS_PER_CORE="${BENCH_THREADS_PER_CORE:-1}"
BENCH_TURBO="${BENCH_TURBO:-ALL_CORE_MAX}"

# Cores 1..3 measure, core 0 absorbs the operating system. GOMAXPROCS matches
# the core count, and lands in the -N suffix of every benchmark name.
BENCH_CPUS="${BENCH_CPUS:-1-3}"
BENCH_GOMAXPROCS="${BENCH_GOMAXPROCS:-3}"

# The exact toolchain, fetched and checksum-verified by the bootstrap Go.
BENCH_GOTOOLCHAIN="${BENCH_GOTOOLCHAIN:-go1.26.6}"

# Ten runs of every benchmark is what makes benchstat's confidence interval
# worth reading. Three, which is what `make bench` does locally, is not enough
# to separate a 2% change from noise.
BENCH_COUNT="${BENCH_COUNT:-10}"
BENCH_TIMEOUT="${BENCH_TIMEOUT:-3600s}"

# Compute Engine deletes the VM at this age no matter what happens here. Long
# enough for the run plus boot and installation, short enough that a leak costs
# cents. A c4-standard-8 is roughly $0.40 an hour on demand.
BENCH_MAX_RUN="${BENCH_MAX_RUN:-90m}"

# Extra flags for `gcloud compute ssh` and `scp`.
BENCH_SSH_EXTRA="${BENCH_SSH_EXTRA:-}"

# How to reach the VM: `direct` opens TCP 22 to its external address, `iap`
# tunnels ssh inside HTTPS to Google's Identity-Aware Proxy, and `auto` tries
# direct first and switches after BENCH_DIRECT_SSH_SECONDS.
#
# The fallback exists because a network that reaches googleapis.com does not
# necessarily reach a bare VM address on port 22, and the symptom is a five
# minute wait ending in `connect to host <ip> port 22: Connection timed out`
# with the VPC firewall allowing 22 from everywhere. IAP needs no inbound path
# of its own: the connection is outbound HTTPS from here, and it arrives at the
# VM from 35.235.240.0/20, which `default-allow-ssh` already covers.
BENCH_SSH_TRANSPORT="${BENCH_SSH_TRANSPORT:-auto}"
BENCH_DIRECT_SSH_SECONDS="${BENCH_DIRECT_SSH_SECONDS:-60}"

# The ssh config to read, and by default none. gcloud passes the identity file,
# the user and the address on the command line, so a throwaway VM needs nothing
# out of ~/.ssh/config, and reading it is a way to fail for reasons that have
# nothing to do with benchmarking: a config carrying macOS-only UseKeychain
# lines makes OpenSSH on Linux refuse to start at all, which surfaces here as
# every connection attempt exiting 255 until the five-minute wait gives up.
# Point this at a real file if the VM is only reachable through a ProxyCommand.
BENCH_SSH_CONFIG="${BENCH_SSH_CONFIG:-/dev/null}"

# How the local side watches the detached run. The timeout is the ceiling on
# waiting, and it is deliberately shorter than BENCH_MAX_RUN so that this script
# gives up and deletes the VM before Compute Engine has to.
BENCH_POLL_INTERVAL="${BENCH_POLL_INTERVAL:-20}"
BENCH_POLL_TIMEOUT="${BENCH_POLL_TIMEOUT:-4800}"

LABEL_PURPOSE="dateparsa-bench"
OUT_DIR="$REPO_ROOT/benchmarks"

# ── Output ───────────────────────────────────────────────────────────────────

if [ -t 2 ]; then
	C_RED=$'\033[31m'; C_YELLOW=$'\033[33m'; C_DIM=$'\033[2m'; C_OFF=$'\033[0m'
else
	C_RED=''; C_YELLOW=''; C_DIM=''; C_OFF=''
fi

log()  { printf '==> %s\n' "$*" >&2; }
step() { printf '%s--- %s%s\n' "$C_DIM" "$*" "$C_OFF" >&2; }
warn() { printf '%sWARNING: %s%s\n' "$C_YELLOW" "$*" "$C_OFF" >&2; }
die()  { printf '%sERROR: %s%s\n' "$C_RED" "$*" "$C_OFF" >&2; exit 1; }

# ── Teardown ─────────────────────────────────────────────────────────────────
#
# INSTANCE_PENDING is set before the create call and not after it, because a
# Ctrl-C during create leaves an instance that this script never saw succeed.
# Deleting something that is not there is not an error worth reporting.

INSTANCE_NAME=""
INSTANCE_PENDING=""
KEEP_INSTANCE=""
SCRATCH=""

delete_instance() {
	[ -n "$INSTANCE_NAME" ] || return 0

	local attempt
	for attempt in 1 2 3; do
		if gcloud compute instances delete "$INSTANCE_NAME" \
			--project="$BENCH_PROJECT" --zone="$BENCH_ZONE" \
			--delete-disks=all --quiet >/dev/null 2>&1; then
			log "deleted $INSTANCE_NAME"
			return 0
		fi
		if ! gcloud compute instances describe "$INSTANCE_NAME" \
			--project="$BENCH_PROJECT" --zone="$BENCH_ZONE" \
			--format='value(name)' >/dev/null 2>&1; then
			log "$INSTANCE_NAME is gone"
			return 0
		fi
		warn "delete attempt $attempt failed, retrying"
		sleep 5
	done

	printf '%s\n' \
		"" \
		"${C_RED}COULD NOT DELETE THE BENCHMARK VM. IT IS STILL BILLING.${C_OFF}" \
		"" \
		"  gcloud compute instances delete $INSTANCE_NAME \\" \
		"    --project=$BENCH_PROJECT --zone=$BENCH_ZONE --delete-disks=all --quiet" \
		"" \
		"Compute Engine deletes it by itself $BENCH_MAX_RUN after it started running," \
		"which is the --max-run-duration on the instance, so the worst case is bounded." \
		"" >&2
	return 1
}

cleanup() {
	local rc=$?
	trap - EXIT INT TERM HUP

	if [ -n "$SCRATCH" ]; then rm -rf "$SCRATCH"; fi

	if [ -n "$INSTANCE_PENDING" ]; then
		if [ -n "$KEEP_INSTANCE" ]; then
			warn "--keep: $INSTANCE_NAME is still running and still billing"
			printf '  ssh:    gcloud compute ssh %s --project=%s --zone=%s\n' \
				"$INSTANCE_NAME" "$BENCH_PROJECT" "$BENCH_ZONE" >&2
			printf '  delete: gcloud compute instances delete %s --project=%s --zone=%s --delete-disks=all --quiet\n' \
				"$INSTANCE_NAME" "$BENCH_PROJECT" "$BENCH_ZONE" >&2
			printf '  it self-deletes %s after boot regardless\n' "$BENCH_MAX_RUN" >&2
		else
			step "deleting $INSTANCE_NAME"
			delete_instance || rc=$(( rc == 0 ? 1 : rc ))
		fi
	fi

	exit "$rc"
}

# ── Preflight ────────────────────────────────────────────────────────────────

preflight() {
	command -v gcloud >/dev/null 2>&1 || die "gcloud is not on PATH"
	command -v git >/dev/null 2>&1 || die "git is not on PATH"

	local account
	account="$(gcloud auth list --filter=status:ACTIVE --format='value(account)' 2>/dev/null || true)"
	[ -n "$account" ] || die "no active gcloud account. Run: gcloud auth login"

	if [ -z "$BENCH_PROJECT" ]; then
		BENCH_PROJECT="$(gcloud config get-value project 2>/dev/null || true)"
		[ -n "$BENCH_PROJECT" ] && [ "$BENCH_PROJECT" != "(unset)" ] \
			|| die "no project. Set BENCH_PROJECT or run: gcloud config set project PROJECT"
	fi

	# One authenticated call that fails the same way for an expired token, a
	# project that does not exist, a Compute API that is not enabled, and a zone
	# that is misspelled. Better to find out here than after the tarball.
	local zone_err
	if ! zone_err="$(gcloud compute zones describe "$BENCH_ZONE" \
		--project="$BENCH_PROJECT" --format='value(name)' 2>&1)"; then
		printf '%s\n' "$zone_err" >&2
		case "$zone_err" in
		*Reauthentication*|*"invalid_grant"*|*"credentials"*)
			die "gcloud credentials are stale. Run: gcloud auth login" ;;
		*"has not been used"*|*"is disabled"*)
			die "the Compute Engine API is off for $BENCH_PROJECT. Run: gcloud services enable compute.googleapis.com --project=$BENCH_PROJECT" ;;
		*)
			die "cannot reach zone $BENCH_ZONE in project $BENCH_PROJECT" ;;
		esac
	fi

	# Resolving the image family here costs one call and turns "the create
	# failed twenty seconds in" into "that family does not exist".
	if ! gcloud compute images describe-from-family "$BENCH_IMAGE_FAMILY" \
		--project="$BENCH_IMAGE_PROJECT" --format='value(name)' >/dev/null 2>&1; then
		die "no image family $BENCH_IMAGE_FAMILY in project $BENCH_IMAGE_PROJECT. Set BENCH_IMAGE_FAMILY"
	fi

	local stale
	stale="$(gcloud compute instances list --project="$BENCH_PROJECT" \
		--filter="labels.purpose=$LABEL_PURPOSE" \
		--format='value(name,zone,creationTimestamp)' 2>/dev/null || true)"
	if [ -n "$stale" ]; then
		warn "benchmark VMs already exist in $BENCH_PROJECT:"
		printf '%s\n' "$stale" | sed 's/^/    /' >&2
		warn "another run may be in progress. If not: scripts/bench-gcloud.sh reap"
	fi

	log "account       $account"
	log "project       $BENCH_PROJECT"
	log "zone          $BENCH_ZONE (falling back to $BENCH_ZONE_FALLBACKS on a stockout)"
	log "machine       $BENCH_MACHINE (SMT threads/core=$BENCH_THREADS_PER_CORE, turbo=${BENCH_TURBO:-unset})"
	log "toolchain     $BENCH_GOTOOLCHAIN"
	log "measurement   -count=$BENCH_COUNT on cores $BENCH_CPUS, GOMAXPROCS=$BENCH_GOMAXPROCS"
	log "self-destruct $BENCH_MAX_RUN after boot, whatever happens to this shell"
}

# ── Source ───────────────────────────────────────────────────────────────────

pack_source() {
	local tarball="$1" dirty="$2"

	cd "$REPO_ROOT"
	COMMIT="$(git rev-parse HEAD)"
	COMMIT_SHORT="$(git rev-parse --short=12 HEAD)"

	if [ -n "$dirty" ]; then
		# Tracked files as they are on disk, plus anything untracked that is not
		# ignored, which is what "measure what I am looking at" means.
		SOURCE_DESC="working tree at $COMMIT_SHORT plus local changes"
		COMMIT_LABEL="$COMMIT_SHORT-dirty"
		git ls-files -z --cached --others --exclude-standard \
			| tar --null -T - -czf "$tarball"
	else
		if ! git diff --quiet HEAD 2>/dev/null; then
			warn "the working tree has changes and they are NOT being measured"
			warn "measuring commit $COMMIT_SHORT. Pass --dirty to measure the tree instead"
		fi
		SOURCE_DESC="commit $COMMIT_SHORT"
		COMMIT_LABEL="$COMMIT_SHORT"
		git archive --format=tar HEAD | gzip > "$tarball"
	fi

	step "packed $SOURCE_DESC ($(wc -c < "$tarball" | tr -d ' ') bytes)"
}

# ── The VM ───────────────────────────────────────────────────────────────────

# One attempt in one zone. Sets CREATE_ERR. Clears BENCH_TURBO if the machine
# family turned out not to take it.
try_create() {
	local zone="$1"

	# The commas below are inside single flag values, not element separators.
	# shellcheck disable=SC2054
	local args=(
		compute instances create "$INSTANCE_NAME"
		--project="$BENCH_PROJECT"
		--zone="$zone"
		--machine-type="$BENCH_MACHINE"
		--image-family="$BENCH_IMAGE_FAMILY"
		--image-project="$BENCH_IMAGE_PROJECT"
		--boot-disk-type="$BENCH_BOOT_DISK_TYPE"
		--boot-disk-size="$BENCH_BOOT_DISK_SIZE"
		--boot-disk-auto-delete
		--network-interface=network=default,nic-type=GVNIC,stack-type=IPV4_ONLY
		--provisioning-model=STANDARD
		--max-run-duration="$BENCH_MAX_RUN"
		--instance-termination-action=DELETE
		--no-service-account
		--no-scopes
		--labels="purpose=$LABEL_PURPOSE,commit=$(printf '%s' "$COMMIT_LABEL" | tr -c 'a-z0-9_-' '-')"
		--quiet
	)
	if [ -n "$BENCH_THREADS_PER_CORE" ]; then
		args+=( --threads-per-core="$BENCH_THREADS_PER_CORE" )
	fi

	local with_turbo=( "${args[@]}" )
	if [ -n "$BENCH_TURBO" ]; then
		with_turbo+=( --turbo-mode="$BENCH_TURBO" )
	fi

	if CREATE_ERR="$(gcloud "${with_turbo[@]}" 2>&1)"; then
		return 0
	fi

	# A machine family that does not do all-core turbo should still be
	# measurable, and should say in benchmarks/current.env that it was not
	# pinned rather than quietly pretending it was.
	if [ -n "$BENCH_TURBO" ] && printf '%s' "$CREATE_ERR" | grep -qi 'turbo'; then
		warn "$BENCH_MACHINE rejected --turbo-mode=$BENCH_TURBO, retrying without it"
		warn "clock speed is then whatever the host decides, which is less repeatable"
		BENCH_TURBO=""
		if CREATE_ERR="$(gcloud "${args[@]}" 2>&1)"; then
			return 0
		fi
	fi

	return 1
}

CREATE_ERR=""

create_instance() {
	local zones=( "$BENCH_ZONE" ) zone seen known
	# tr rather than a local IFS, which unset would have to put back
	for zone in $(printf '%s' "$BENCH_ZONE_FALLBACKS" | tr ',' ' '); do
		seen=""
		for known in "${zones[@]}"; do
			if [ "$known" = "$zone" ]; then seen=1; fi
		done
		if [ -z "$seen" ]; then zones+=( "$zone" ); fi
	done

	for zone in "${zones[@]}"; do
		step "creating $INSTANCE_NAME in $zone"

		# Set before the call and cleared only on a create that definitely did
		# not happen, because a Ctrl-C mid-create leaves an instance this script
		# never saw succeed.
		INSTANCE_PENDING=1
		if try_create "$zone"; then
			BENCH_ZONE="$zone"
			printf '%s\n' "$CREATE_ERR" >&2
			return 0
		fi
		INSTANCE_PENDING=""

		# Out of capacity is the one failure worth walking past. Quota, a bad
		# flag and a missing image are all the same in every zone.
		case "$CREATE_ERR" in
		*ZONE_RESOURCE_POOL_EXHAUSTED*|*STOCKOUT*|*does\ not\ have\ enough\ resources*)
			warn "$zone has no $BENCH_MACHINE capacity right now, trying the next zone" ;;
		*)
			printf '%s\n' "$CREATE_ERR" >&2
			die "could not create the instance" ;;
		esac
	done

	printf '%s\n' "$CREATE_ERR" >&2
	warn "tried ${#zones[@]} zones: ${zones[*]}"
	warn "C3 is the other single-platform family, so BENCH_MACHINE=c3-standard-8"
	warn "is the fallback that keeps the CPU platform pinned. It does not take"
	warn "--turbo-mode, which this script drops by itself and records as unset."
	die "no zone had $BENCH_MACHINE capacity"
}

# BENCH_SSH_EXTRA is a flag string a caller supplies, so it is the one thing
# here that is meant to word-split.
# Set by wait_for_ssh, then used by every later connection.
TUNNEL_ARGS=()

remote() {
	local common=( --project="$BENCH_PROJECT" --zone="$BENCH_ZONE" --quiet "${TUNNEL_ARGS[@]}" )
	# shellcheck disable=SC2086
	gcloud compute ssh "$INSTANCE_NAME" "${common[@]}" $BENCH_SSH_EXTRA \
		--ssh-flag=-F"$BENCH_SSH_CONFIG" \
		--ssh-flag=-oConnectTimeout=15 \
		--ssh-flag=-oServerAliveInterval=30 \
		--ssh-flag=-oServerAliveCountMax=6 \
		--ssh-flag=-oStrictHostKeyChecking=no \
		--ssh-flag=-oUserKnownHostsFile=/dev/null \
		--ssh-flag=-oLogLevel=ERROR \
		--command "$1"
}

wait_for_ssh() {
	local started=$SECONDS
	local deadline=$(( SECONDS + 420 ))

	if [ "$BENCH_SSH_TRANSPORT" = iap ]; then
		TUNNEL_ARGS=( --tunnel-through-iap )
	fi
	step "waiting for ssh (${BENCH_SSH_TRANSPORT})"

	while [ "$SECONDS" -lt "$deadline" ]; do
		if remote 'true' >/dev/null 2>&1; then
			step "ssh is up after $(( SECONDS - started ))s${TUNNEL_ARGS:+ over IAP}"
			return 0
		fi

		if [ "$BENCH_SSH_TRANSPORT" = auto ] && [ ${#TUNNEL_ARGS[@]} -eq 0 ] \
			&& [ $(( SECONDS - started )) -ge "$BENCH_DIRECT_SSH_SECONDS" ]; then
			warn "port 22 on the VM address is not answering, switching to --tunnel-through-iap"
			TUNNEL_ARGS=( --tunnel-through-iap )
		fi

		sleep 5
	done
	warn "the last attempt reported:"
	remote 'true' 2>&1 | sed 's/^/    /' >&2 || true

	# The serial console needs no network in the guest, so it separates "the VM
	# never finished booting" from "the VM is fine and this machine cannot reach
	# it". Those two have the same symptom here and opposite fixes.
	warn "last lines of the serial console:"
	gcloud compute instances get-serial-port-output "$INSTANCE_NAME" \
		--project="$BENCH_PROJECT" --zone="$BENCH_ZONE" 2>/dev/null \
		| tail -25 | sed 's/^/    /' >&2 || warn "    (no serial output)"

	die "ssh never came up on $INSTANCE_NAME"
}

push() {
	local common=( --project="$BENCH_PROJECT" --zone="$BENCH_ZONE" --quiet "${TUNNEL_ARGS[@]}" )
	# shellcheck disable=SC2086
	gcloud compute scp "$1" "$INSTANCE_NAME:$2" "${common[@]}" $BENCH_SSH_EXTRA \
		--scp-flag=-F"$BENCH_SSH_CONFIG" \
		--scp-flag=-oStrictHostKeyChecking=no \
		--scp-flag=-oUserKnownHostsFile=/dev/null \
		--scp-flag=-oLogLevel=ERROR >/dev/null
}

pull() {
	local common=( --project="$BENCH_PROJECT" --zone="$BENCH_ZONE" --quiet "${TUNNEL_ARGS[@]}" )
	# shellcheck disable=SC2086
	gcloud compute scp "$INSTANCE_NAME:$1" "$2" "${common[@]}" $BENCH_SSH_EXTRA \
		--scp-flag=-F"$BENCH_SSH_CONFIG" \
		--scp-flag=-oStrictHostKeyChecking=no \
		--scp-flag=-oUserKnownHostsFile=/dev/null \
		--scp-flag=-oLogLevel=ERROR >/dev/null
}

# ── Result checking ──────────────────────────────────────────────────────────
#
# A benchmark file is promoted to a baseline and then trusted for months, so a
# truncated one is worse than a missing one. `go test | tee` cannot report a
# failing exit status through the pipe, which is exactly how a half-written file
# gets written, so the file itself is what gets checked.

MIN_BENCH_NAMES=50

# Tracked already, or untracked and not ignored. Anything else is a file `git
# add` will refuse without saying so.
committable() {
	git -C "$REPO_ROOT" ls-files --error-unmatch -- "$1" >/dev/null 2>&1 && return 0
	[ -n "$(git -C "$REPO_ROOT" ls-files --others --exclude-standard -- "$1")" ]
}

bench_names() { awk '/^Benchmark/ { sub(/-[0-9]+$/, "", $1); print $1 }' "$1" | sort -u; }

check_results() {
	local file="$1"

	[ -s "$file" ] || die "$file is empty"
	grep -q '^goos:' "$file" || die "$file has no goos: header, so it is not benchmark output"

	if grep -q '^FAIL' "$file"; then
		grep -n '^FAIL' "$file" | head -5 >&2
		die "the benchmark run failed on the VM"
	fi

	local lines names
	lines="$(grep -c '^Benchmark' "$file" || true)"
	names="$(bench_names "$file" | wc -l | tr -d ' ')"

	# A package that failed to build drops every one of its benchmarks and
	# leaves a file that still looks like benchmark output.
	[ "$names" -ge "$MIN_BENCH_NAMES" ] \
		|| die "$file holds $names distinct benchmarks, fewer than the $MIN_BENCH_NAMES floor. The run was cut short"

	step "$names distinct benchmarks, $lines result lines, no failures"

	# Missing is a warning and not an error, because deleting a benchmark is a
	# thing a change is allowed to do. Silently is not.
	if [ -f "$OUT_DIR/baseline.txt" ]; then
		local gone
		gone="$(comm -23 <(bench_names "$OUT_DIR/baseline.txt") <(bench_names "$file"))"
		if [ -n "$gone" ]; then
			warn "in the baseline and not in this run:"
			printf '%s\n' "$gone" | sed 's/^/    /' >&2
		fi
	fi
}

# ── Driving the run ──────────────────────────────────────────────────────────
#
# The measured run takes twenty-five minutes or so, and holding one ssh session
# open for all of it means a dropped connection throws away the run and the
# money. So the remote side detaches and this side polls: every failure here
# costs one retry, not the whole thing.

start_remote() {
	step "starting the run on $INSTANCE_NAME"
	remote "chmod +x /tmp/bench-remote.sh && \
		BENCH_GOTOOLCHAIN='$BENCH_GOTOOLCHAIN' \
		BENCH_COUNT='$BENCH_COUNT' \
		BENCH_TIMEOUT='$BENCH_TIMEOUT' \
		BENCH_CPUS='$BENCH_CPUS' \
		BENCH_GOMAXPROCS='$BENCH_GOMAXPROCS' \
		BENCH_COMMIT='$COMMIT' \
		BENCH_SOURCE_DESC='$SOURCE_DESC' \
		BENCH_TURBO='${BENCH_TURBO:-unset}' \
		BENCH_THREADS_PER_CORE='$BENCH_THREADS_PER_CORE' \
		BENCH_AGAINST='${BENCH_AGAINST:-}' \
		/tmp/bench-remote.sh --detach"
}

# Sets POLL_STATE, streams the remote log to stderr. Deliberately not a command
# substitution: a subshell here would swallow a SIGINT aimed at this script and
# delay teardown until the poll ended by itself.
POLL_STATE=""

poll_remote() {
	local offset=1 state=running chunk consecutive_errors=0
	local deadline=$(( SECONDS + BENCH_POLL_TIMEOUT ))

	while [ "$SECONDS" -lt "$deadline" ]; do
		sleep "$BENCH_POLL_INTERVAL"

		# A reply is good only if it came back and carries the separator. A
		# connection refused and a truncated reply are the same event here.
		if ! chunk="$(remote "tail -n +$offset /tmp/bench-out/run.log 2>/dev/null; printf '\034'; cat /tmp/bench-out/state 2>/dev/null || printf 'running'" 2>/dev/null)"; then
			chunk=""
		fi
		case "$chunk" in
		*$'\034'*)
			consecutive_errors=0 ;;
		*)
			consecutive_errors=$(( consecutive_errors + 1 ))
			if [ "$consecutive_errors" -ge 10 ]; then
				POLL_STATE=unreachable
				return 0
			fi
			continue ;;
		esac

		state="${chunk##*$'\034'}"
		chunk="${chunk%$'\034'*}"
		chunk="${chunk%$'\n'}"

		if [ -n "$chunk" ]; then
			printf '%s\n' "$chunk" | sed 's/^/    /' >&2
			offset=$(( offset + $(printf '%s\n' "$chunk" | wc -l) ))
		fi

		case "$state" in
		done|failed) POLL_STATE="$state"; return 0 ;;
		esac
	done

	POLL_STATE=timeout
}

# ── Subcommands ──────────────────────────────────────────────────────────────

cmd_run() {
	local update="" dirty="" against=""

	while [ $# -gt 0 ]; do
		case "$1" in
		--update)  update=1 ;;
		--dirty)   dirty=1 ;;
		--keep)    KEEP_INSTANCE=1 ;;
		--against) shift; against="${1:-}"; [ -n "$against" ] || die "--against needs a ref" ;;
		*) die "unknown flag: $1" ;;
		esac
		shift
	done

	if [ -n "$against" ] && [ -n "$update" ]; then
		die "--against measures two trees against each other and produces no baseline; drop --update"
	fi

	preflight

	SCRATCH="$(mktemp -d)"
	INSTANCE_NAME="dateparsa-bench-$(date -u +%Y%m%d-%H%M%S)-$$"
	trap cleanup EXIT INT TERM HUP

	pack_source "$SCRATCH/src.tar.gz" "$dirty"
	local headdesc="$SOURCE_DESC"

	# --against packs a second tree and the VM alternates between the two. One
	# machine, one boot, one thermal state, samples taken in turn.
	#
	# Two separate runs cannot answer the same question and it took a shipped
	# regression to learn why. The hot path measured 9 to 11 percent slower
	# between two baselines taken three days apart on two rented machines, and
	# nothing in either number said whether that was the code or the host. A
	# local A/B could not settle it either: this repository is developed on
	# arm64 and the baseline machine is amd64, so "the hot path did not move"
	# was true where it was measured and untested where it counts.
	local basedesc=""
	if [ -n "$against" ]; then
		local baseref
		baseref="$(git -C "$REPO_ROOT" rev-parse --short=12 "$against")" 			|| die "--against: no such ref: $against"
		step "packing $baseref to measure against"
		git -C "$REPO_ROOT" archive --format=tar "$against" | gzip > "$SCRATCH/src-base.tar.gz"
		basedesc="commit $baseref"
	fi

	create_instance
	wait_for_ssh

	step "uploading source and runner"
	push "$SCRATCH/src.tar.gz" "/tmp/dateparsa-src.tar.gz"
	push "$REPO_ROOT/scripts/bench-gcloud-remote.sh" "/tmp/bench-remote.sh"
	if [ -n "$against" ]; then
		push "$SCRATCH/src-base.tar.gz" "/tmp/dateparsa-src-base.tar.gz"
	fi

	BENCH_AGAINST="$against"
	start_remote
	poll_remote

	if [ -n "$against" ]; then
		step "fetching both sides"
		pull "/tmp/bench-out/a.txt" "$OUT_DIR/ab-base.txt" || true
		pull "/tmp/bench-out/b.txt" "$OUT_DIR/ab-head.txt" || true
		if [ "$POLL_STATE" != "done" ]; then
			pull "/tmp/bench-out/run.log" "$OUT_DIR/current.log" 2>/dev/null || true
			die "the benchmark did not complete (state '$POLL_STATE')"
		fi
		log ""
		log "base: $basedesc"
		log "head: $headdesc"
		log ""
		if command -v benchstat >/dev/null 2>&1; then
			benchstat "$OUT_DIR/ab-base.txt" "$OUT_DIR/ab-head.txt" || true
		else
			log "benchstat is not installed; the two files are in benchmarks/ab-*.txt"
		fi
		return 0
	fi

	step "fetching results"
	pull "/tmp/bench-out/bench.txt" "$OUT_DIR/current.txt" || true
	pull "/tmp/bench-out/bench.env" "$OUT_DIR/current.env" || true

	if [ "$POLL_STATE" != "done" ]; then
		warn "the run on the VM ended in state '$POLL_STATE'"
		pull "/tmp/bench-out/run.log" "$OUT_DIR/current.log" 2>/dev/null \
			&& warn "its log is in benchmarks/current.log"
		die "the benchmark did not complete"
	fi

	check_results "$OUT_DIR/current.txt"

	log "wrote benchmarks/current.txt and benchmarks/current.env"

	if [ -n "$update" ]; then
		cp "$OUT_DIR/current.txt" "$OUT_DIR/baseline.txt"
		cp "$OUT_DIR/current.env" "$OUT_DIR/baseline.env"
		log "promoted to benchmarks/baseline.txt and benchmarks/baseline.env"

		# `*.env` in .gitignore matched baseline.env for as long as it existed,
		# so the file recording which machine produced the baseline was one
		# nobody could commit. A negation un-ignores it; this checks the
		# negation is still there, because the failure is silent otherwise.
		if ! committable benchmarks/baseline.txt || ! committable benchmarks/baseline.env; then
			warn "benchmarks/baseline.env is ignored by .gitignore and cannot be committed"
			warn "a baseline whose machine is not in the repository is not reproducible"
			warn "the fix is the !benchmarks/baseline.env line in .gitignore"
		fi

		log "commit both on their own, saying what moved and why"
	elif [ -f "$OUT_DIR/baseline.txt" ] && command -v benchstat >/dev/null 2>&1; then
		printf '\n' >&2
		log "benchstat baseline.txt current.txt"

		# benchstat matches rows by name, and Go writes GOMAXPROCS into every
		# name; it also reads goos/goarch/cpu as the configuration a run belongs
		# to and puts two files that disagree in separate tables. Either one
		# turns a comparison into two lists printed next to each other, so both
		# are normalised away in a copy. See the same fix in the Makefile.
		local norm="$SCRATCH/norm" f
		mkdir -p "$norm"
		for f in baseline current; do
			sed -E 's/^(Benchmark[^ 	]*)-[0-9]+/\1/' "$OUT_DIR/$f.txt" \
				| grep -v -E '^(goos|goarch|cpu):' > "$norm/$f.txt"
		done
		benchstat "$norm/baseline.txt" "$norm/current.txt" || true
	fi
}

resolve_project() {
	[ -n "$BENCH_PROJECT" ] && return 0
	BENCH_PROJECT="$(gcloud config get-value project 2>/dev/null || true)"
	[ -n "$BENCH_PROJECT" ] && [ "$BENCH_PROJECT" != "(unset)" ] \
		|| die "no project. Set BENCH_PROJECT or run: gcloud config set project PROJECT"
}

cmd_list() {
	resolve_project
	gcloud compute instances list --project="$BENCH_PROJECT" \
		--filter="labels.purpose=$LABEL_PURPOSE" \
		--format='table(name,zone.basename(),machineType.basename(),status,creationTimestamp)'
}

cmd_reap() {
	local assume_yes=""
	while [ $# -gt 0 ]; do
		case "$1" in
		--yes|-y) assume_yes=1 ;;
		*) die "unknown flag: $1" ;;
		esac
		shift
	done

	resolve_project

	local rows
	rows="$(gcloud compute instances list --project="$BENCH_PROJECT" \
		--filter="labels.purpose=$LABEL_PURPOSE" \
		--format='value(name,zone.basename())' 2>/dev/null || true)"

	if [ -z "$rows" ]; then
		log "no instances labelled purpose=$LABEL_PURPOSE in $BENCH_PROJECT"
		return 0
	fi

	printf 'These instances will be permanently deleted from project %s:\n\n' "$BENCH_PROJECT" >&2
	printf '%s\n' "$rows" | sed 's/^/    /' >&2
	printf '\n' >&2

	if [ -z "$assume_yes" ]; then
		if [ ! -t 0 ]; then
			die "not a terminal. Re-run with --yes to delete these without asking"
		fi
		local reply
		read -r -p "Delete them? [y/N] " reply
		case "$reply" in
		y|Y|yes|YES) ;;
		*) log "nothing deleted"; return 0 ;;
		esac
	fi

	local name zone failed=0
	while read -r name zone; do
		[ -n "$name" ] || continue
		if gcloud compute instances delete "$name" --project="$BENCH_PROJECT" \
			--zone="$zone" --delete-disks=all --quiet >/dev/null 2>&1; then
			log "deleted $name"
		else
			warn "could not delete $name in $zone"
			failed=1
		fi
	done <<< "$rows"

	return "$failed"
}

usage() {
	sed -n '2,/^set -euo/p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//;$d'
}

main() {
	local cmd="${1:-run}"
	[ $# -gt 0 ] && shift || true

	case "$cmd" in
	run)          cmd_run "$@" ;;
	list)         cmd_list "$@" ;;
	reap)         cmd_reap "$@" ;;
	-h|--help|help) usage ;;
	*) die "unknown command: $cmd. One of: run, list, reap" ;;
	esac
}

main "$@"
