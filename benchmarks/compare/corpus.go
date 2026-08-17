// Package compare benchmarks dateparsa against github.com/araddon/dateparse,
// the other Go library that detects a date format rather than being told one.
//
// It is a separate module. See go.mod for why, and README.md for what the
// numbers mean and what they do not.
package compare

// Corpus is the set of inputs both libraries are asked to parse.
//
// Every entry is a format a real ingest column carries, and the set is fixed
// rather than generated so a number reproduces. What it is NOT is a list of
// formats chosen because dateparsa wins them: TestBothParseTheCorpus asserts
// that both libraries accept every entry, so a format either library cannot
// read fails the suite rather than quietly becoming a benchmark of one library
// against the other's error path. An error path is much cheaper than a parse in
// one library and much dearer in the other, and either way it is not a
// comparison of parsing.
//
// Formats only one side supports are listed in README.md instead, with which
// side, because that is a capability difference and not a speed one.
var Corpus = []struct {
	Name  string
	Input string
}{
	{"ISO8601_date", "2024-03-15"},
	{"ISO8601_datetime", "2024-03-15T10:30:00"},
	{"ISO8601_datetime_Z", "2024-03-15T10:30:00Z"},
	{"RFC3339_offset", "2024-03-15T10:30:00+05:30"},
	{"RFC3339_nano", "2024-03-15T10:30:00.123456789Z"},
	{"SQL_datetime", "2024-03-15 10:30:00"},
	{"SQL_datetime_frac", "2024-03-15 10:30:00.123"},
	{"US_slash", "03/15/2024"},
	{"US_slash_time", "03/15/2024 10:30:00"},
	{"compact_date", "20240315"},
	{"textual_month", "March 15, 2024"},
	{"textual_month_abbr", "Mar 15, 2024"},
	{"day_month_year_text", "15 March 2024"},
	{"RFC1123", "Fri, 15 Mar 2024 10:30:00 UTC"},
	{"ANSIC", "Mon Jan  2 15:04:05 2006"},
	{"unix_seconds", "1710498600"},
}

// Misses are values a date column contains that are not dates.
//
// A real import has empty cells, "N/A", and free text somebody typed into the
// wrong field, and both libraries walk their whole detection path before
// refusing. It is a cost neither library's own benchmarks measure and every
// caller pays.
var Misses = []struct {
	Name  string
	Input string
}{
	{"short", "N/A"},
	{"text", "not a date at all"},
	{"empty", ""},
	{"numeric_junk", "12345678901234567890"},
}
