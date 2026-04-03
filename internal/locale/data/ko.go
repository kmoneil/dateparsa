// Code generated from CLDR data. DO NOT EDIT.
package data

import "github.com/kmoneil/dateparsa/internal/locale"

func init() { locale.Register(&KO) }

var KO = locale.Data{
	Tag:  "ko",
	Name: "Korean",
	MonthsWide: [12]string{
		"1월", "2월", "3월", "4월", "5월", "6월",
		"7월", "8월", "9월", "10월", "11월", "12월",
	},
	MonthsAbbr: [12]string{
		"1월", "2월", "3월", "4월", "5월", "6월",
		"7월", "8월", "9월", "10월", "11월", "12월",
	},
	WeekdaysWide: [7]string{"일요일", "월요일", "화요일", "수요일", "목요일", "금요일", "토요일"},
	WeekdaysAbbr: [7]string{"일", "월", "화", "수", "목", "금", "토"},
	AM:           "오전",
	PM:           "오후",
	Relative: locale.RelativeKeywords{
		Now:       []string{"지금"},
		Today:     []string{"오늘"},
		Yesterday: []string{"어제"},
		Tomorrow:  []string{"내일"},
		Ago:       []string{"전"},
		InFuture:  []string{"후"},
		Last:      []string{"지난"},
		Next:      []string{"다음"},
		This:      []string{"이번"},
		Seconds:   []string{"초"},
		Minutes:   []string{"분"},
		Hours:     []string{"시간", "시"},
		Days:      []string{"일"},
		Weeks:     []string{"주"},
		Months:    []string{"개월", "달"},
		Years:     []string{"년", "해"},
	},
}
