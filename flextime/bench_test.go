package flextime

import (
	"testing"
	"time"
)

// The src arguments are boxed into an any once, outside the loop, because a
// database driver hands sql.Scanner a value that is already an interface. A
// benchmark that writes ft.Scan(now) boxes a time.Time on every iteration and
// reports the compiler's decision about that box rather than the cost of the
// method it names: this one measured 21.00 ns and 1 alloc/op that way against
// 2.1 to 2.5 ns and 0 allocs, and the []byte arm reported 3 allocations where
// Scan makes 2. See README's flextime allocation table, whose rows are these
// benchmarks.
func BenchmarkScanTimeTime(b *testing.B) {
	var src any = time.Now()
	var ft FlexTime
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ft.Scan(src)
	}
	_ = ft
}

func BenchmarkScanString(b *testing.B) {
	var ft FlexTime
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ft.Scan("2024-03-15T10:30:00Z")
	}
	_ = ft
}

func BenchmarkScanBytes(b *testing.B) {
	var src any = []byte("2024-03-15T10:30:00Z")
	var ft FlexTime
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ft.Scan(src)
	}
	_ = ft
}

func BenchmarkScanInt64(b *testing.B) {
	var ft FlexTime
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ft.Scan(int64(1710505800))
	}
	_ = ft
}

// A float64 column takes the other epoch arm, epoch.FromSeconds, and nothing
// measured it: the int64 arm has had a benchmark since this file was written
// and its float sibling has not.
func BenchmarkScanFloat64(b *testing.B) {
	var src any = 1710505800.5
	var ft FlexTime
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ft.Scan(src)
	}
	_ = ft
}

// Scanner is the type documented as being for database rows, and it was the one
// type in this file nothing measured. It used to rebuild its []dateparsa.Option
// from fixed configuration on every value and then run a full detection, so a
// column scanned through it paid per row what the library exists to charge once.
//
// These are here so that stops being invisible. See
// _plans/backlog/done/p5-flextime-scanner-rebuilds-its-options-per-row.md.
func BenchmarkScanner_Default(b *testing.B) {
	s := NewScanner()
	var ft FlexTime
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Scan(&ft, "2024-03-15T10:30:00Z")
	}
	_ = ft
}

func BenchmarkScanner_Configured(b *testing.B) {
	s := NewScanner(WithPreferDayFirst(true), WithTimezone(time.UTC))
	var ft FlexTime
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Scan(&ft, "2024-03-15T10:30:00Z")
	}
	_ = ft
}

// BenchmarkScanner_Parallel measures one Scanner shared by every available
// goroutine, which is what a package-level scanner in a request handler looks
// like. It is here because the design of the layout cache turned on this
// number: a mutex around the cache measured 130 to 150 ns/op here against 99 to
// 119 for the uncached code it would have replaced, so the cache is an atomic
// pointer to an immutable Layout and readers never exclude each other.
func BenchmarkScanner_Parallel(b *testing.B) {
	s := NewScanner()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		var ft FlexTime
		for pb.Next() {
			s.Scan(&ft, "2024-03-15T10:30:00Z")
		}
	})
}

func BenchmarkValue(b *testing.B) {
	ft := New(time.Now())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ft.Value()
	}
}

func BenchmarkMarshalJSON(b *testing.B) {
	ft := New(time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ft.MarshalJSON()
	}
}

func BenchmarkUnmarshalJSONString(b *testing.B) {
	data := []byte(`"2024-03-15T10:30:00Z"`)
	var ft FlexTime
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ft.UnmarshalJSON(data)
	}
}

func BenchmarkUnmarshalJSONNumber(b *testing.B) {
	data := []byte("1710505800")
	var ft FlexTime
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ft.UnmarshalJSON(data)
	}
}

// A number written with a fraction takes the seconds arm, which decodes through
// encoding/json rather than reading the bytes, so it is the one numeric shape
// that allocates. "A JSON number costs nothing" is true of the integer form
// above and false here, and nothing measured the difference until this.
func BenchmarkUnmarshalJSONFloat(b *testing.B) {
	data := []byte("1710505800.5")
	var ft FlexTime
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ft.UnmarshalJSON(data)
	}
}

// A null column is what the NULL-safe promise is about, and it is a string
// compare and a store.
func BenchmarkUnmarshalJSONNull(b *testing.B) {
	data := []byte("null")
	var ft FlexTime
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ft.UnmarshalJSON(data)
	}
}
