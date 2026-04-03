// Code generated from CLDR data. DO NOT EDIT.
package data

import "github.com/kmoneil/dateparsa/internal/locale"

func init() { locale.Register(&AR) }

var AR = locale.Data{
	Tag:  "ar",
	Name: "Arabic",
	MonthsWide: [12]string{
		"يناير", "فبراير", "مارس", "أبريل", "مايو", "يونيو",
		"يوليو", "أغسطس", "سبتمبر", "أكتوبر", "نوفمبر", "ديسمبر",
	},
	MonthsAbbr: [12]string{
		"يناير", "فبراير", "مارس", "أبريل", "مايو", "يونيو",
		"يوليو", "أغسطس", "سبتمبر", "أكتوبر", "نوفمبر", "ديسمبر",
	},
	WeekdaysWide: [7]string{"الأحد", "الاثنين", "الثلاثاء", "الأربعاء", "الخميس", "الجمعة", "السبت"},
	WeekdaysAbbr: [7]string{"أحد", "إثنين", "ثلاثاء", "أربعاء", "خميس", "جمعة", "سبت"},
	AM:           "ص",
	PM:           "م",
	Relative: locale.RelativeKeywords{
		Now:       []string{"الآن"},
		Today:     []string{"اليوم"},
		Yesterday: []string{"أمس"},
		Tomorrow:  []string{"غداً", "غدا"},
		Ago:       []string{"منذ", "قبل"},
		InFuture:  []string{"بعد", "خلال"},
		Last:      []string{"الماضي", "السابق"},
		Next:      []string{"القادم", "التالي"},
		This:      []string{"هذا", "هذه"},
		Seconds:   []string{"ثانية", "ثوان", "ثواني"},
		Minutes:   []string{"دقيقة", "دقائق"},
		Hours:     []string{"ساعة", "ساعات"},
		Days:      []string{"يوم", "أيام"},
		Weeks:     []string{"أسبوع", "أسابيع"},
		Months:    []string{"شهر", "أشهر", "شهور"},
		Years:     []string{"سنة", "سنوات", "عام"},
	},
}
