package dateparsa_test

import (
	"fmt"
	"log"

	"github.com/kmoneil/dateparsa"
)

func ExampleParse() {
	result, err := dateparsa.Parse("2024-03-15T10:30:00Z")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result.Time.UTC())
	// Output: 2024-03-15 10:30:00 +0000 UTC
}

func ExampleParse_textualMonth() {
	result, err := dateparsa.Parse("March 15, 2024")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result.Time.UTC())
	fmt.Println(result.Ambiguous)
	// Output:
	// 2024-03-15 00:00:00 +0000 UTC
	// false
}

func ExampleLayout_Parse() {
	// Detect the format once
	result, err := dateparsa.Parse("2024-03-15")
	if err != nil {
		log.Fatal(err)
	}

	// Reuse the layout for subsequent parses — zero allocations
	t, err := result.Layout.Parse("2025-06-01")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(t.UTC())
	// Output: 2025-06-01 00:00:00 +0000 UTC
}

func ExampleParseWith_dayFirst() {
	result, err := dateparsa.ParseWith("01/02/2024",
		dateparsa.WithPreferDayFirst(true))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result.Time.UTC())
	// Output: 2024-02-01 00:00:00 +0000 UTC
}

func ExampleNewParser() {
	p := dateparsa.NewParser()

	// Detects format on first row, reuses for the rest
	times, errs := p.ParseColumn([]string{
		"2024-01-01",
		"2024-06-15",
		"2024-12-31",
	})

	for i, t := range times {
		if errs[i] != nil {
			continue
		}
		fmt.Println(t.UTC().Format("2006-01-02"))
	}
	// Output:
	// 2024-01-01
	// 2024-06-15
	// 2024-12-31
}
