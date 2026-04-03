// Code generated from CLDR data. DO NOT EDIT.
package data

import "github.com/kmoneil/dateparsa/internal/locale"

func init() { locale.Register(&IT) }

var IT = locale.Data{
	Tag:  "it",
	Name: "Italian",
	MonthsWide: [12]string{
		"gennaio", "febbraio", "marzo", "aprile", "maggio", "giugno",
		"luglio", "agosto", "settembre", "ottobre", "novembre", "dicembre",
	},
	MonthsAbbr: [12]string{
		"gen", "feb", "mar", "apr", "mag", "giu",
		"lug", "ago", "set", "ott", "nov", "dic",
	},
	WeekdaysWide: [7]string{"domenica", "lunedì", "martedì", "mercoledì", "giovedì", "venerdì", "sabato"},
	WeekdaysAbbr: [7]string{"dom", "lun", "mar", "mer", "gio", "ven", "sab"},
	AM:           "AM",
	PM:           "PM",
	Relative: locale.RelativeKeywords{
		Now:       []string{"adesso", "ora"},
		Today:     []string{"oggi"},
		Yesterday: []string{"ieri"},
		Tomorrow:  []string{"domani"},
		Ago:       []string{"fa"},
		InFuture:  []string{"tra", "fra"},
		Last:      []string{"scorso", "scorsa", "passato", "passata", "ultimo", "ultima"},
		Next:      []string{"prossimo", "prossima"},
		This:      []string{"questo", "questa"},
		Seconds:   []string{"secondo", "secondi", "sec"},
		Minutes:   []string{"minuto", "minuti", "min"},
		Hours:     []string{"ora", "ore"},
		Days:      []string{"giorno", "giorni"},
		Weeks:     []string{"settimana", "settimane"},
		Months:    []string{"mese", "mesi"},
		Years:     []string{"anno", "anni"},
	},
}
