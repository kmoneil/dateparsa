// Code generated from CLDR data. DO NOT EDIT.
package data

import "github.com/kmoneil/dateparsa/internal/locale"

func init() { locale.Register(&TR) }

var TR = locale.Data{
	Tag:  "tr",
	Name: "Turkish",
	MonthsWide: [12]string{
		"Ocak", "Şubat", "Mart", "Nisan", "Mayıs", "Haziran",
		"Temmuz", "Ağustos", "Eylül", "Ekim", "Kasım", "Aralık",
	},
	MonthsAbbr: [12]string{
		"Oca", "Şub", "Mar", "Nis", "May", "Haz",
		"Tem", "Ağu", "Eyl", "Eki", "Kas", "Ara",
	},
	WeekdaysWide: [7]string{"Pazar", "Pazartesi", "Salı", "Çarşamba", "Perşembe", "Cuma", "Cumartesi"},
	WeekdaysAbbr: [7]string{"Paz", "Pzt", "Sal", "Çar", "Per", "Cum", "Cmt"},
	AM:           "ÖÖ",
	PM:           "ÖS",
	Relative: locale.RelativeKeywords{
		Now:       []string{"şimdi"},
		Today:     []string{"bugün"},
		Yesterday: []string{"dün"},
		Tomorrow:  []string{"yarın"},
		Ago:       []string{"önce"},
		InFuture:  []string{"sonra"},
		Last:      []string{"geçen", "önceki"},
		Next:      []string{"gelecek", "önümüzdeki"},
		This:      []string{"bu"},
		Seconds:   []string{"saniye", "sn"},
		Minutes:   []string{"dakika", "dk"},
		Hours:     []string{"saat"},
		Days:      []string{"gün"},
		Weeks:     []string{"hafta"},
		Months:    []string{"ay"},
		Years:     []string{"yıl"},
	},
}
