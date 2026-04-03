// Code generated from CLDR data. DO NOT EDIT.
package data

import "github.com/kmoneil/dateparsa/internal/locale"

func init() { locale.Register(&NO) }

var NO = locale.Data{
	Tag:  "no",
	Name: "Norwegian Bokmål",
	MonthsWide: [12]string{
		"januar", "februar", "mars", "april", "mai", "juni",
		"juli", "august", "september", "oktober", "november", "desember",
	},
	MonthsAbbr: [12]string{
		"jan.", "feb.", "mar.", "apr.", "mai", "jun.",
		"jul.", "aug.", "sep.", "okt.", "nov.", "des.",
	},
	WeekdaysWide: [7]string{"søndag", "mandag", "tirsdag", "onsdag", "torsdag", "fredag", "lørdag"},
	WeekdaysAbbr: [7]string{"søn.", "man.", "tir.", "ons.", "tor.", "fre.", "lør."},
	AM:           "a.m.",
	PM:           "p.m.",
	Relative: locale.RelativeKeywords{
		Now:       []string{"nå"},
		Today:     []string{"i dag"},
		Yesterday: []string{"i går"},
		Tomorrow:  []string{"i morgen"},
		Ago:       []string{"siden"},
		InFuture:  []string{"om"},
		Last:      []string{"forrige", "siste"},
		Next:      []string{"neste"},
		This:      []string{"denne", "dette", "denne her"},
		Seconds:   []string{"sekund", "sekunder", "sek"},
		Minutes:   []string{"minutt", "minutter", "min"},
		Hours:     []string{"time", "timer"},
		Days:      []string{"dag", "dager"},
		Weeks:     []string{"uke", "uker"},
		Months:    []string{"måned", "måneder"},
		Years:     []string{"år"},
	},
}
