package dateparsa

import (
	"testing"
	"time"
)

// === Compact Formats ===

func TestParse_CompactDate(t *testing.T) {
	result, err := Parse("20240315")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	if !result.Time.Equal(expected) {
		t.Errorf("got %v, want %v", result.Time, expected)
	}
	if result.Layout.String() != "COMPACT_DATE" {
		t.Errorf("layout = %q, want COMPACT_DATE", result.Layout.String())
	}
}

func TestParse_CompactDateTime(t *testing.T) {
	result, err := Parse("20240315T103000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)
	if !result.Time.Equal(expected) {
		t.Errorf("got %v, want %v", result.Time, expected)
	}
}

func TestParse_CompactDateTimeNoSep(t *testing.T) {
	result, err := Parse("20240315103000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)
	if !result.Time.Equal(expected) {
		t.Errorf("got %v, want %v", result.Time, expected)
	}
}

func TestParse_CompactDateTimeZ(t *testing.T) {
	result, err := Parse("20240315T103000Z")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)
	if !result.Time.Equal(expected) {
		t.Errorf("got %v, want %v", result.Time, expected)
	}
}

// === Unix Timestamps ===

func TestParse_UnixSeconds(t *testing.T) {
	result, err := Parse("1710504800")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Unix(1710504800, 0).UTC()
	if !result.Time.Equal(expected) {
		t.Errorf("got %v, want %v", result.Time, expected)
	}
}

func TestParse_UnixMillis(t *testing.T) {
	result, err := Parse("1710504800000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Unix(1710504800, 0).UTC()
	if !result.Time.Equal(expected) {
		t.Errorf("got %v, want %v", result.Time, expected)
	}
}

func TestParse_UnixMicros(t *testing.T) {
	result, err := Parse("1710504800000000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Unix(1710504800, 0).UTC()
	if !result.Time.Equal(expected) {
		t.Errorf("got %v, want %v", result.Time, expected)
	}
}

func TestParse_UnixNanos(t *testing.T) {
	result, err := Parse("1710504800000000000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Unix(1710504800, 0).UTC()
	if !result.Time.Equal(expected) {
		t.Errorf("got %v, want %v", result.Time, expected)
	}
}

func TestParse_UnixFractional(t *testing.T) {
	result, err := Parse("1710504800.500")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Time.Nanosecond() != 500000000 {
		t.Errorf("nsec = %d, want 500000000", result.Time.Nanosecond())
	}
}

func TestParse_UnixNegative(t *testing.T) {
	result, err := Parse("-1710504800")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Time.After(time.Unix(0, 0)) {
		t.Errorf("expected time before epoch, got %v", result.Time)
	}
}

// === ISO Week Dates ===

func TestParse_ISOWeekDate(t *testing.T) {
	// 2024-W11-5 = Friday of ISO week 11 of 2024.
	// ISO week 11 of 2024: Mon March 11 to Sun March 17.
	// Day 5 (Friday) = March 15, 2024.
	result, err := Parse("2024-W11-5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	if !result.Time.Equal(expected) {
		t.Errorf("got %v, want %v", result.Time, expected)
	}
}

func TestParse_ISOWeekOnly(t *testing.T) {
	// 2024-W11 = Monday of ISO week 11 of 2024 = March 11, 2024.
	result, err := Parse("2024-W11")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Date(2024, 3, 11, 0, 0, 0, 0, time.UTC)
	if !result.Time.Equal(expected) {
		t.Errorf("got %v, want %v", result.Time, expected)
	}
}

// === ISO Ordinal Dates ===

func TestParse_ISOOrdinal(t *testing.T) {
	// 2024-074 = 74th day of 2024.
	// 2024 is a leap year: Jan=31, Feb=29 → 60 days. March 14 = day 74.
	// Wait: 31+29=60, so day 74 = March 14? 60+14=74. Yes.
	result, err := Parse("2024-074")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Date(2024, 3, 14, 0, 0, 0, 0, time.UTC)
	if !result.Time.Equal(expected) {
		t.Errorf("got %v, want %v", result.Time, expected)
	}
}

func TestParse_ISOOrdinalDay1(t *testing.T) {
	result, err := Parse("2024-001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	if !result.Time.Equal(expected) {
		t.Errorf("got %v, want %v", result.Time, expected)
	}
}

func TestParse_ISOOrdinalDay366(t *testing.T) {
	// 2024 is a leap year, so day 366 = December 31.
	result, err := Parse("2024-366")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)
	if !result.Time.Equal(expected) {
		t.Errorf("got %v, want %v", result.Time, expected)
	}
}

// === Time with AM/PM ===

func TestParse_TimeAMPM(t *testing.T) {
	tests := []struct {
		input string
		hour  int
		min   int
	}{
		{"10:30 AM", 10, 30},
		{"10:30 PM", 22, 30},
		{"12:00 AM", 0, 0},
		{"12:00 PM", 12, 0},
		{"01:15 PM", 13, 15},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.input, err)
			}
			if result.Time.Hour() != tt.hour || result.Time.Minute() != tt.min {
				t.Errorf("Parse(%q) = %02d:%02d, want %02d:%02d",
					tt.input, result.Time.Hour(), result.Time.Minute(), tt.hour, tt.min)
			}
		})
	}
}

