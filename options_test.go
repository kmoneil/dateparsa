package dateparsa

import (
	"testing"
	"time"
)

// TestEveryOptionTakesEffect covers all seven exported With functions in one
// place. An Option returns a modified copy of the config, so an option whose
// literal forgets its return statement compiles and silently does nothing.
// Nothing else in the suite would fail: the options that steer detection are
// covered by parse tests, and the rest only change what a later branch reads.
//
// It asserts that each option reaches the config and nothing about what reads
// it afterwards, which is exactly how WithPreferYearFirst passed this test for
// months while no detector read the field at all. An option needs a row here
// and a behaviour test somewhere else; this one's is
// TestPreferYearFirstMovesTheYear below.
func TestEveryOptionTakesEffect(t *testing.T) {
	base := time.Date(2019, 7, 4, 12, 0, 0, 0, time.UTC)
	berlin := time.FixedZone("CET", 3600)

	for _, tt := range []struct {
		name   string
		opt    Option
		change func(config) bool
	}{
		{"WithBaseTime", WithBaseTime(base), func(c config) bool { return c.baseTime.Equal(base) }},
		{"WithTimezone", WithTimezone(berlin), func(c config) bool { return c.timezone == berlin }},
		{"WithPreferDayFirst", WithPreferDayFirst(true), func(c config) bool { return c.preferDayFirst }},
		{"WithPreferYearFirst", WithPreferYearFirst(true), func(c config) bool { return c.preferYearFirst }},
		{"WithPreferFuture", WithPreferFuture(true), func(c config) bool { return c.preferFuture }},
		{"WithStrictMode", WithStrictMode(true), func(c config) bool { return c.strictMode }},
		{"WithLocales", WithLocales(FR), func(c config) bool { return len(c.locales) == 1 }},
	} {
		if got := buildConfig([]Option{tt.opt}); !tt.change(got) {
			t.Errorf("%s: buildConfig returned a config the option did not change", tt.name)
		}
	}

	// Adding a field to config makes this unkeyed literal fail to compile,
	// which is the reminder that the new field needs a row above.
	_ = config{time.Time{}, nil, false, false, false, false, nil}

	// A nil location is documented as UTC rather than as a nil dereference
	// later, and that branch is inside the option itself.
	if got := buildConfig([]Option{WithTimezone(nil)}); got.timezone != time.UTC {
		t.Errorf("WithTimezone(nil) = %v, want UTC", got.timezone)
	}
}

// TestOptionsDoNotSeeEachOther fixes the order semantics of the value form:
// each option receives the config the one before it returned, so a later
// option wins and an earlier one is not lost.
func TestOptionsDoNotSeeEachOther(t *testing.T) {
	c := buildConfig([]Option{
		WithPreferDayFirst(true),
		WithStrictMode(true),
		WithPreferDayFirst(false),
	})
	if c.preferDayFirst {
		t.Error("preferDayFirst = true, want the last option to win")
	}
	if !c.strictMode {
		t.Error("strictMode = false, want an earlier option to survive a later one")
	}
}

// TestBuildConfigDefaults pins what a caller who passes no options gets, which
// is what Parse writes inline rather than routing through buildConfig.
func TestBuildConfigDefaults(t *testing.T) {
	if got := buildConfig(nil); got.timezone != time.UTC {
		t.Errorf("buildConfig(nil).timezone = %v, want UTC", got.timezone)
	}
	if got := buildConfig(nil); !got.baseTime.IsZero() {
		t.Errorf("buildConfig(nil).baseTime = %v, want the zero time", got.baseTime)
	}
}

// TestPreferYearFirstMovesTheYear is C22.
//
// WithPreferYearFirst was accepted, stored, threaded into detect.Config, and
// read by nothing: "01/02/03" was the second of January 2003 with the option on
// and with it off, while README documented it as a preference rule in two
// places. This is the test that would have failed the whole time.
func TestPreferYearFirstMovesTheYear(t *testing.T) {
	base := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		in        string
		off, on   string // the day parsed without the option, and with it
		ambigWith bool   // Ambiguous, with the option on
		why       string
	}{
		{
			"01/02/03", "2003-01-02", "2001-02-03", true,
			"three small parts, so the year can be either end",
		},
		{
			"01-02-03", "2003-01-02", "2001-02-03", true,
			"the separator does not decide the year's position",
		},
		{
			"25/12/01", "2001-12-25", "2025-12-01", true,
			"25 cannot be a month either way, and both readings still exist",
		},
		{
			"13/01/03", "2003-01-13", "2013-01-03", true,
			"13 cannot be a month, which settles the order and not the year",
		},

		// Inputs the reading cannot describe. The option is a preference, so it
		// applies where it can and stands aside where it cannot, rather than
		// refusing the row.
		{
			"01/13/03", "2003-01-13", "2003-01-13", false,
			"year-first would need month 13",
		},
		{
			"1/02/03", "2003-01-02", "2003-01-02", true,
			"a year field reads two bytes or four, never one",
		},
		{
			"01/02/2024", "2024-01-02", "2024-01-02", true,
			"a four-digit year is where it is written",
		},
		{
			"2024/01/02", "2024-01-02", "2024-01-02", false,
			"already year-first, and the trie reads it",
		},
		{
			"01/02/03 10:30:00", "2003-01-02", "2001-02-03", true,
			"a trailing time does not change which part is the year",
		},
	} {
		off, err := ParseWith(tt.in, WithBaseTime(base))
		if err != nil {
			t.Errorf("ParseWith(%q): %v", tt.in, err)
			continue
		}
		if got := off.Time.Format("2006-01-02"); got != tt.off {
			t.Errorf("ParseWith(%q) = %s, want %s (%s)", tt.in, got, tt.off, tt.why)
		}

		on, err := ParseWith(tt.in, WithBaseTime(base), WithPreferYearFirst(true))
		if err != nil {
			t.Errorf("ParseWith(%q, yearFirst): %v", tt.in, err)
			continue
		}
		if got := on.Time.Format("2006-01-02"); got != tt.on {
			t.Errorf("ParseWith(%q, yearFirst) = %s, want %s (%s)", tt.in, got, tt.on, tt.why)
		}
		if on.Ambiguous != tt.ambigWith {
			t.Errorf("ParseWith(%q, yearFirst).Ambiguous = %v, want %v (%s)",
				tt.in, on.Ambiguous, tt.ambigWith, tt.why)
		}
	}
}
