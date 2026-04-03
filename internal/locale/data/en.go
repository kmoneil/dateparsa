// Code generated from CLDR data. DO NOT EDIT.
package data

import "github.com/kmoneil/dateparsa/internal/locale"

func init() { locale.Register(&EN) }

var EN = locale.Data{
	Tag:  "en",
	Name: "English",
	MonthsWide: [12]string{
		"January", "February", "March", "April", "May", "June",
		"July", "August", "September", "October", "November", "December",
	},
	MonthsAbbr: [12]string{
		"Jan", "Feb", "Mar", "Apr", "May", "Jun",
		"Jul", "Aug", "Sep", "Oct", "Nov", "Dec",
	},
	WeekdaysWide: [7]string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"},
	WeekdaysAbbr: [7]string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"},
	AM:           "AM",
	PM:           "PM",
	Relative: locale.RelativeKeywords{
		Now:       []string{"now"},
		Today:     []string{"today"},
		Yesterday: []string{"yesterday"},
		Tomorrow:  []string{"tomorrow"},
		Ago:       []string{"ago"},
		InFuture:  []string{"in"},
		Last:      []string{"last", "previous", "past"},
		Next:      []string{"next", "coming"},
		This:      []string{"this"},
		Seconds:   []string{"second", "seconds", "sec", "secs"},
		Minutes:   []string{"minute", "minutes", "min", "mins"},
		Hours:     []string{"hour", "hours", "hr", "hrs"},
		Days:      []string{"day", "days"},
		Weeks:     []string{"week", "weeks"},
		Months:    []string{"month", "months"},
		Years:     []string{"year", "years"},
	},
}
