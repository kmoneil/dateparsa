// Code generated from CLDR data. DO NOT EDIT.
package data

import "github.com/kmoneil/dateparsa/internal/locale"

func init() { locale.Register(&RU) }

var RU = locale.Data{
	Tag:  "ru",
	Name: "Russian",
	MonthsWide: [12]string{
		"января", "февраля", "марта", "апреля", "мая", "июня",
		"июля", "августа", "сентября", "октября", "ноября", "декабря",
	},
	MonthsAbbr: [12]string{
		"янв.", "февр.", "мар.", "апр.", "мая", "июн.",
		"июл.", "авг.", "сент.", "окт.", "нояб.", "дек.",
	},
	WeekdaysWide: [7]string{"воскресенье", "понедельник", "вторник", "среда", "четверг", "пятница", "суббота"},
	WeekdaysAbbr: [7]string{"вс", "пн", "вт", "ср", "чт", "пт", "сб"},
	AM:           "AM",
	PM:           "PM",
	Relative: locale.RelativeKeywords{
		Now:       []string{"сейчас"},
		Today:     []string{"сегодня"},
		Yesterday: []string{"вчера"},
		Tomorrow:  []string{"завтра"},
		Ago:       []string{"назад"},
		InFuture:  []string{"через"},
		Last:      []string{"прошлый", "прошлая", "прошлое", "прошлую", "прошлом"},
		Next:      []string{"следующий", "следующая", "следующее", "следующую", "следующем"},
		This:      []string{"этот", "эта", "это", "этом", "эту"},
		Seconds:   []string{"секунда", "секунды", "секунд", "сек"},
		Minutes:   []string{"минута", "минуты", "минут", "мин"},
		Hours:     []string{"час", "часа", "часов", "ч"},
		Days:      []string{"день", "дня", "дней"},
		Weeks:     []string{"неделя", "недели", "недель", "неделю"},
		Months:    []string{"месяц", "месяца", "месяцев"},
		Years:     []string{"год", "года", "лет"},
	},
}
