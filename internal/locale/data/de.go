// Code generated from CLDR data. DO NOT EDIT.
package data

import "github.com/kmoneil/dateparsa/internal/locale"

func init() { locale.Register(&DE) }

var DE = locale.Data{
	Tag:  "de",
	Name: "German",
	MonthsWide: [12]string{
		"Januar", "Februar", "März", "April", "Mai", "Juni",
		"Juli", "August", "September", "Oktober", "November", "Dezember",
	},
	MonthsAbbr: [12]string{
		"Jan.", "Feb.", "März", "Apr.", "Mai", "Juni",
		"Juli", "Aug.", "Sept.", "Okt.", "Nov.", "Dez.",
	},
	WeekdaysWide: [7]string{"Sonntag", "Montag", "Dienstag", "Mittwoch", "Donnerstag", "Freitag", "Samstag"},
	WeekdaysAbbr: [7]string{"So.", "Mo.", "Di.", "Mi.", "Do.", "Fr.", "Sa."},
	AM:           "AM",
	PM:           "PM",
	Relative: locale.RelativeKeywords{
		Now:       []string{"jetzt"},
		Today:     []string{"heute"},
		Yesterday: []string{"gestern"},
		Tomorrow:  []string{"morgen"},
		Ago:       []string{"vor"},
		InFuture:  []string{"in"},
		Last:      []string{"letzter", "letzte", "letztem", "letzten", "vergangener", "vergangene"},
		Next:      []string{"nächster", "nächste", "nächstem", "nächsten"},
		This:      []string{"dieser", "diese", "diesem", "diesen"},
		Seconds:   []string{"Sekunde", "Sekunden", "Sek."},
		Minutes:   []string{"Minute", "Minuten", "Min."},
		Hours:     []string{"Stunde", "Stunden", "Std."},
		Days:      []string{"Tag", "Tage", "Tagen"},
		Weeks:     []string{"Woche", "Wochen"},
		Months:    []string{"Monat", "Monate", "Monaten"},
		Years:     []string{"Jahr", "Jahre", "Jahren"},
	},
}
