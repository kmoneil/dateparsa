// Code generated from CLDR data. DO NOT EDIT.
package data

import "github.com/kmoneil/dateparsa/internal/locale"

func init() { locale.Register(&ZH) }

var ZH = locale.Data{
	Tag:  "zh",
	Name: "Chinese",
	MonthsWide: [12]string{
		"一月", "二月", "三月", "四月", "五月", "六月",
		"七月", "八月", "九月", "十月", "十一月", "十二月",
	},
	MonthsAbbr: [12]string{
		"1月", "2月", "3月", "4月", "5月", "6月",
		"7月", "8月", "9月", "10月", "11月", "12月",
	},
	WeekdaysWide: [7]string{"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"},
	WeekdaysAbbr: [7]string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"},
	AM:           "上午",
	PM:           "下午",
	Relative: locale.RelativeKeywords{
		Now:       []string{"现在"},
		Today:     []string{"今天"},
		Yesterday: []string{"昨天"},
		Tomorrow:  []string{"明天"},
		Ago:       []string{"前"},
		InFuture:  []string{"后"},
		Last:      []string{"上", "上个"},
		Next:      []string{"下", "下个"},
		This:      []string{"这个", "本"},
		Seconds:   []string{"秒", "秒钟"},
		Minutes:   []string{"分钟", "分"},
		Hours:     []string{"小时"},
		Days:      []string{"天", "日"},
		Weeks:     []string{"周", "星期"},
		Months:    []string{"月", "个月"},
		Years:     []string{"年"},
	},
}
