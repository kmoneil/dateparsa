// Code generated from CLDR data. DO NOT EDIT.
package data

import "github.com/kmoneil/dateparsa/internal/locale"

func init() { locale.Register(&UK) }

var UK = locale.Data{
	Tag:  "uk",
	Name: "Ukrainian",
	MonthsWide: [12]string{
		"січень", "лютий", "березень", "квітень", "травень", "червень",
		"липень", "серпень", "вересень", "жовтень", "листопад", "грудень",
	},
	MonthsAbbr: [12]string{
		"січ.", "лют.", "бер.", "кві.", "тра.", "чер.",
		"лип.", "сер.", "вер.", "жов.", "лис.", "гру.",
	},
	WeekdaysWide: [7]string{"неділя", "понеділок", "вівторок", "середа", "четвер", "п'ятниця", "субота"},
	WeekdaysAbbr: [7]string{"нд", "пн", "вт", "ср", "чт", "пт", "сб"},
	AM:           "дп",
	PM:           "пп",
	Relative: locale.RelativeKeywords{
		Now:       []string{"зараз"},
		Today:     []string{"сьогодні"},
		Yesterday: []string{"вчора"},
		Tomorrow:  []string{"завтра"},
		Ago:       []string{"тому"},
		InFuture:  []string{"через"},
		Last:      []string{"минулий", "минулого", "минулу"},
		Next:      []string{"наступний", "наступного", "наступну"},
		This:      []string{"цей", "цього", "цю"},
		Seconds:   []string{"секунда", "секунди", "секунд", "сек"},
		Minutes:   []string{"хвилина", "хвилини", "хвилин", "хв"},
		Hours:     []string{"година", "години", "годин", "год"},
		Days:      []string{"день", "дні", "днів"},
		Weeks:     []string{"тиждень", "тижні", "тижнів"},
		Months:    []string{"місяць", "місяці", "місяців"},
		Years:     []string{"рік", "роки", "років"},
	},
}
