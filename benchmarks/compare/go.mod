// This module is deliberately separate from github.com/kmoneil/dateparsa.
//
// The library has zero module dependencies and README promises exactly that, so
// the comparison against another parser cannot live in it: a require line here
// would otherwise land in every downstream binary and every downstream
// vulnerability scan. A nested module is also excluded from the parent's zip by
// the module proxy, so nothing here is even fetched by `go get` on the library.
//
// Being separate is what keeps `go build ./...`, `go test ./...`, `make ci` and
// govulncheck at the repository root blind to it. Run it explicitly:
// `make bench-vs`, or `go test -bench=. ./benchmarks/compare/`.
module github.com/kmoneil/dateparsa/benchmarks/compare

go 1.26.1

replace github.com/kmoneil/dateparsa => ../..

require (
	github.com/araddon/dateparse v0.0.0-20210429162001-6b43995a97de
	github.com/kmoneil/dateparsa v0.0.0-00010101000000-000000000000
)