func TestParse_TimeHMSAMPM(t *testing.T) {
	result, err := Parse("03:30:45 PM")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Time.Hour() != 15 || result.Time.Minute() != 30 || result.Time.Second() != 45 {
		t.Errorf("got %02d:%02d:%02d, want 15:30:45",
			result.Time.Hour(), result.Time.Minute(), result.Time.Second())
	}
}

// === Time with Fractional Seconds ===

func TestParse_TimeFrac3(t *testing.T) {
	result, err := Parse("10:30:00.123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Time.Nanosecond() != 123000000 {
		t.Errorf("nsec = %d, want 123000000", result.Time.Nanosecond())
	}
}

func TestParse_TimeFrac6(t *testing.T) {
	result, err := Parse("10:30:00.123456")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Time.Nanosecond() != 123456000 {
		t.Errorf("nsec = %d, want 123456000", result.Time.Nanosecond())
	}
}

// === SQL Datetime Variants ===

func TestParse_SQLDatetimeFrac3(t *testing.T) {
	result, err := Parse("2024-03-15 10:30:00.123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Date(2024, 3, 15, 10, 30, 0, 123000000, time.UTC)
	if !result.Time.Equal(expected) {
		t.Errorf("got %v, want %v", result.Time, expected)
	}
}

func TestParse_SQLDatetimeFrac6(t *testing.T) {
	result, err := Parse("2024-03-15 10:30:00.000000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)
	if !result.Time.Equal(expected) {
		t.Errorf("got %v, want %v", result.Time, expected)
	}
}

func TestParse_SQLDatetimeTZShort(t *testing.T) {
	result, err := Parse("2024-03-15 10:30:00+00")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Date(2024, 3, 15, 10, 30, 0, 0, time.FixedZone("+00", 0))
	if !result.Time.Equal(expected) {
		t.Errorf("got %v, want %v", result.Time, expected)
	}
}

func TestParse_SQLDatetimeTZ(t *testing.T) {
	result, err := Parse("2024-03-15 10:30:00+05:30")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Date(2024, 3, 15, 10, 30, 0, 0, time.FixedZone("+05:30", 5*3600+30*60))
	if !result.Time.Equal(expected) {
		t.Errorf("got %v, want %v", result.Time, expected)
	}
}

// === Textual Month — Extended ===

func TestParse_RFC2822Style(t *testing.T) {
	// Fri, 15 Mar 2024 10:30:00 +0000
	result, err := Parse("Fri, 15 Mar 2024 10:30:00 +0000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Date(2024, 3, 15, 10, 30, 0, 0, time.FixedZone("+0000", 0))
	if !result.Time.Equal(expected) {
		t.Errorf("got %v, want %v", result.Time, expected)
	}
}

func TestParse_CommonLogFormat(t *testing.T) {
	// 15/Mar/2024:10:30:00 +0000
	result, err := Parse("15/Mar/2024:10:30:00 +0000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Date(2024, 3, 15, 10, 30, 0, 0, time.FixedZone("+0000", 0))
	if !result.Time.Equal(expected) {
		t.Errorf("got %v, want %v", result.Time, expected)
	}
}

func TestParse_SyslogStyle(t *testing.T) {
	// Mar 15 10:30:00 (syslog — no year)
	result, err := Parse("Mar 15 10:30:00")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Time.Month() != 3 || result.Time.Day() != 15 {
		t.Errorf("got month=%d day=%d, want March 15", result.Time.Month(), result.Time.Day())
	}
	if result.Time.Hour() != 10 || result.Time.Minute() != 30 {
		t.Errorf("got %02d:%02d, want 10:30", result.Time.Hour(), result.Time.Minute())
	}
}

func TestParse_PartialMonthYear(t *testing.T) {
	// March 2024
	result, err := Parse("March 2024")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Time.Month() != 3 || result.Time.Year() != 2024 {
		t.Errorf("got %v, want March 2024", result.Time)
	}
}

func TestParse_PartialMonthDay(t *testing.T) {
	// Mar 15
	result, err := Parse("Mar 15")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Time.Month() != 3 || result.Time.Day() != 15 {
		t.Errorf("got %v, want March 15", result.Time)
	}
}

func TestParse_PartialDayMonth(t *testing.T) {
	// 15 Mar
	result, err := Parse("15 Mar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Time.Month() != 3 || result.Time.Day() != 15 {
		t.Errorf("got %v, want March 15", result.Time)
	}
}

func TestParse_TextualWithTime(t *testing.T) {
	// March 15, 2024 10:30:00
	result, err := Parse("March 15, 2024 10:30:00")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)
	if !result.Time.Equal(expected) {
		t.Errorf("got %v, want %v", result.Time, expected)
	}
}

// === Layout Reuse for Phase 2 Formats ===

func TestLayout_Reuse_Compact(t *testing.T) {
	result, err := Parse("20240315")
	if err != nil {
		t.Fatal(err)
	}
	t2, err := result.Layout.Parse("20250101")
	if err != nil {
		t.Fatal(err)
	}
	expected := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	if !t2.Equal(expected) {
		t.Errorf("got %v, want %v", t2, expected)
	}
}

func TestLayout_Reuse_CompactDateTime(t *testing.T) {
	result, err := Parse("20240315T103000")
	if err != nil {
		t.Fatal(err)
	}
	t2, err := result.Layout.Parse("20250101T000000")
	if err != nil {
		t.Fatal(err)
	}
	expected := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	if !t2.Equal(expected) {
		t.Errorf("got %v, want %v", t2, expected)
	}
}

// === Benchmarks for Phase 2 ===

func BenchmarkParse_Compact(b *testing.B) {
	for b.Loop() {
		Parse("20240315")
	}
}

func BenchmarkParse_CompactDateTime(b *testing.B) {
	for b.Loop() {
		Parse("20240315T103000")
	}
}

func BenchmarkParse_UnixTimestamp(b *testing.B) {
	for b.Loop() {
		Parse("1710504800")
	}
}

func BenchmarkParse_ISOWeekDate(b *testing.B) {
	for b.Loop() {
		Parse("2024-W11-5")
	}
}

func BenchmarkParse_ISOOrdinal(b *testing.B) {
	for b.Loop() {
		Parse("2024-074")
	}
}

func BenchmarkParse_TimeAMPM(b *testing.B) {
	for b.Loop() {
		Parse("10:30 PM")
	}
}

func BenchmarkParse_SQLFrac6(b *testing.B) {
	for b.Loop() {
		Parse("2024-03-15 10:30:00.000000")
	}
}

func BenchmarkLayout_Parse_Compact(b *testing.B) {
	result, _ := Parse("20240315")
	layout := result.Layout
	b.ResetTimer()
	for b.Loop() {
		layout.Parse("20250101")
	}
}
