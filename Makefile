MODULE := github.com/kmoneil/dateparsa

# Tool versions, pinned here and nowhere else.
#
# A floating tool is a tool that can fail a pull request which changed nothing,
# and can fail a release, because the gate re-runs on a tag that cannot be
# moved. CI installs each one at the version printed by `make -s print-<VAR>`,
# so this file is the single declaration.
GOFUMPT_VERSION := v0.11.0
GOLANGCI_LINT_VERSION := v2.12.2
GOVULNCHECK_VERSION := v1.6.0

# Directories that hold notes rather than code. Never formatted, never linted.
EXCLUDE := ^_plans/\|^_tmp/\|^_reviews/

FUZZTIME ?= 30s
FUZZPKGS ?= $(shell $(MAKE) -s fuzz-packages)

# How many times every benchmark is repeated, and how long a package gets.
# Three is enough to eyeball a local run and too few for benchstat to give a
# confidence interval worth reading, which is why the cloud runner raises it to
# ten. Both live here so the cloud runner overrides one definition rather than
# carrying a second copy of the flags.
BENCH_COUNT ?= 3
BENCH_TIMEOUT ?= 900s

# The linked size a caller pays for importing this library. Twenty locales of
# compiled data are the bulk of it, so this is the gate that notices when a
# data addition starts costing every downstream binary.
SIZE_BUDGET ?= 10485760

.DEFAULT_GOAL := help

## help: list the targets
.PHONY: help
help:
	@grep -hE '^## ' $(MAKEFILE_LIST) | sed 's/^## /  /'

# ── Format, vet, lint ────────────────────────────────────────────────────────

## fmt: format everything with gofumpt
.PHONY: fmt
fmt:
	@gofumpt -w .

## fmt-check: fail if anything is unformatted
.PHONY: fmt-check
fmt-check:
	@unformatted=$$(gofumpt -l . | grep -v '$(EXCLUDE)' || true); \
	if [ -n "$$unformatted" ]; then \
		echo "not formatted:"; printf '%s\n' "$$unformatted" | sed 's/^/  /'; \
		echo "run: make fmt"; exit 1; \
	fi

## vet: go vet
.PHONY: vet
vet:
	@go vet ./...

## lint: golangci-lint, config first
#
# `config verify` before `run`, because `run` ignores a top-level key it does
# not recognise instead of refusing it. .golangci.yml said `version: "2"` and
# used the v1 names `linters-settings` and `issues.exclude-dirs` for its whole
# life, so `govet: enable-all` was never actually on, `make lint` was green
# throughout, and the first CI run to execute the golangci-lint action failed
# on a config the local gate had never checked.
.PHONY: lint
lint:
	@golangci-lint config verify
	@golangci-lint run --timeout=2m

# ── Test ─────────────────────────────────────────────────────────────────────

## test: the suite with the race detector
.PHONY: test
test:
	@go test -race -count=1 -timeout=180s ./...

## alloc: assert Layout.Parse allocates zero times
.PHONY: alloc
alloc:
	@echo "==> Layout.Parse is zero-alloc"
	@go test -run='^TestLayoutParseZeroAlloc$$' -count=1 -timeout=30s .

## cover: the suite with a coverage profile
.PHONY: cover
cover:
	@go test -coverprofile=coverage.out -covermode=atomic ./...
	@go tool cover -func=coverage.out | tail -1

# ── Fuzz ─────────────────────────────────────────────────────────────────────

## fuzz-packages: list the packages that hold fuzz targets
#
# Discovered, never listed. A hand-written list is a list that goes stale
# silently: `make fuzz` used to name two targets in the root package by hand
# while six others across two packages were never swept, and a green run that
# fuzzed a quarter of the targets reads exactly like one that fuzzed them all.
.PHONY: fuzz-packages
fuzz-packages:
	@go list -f '{{.Dir}}' ./... | sed 's|^$(CURDIR)$$|.|;s|^$(CURDIR)/||' | while read -r d; do \
		if go test -list='^Fuzz' "./$$d" 2>/dev/null | grep -q '^Fuzz'; then echo "$$d"; fi; \
	done

