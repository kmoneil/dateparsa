// Code generated from CLDR data. DO NOT EDIT.
package data

import "github.com/kmoneil/dateparsa/internal/locale"

func init() { locale.Register(&FR) }

var FR = locale.Data{
	Tag:  "fr",
	Name: "French",
	MonthsWide: [12]string{
		"janvier", "février", "mars", "avril", "mai", "juin",
		"juillet", "août", "septembre", "octobre", "novembre", "décembre",
	},
	MonthsAbbr: [12]string{
		"janv.", "févr.", "mars", "avr.", "mai", "juin",
		"juil.", "août", "sept.", "oct.", "nov.", "déc.",
	},
	WeekdaysWide: [7]string{"dimanche", "lundi", "mardi", "mercredi", "jeudi", "vendredi", "samedi"},
	WeekdaysAbbr: [7]string{"dim.", "lun.", "mar.", "mer.", "jeu.", "ven.", "sam."},
	AM:           "AM",
	PM:           "PM",
	Relative: locale.RelativeKeywords{
		Now:       []string{"maintenant"},
		Today:     []string{"aujourd'hui"},
		Yesterday: []string{"hier"},
		Tomorrow:  []string{"demain"},
		Ago:       []string{"il y a"},
		InFuture:  []string{"dans"},
		Last:      []string{"dernier", "dernière", "précédent", "précédente", "passé", "passée"},
		Next:      []string{"prochain", "prochaine"},
		This:      []string{"ce", "cet", "cette"},
		Seconds:   []string{"seconde", "secondes", "sec"},
		Minutes:   []string{"minute", "minutes", "min", "mins"},
		Hours:     []string{"heure", "heures"},
		Days:      []string{"jour", "jours"},
		Weeks:     []string{"semaine", "semaines"},
		Months:    []string{"mois"},
		Years:     []string{"an", "ans", "année", "années"},
	},
}
