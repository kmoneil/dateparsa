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