## fuzz: run every fuzz target for FUZZTIME each (default 30s)
.PHONY: fuzz
fuzz:
	@echo "==> Fuzz sweep (FUZZTIME=$(FUZZTIME) per target, pkgs=$(FUZZPKGS))"
	@failed=0; ran=0; flaked=0; start=$$(date +%s); \
	for pkg in $(FUZZPKGS); do \
		for target in $$(go test -list='^Fuzz' "./$$pkg" 2>/dev/null | grep '^Fuzz' || true); do \
			ran=$$((ran + 1)); \
			printf "    %-46s " "$$pkg $$target"; \
			if out=$$(go test "./$$pkg" -run='^$$' -fuzz="^$$target$$" -fuzztime=$(FUZZTIME) 2>&1); then \
				status=0; \
			else \
				status=$$?; \
			fi; \
			case $$(printf '%s\n' "$$out" | sh scripts/fuzz-verdict.sh $$status) in \
			pass) printf '%s\n' "$$out" | tail -1 ;; \
			flake) \
				flaked=$$((flaked + 1)); \
				echo "FLAKE (golang/go#75804, not this target)"; \
				printf '%s\n' "$$out" | sed 's/^/        /' ;; \
			*) \
				failed=1; echo "FAIL"; \
				printf '%s\n' "$$out" | sed 's/^/        /' ;; \
			esac; \
		done; \
	done; \
	echo "==> $$ran target(s) in $$(($$(date +%s) - start))s, $$flaked flaked in the toolchain"; \
	if [ "$$ran" -eq 0 ]; then echo "==> no targets found, which is a failure and not a pass"; exit 1; fi; \
	exit $$failed

# ── Security and size ────────────────────────────────────────────────────────

## vuln: govulncheck, including the standard library this builds against
.PHONY: vuln
vuln:
	@govulncheck ./...

## size: fail if a binary importing this library exceeds SIZE_BUDGET
.PHONY: size
size:
	@tmp=$$(mktemp -d); trap 'rm -rf "$$tmp"' EXIT; \
	printf '%s\n' \
		'package main' \
		'' \
		'import (' \
		'	"fmt"' \
		'' \
		'	"$(MODULE)"' \
		')' \
		'' \
		'func main() {' \
		'	r, _ := dateparsa.Parse("2024-03-15")' \
		'	fmt.Println(r.Time)' \
		'}' > "$$tmp/main.go"; \
	cd "$$tmp" && go mod init sizecheck >/dev/null && \
		go mod edit -replace $(MODULE)=$(CURDIR) && \
		go mod tidy >/dev/null 2>&1 && \
		go build -o sizecheck . ; \
	bytes=$$(wc -c < "$$tmp/sizecheck"); \
	printf '==> linked size: %s bytes (%s MiB), budget %s\n' \
		"$$bytes" "$$(($$bytes / 1048576))" "$(SIZE_BUDGET)"; \
	if [ "$$bytes" -gt "$(SIZE_BUDGET)" ]; then echo "FAIL: over budget"; exit 1; fi

# ── Benchmarks ───────────────────────────────────────────────────────────────

## bench: run the benchmarks into benchmarks/current.txt
#
# Bash with pipefail, because `go test | tee` reports tee's exit status and not
# go test's. A build failure or a panicking benchmark then writes a short file
# and returns success, and a short file is the one thing that must never reach
# benchmarks/baseline.txt.
#
# ./... because the benchmarks are not all in the root package. This swept the
# root package alone, which is 40 of the 50 benchmark functions in the tree; the
# other 10 are flextime's, so the surface a caller actually scans database rows
# through was measured by nothing and never reached benchmarks/baseline.txt.
# Same shape as the fuzz sweep before it discovered its own targets, and the same
# fix: ask the tree rather than name the package.
#
# -run='^$$' because this used to run the whole test suite as well, on every
# package, and write its PASS lines into the benchmark file. benchstat ignores
# them, a reader does not, and `make check` is where tests belong.
.PHONY: bench
bench: SHELL := /usr/bin/env bash
bench: .SHELLFLAGS := -o pipefail -c
bench:
	@go test -run='^$$' -bench=. -benchmem -count=$(BENCH_COUNT) -timeout=$(BENCH_TIMEOUT) ./... | tee benchmarks/current.txt

