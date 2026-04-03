// Code generated from CLDR data. DO NOT EDIT.
package data

import "github.com/kmoneil/dateparsa/internal/locale"

func init() { locale.Register(&DA) }

var DA = locale.Data{
	Tag:  "da",
	Name: "Danish",
	MonthsWide: [12]string{
		"januar", "februar", "marts", "april", "maj", "juni",
		"juli", "august", "september", "oktober", "november", "december",
	},
	MonthsAbbr: [12]string{
		"jan.", "feb.", "mar.", "apr.", "maj", "jun.",
		"jul.", "aug.", "sep.", "okt.", "nov.", "dec.",
	},
	WeekdaysWide: [7]string{"søndag", "mandag", "tirsdag", "onsdag", "torsdag", "fredag", "lørdag"},
	WeekdaysAbbr: [7]string{"søn.", "man.", "tir.", "ons.", "tor.", "fre.", "lør."},
	AM:           "AM",
	PM:           "PM",
	Relative: locale.RelativeKeywords{
		Now:       []string{"nu"},
		Today:     []string{"i dag"},
		Yesterday: []string{"i går"},
		Tomorrow:  []string{"i morgen"},
		Ago:       []string{"siden"},
		InFuture:  []string{"om"},
		Last:      []string{"sidste", "forrige"},
		Next:      []string{"næste", "kommende"},
		This:      []string{"denne", "dette"},
		Seconds:   []string{"sekund", "sekunder", "sek"},
		Minutes:   []string{"minut", "minutter", "min"},
		Hours:     []string{"time", "timer"},
		Days:      []string{"dag", "dage"},
		Weeks:     []string{"uge", "uger"},
		Months:    []string{"måned", "måneder"},
		Years:     []string{"år"},
	},
}
