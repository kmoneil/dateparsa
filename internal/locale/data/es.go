// Code generated from CLDR data. DO NOT EDIT.
package data

import "github.com/kmoneil/dateparsa/internal/locale"

func init() { locale.Register(&ES) }

var ES = locale.Data{
	Tag:  "es",
	Name: "Spanish",
	MonthsWide: [12]string{
		"enero", "febrero", "marzo", "abril", "mayo", "junio",
		"julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre",
	},
	MonthsAbbr: [12]string{
		"ene.", "feb.", "mar.", "abr.", "may.", "jun.",
		"jul.", "ago.", "sept.", "oct.", "nov.", "dic.",
	},
	WeekdaysWide: [7]string{"domingo", "lunes", "martes", "miércoles", "jueves", "viernes", "sábado"},
	WeekdaysAbbr: [7]string{"dom.", "lun.", "mar.", "mié.", "jue.", "vie.", "sáb."},
	AM:           "a.\u00a0m.",
	PM:           "p.\u00a0m.",
	Relative: locale.RelativeKeywords{
		Now:       []string{"ahora"},
		Today:     []string{"hoy"},
		Yesterday: []string{"ayer"},
		Tomorrow:  []string{"mañana"},
		Ago:       []string{"hace"},
		InFuture:  []string{"dentro de", "en"},
		Last:      []string{"pasado", "pasada", "último", "última"},
		Next:      []string{"próximo", "próxima"},
		This:      []string{"este", "esta"},
		Seconds:   []string{"segundo", "segundos", "seg"},
		Minutes:   []string{"minuto", "minutos", "min", "mins"},
		Hours:     []string{"hora", "horas"},
		Days:      []string{"día", "días"},
		Weeks:     []string{"semana", "semanas"},
		Months:    []string{"mes", "meses"},
		Years:     []string{"año", "años"},
	},
}