## bench-compare: benchstat current against the committed baseline
#
# Two things have to be normalised into a temporary copy or benchstat compares
# nothing, and says so only by printing a table with one column where there
# should be two.
#
#   the -N suffix   Go writes GOMAXPROCS into every benchmark name. The cloud
#                   runner pins it to 3 and a laptop uses however many cores it
#                   has, and benchstat matches rows by name.
#   goos/goarch/cpu benchstat reads those header lines as the configuration a
#                   run belongs to, so two files that disagree about them land
#                   in separate tables instead of being compared.
#
# The warning above it is the mistake this repository has already made once: a
# baseline recorded on one machine and a current run recorded on another produce
# deltas that are the difference between two machines, printed in a table that
# says they are the difference between two commits. Allocations survive that
# (they are a property of the code) and nanoseconds do not.
#
# It compares the goos/goarch/cpu header the two files carry, not the cpu_model
# in baseline.env, because Go writes no `cpu:` line at all on some platforms and
# this printed `this run was measured on ''` on the first machine that tried it.
.PHONY: bench-compare
bench-compare: bench
	@want=$$(sed -n 's/^goos: /goos=/p;s/^goarch: /goarch=/p;s/^cpu: /cpu=/p' benchmarks/baseline.txt | awk '!seen[$$0]++' | paste -sd' ' -); \
	have=$$(sed -n 's/^goos: /goos=/p;s/^goarch: /goarch=/p;s/^cpu: /cpu=/p' benchmarks/current.txt | awk '!seen[$$0]++' | paste -sd' ' -); \
	if [ "$$want" != "$$have" ]; then \
		echo "WARNING: the baseline was measured on"; \
		echo "           $$want"; \
		echo "         this run was measured on"; \
		echo "           $$have"; \
		echo "         Read the allocs/op columns below and ignore the rest:"; \
		echo "         an allocation count is a property of the code, a"; \
		echo "         nanosecond is a property of the machine."; \
		echo "         For a comparable delta: make bench-cloud"; \
		echo; \
	fi
	@tmp=$$(mktemp -d); trap 'rm -rf "$$tmp"' EXIT; \
	for f in baseline current; do \
		sed -E 's/^(Benchmark[^ 	]*)-[0-9]+/\1/' "benchmarks/$$f.txt" \
			| grep -v -E '^(goos|goarch|cpu):' > "$$tmp/$$f.txt"; \
	done; \
	benchstat "$$tmp/baseline.txt" "$$tmp/current.txt"

## bench-update: promote current to baseline (a deliberate act, say why in the commit)
#
# Refused once a cloud baseline exists, because promoting a laptop run over it
# retargets the committed reference to a different machine and every later
# comparison silently measures the move. `make bench-cloud-update` is the way to
# move it. BENCH_ALLOW_LOCAL_BASELINE=1 overrides, deliberately and in writing.
.PHONY: bench-update
bench-update: bench
	@if [ -f benchmarks/baseline.env ] && [ -z "$(BENCH_ALLOW_LOCAL_BASELINE)" ]; then \
		echo "refusing: benchmarks/baseline.txt was measured on a pinned cloud machine"; \
		sed -n 's/^machine_type=/  machine:  /p;s/^cpu_model=/  cpu:      /p;s/^measured_at=/  measured: /p' benchmarks/baseline.env; \
		echo "  use: make bench-cloud-update"; \
		echo "  or:  make bench-update BENCH_ALLOW_LOCAL_BASELINE=1"; \
		exit 1; \
	fi
	@cp benchmarks/current.txt benchmarks/baseline.txt
	@rm -f benchmarks/baseline.env

