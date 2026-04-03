// Code generated from CLDR data. DO NOT EDIT.
package data

import "github.com/kmoneil/dateparsa/internal/locale"

func init() { locale.Register(&PL) }

var PL = locale.Data{
	Tag:  "pl",
	Name: "Polish",
	MonthsWide: [12]string{
		"stycznia", "lutego", "marca", "kwietnia", "maja", "czerwca",
		"lipca", "sierpnia", "września", "października", "listopada", "grudnia",
	},
	MonthsAbbr: [12]string{
		"sty", "lut", "mar", "kwi", "maj", "cze",
		"lip", "sie", "wrz", "paź", "lis", "gru",
	},
	WeekdaysWide: [7]string{"niedziela", "poniedziałek", "wtorek", "środa", "czwartek", "piątek", "sobota"},
	WeekdaysAbbr: [7]string{"niedz.", "pon.", "wt.", "śr.", "czw.", "pt.", "sob."},
	AM:           "AM",
	PM:           "PM",
	Relative: locale.RelativeKeywords{
		Now:       []string{"teraz"},
		Today:     []string{"dzisiaj", "dziś"},
		Yesterday: []string{"wczoraj"},
		Tomorrow:  []string{"jutro"},
		Ago:       []string{"temu"},
		InFuture:  []string{"za"},
		Last:      []string{"zeszły", "zeszła", "zeszłe", "ubiegły", "ubiegła", "ubiegłe"},
		Next:      []string{"następny", "następna", "następne", "przyszły", "przyszła", "przyszłe"},
		This:      []string{"ten", "ta", "to", "tym", "tę"},
		Seconds:   []string{"sekunda", "sekundy", "sekund", "sek"},
		Minutes:   []string{"minuta", "minuty", "minut", "min"},
		Hours:     []string{"godzina", "godziny", "godzin", "godz"},
		Days:      []string{"dzień", "dni"},
		Weeks:     []string{"tydzień", "tygodnie", "tygodni"},
		Months:    []string{"miesiąc", "miesiące", "miesięcy"},
		Years:     []string{"rok", "lata", "lat"},
	},
}
