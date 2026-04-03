// Code generated from CLDR data. DO NOT EDIT.
package data

import "github.com/kmoneil/dateparsa/internal/locale"

func init() { locale.Register(&PT) }

var PT = locale.Data{
	Tag:  "pt",
	Name: "Portuguese",
	MonthsWide: [12]string{
		"janeiro", "fevereiro", "março", "abril", "maio", "junho",
		"julho", "agosto", "setembro", "outubro", "novembro", "dezembro",
	},
	MonthsAbbr: [12]string{
		"jan.", "fev.", "mar.", "abr.", "mai.", "jun.",
		"jul.", "ago.", "set.", "out.", "nov.", "dez.",
	},
	WeekdaysWide: [7]string{"domingo", "segunda-feira", "terça-feira", "quarta-feira", "quinta-feira", "sexta-feira", "sábado"},
	WeekdaysAbbr: [7]string{"dom.", "seg.", "ter.", "qua.", "qui.", "sex.", "sáb."},
	AM:           "AM",
	PM:           "PM",
	Relative: locale.RelativeKeywords{
		Now:       []string{"agora"},
		Today:     []string{"hoje"},
		Yesterday: []string{"ontem"},
		Tomorrow:  []string{"amanhã"},
		Ago:       []string{"há", "atrás"},
		InFuture:  []string{"em", "dentro de"},
		Last:      []string{"passado", "passada", "último", "última"},
		Next:      []string{"próximo", "próxima"},
		This:      []string{"este", "esta"},
		Seconds:   []string{"segundo", "segundos", "seg"},
		Minutes:   []string{"minuto", "minutos", "min", "mins"},
		Hours:     []string{"hora", "horas"},
		Days:      []string{"dia", "dias"},
		Weeks:     []string{"semana", "semanas"},
		Months:    []string{"mês", "meses"},
		Years:     []string{"ano", "anos"},
	},
}
