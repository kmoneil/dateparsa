// Code generated from CLDR data. DO NOT EDIT.
package data

import "github.com/kmoneil/dateparsa/internal/locale"

func init() { locale.Register(&NL) }

var NL = locale.Data{
	Tag:  "nl",
	Name: "Dutch",
	MonthsWide: [12]string{
		"januari", "februari", "maart", "april", "mei", "juni",
		"juli", "augustus", "september", "oktober", "november", "december",
	},
	MonthsAbbr: [12]string{
		"jan.", "feb.", "mrt.", "apr.", "mei", "jun.",
		"jul.", "aug.", "sep.", "okt.", "nov.", "dec.",
	},
	WeekdaysWide: [7]string{"zondag", "maandag", "dinsdag", "woensdag", "donderdag", "vrijdag", "zaterdag"},
	WeekdaysAbbr: [7]string{"zo", "ma", "di", "wo", "do", "vr", "za"},
	AM:           "a.m.",
	PM:           "p.m.",
	Relative: locale.RelativeKeywords{
		Now:       []string{"nu"},
		Today:     []string{"vandaag"},
		Yesterday: []string{"gisteren"},
		Tomorrow:  []string{"morgen"},
		Ago:       []string{"geleden"},
		InFuture:  []string{"over"},
		Last:      []string{"afgelopen", "vorige", "vorig"},
		Next:      []string{"volgende", "volgend", "aanstaande"},
		This:      []string{"deze", "dit"},
		Seconds:   []string{"seconde", "seconden", "sec"},
		Minutes:   []string{"minuut", "minuten", "min"},
		Hours:     []string{"uur", "uren"},
		Days:      []string{"dag", "dagen"},
		Weeks:     []string{"week", "weken"},
		Months:    []string{"maand", "maanden"},
		Years:     []string{"jaar", "jaren"},
	},
}
