// Code generated from CLDR data. DO NOT EDIT.
package data

import "github.com/kmoneil/dateparsa/internal/locale"

func init() { locale.Register(&SV) }

var SV = locale.Data{
	Tag:  "sv",
	Name: "Swedish",
	MonthsWide: [12]string{
		"januari", "februari", "mars", "april", "maj", "juni",
		"juli", "augusti", "september", "oktober", "november", "december",
	},
	MonthsAbbr: [12]string{
		"jan.", "feb.", "mars", "apr.", "maj", "juni",
		"juli", "aug.", "sep.", "okt.", "nov.", "dec.",
	},
	WeekdaysWide: [7]string{"söndag", "måndag", "tisdag", "onsdag", "torsdag", "fredag", "lördag"},
	WeekdaysAbbr: [7]string{"sön", "mån", "tis", "ons", "tors", "fre", "lör"},
	AM:           "fm",
	PM:           "em",
	Relative: locale.RelativeKeywords{
		Now:       []string{"nu"},
		Today:     []string{"idag", "i dag"},
		Yesterday: []string{"igår", "i går"},
		Tomorrow:  []string{"imorgon", "i morgon"},
		Ago:       []string{"sedan"},
		InFuture:  []string{"om"},
		Last:      []string{"förra", "föregående"},
		Next:      []string{"nästa", "kommande"},
		This:      []string{"denna", "detta", "den här", "det här"},
		Seconds:   []string{"sekund", "sekunder", "sek"},
		Minutes:   []string{"minut", "minuter", "min"},
		Hours:     []string{"timme", "timmar", "tim"},
		Days:      []string{"dag", "dagar"},
		Weeks:     []string{"vecka", "veckor"},
		Months:    []string{"månad", "månader"},
		Years:     []string{"år"},
	},
}
