// Code generated from CLDR data. DO NOT EDIT.
package data

import "github.com/kmoneil/dateparsa/internal/locale"

func init() { locale.Register(&JA) }

var JA = locale.Data{
	Tag:  "ja",
	Name: "Japanese",
	MonthsWide: [12]string{
		"1月", "2月", "3月", "4月", "5月", "6月",
		"7月", "8月", "9月", "10月", "11月", "12月",
	},
	MonthsAbbr: [12]string{
		"1月", "2月", "3月", "4月", "5月", "6月",
		"7月", "8月", "9月", "10月", "11月", "12月",
	},
	WeekdaysWide: [7]string{"日曜日", "月曜日", "火曜日", "水曜日", "木曜日", "金曜日", "土曜日"},
	WeekdaysAbbr: [7]string{"日", "月", "火", "水", "木", "金", "土"},
	AM:           "午前",
	PM:           "午後",
	Relative: locale.RelativeKeywords{
		Now:       []string{"今"},
		Today:     []string{"今日"},
		Yesterday: []string{"昨日"},
		Tomorrow:  []string{"明日"},
		Ago:       []string{"前"},
		InFuture:  []string{"後"},
		Last:      []string{"先", "前の"},
		Next:      []string{"来", "次の"},
		This:      []string{"今", "この"},
		Seconds:   []string{"秒"},
		Minutes:   []string{"分"},
		Hours:     []string{"時間"},
		Days:      []string{"日"},
		Weeks:     []string{"週間", "週"},
		Months:    []string{"ヶ月", "か月"},
		Years:     []string{"年"},
	},
}
