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

## lint: golangci-lint
.PHONY: lint
lint:
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
.PHONY: bench
bench:
	@go test -bench=. -benchmem -count=3 -timeout=600s | tee benchmarks/current.txt

## bench-compare: benchstat current against the committed baseline
.PHONY: bench-compare
bench-compare: bench
	@benchstat benchmarks/baseline.txt benchmarks/current.txt

## bench-update: promote current to baseline (a deliberate act, say why in the commit)
.PHONY: bench-update
bench-update: bench
	@cp benchmarks/current.txt benchmarks/baseline.txt

# ── Build and gates ──────────────────────────────────────────────────────────

## build: compile every package
.PHONY: build
build:
	@go build ./...

## check: what the pre-commit hook runs
.PHONY: check
check: fmt-check vet lint test alloc

## ci: everything CI enforces
.PHONY: ci
ci: fmt-check vet lint test alloc vuln size fuzz

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
	@rm -f benchmarks/current.txt coverage.out *.test
	@go clean -testcache

# Print one variable, so CI installs the version this file declares rather than
# a second copy of it written into a workflow.
print-%:
	@echo $($*)
