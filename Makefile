.PHONY: test lint vet fuzz bench bench-compare alloc build clean hooks

# Run all tests with race detection
test:
	go test -race -count=1 -timeout=120s ./...

# Run linter
lint:
	golangci-lint run --timeout=2m

# Run go vet
vet:
	go vet ./...

# Run fuzz tests (30 seconds each)
fuzz:
	go test -fuzz=FuzzParse -fuzztime=30s -timeout=120s
	go test -fuzz=FuzzDetect -fuzztime=30s -timeout=120s

# Run benchmarks and save results
bench:
	go test -bench=. -benchmem -count=3 -timeout=300s | tee benchmarks/current.txt

# Compare benchmarks against baseline
bench-compare: bench
	benchstat benchmarks/baseline.txt benchmarks/current.txt

# Update the benchmark baseline
bench-update: bench
	cp benchmarks/current.txt benchmarks/baseline.txt

# Verify zero-alloc guarantee on Layout.Parse
alloc:
	go test -run=TestLayoutParseZeroAlloc -count=1 -timeout=30s

# Build (verify compilation)
build:
	go build ./...

# Pre-commit: fast checks
check: vet test

# Full CI suite locally
ci: vet lint test alloc fuzz

# Clean build artifacts
clean:
	rm -f benchmarks/current.txt
	go clean -testcache

# Install git hooks
hooks:
	@echo "Installing git hooks..."
	@cp scripts/pre-commit .git/hooks/pre-commit
	@chmod +x .git/hooks/pre-commit
	@echo "Done. Pre-commit hook installed."
