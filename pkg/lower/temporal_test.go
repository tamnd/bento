package lower

import (
	"strings"
	"testing"
)

// TestPlainDateConstruction pins the construction: a PlainDate is built by
// value.NewPlainDate over its three number components, and a field read lowers to a
// getter method.
func TestPlainDateConstruction(t *testing.T) {
	const src = `const d = new Temporal.PlainDate(2020, 2, 29);
console.log(d.year);`
	got := renderProgram(t, src)
	for _, want := range []string{
		"value.NewPlainDate(2020, 2, 29)",
		".Year()",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestPlainDateTypeSlot pins the type slot: a parameter typed Temporal.PlainDate
// lowers to a pointer to value.PlainDate rather than an interned struct shape.
func TestPlainDateTypeSlot(t *testing.T) {
	const src = `function dayOf(d: Temporal.PlainDate): number { return d.day; }
console.log(dayOf(new Temporal.PlainDate(2020, 2, 29)));`
	got := renderProgram(t, src)
	if !strings.Contains(got, "*value.PlainDate") {
		t.Errorf("rendered program missing %q:\n%s", "*value.PlainDate", got)
	}
}

// TestPlainDateGetters pins the clean field getters: each lowers to the matching
// method on the value.PlainDate receiver.
func TestPlainDateGetters(t *testing.T) {
	cases := map[string]string{
		"year":         ".Year()",
		"month":        ".Month()",
		"day":          ".Day()",
		"calendarId":   ".CalendarId()",
		"monthCode":    ".MonthCode()",
		"dayOfWeek":    ".DayOfWeek()",
		"dayOfYear":    ".DayOfYear()",
		"daysInWeek":   ".DaysInWeek()",
		"daysInMonth":  ".DaysInMonth()",
		"daysInYear":   ".DaysInYear()",
		"monthsInYear": ".MonthsInYear()",
		"inLeapYear":   ".InLeapYear()",
	}
	for prop, want := range cases {
		src := "const d = new Temporal.PlainDate(2020, 2, 29);\nconsole.log(d." + prop + ");"
		got := renderProgram(t, src)
		if !strings.Contains(got, want) {
			t.Errorf("getter .%s missing %q:\n%s", prop, want, got)
		}
	}
}

// TestPlainDateMethods pins equals, toString, and toJSON to their value.PlainDate
// methods.
func TestPlainDateMethods(t *testing.T) {
	const src = `const a = new Temporal.PlainDate(2020, 1, 1);
const b = new Temporal.PlainDate(2020, 3, 15);
console.log(a.equals(b));
console.log(a.toString());
console.log(a.toJSON());`
	got := renderProgram(t, src)
	for _, want := range []string{".Equals(", ".ToString()", ".ToJSON()"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestPlainDateStatics pins Temporal.PlainDate.compare and .from over the two-level
// Temporal namespace access to their value package functions.
func TestPlainDateStatics(t *testing.T) {
	const src = `const a = new Temporal.PlainDate(2020, 1, 1);
const b = new Temporal.PlainDate(2020, 3, 15);
console.log(Temporal.PlainDate.compare(a, b));
const c = Temporal.PlainDate.from(a);
console.log(c.day);`
	got := renderProgram(t, src)
	for _, want := range []string{"value.PlainDateCompare(", "value.PlainDateFrom("} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestPlainDateHandBacks pins the honest ceilings: the union getters, the arithmetic
// and conversion methods, from over a string, and the other Temporal types each hand
// back with a reason naming where the work belongs.
func TestPlainDateHandBacks(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "add arithmetic",
			src:  "const d = new Temporal.PlainDate(2020, 2, 29);\nconst e = d.add({ days: 1 });\nconsole.log(e.day);",
			want: "Temporal.PlainDate.prototype.add is a later slice",
		},
		{
			name: "from a string",
			src:  "const d = Temporal.PlainDate.from(\"2020-02-29\");\nconsole.log(d.day);",
			want: "Temporal.PlainDate.from over a string or a property bag is a later slice",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderProgramHandBack(t, c.src)
			if !strings.Contains(got, c.want) {
				t.Errorf("hand-back reason = %q, want it to contain %q", got, c.want)
			}
		})
	}
}

// TestDurationConstruction pins the construction: a Duration is built by value.NewDuration
// over its ten number components, the missing trailing ones padded with zero, and a field
// read lowers to a getter method.
func TestDurationConstruction(t *testing.T) {
	const src = `const d = new Temporal.Duration(1, 2, 3);
console.log(d.days);`
	got := renderProgram(t, src)
	for _, want := range []string{
		"value.NewDuration(1, 2, 3, 0, 0, 0, 0, 0, 0, 0)",
		".Days()",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestDurationTypeSlot pins the type slot: a parameter typed Temporal.Duration lowers to a
// pointer to value.Duration rather than an interned struct shape.
func TestDurationTypeSlot(t *testing.T) {
	const src = `function daysOf(d: Temporal.Duration): number { return d.days; }
console.log(daysOf(new Temporal.Duration(1, 2, 3)));`
	got := renderProgram(t, src)
	if !strings.Contains(got, "*value.Duration") {
		t.Errorf("rendered program missing %q:\n%s", "*value.Duration", got)
	}
}

// TestDurationGetters pins the ten field getters plus sign and blank: each lowers to the
// matching method on the value.Duration receiver.
func TestDurationGetters(t *testing.T) {
	cases := map[string]string{
		"years":        ".Years()",
		"months":       ".Months()",
		"weeks":        ".Weeks()",
		"days":         ".Days()",
		"hours":        ".Hours()",
		"minutes":      ".Minutes()",
		"seconds":      ".Seconds()",
		"milliseconds": ".Milliseconds()",
		"microseconds": ".Microseconds()",
		"nanoseconds":  ".Nanoseconds()",
		"sign":         ".Sign()",
		"blank":        ".Blank()",
	}
	for prop, want := range cases {
		src := "const d = new Temporal.Duration(1, 2, 3, 4, 5, 6, 7, 8, 9, 10);\nconsole.log(d." + prop + ");"
		got := renderProgram(t, src)
		if !strings.Contains(got, want) {
			t.Errorf("getter .%s missing %q:\n%s", prop, want, got)
		}
	}
}

// TestDurationMethods pins negated, abs, toString, and toJSON to their value.Duration
// methods.
func TestDurationMethods(t *testing.T) {
	const src = `const d = new Temporal.Duration(1, 2, 3);
console.log(d.negated().days);
console.log(d.abs().days);
console.log(d.toString());
console.log(d.toJSON());`
	got := renderProgram(t, src)
	for _, want := range []string{".Negated()", ".Abs()", ".ToString()", ".ToJSON()"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestDurationStatics pins Temporal.Duration.from over a Duration to value.DurationFrom.
func TestDurationStatics(t *testing.T) {
	const src = `const a = new Temporal.Duration(1, 2, 3);
const b = Temporal.Duration.from(a);
console.log(b.days);`
	got := renderProgram(t, src)
	if !strings.Contains(got, "value.DurationFrom(") {
		t.Errorf("rendered program missing %q:\n%s", "value.DurationFrom(", got)
	}
}

// TestDurationHandBacks pins the honest ceilings for Duration: the balancing and rounding
// methods, from over a string, and compare each hand back with a reason naming where the
// work belongs, waiting on the relativeTo reference and the calendar model.
func TestDurationHandBacks(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "add arithmetic",
			src:  "const d = new Temporal.Duration(0, 0, 0, 1);\nconst e = d.add(new Temporal.Duration(0, 0, 0, 1));\nconsole.log(e.days);",
			want: "Temporal.Duration.prototype.add is a later slice",
		},
		{
			name: "round",
			src:  "const d = new Temporal.Duration(0, 0, 0, 1);\nconst r = d.round({ largestUnit: \"hours\" });\nconsole.log(r.hours);",
			want: "Temporal.Duration.prototype.round is a later slice",
		},
		{
			name: "total",
			src:  "const d = new Temporal.Duration(0, 0, 0, 1);\nconsole.log(d.total({ unit: \"hours\" }));",
			want: "Temporal.Duration.prototype.total is a later slice",
		},
		{
			name: "from a string",
			src:  "const d = Temporal.Duration.from(\"P1Y\");\nconsole.log(d.years);",
			want: "Temporal.Duration.from over a string or a property bag is a later slice",
		},
		{
			name: "compare",
			src:  "const a = new Temporal.Duration(0, 0, 0, 1);\nconst b = new Temporal.Duration(0, 0, 0, 2);\nconsole.log(Temporal.Duration.compare(a, b));",
			want: "Temporal.Duration.compare is a later slice",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderProgramHandBack(t, c.src)
			if !strings.Contains(got, c.want) {
				t.Errorf("hand-back reason = %q, want it to contain %q", got, c.want)
			}
		})
	}
}

// TestPlainTimeConstruction pins the construction: a PlainTime is built by
// value.NewPlainTime over its six number components, the missing trailing ones padded
// with zero, and a field read lowers to a getter method.
func TestPlainTimeConstruction(t *testing.T) {
	const src = `const t = new Temporal.PlainTime(12, 30);
console.log(t.hour);`
	got := renderProgram(t, src)
	for _, want := range []string{
		"value.NewPlainTime(12, 30, 0, 0, 0, 0)",
		".Hour()",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestPlainTimeTypeSlot pins the type slot: a parameter typed Temporal.PlainTime lowers
// to a pointer to value.PlainTime rather than an interned struct shape.
func TestPlainTimeTypeSlot(t *testing.T) {
	const src = `function hourOf(t: Temporal.PlainTime): number { return t.hour; }
console.log(hourOf(new Temporal.PlainTime(12, 30)));`
	got := renderProgram(t, src)
	if !strings.Contains(got, "*value.PlainTime") {
		t.Errorf("rendered program missing %q:\n%s", "*value.PlainTime", got)
	}
}

// TestPlainTimeGetters pins the six clean field getters: each lowers to the matching
// method on the value.PlainTime receiver.
func TestPlainTimeGetters(t *testing.T) {
	cases := map[string]string{
		"hour":        ".Hour()",
		"minute":      ".Minute()",
		"second":      ".Second()",
		"millisecond": ".Millisecond()",
		"microsecond": ".Microsecond()",
		"nanosecond":  ".Nanosecond()",
	}
	for prop, want := range cases {
		src := "const t = new Temporal.PlainTime(1, 2, 3, 4, 5, 6);\nconsole.log(t." + prop + ");"
		got := renderProgram(t, src)
		if !strings.Contains(got, want) {
			t.Errorf("getter .%s missing %q:\n%s", prop, want, got)
		}
	}
}

// TestPlainTimeMethods pins equals, toString, and toJSON to their value.PlainTime
// methods.
func TestPlainTimeMethods(t *testing.T) {
	const src = `const a = new Temporal.PlainTime(1, 0, 0);
const b = new Temporal.PlainTime(2, 0, 0);
console.log(a.equals(b));
console.log(a.toString());
console.log(a.toJSON());`
	got := renderProgram(t, src)
	for _, want := range []string{".Equals(", ".ToString()", ".ToJSON()"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestPlainTimeStatics pins Temporal.PlainTime.compare and .from over the two-level
// Temporal namespace access to their value package functions.
func TestPlainTimeStatics(t *testing.T) {
	const src = `const a = new Temporal.PlainTime(1, 0, 0);
const b = new Temporal.PlainTime(2, 0, 0);
console.log(Temporal.PlainTime.compare(a, b));
const c = Temporal.PlainTime.from(a);
console.log(c.hour);`
	got := renderProgram(t, src)
	for _, want := range []string{"value.PlainTimeCompare(", "value.PlainTimeFrom("} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestPlainTimeHandBacks pins the honest ceilings for PlainTime: the arithmetic, from
// over a string, and a conversion method each hand back with a reason naming where the
// work belongs.
func TestPlainTimeHandBacks(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "add arithmetic",
			src:  "const t = new Temporal.PlainTime(12, 30);\nconst e = t.add({ hours: 1 });\nconsole.log(e.hour);",
			want: "Temporal.PlainTime.prototype.add is a later slice",
		},
		{
			name: "from a string",
			src:  "const t = Temporal.PlainTime.from(\"12:30:00\");\nconsole.log(t.hour);",
			want: "Temporal.PlainTime.from over a string or a property bag is a later slice",
		},
		{
			name: "round",
			src:  "const t = new Temporal.PlainTime(12, 30, 45);\nconst r = t.round({ smallestUnit: \"minute\" });\nconsole.log(r.minute);",
			want: "Temporal.PlainTime.prototype.round is a later slice",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderProgramHandBack(t, c.src)
			if !strings.Contains(got, c.want) {
				t.Errorf("hand-back reason = %q, want it to contain %q", got, c.want)
			}
		})
	}
}

// TestPlainDateTimeConstruction pins the construction: a PlainDateTime is built by
// value.NewPlainDateTime over its nine number components, the missing trailing time ones
// padded with zero, and a field read lowers to a getter method.
func TestPlainDateTimeConstruction(t *testing.T) {
	const src = `const dt = new Temporal.PlainDateTime(2020, 1, 1, 12, 30);
console.log(dt.hour);`
	got := renderProgram(t, src)
	for _, want := range []string{
		"value.NewPlainDateTime(2020, 1, 1, 12, 30, 0, 0, 0, 0)",
		".Hour()",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestPlainDateTimeTypeSlot pins the type slot: a parameter typed Temporal.PlainDateTime
// lowers to a pointer to value.PlainDateTime rather than an interned struct shape.
func TestPlainDateTimeTypeSlot(t *testing.T) {
	const src = `function hourOf(dt: Temporal.PlainDateTime): number { return dt.hour; }
console.log(hourOf(new Temporal.PlainDateTime(2020, 1, 1, 12, 30)));`
	got := renderProgram(t, src)
	if !strings.Contains(got, "*value.PlainDateTime") {
		t.Errorf("rendered program missing %q:\n%s", "*value.PlainDateTime", got)
	}
}

// TestPlainDateTimeGetters pins the union of the clean date getters and the six time
// getters: each lowers to the matching method on the value.PlainDateTime receiver.
func TestPlainDateTimeGetters(t *testing.T) {
	cases := map[string]string{
		"year":        ".Year()",
		"month":       ".Month()",
		"day":         ".Day()",
		"hour":        ".Hour()",
		"minute":      ".Minute()",
		"second":      ".Second()",
		"millisecond": ".Millisecond()",
		"microsecond": ".Microsecond()",
		"nanosecond":  ".Nanosecond()",
		"monthCode":   ".MonthCode()",
		"dayOfWeek":   ".DayOfWeek()",
		"daysInMonth": ".DaysInMonth()",
		"inLeapYear":  ".InLeapYear()",
		"calendarId":  ".CalendarId()",
	}
	for prop, want := range cases {
		src := "const dt = new Temporal.PlainDateTime(2020, 1, 1, 12, 30, 45);\nconsole.log(dt." + prop + ");"
		got := renderProgram(t, src)
		if !strings.Contains(got, want) {
			t.Errorf("getter .%s missing %q:\n%s", prop, want, got)
		}
	}
}

// TestPlainDateTimeMethods pins equals, toString, and toJSON to their value.PlainDateTime
// methods.
func TestPlainDateTimeMethods(t *testing.T) {
	const src = `const a = new Temporal.PlainDateTime(2020, 1, 1, 12, 30);
const b = new Temporal.PlainDateTime(2020, 3, 15, 8, 0);
console.log(a.equals(b));
console.log(a.toString());
console.log(a.toJSON());`
	got := renderProgram(t, src)
	for _, want := range []string{".Equals(", ".ToString()", ".ToJSON()"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestPlainDateTimeStatics pins Temporal.PlainDateTime.compare and .from over the two-level
// Temporal namespace access to their value package functions.
func TestPlainDateTimeStatics(t *testing.T) {
	const src = `const a = new Temporal.PlainDateTime(2020, 1, 1, 12, 30);
const b = new Temporal.PlainDateTime(2020, 3, 15, 8, 0);
console.log(Temporal.PlainDateTime.compare(a, b));
const c = Temporal.PlainDateTime.from(a);
console.log(c.hour);`
	got := renderProgram(t, src)
	for _, want := range []string{"value.PlainDateTimeCompare(", "value.PlainDateTimeFrom("} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestPlainDateTimeHandBacks pins the honest ceilings for PlainDateTime: the union getters,
// the arithmetic, from over a string, and a conversion method each hand back with a reason
// naming where the work belongs.
func TestPlainDateTimeHandBacks(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "add arithmetic",
			src:  "const dt = new Temporal.PlainDateTime(2020, 1, 1, 12, 30);\nconst e = dt.add({ hours: 1 });\nconsole.log(e.hour);",
			want: "Temporal.PlainDateTime.prototype.add is a later slice",
		},
		{
			name: "from a string",
			src:  "const dt = Temporal.PlainDateTime.from(\"2020-01-01T12:30:00\");\nconsole.log(dt.hour);",
			want: "Temporal.PlainDateTime.from over a string or a property bag is a later slice",
		},
		{
			name: "toPlainDate conversion",
			src:  "const dt = new Temporal.PlainDateTime(2020, 1, 1, 12, 30);\nconst d = dt.toPlainDate();\nconsole.log(d.day);",
			want: "Temporal.PlainDateTime.prototype.toPlainDate is a later slice",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderProgramHandBack(t, c.src)
			if !strings.Contains(got, c.want) {
				t.Errorf("hand-back reason = %q, want it to contain %q", got, c.want)
			}
		})
	}
}

// TestPlainYearMonthConstruction pins the construction: a PlainYearMonth is built by
// value.NewPlainYearMonth over its two number components, and a field read lowers to a getter.
func TestPlainYearMonthConstruction(t *testing.T) {
	const src = `const ym = new Temporal.PlainYearMonth(2020, 3);
console.log(ym.month);`
	got := renderProgram(t, src)
	for _, want := range []string{"value.NewPlainYearMonth(2020, 3)", ".Month()"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestPlainYearMonthTypeSlot pins the type slot: a parameter typed Temporal.PlainYearMonth
// lowers to a pointer to value.PlainYearMonth rather than an interned struct shape.
func TestPlainYearMonthTypeSlot(t *testing.T) {
	const src = `function monthOf(ym: Temporal.PlainYearMonth): number { return ym.month; }
console.log(monthOf(new Temporal.PlainYearMonth(2020, 3)));`
	got := renderProgram(t, src)
	if !strings.Contains(got, "*value.PlainYearMonth") {
		t.Errorf("rendered program missing %q:\n%s", "*value.PlainYearMonth", got)
	}
}

// TestPlainYearMonthGetters pins the clean field getters: each lowers to the matching method
// on the value.PlainYearMonth receiver.
func TestPlainYearMonthGetters(t *testing.T) {
	cases := map[string]string{
		"year":         ".Year()",
		"month":        ".Month()",
		"calendarId":   ".CalendarId()",
		"monthCode":    ".MonthCode()",
		"daysInMonth":  ".DaysInMonth()",
		"daysInYear":   ".DaysInYear()",
		"monthsInYear": ".MonthsInYear()",
		"inLeapYear":   ".InLeapYear()",
	}
	for prop, want := range cases {
		src := "const ym = new Temporal.PlainYearMonth(2020, 3);\nconsole.log(ym." + prop + ");"
		got := renderProgram(t, src)
		if !strings.Contains(got, want) {
			t.Errorf("getter .%s missing %q:\n%s", prop, want, got)
		}
	}
}

// TestPlainYearMonthMethods pins equals, toString, and toJSON to their value.PlainYearMonth
// methods.
func TestPlainYearMonthMethods(t *testing.T) {
	const src = `const a = new Temporal.PlainYearMonth(2020, 3);
const b = new Temporal.PlainYearMonth(2020, 4);
console.log(a.equals(b));
console.log(a.toString());
console.log(a.toJSON());`
	got := renderProgram(t, src)
	for _, want := range []string{".Equals(", ".ToString()", ".ToJSON()"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestPlainYearMonthStatics pins Temporal.PlainYearMonth.compare and .from to their value
// package functions.
func TestPlainYearMonthStatics(t *testing.T) {
	const src = `const a = new Temporal.PlainYearMonth(2020, 3);
const b = new Temporal.PlainYearMonth(2020, 4);
console.log(Temporal.PlainYearMonth.compare(a, b));
const c = Temporal.PlainYearMonth.from(a);
console.log(c.month);`
	got := renderProgram(t, src)
	for _, want := range []string{"value.PlainYearMonthCompare(", "value.PlainYearMonthFrom("} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestPlainYearMonthHandBacks pins the honest ceilings: the arithmetic and conversion methods,
// from over a string, and the union getters each hand back with a reason naming where the work
// belongs.
func TestPlainYearMonthHandBacks(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "era union getter",
			src:  "const ym = new Temporal.PlainYearMonth(2020, 3);\nconsole.log(ym.era);",
			want: "Temporal.PlainYearMonth.era is a later slice",
		},
		{
			name: "add arithmetic",
			src:  "const ym = new Temporal.PlainYearMonth(2020, 3);\nconst e = ym.add({ months: 1 });\nconsole.log(e.month);",
			want: "Temporal.PlainYearMonth.prototype.add is a later slice",
		},
		{
			name: "from a string",
			src:  "const ym = Temporal.PlainYearMonth.from(\"2020-03\");\nconsole.log(ym.month);",
			want: "Temporal.PlainYearMonth.from over a string or a property bag is a later slice",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderProgramHandBack(t, c.src)
			if !strings.Contains(got, c.want) {
				t.Errorf("hand-back reason = %q, want it to contain %q", got, c.want)
			}
		})
	}
}

// TestPlainMonthDayConstruction pins the construction: a PlainMonthDay is built by
// value.NewPlainMonthDay over its month-then-day components, and a field read lowers to a getter.
func TestPlainMonthDayConstruction(t *testing.T) {
	const src = `const md = new Temporal.PlainMonthDay(3, 15);
console.log(md.day);`
	got := renderProgram(t, src)
	for _, want := range []string{"value.NewPlainMonthDay(3, 15)", ".Day()"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestPlainMonthDayTypeSlot pins the type slot: a parameter typed Temporal.PlainMonthDay
// lowers to a pointer to value.PlainMonthDay rather than an interned struct shape.
func TestPlainMonthDayTypeSlot(t *testing.T) {
	const src = `function dayOf(md: Temporal.PlainMonthDay): number { return md.day; }
console.log(dayOf(new Temporal.PlainMonthDay(3, 15)));`
	got := renderProgram(t, src)
	if !strings.Contains(got, "*value.PlainMonthDay") {
		t.Errorf("rendered program missing %q:\n%s", "*value.PlainMonthDay", got)
	}
}

// TestPlainMonthDayGetters pins the clean field getters: monthCode, day, and calendarId each
// lower to the matching method on the value.PlainMonthDay receiver.
func TestPlainMonthDayGetters(t *testing.T) {
	cases := map[string]string{
		"monthCode":  ".MonthCode()",
		"day":        ".Day()",
		"calendarId": ".CalendarId()",
	}
	for prop, want := range cases {
		src := "const md = new Temporal.PlainMonthDay(3, 15);\nconsole.log(md." + prop + ");"
		got := renderProgram(t, src)
		if !strings.Contains(got, want) {
			t.Errorf("getter .%s missing %q:\n%s", prop, want, got)
		}
	}
}

// TestPlainMonthDayMethods pins equals, toString, and toJSON to their value.PlainMonthDay
// methods.
func TestPlainMonthDayMethods(t *testing.T) {
	const src = `const a = new Temporal.PlainMonthDay(3, 15);
const b = new Temporal.PlainMonthDay(3, 16);
console.log(a.equals(b));
console.log(a.toString());
console.log(a.toJSON());`
	got := renderProgram(t, src)
	for _, want := range []string{".Equals(", ".ToString()", ".ToJSON()"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestPlainMonthDayStatics pins Temporal.PlainMonthDay.from to value.PlainMonthDayFrom. A
// month-day has no compare static, so from is the only static this type carries.
func TestPlainMonthDayStatics(t *testing.T) {
	const src = `const a = new Temporal.PlainMonthDay(3, 15);
const c = Temporal.PlainMonthDay.from(a);
console.log(c.day);`
	got := renderProgram(t, src)
	if !strings.Contains(got, "value.PlainMonthDayFrom(") {
		t.Errorf("rendered program missing %q:\n%s", "value.PlainMonthDayFrom(", got)
	}
}

// TestPlainMonthDayHandBacks pins the honest ceilings: the reshaping and conversion methods and
// from over a string each hand back with a reason naming where the work belongs.
func TestPlainMonthDayHandBacks(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "with reshaping",
			src:  "const md = new Temporal.PlainMonthDay(3, 15);\nconst e = md.with({ day: 16 });\nconsole.log(e.day);",
			want: "Temporal.PlainMonthDay.prototype.with is a later slice",
		},
		{
			name: "toPlainDate conversion",
			src:  "const md = new Temporal.PlainMonthDay(3, 15);\nconst d = md.toPlainDate({ year: 2020 });\nconsole.log(d.day);",
			want: "Temporal.PlainMonthDay.prototype.toPlainDate is a later slice",
		},
		{
			name: "from a string",
			src:  "const md = Temporal.PlainMonthDay.from(\"03-15\");\nconsole.log(md.day);",
			want: "Temporal.PlainMonthDay.from over a string or a property bag is a later slice",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderProgramHandBack(t, c.src)
			if !strings.Contains(got, c.want) {
				t.Errorf("hand-back reason = %q, want it to contain %q", got, c.want)
			}
		})
	}
}

// TestPlainDateCalendarFieldsLower pins that the calendar-dependent getters lower to
// their value.PlainDate methods, the optional era read and the week-date read the
// dynamic boxing then renders.
func TestPlainDateCalendarFieldsLower(t *testing.T) {
	const src = `const d = new Temporal.PlainDate(2021, 1, 1);
console.log(d.weekOfYear);
console.log(d.yearOfWeek);
console.log(d.era);
`
	source := renderProgram(t, src)
	for _, want := range []string{".WeekOfYear()", ".YearOfWeek()", ".Era()"} {
		if !strings.Contains(source, want) {
			t.Errorf("calendar getter did not lower to %s:\n%s", want, source)
		}
	}
}

// TestPlainDateCalendarFieldsRun builds and runs the generated Go, proving the week
// date renders across a year boundary, the undefined era prints as undefined, and a
// narrowed week value reads as a plain number.
func TestPlainDateCalendarFieldsRun(t *testing.T) {
	skipIfShort(t)
	const src = `const d = new Temporal.PlainDate(2021, 1, 1);
console.log(d.weekOfYear);
console.log(d.yearOfWeek);
console.log(d.era);
console.log(d.eraYear);
const w = d.weekOfYear;
if (w !== undefined) {
  console.log(w + 100);
}
const dt = new Temporal.PlainDateTime(2020, 3, 15, 10, 30);
console.log(dt.weekOfYear);
console.log(dt.era);
`
	got := runProgramGo(t, src)
	const want = "53\n2020\nundefined\nundefined\n153\n11\nundefined\n"
	if got != want {
		t.Fatalf("calendar fields printed %q, want %q", got, want)
	}
}

// TestInstantConstruction pins new Temporal.Instant over a bigint argument to
// value.NewInstant and the epoch-milliseconds getter to its method.
func TestInstantConstruction(t *testing.T) {
	const src = `const i = new Temporal.Instant(1000000000n);
console.log(i.epochMilliseconds);`
	got := renderProgram(t, src)
	for _, want := range []string{"value.NewInstant(", ".EpochMilliseconds()"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestInstantTypeSlot pins the type slot: a parameter typed Temporal.Instant lowers to a
// pointer to value.Instant rather than an interned struct shape.
func TestInstantTypeSlot(t *testing.T) {
	const src = `function msOf(i: Temporal.Instant): number { return i.epochMilliseconds; }
console.log(msOf(new Temporal.Instant(0n)));`
	got := renderProgram(t, src)
	if !strings.Contains(got, "*value.Instant") {
		t.Errorf("rendered program missing %q:\n%s", "*value.Instant", got)
	}
}

// TestInstantGetters pins the two clean getters: epochMilliseconds and epochNanoseconds
// each lower to the matching method on the value.Instant receiver.
func TestInstantGetters(t *testing.T) {
	cases := map[string]string{
		"epochMilliseconds": ".EpochMilliseconds()",
		"epochNanoseconds":  ".EpochNanoseconds()",
	}
	for prop, want := range cases {
		src := "const i = new Temporal.Instant(1000000000n);\nconsole.log(i." + prop + ");"
		got := renderProgram(t, src)
		if !strings.Contains(got, want) {
			t.Errorf("getter .%s missing %q:\n%s", prop, want, got)
		}
	}
}

// TestInstantMethods pins equals, toString, and toJSON to their value.Instant methods.
func TestInstantMethods(t *testing.T) {
	const src = `const a = new Temporal.Instant(1n);
const b = new Temporal.Instant(2n);
console.log(a.equals(b));
console.log(a.toString());
console.log(a.toJSON());`
	got := renderProgram(t, src)
	for _, want := range []string{".Equals(", ".ToString()", ".ToJSON()"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestInstantStatics pins the four statics: compare, from over an Instant, and the two
// epoch factories, each to its value function.
func TestInstantStatics(t *testing.T) {
	const src = `const a = new Temporal.Instant(1n);
const b = new Temporal.Instant(2n);
console.log(Temporal.Instant.compare(a, b));
const c = Temporal.Instant.from(a);
console.log(c.epochMilliseconds);
const d = Temporal.Instant.fromEpochMilliseconds(1000);
console.log(d.epochMilliseconds);
const e = Temporal.Instant.fromEpochNanoseconds(5n);
console.log(e.epochMilliseconds);`
	got := renderProgram(t, src)
	for _, want := range []string{
		"value.InstantCompare(",
		"value.InstantFrom(",
		"value.InstantFromEpochMilliseconds(",
		"value.InstantFromEpochNanoseconds(",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestInstantHandBacks pins the honest ceilings: the arithmetic and rounding methods, the
// zoned conversion, and from over a string each hand back with a reason naming where the
// work belongs.
func TestInstantHandBacks(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "add arithmetic",
			src:  "const i = new Temporal.Instant(0n);\nconst j = i.add({ seconds: 1 });\nconsole.log(j.epochMilliseconds);",
			want: "Temporal.Instant.prototype.add is a later slice",
		},
		{
			name: "round",
			src:  "const i = new Temporal.Instant(1500000000n);\nconst j = i.round(\"second\");\nconsole.log(j.epochMilliseconds);",
			want: "Temporal.Instant.prototype.round is a later slice",
		},
		{
			name: "from a string",
			src:  "const i = Temporal.Instant.from(\"1970-01-01T00:00:00Z\");\nconsole.log(i.epochMilliseconds);",
			want: "Temporal.Instant.from over a string is a later slice",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderProgramHandBack(t, c.src)
			if !strings.Contains(got, c.want) {
				t.Errorf("hand-back reason = %q, want it to contain %q", got, c.want)
			}
		})
	}
}

// TestInstantRun builds and runs the generated Go, proving the epoch getters read, the UTC
// ISO string renders including a fractional second and a negative instant borrowing into
// the previous day, and compare and equals answer.
func TestInstantRun(t *testing.T) {
	skipIfShort(t)
	const src = `const a = new Temporal.Instant(1000000000n);
console.log(a.epochMilliseconds);
console.log(a.epochNanoseconds.toString());
console.log(a.toString());
const neg = new Temporal.Instant(-1n);
console.log(neg.toString());
console.log(neg.epochMilliseconds);
const frac = new Temporal.Instant(123456789n);
console.log(frac.toString());
const b = Temporal.Instant.fromEpochMilliseconds(2000);
console.log(Temporal.Instant.compare(a, b));
console.log(a.equals(a));`
	got := runProgramGo(t, src)
	const want = "1000\n1000000000\n1970-01-01T00:00:01Z\n1969-12-31T23:59:59.999999999Z\n-1\n1970-01-01T00:00:00.123456789Z\n-1\ntrue\n"
	if got != want {
		t.Fatalf("instant run printed %q, want %q", got, want)
	}
}

// TestZonedDateTimeConstruction pins the constructor: an epoch bigint and a zone string
// lower to value.NewZonedDateTime.
func TestZonedDateTimeConstruction(t *testing.T) {
	const src = `const z = new Temporal.ZonedDateTime(0n, "UTC");
console.log(z.epochMilliseconds);`
	got := renderProgram(t, src)
	for _, want := range []string{"value.NewZonedDateTime(", ".EpochMilliseconds()"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestZonedDateTimeTypeSlot pins the type slot: a parameter typed Temporal.ZonedDateTime
// lowers to a pointer to value.ZonedDateTime rather than an interned struct shape.
func TestZonedDateTimeTypeSlot(t *testing.T) {
	const src = `function yearOf(z: Temporal.ZonedDateTime): number { return z.year; }
console.log(yearOf(new Temporal.ZonedDateTime(0n, "UTC")));`
	got := renderProgram(t, src)
	if !strings.Contains(got, "*value.ZonedDateTime") {
		t.Errorf("rendered program missing %q:\n%s", "*value.ZonedDateTime", got)
	}
}

// TestZonedDateTimeGetters pins the exact-time, zone, and wall-clock getters, each to the
// matching method on the value.ZonedDateTime receiver, including the bigint and optional
// getters that route through the same method path.
func TestZonedDateTimeGetters(t *testing.T) {
	cases := map[string]string{
		"epochMilliseconds": ".EpochMilliseconds()",
		"epochNanoseconds":  ".EpochNanoseconds()",
		"timeZoneId":        ".TimeZoneId()",
		"calendarId":        ".CalendarId()",
		"offset":            ".Offset()",
		"offsetNanoseconds": ".OffsetNanoseconds()",
		"year":              ".Year()",
		"hour":              ".Hour()",
		"monthCode":         ".MonthCode()",
		"weekOfYear":        ".WeekOfYear()",
	}
	for prop, want := range cases {
		src := "const z = new Temporal.ZonedDateTime(0n, \"UTC\");\nconsole.log(z." + prop + ");"
		got := renderProgram(t, src)
		if !strings.Contains(got, want) {
			t.Errorf("getter .%s missing %q:\n%s", prop, want, got)
		}
	}
}

// TestZonedDateTimeMethods pins equals, toString, toJSON, and the four conversions to their
// value.ZonedDateTime methods.
func TestZonedDateTimeMethods(t *testing.T) {
	const src = `const a = new Temporal.ZonedDateTime(0n, "UTC");
const b = new Temporal.ZonedDateTime(1n, "UTC");
console.log(a.equals(b));
console.log(a.toString());
console.log(a.toJSON());
console.log(a.toInstant().toString());
console.log(a.toPlainDate().toString());
console.log(a.toPlainTime().toString());
console.log(a.toPlainDateTime().toString());`
	got := renderProgram(t, src)
	for _, want := range []string{".Equals(", ".ToString()", ".ToJSON()", ".ToInstant()", ".ToPlainDate()", ".ToPlainTime()", ".ToPlainDateTime()"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestZonedDateTimeStatics pins compare and from over a ZonedDateTime, each to its value
// function.
func TestZonedDateTimeStatics(t *testing.T) {
	const src = `const a = new Temporal.ZonedDateTime(0n, "UTC");
const b = new Temporal.ZonedDateTime(1n, "UTC");
console.log(Temporal.ZonedDateTime.compare(a, b));
const c = Temporal.ZonedDateTime.from(a);
console.log(c.epochMilliseconds);`
	got := renderProgram(t, src)
	for _, want := range []string{"value.ZonedDateTimeCompare(", "value.ZonedDateTimeFrom("} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestZonedDateTimeHandBacks pins the honest ceilings: the arithmetic and rounding methods,
// the reshaping, the start-of-day and transition queries, from over a string, and a
// constructor with a calendar argument each hand back with a reason naming where the work
// belongs.
func TestZonedDateTimeHandBacks(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "add arithmetic",
			src:  "const z = new Temporal.ZonedDateTime(0n, \"UTC\");\nconst j = z.add({ hours: 1 });\nconsole.log(j.epochMilliseconds);",
			want: "Temporal.ZonedDateTime.prototype.add is a later slice",
		},
		{
			name: "with reshape",
			src:  "const z = new Temporal.ZonedDateTime(0n, \"UTC\");\nconst j = z.with({ hour: 3 });\nconsole.log(j.epochMilliseconds);",
			want: "Temporal.ZonedDateTime.prototype.with is a later slice",
		},
		{
			name: "startOfDay",
			src:  "const z = new Temporal.ZonedDateTime(0n, \"UTC\");\nconst j = z.startOfDay();\nconsole.log(j.epochMilliseconds);",
			want: "Temporal.ZonedDateTime.prototype.startOfDay is a later slice",
		},
		{
			name: "from a string",
			src:  "const z = Temporal.ZonedDateTime.from(\"1970-01-01T00:00:00+00:00[UTC]\");\nconsole.log(z.epochMilliseconds);",
			want: "Temporal.ZonedDateTime.from over a string or a property bag is a later slice",
		},
		{
			name: "constructor with a calendar",
			src:  "const z = new Temporal.ZonedDateTime(0n, \"UTC\", \"iso8601\");\nconsole.log(z.epochMilliseconds);",
			want: "new Temporal.ZonedDateTime with a calendar argument or fewer than two components is a later slice",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderProgramHandBack(t, c.src)
			if !strings.Contains(got, c.want) {
				t.Errorf("hand-back reason = %q, want it to contain %q", got, c.want)
			}
		})
	}
}

// TestZonedDateTimeRun builds and runs the generated Go, proving the exact-time and
// wall-clock getters read, the offset follows the zone, the round-trippable string renders,
// and compare orders on the instant alone while equals also weighs the zone: the same instant
// in UTC and in New York compares equal but is not equal.
func TestZonedDateTimeRun(t *testing.T) {
	skipIfShort(t)
	const src = `const z = new Temporal.ZonedDateTime(0n, "UTC");
console.log(z.epochMilliseconds);
console.log(z.timeZoneId);
console.log(z.year);
console.log(z.offset);
console.log(z.toString());
const ny = new Temporal.ZonedDateTime(0n, "America/New_York");
console.log(ny.hour);
console.log(ny.day);
console.log(ny.offset);
console.log(ny.toString());
console.log(Temporal.ZonedDateTime.compare(z, ny));
console.log(z.equals(ny));
console.log(z.toInstant().toString());
console.log(z.toPlainDate().toString());`
	got := runProgramGo(t, src)
	const want = "0\nUTC\n1970\n+00:00\n1970-01-01T00:00:00+00:00[UTC]\n19\n31\n-05:00\n1969-12-31T19:00:00-05:00[America/New_York]\n0\nfalse\n1970-01-01T00:00:00Z\n1970-01-01\n"
	if got != want {
		t.Fatalf("zoned date-time run printed %q, want %q", got, want)
	}
}
