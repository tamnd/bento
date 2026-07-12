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
			name: "weekOfYear union getter",
			src:  "const d = new Temporal.PlainDate(2020, 2, 29);\nconsole.log(d.weekOfYear);",
			want: "Temporal.PlainDate.weekOfYear is a later slice",
		},
		{
			name: "era union getter",
			src:  "const d = new Temporal.PlainDate(2020, 2, 29);\nconsole.log(d.era);",
			want: "Temporal.PlainDate.era is a later slice",
		},
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
		{
			name: "Instant construction",
			src:  "function makeInstant(): void { new Temporal.Instant(0n); }",
			want: "new Temporal.Instant is a later slice",
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
			name: "weekOfYear union getter",
			src:  "const dt = new Temporal.PlainDateTime(2020, 1, 1, 12, 30);\nconsole.log(dt.weekOfYear);",
			want: "Temporal.PlainDateTime.weekOfYear is a later slice",
		},
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