## bench-ab: the working tree against a base commit, interleaved on a quiet box
.PHONY: bench-ab
bench-ab:
	@./scripts/bench-ab.sh

## bench-cloud: benchmark on a fresh Compute Engine VM, then delete it
#
# The point is repeatability. A laptop throttles, has a browser open, and is not
# the machine it was three months ago, so a delta measured on one is as likely
# to be the machine as the change. scripts/bench-gcloud.sh rents one pinned
# machine type with SMT off and the turbo clock held, measures on it, and gives
# it back. scripts/bench-gcloud.sh says what each pin is for.
#
# The VM is deleted by a trap, by --max-run-duration on the instance itself, and
# by `make bench-cloud-reap`. Costs roughly $0.30 a run.
.PHONY: bench-cloud
bench-cloud:
	@scripts/bench-gcloud.sh run

## bench-cloud-update: same, and promote the result to benchmarks/baseline.txt
.PHONY: bench-cloud-update
bench-cloud-update:
	@scripts/bench-gcloud.sh run --update

## bench-cloud-dirty: same as bench-cloud, but measure the working tree
.PHONY: bench-cloud-dirty
bench-cloud-dirty:
	@scripts/bench-gcloud.sh run --dirty

## bench-cloud-list: show any benchmark VMs still running
.PHONY: bench-cloud-list
bench-cloud-list:
	@scripts/bench-gcloud.sh list

## bench-cloud-reap: delete every leftover benchmark VM in the project
.PHONY: bench-cloud-reap
bench-cloud-reap:
	@scripts/bench-gcloud.sh reap

## bench-vs: benchmark against araddon/dateparse (separate module, needs network)
#
# benchmarks/compare is its own module, so `bench` above does not reach it and
# neither does anything else in this file: ./... stops at a nested go.mod. That
# is the point. The library has zero dependencies and this comparison needs one,
# so it is quarantined where `go build ./...`, `go test ./...`, govulncheck and
# `make ci` cannot see it, and run by hand when somebody wants the numbers.
#
# Not part of `ci` on purpose. It downloads a module, and a gate that reaches
# the network fails when the network does.
.PHONY: bench-vs
bench-vs:
	@cd benchmarks/compare && go test -run='^$$' -bench=. -benchmem -count=6 -timeout=1800s .

## bench-vs-check: run the comparison module's correctness tests
.PHONY: bench-vs-check
bench-vs-check:
	@cd benchmarks/compare && go vet ./... && go test -v -run=Test .

# ── Build and gates ──────────────────────────────────────────────────────────

## build: compile every package
.PHONY: build
build:
	@go build ./...

## check: what the pre-commit hook runs
.PHONY: check
check: fmt-check vet lint test alloc codegen

## ci: everything CI enforces
.PHONY: ci
ci: fmt-check vet lint test alloc codegen vuln size fuzz

## codegen: fail if the compiler's bounds checks, inlining or escapes got worse
.PHONY: codegen
codegen:
	@./scripts/codegen-gates.sh check

## codegen-update: record today's codegen numbers (a deliberate act, say why)
.PHONY: codegen-update
codegen-update:
	@./scripts/codegen-gates.sh update

## hooks: point git at .githooks (once per clone)
.PHONY: hooks
hooks:
	@git config core.hooksPath .githooks
	@echo "core.hooksPath set to .githooks"

## tools: install the pinned tool versions
.PHONY: tools
tools:
	go install mvdan.cc/gofumpt@$(GOFUMPT_VERSION)
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
	go install golang.org/x/perf/cmd/benchstat@latest

## clean: drop build and benchmark artifacts
.PHONY: clean
clean:
	@rm -f benchmarks/current.txt benchmarks/current.env benchmarks/current.log coverage.out *.test
	@go clean -testcache

# Print one variable, so CI installs the version this file declares rather than
# a second copy of it written into a workflow.
print-%:
	@echo $($*)
