// Code generated from CLDR data. DO NOT EDIT.
package data

import "github.com/kmoneil/dateparsa/internal/locale"

func init() { locale.Register(&FI) }

var FI = locale.Data{
	Tag:  "fi",
	Name: "Finnish",
	MonthsWide: [12]string{
		"tammikuu", "helmikuu", "maaliskuu", "huhtikuu", "toukokuu", "kesäkuu",
		"heinäkuu", "elokuu", "syyskuu", "lokakuu", "marraskuu", "joulukuu",
	},
	MonthsAbbr: [12]string{
		"tammi", "helmi", "maalis", "huhti", "touko", "kesä",
		"heinä", "elo", "syys", "loka", "marras", "joulu",
	},
	WeekdaysWide: [7]string{"sunnuntai", "maanantai", "tiistai", "keskiviikko", "torstai", "perjantai", "lauantai"},
	WeekdaysAbbr: [7]string{"su", "ma", "ti", "ke", "to", "pe", "la"},
	AM:           "ap.",
	PM:           "ip.",
	Relative: locale.RelativeKeywords{
		Now:       []string{"nyt"},
		Today:     []string{"tänään"},
		Yesterday: []string{"eilen"},
		Tomorrow:  []string{"huomenna"},
		Ago:       []string{"sitten"},
		InFuture:  []string{"kuluttua"},
		Last:      []string{"viime", "edellinen"},
		Next:      []string{"ensi", "seuraava"},
		This:      []string{"tämä", "tänä"},
		Seconds:   []string{"sekunti", "sekuntia", "sekunteja", "sek"},
		Minutes:   []string{"minuutti", "minuuttia", "minuutteja", "min"},
		Hours:     []string{"tunti", "tuntia", "tunteja"},
		Days:      []string{"päivä", "päivää", "päiviä"},
		Weeks:     []string{"viikko", "viikkoa", "viikkoja"},
		Months:    []string{"kuukausi", "kuukautta", "kuukausia"},
		Years:     []string{"vuosi", "vuotta", "vuosia"},
	},
}
