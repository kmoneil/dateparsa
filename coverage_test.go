package dateparsa

import (
	"fmt"
	"testing"
)

func TestFormatCoverage(t *testing.T) {
	type testCase struct {
		input    string
		dayFirst bool
		desc     string
	}

	tests := []testCase{
		// === Structured ISO / RFC ===
		{input: "2024-03-15", desc: "1. ISO date"},
		{input: "2024-03-15T10:30:00", desc: "2. ISO datetime"},
		{input: "2024-03-15T10:30:00Z", desc: "3. ISO datetime UTC"},
		{input: "2024-03-15T10:30:00+05:30", desc: "4. RFC 3339"},
		{input: "2024-03-15T10:30:00-08:00", desc: "5. RFC 3339 negative"},
		{input: "2024-03-15T10:30:00.123456789Z", desc: "6. RFC 3339 nano"},
		{input: "2024-03-15T10:30:00.123+05:30", desc: "7. RFC 3339 frac + tz"},

		// === Text-based RFC formats ===
		{input: "Fri, 15 Mar 2024 10:30:00 +0000", desc: "8. RFC 2822"},
		{input: "Friday, 15-Mar-24 10:30:00 UTC", desc: "9. RFC 850"},
		{input: "Fri Mar 15 10:30:00 2024", desc: "10. ANSIC"},
		{input: "Fri Mar 15 10:30:00 UTC 2024", desc: "11. Unix date"},

		// === US formats ===
		{input: "03/15/2024", desc: "12. US slash"},
		{input: "3/15/24", desc: "13. US short"},
		{input: "3/15/2024", desc: "14. US variable width"},
		{input: "March 15, 2024", desc: "15. US long"},
		{input: "Mar 15, 2024", desc: "16. US abbreviated"},

		// === European formats ===
		{input: "15/03/2024", dayFirst: true, desc: "17. European slash (PreferDayFirst)"},
		{input: "15.03.2024", desc: "18. European dot"},
		{input: "15 March 2024", desc: "19. European long"},
		{input: "15-Mar-2024", desc: "20. European dash abbreviated"},

		// === Asian formats ===
		{input: "2024/03/15", desc: "21. Asian slash"},
		{input: "2024.03.15", desc: "22. Asian dot"},

		// === Time only ===
		{input: "10:30", desc: "23. time HH:MM"},
		{input: "10:30:00", desc: "24. time HH:MM:SS"},
		{input: "10:30 PM", desc: "25. time 12h"},
		{input: "10:30:00 PM", desc: "26. time 12h with seconds"},
		{input: "10:30:00.123", desc: "27. time with millis"},
		{input: "10:30:00.123456", desc: "28. time with micros"},

		// === Partial dates ===
		{input: "Mar 15", desc: "29. partial month day"},
		{input: "15 Mar", desc: "30. partial day month"},
		{input: "March 2024", desc: "31. partial month year"},

		// === Unix timestamps ===
		{input: "1710500000", desc: "32. unix seconds"},
		{input: "1710500000000", desc: "33. unix millis"},
		{input: "1710500000.123", desc: "34. unix fractional"},

		// === Compact formats ===
		{input: "20240315", desc: "35. compact date"},
		{input: "20240315T103000", desc: "36. compact datetime"},
		{input: "20240315103000", desc: "37. compact datetime no sep"},
		{input: "20240315T103000Z", desc: "38. compact datetime UTC"},

		// === ISO week / ordinal ===
		{input: "2024-W11-5", desc: "39. ISO week date"},
		{input: "2024-W11", desc: "40. ISO week"},
		{input: "2024-074", desc: "41. ISO ordinal"},

		// === Syslog / log formats ===
		{input: "Mar 15 10:30:00", desc: "42. syslog"},
		{input: "15/Mar/2024:10:30:00 +0000", desc: "43. common log format"},

		// === SQL datetime variants ===
		{input: "2024-03-15 10:30:00", desc: "44. SQL datetime"},
		{input: "2024-03-15 10:30:00.000", desc: "45. SQL millis"},
		{input: "2024-03-15 10:30:00.000000", desc: "46. SQL micros"},
		{input: "2024-03-15 10:30:00+00", desc: "47. SQL short tz"},
		{input: "2024-03-15 10:30:00+05:30", desc: "48. SQL full tz"},

		// === Spreadsheet formats ===
		{input: "3/15/2024 10:30:00 AM", desc: "49. spreadsheet US"},
		{input: "15-Mar-2024 10:30", desc: "50. spreadsheet EU"},

		// === Go reference / RFC 822 / RFC 1123 ===
		{input: "Fri Mar 15 10:30:00 EDT 2024", desc: "51. Go reference time"},
		{input: "15 Mar 24 10:30 UTC", desc: "52. RFC 822"},
		{input: "Fri, 15 Mar 2024 10:30:00 UTC", desc: "53. RFC 1123"},
	}

	passed := 0
	failed := 0
	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			var result ParseResult
			var err error

			if tc.dayFirst {
				result, err = ParseWith(tc.input, WithPreferDayFirst(true))
			} else {
				result, err = Parse(tc.input)
			}

			if err != nil {
				failed++
				t.Errorf("FAIL: %s\n  input:  %q\n  error:  %v", tc.desc, tc.input, err)
				return
			}

			passed++
			fmt.Printf("PASS: %-45s -> %v\n", tc.desc, result.Time.Format("2006-01-02 15:04:05.999999999 -0700"))
		})
	}

	t.Logf("\n=== SUMMARY: %d passed, %d failed out of %d total ===", passed, failed, passed+failed)
}
