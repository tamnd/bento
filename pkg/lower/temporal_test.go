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

// TestPlainDateComparisonCoercesArg pins the ToTemporalDate coercion the comparison and
// difference methods apply to an argument that is not already a PlainDate: compare over a
// string literal parses through PlainDateFromString and over a property bag reads through
// PlainDateFromFields, equals coerces its one argument the same way, and until routes the
// coerced date into Until.
func TestPlainDateComparisonCoercesArg(t *testing.T) {
	cases := []struct {
		name  string
		src   string
		wants []string
	}{
		{
			name:  "compare over string literals",
			src:   "console.log(Temporal.PlainDate.compare(\"2020-01-01\", \"2021-06-15\"));",
			wants: []string{"value.PlainDateCompare(", "value.PlainDateFromString(\"2020-01-01\")", "value.PlainDateFromString(\"2021-06-15\")"},
		},
		{
			name:  "compare over property bags",
			src:   "console.log(Temporal.PlainDate.compare({ year: 2020, month: 1, day: 1 }, { year: 2020, month: 1, day: 2 }));",
			wants: []string{"value.PlainDateCompare(", "value.PlainDateFromFields(", `"constrain"`},
		},
		{
			name:  "equals over a string literal",
			src:   "const d = new Temporal.PlainDate(2020, 3, 14);\nconsole.log(d.equals(\"2020-03-14\"));",
			wants: []string{".Equals(", "value.PlainDateFromString(\"2020-03-14\")"},
		},
		{
			name:  "until over a string literal",
			src:   "const a = new Temporal.PlainDate(2020, 1, 1);\nconsole.log(a.until(\"2020-01-11\").days);",
			wants: []string{".Until(", "value.PlainDateFromString(\"2020-01-11\")", `"day"`},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderProgram(t, c.src)
			for _, want := range c.wants {
				if !strings.Contains(got, want) {
					t.Errorf("rendered program missing %q:\n%s", want, got)
				}
			}
		})
	}
}

// TestPlainDateComparisonCoercionRuns builds and runs the coercion end to end, proving a
// string and a property bag reach the runtime as the dates they name: compare orders them,
// equals folds the calendar in, and until measures the day span.
func TestPlainDateComparisonCoercionRuns(t *testing.T) {
	skipIfShort(t)
	const src = `console.log(Temporal.PlainDate.compare("2020-01-01", "2021-06-15"));
console.log(Temporal.PlainDate.compare("2021-06-15", "2020-01-01"));
console.log(Temporal.PlainDate.compare({ year: 2020, month: 1, day: 1 }, { year: 2020, month: 1, day: 2 }));
const d = new Temporal.PlainDate(2020, 3, 14);
console.log(d.equals("2020-03-14"));
console.log(d.equals({ year: 2020, month: 3, day: 15 }));
const a = new Temporal.PlainDate(2020, 1, 1);
console.log(a.until("2020-01-11").days);
`
	got := runProgramGo(t, src)
	const want = "-1\n1\n-1\ntrue\nfalse\n10\n"
	if got != want {
		t.Fatalf("PlainDate comparison coercion printed %q, want %q", got, want)
	}
}

// TestPlainDateAddSubtract pins the arithmetic: add routes to AddDate with the constrain
// default carried as a string, subtract negates the Duration first, and an explicit reject
// overflow rides through as the string argument.
func TestPlainDateAddSubtract(t *testing.T) {
	cases := []struct {
		name  string
		src   string
		wants []string
	}{
		{
			name:  "add a bag with the constrain default",
			src:   "const d = new Temporal.PlainDate(2020, 1, 31);\nconst e = d.add({ months: 1 });\nconsole.log(e.day);",
			wants: []string{".AddDate(", "value.NewDuration(", `"constrain"`},
		},
		{
			name:  "subtract negates the Duration",
			src:   "const d = new Temporal.PlainDate(2020, 3, 31);\nconst e = d.subtract({ months: 1 });\nconsole.log(e.day);",
			wants: []string{".AddDate(", ".Negated()", `"constrain"`},
		},
		{
			name:  "add with an explicit reject overflow",
			src:   "const d = new Temporal.PlainDate(2020, 1, 31);\nconst e = d.add({ months: 1 }, { overflow: \"reject\" });\nconsole.log(e.day);",
			wants: []string{".AddDate(", `"reject"`},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderProgram(t, c.src)
			for _, want := range c.wants {
				if !strings.Contains(got, want) {
					t.Errorf("rendered program missing %q:\n%s", want, got)
				}
			}
		})
	}
}

// TestPlainDateUntilSince pins the calendar date difference: until routes to Until with the
// day default carried as a string, since routes to Since, and an explicit largestUnit rides
// through as its singular string argument.
func TestPlainDateUntilSince(t *testing.T) {
	cases := []struct {
		name  string
		src   string
		wants []string
	}{
		{
			name:  "until with the day default",
			src:   "const a = new Temporal.PlainDate(2020, 1, 31);\nconst b = new Temporal.PlainDate(2021, 3, 30);\nconsole.log(a.until(b).days);",
			wants: []string{".Until(", `"day"`},
		},
		{
			name:  "since with the day default",
			src:   "const a = new Temporal.PlainDate(2020, 1, 31);\nconst b = new Temporal.PlainDate(2021, 3, 30);\nconsole.log(b.since(a).days);",
			wants: []string{".Since(", `"day"`},
		},
		{
			name:  "until with an explicit largestUnit",
			src:   "const a = new Temporal.PlainDate(2020, 1, 31);\nconst b = new Temporal.PlainDate(2021, 3, 30);\nconsole.log(a.until(b, { largestUnit: \"year\" }).months);",
			wants: []string{".Until(", `"year"`},
		},
		{
			name:  "since with a plural largestUnit normalized to singular",
			src:   "const a = new Temporal.PlainDate(2020, 1, 31);\nconst b = new Temporal.PlainDate(2021, 3, 30);\nconsole.log(b.since(a, { largestUnit: \"months\" }).months);",
			wants: []string{".Since(", `"month"`},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderProgram(t, c.src)
			for _, want := range c.wants {
				if !strings.Contains(got, want) {
					t.Errorf("rendered program missing %q:\n%s", want, got)
				}
			}
		})
	}
}

// TestPlainDateWith pins the reshape: with routes to WithFields carrying each recognized
// field as a present or absent optional and the overflow option as a string, with the
// constrain default and an explicit reject riding through.
func TestPlainDateWith(t *testing.T) {
	cases := []struct {
		name  string
		src   string
		wants []string
	}{
		{
			name:  "a single field with the constrain default",
			src:   "const d = new Temporal.PlainDate(2020, 1, 31);\nconsole.log(d.with({ month: 2 }).day);",
			wants: []string{".WithFields(", "value.Some[float64](", "value.None[float64]()", `"constrain"`},
		},
		{
			name:  "all three fields",
			src:   "const d = new Temporal.PlainDate(2020, 1, 31);\nconsole.log(d.with({ year: 2021, month: 6, day: 10 }).day);",
			wants: []string{".WithFields(", "value.Some[float64](2021)", "value.Some[float64](6)", "value.Some[float64](10)"},
		},
		{
			name:  "an explicit reject overflow",
			src:   "const d = new Temporal.PlainDate(2020, 1, 31);\nconsole.log(d.with({ day: 40 }, { overflow: \"reject\" }).day);",
			wants: []string{".WithFields(", `"reject"`},
		},
		{
			name:  "a monthCode resolves to its month",
			src:   "const d = new Temporal.PlainDate(2020, 1, 31);\nconsole.log(d.with({ monthCode: \"M03\" }).day);",
			wants: []string{".WithFields(", "value.Some[float64](3)", "value.None[float64]()"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderProgram(t, c.src)
			for _, want := range c.wants {
				if !strings.Contains(got, want) {
					t.Errorf("rendered program missing %q:\n%s", want, got)
				}
			}
		})
	}
}

func TestPlainDateFromBag(t *testing.T) {
	cases := []struct {
		name  string
		src   string
		wants []string
	}{
		{
			name:  "the three required fields with the constrain default",
			src:   "console.log(Temporal.PlainDate.from({ year: 2020, month: 3, day: 14 }).toString());",
			wants: []string{"value.PlainDateFromFields(2020, 3, 14, ", `"iso8601"`, `"constrain"`},
		},
		{
			name:  "a calendar interprets the year",
			src:   "console.log(Temporal.PlainDate.from({ year: 109, month: 5, day: 15, calendar: \"roc\" }).toString());",
			wants: []string{"value.PlainDateFromFields(109, 5, 15, ", `"roc"`},
		},
		{
			name:  "an explicit reject overflow",
			src:   "console.log(Temporal.PlainDate.from({ year: 2020, month: 2, day: 31 }, { overflow: \"reject\" }).toString());",
			wants: []string{"value.PlainDateFromFields(", `"reject"`},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderProgram(t, c.src)
			for _, want := range c.wants {
				if !strings.Contains(got, want) {
					t.Errorf("rendered program missing %q:\n%s", want, got)
				}
			}
		})
	}
}

func TestPlainDateToPlainDateTime(t *testing.T) {
	cases := []struct {
		name  string
		src   string
		wants []string
	}{
		{
			name:  "no argument defaults to midnight",
			src:   "const d = new Temporal.PlainDate(2020, 3, 14);\nconsole.log(d.toPlainDateTime().toString());",
			wants: []string{".ToPlainDateTime(nil)"},
		},
		{
			name:  "a plain time pairs in",
			src:   "const d = new Temporal.PlainDate(2020, 3, 14);\nconst t = new Temporal.PlainTime(15, 30);\nconsole.log(d.toPlainDateTime(t).toString());",
			wants: []string{".ToPlainDateTime(", "value.NewPlainTime("},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderProgram(t, c.src)
			for _, want := range c.wants {
				if !strings.Contains(got, want) {
					t.Errorf("rendered program missing %q:\n%s", want, got)
				}
			}
		})
	}
}

func TestPlainDateToPlainYearMonth(t *testing.T) {
	src := "const d = new Temporal.PlainDate(2020, 5, 15);\nconsole.log(d.toPlainYearMonth().toString());"
	got := renderProgram(t, src)
	if !strings.Contains(got, ".ToPlainYearMonth()") {
		t.Errorf("rendered program missing .ToPlainYearMonth():\n%s", got)
	}
}

func TestPlainDateToPlainMonthDay(t *testing.T) {
	src := "const d = new Temporal.PlainDate(2020, 5, 15);\nconsole.log(d.toPlainMonthDay().toString());"
	got := renderProgram(t, src)
	if !strings.Contains(got, ".ToPlainMonthDay()") {
		t.Errorf("rendered program missing .ToPlainMonthDay():\n%s", got)
	}
}

func TestPlainDateToZonedDateTime(t *testing.T) {
	src := "const d = new Temporal.PlainDate(2020, 3, 14);\n" +
		"console.log(d.toZonedDateTime(\"UTC\").toString());\n" +
		"console.log(d.toZonedDateTime({ timeZone: \"America/New_York\", plainTime: \"15:30\" }).toString());"
	got := renderProgram(t, src)
	if !strings.Contains(got, ".ToZonedDateTime(") {
		t.Errorf("rendered program missing .ToZonedDateTime():\n%s", got)
	}
	if !strings.Contains(got, "value.PlainTimeFromString(") {
		t.Errorf("rendered program missing PlainTimeFromString for a plainTime string option:\n%s", got)
	}
}

// TestPlainDateHandBacks pins the honest ceilings: the union getters, the arithmetic
// and conversion methods, from over a bag that carries a monthCode or omits a required
// field, and the other Temporal types each hand back with a reason naming where the work
// belongs.
func TestPlainDateHandBacks(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "add over a dynamic string",
			src:  "function at(s: string) { return new Temporal.PlainDate(2020, 2, 29).add(s).day; }\nconsole.log(at(\"P1D\"));",
			want: "Temporal.PlainDate.prototype.add over an argument that is not a Duration, a duration-like bag of numbers, or a string literal is a later slice",
		},
		{
			name: "until with a rounding option",
			src:  "const a = new Temporal.PlainDate(2020, 1, 31);\nconst b = new Temporal.PlainDate(2021, 3, 30);\nconsole.log(a.until(b, { smallestUnit: \"month\" }).months);",
			want: "Temporal.PlainDate.prototype.until with the rounding option smallestUnit is a later slice",
		},
		{
			name: "with an era field",
			src:  "const d = new Temporal.PlainDate(2020, 1, 31);\nconsole.log(d.with({ era: \"gregory\", eraYear: 2000 }).day);",
			want: "Temporal.PlainDate.prototype.with over a bag with the field era is a later slice",
		},
		{
			name: "from a property bag with a monthCode field",
			src:  "const d = Temporal.PlainDate.from({ monthCode: \"M02\", day: 29 });\nconsole.log(d.day);",
			want: "Temporal.PlainDate.from over a bag with the field monthCode is a later slice",
		},
		{
			name: "toPlainDateTime over a time bag",
			src:  "const d = new Temporal.PlainDate(2020, 3, 14);\nconsole.log(d.toPlainDateTime({ hour: 12 }).toString());",
			want: "Temporal.PlainDate.prototype.toPlainDateTime over an argument that is not a Temporal.PlainTime is a later slice",
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

// TestDurationFromStringConstruction pins Temporal.Duration.from over a string literal to
// value.DurationFromString with the string carried through verbatim.
func TestDurationFromStringConstruction(t *testing.T) {
	const src = `const d = Temporal.Duration.from("P1Y2M3DT4H");
console.log(d.days);`
	got := renderProgram(t, src)
	if !strings.Contains(got, `value.DurationFromString("P1Y2M3DT4H")`) {
		t.Errorf("rendered program missing the from-string call:\n%s", got)
	}
}

// TestDurationFromDynamicStringConstruction pins Temporal.Duration.from over a string value
// not known until run time: the argument lowers and reaches value.DurationFromString through
// its Go string. A Duration carries no calendar, so a dynamic string is always safe.
func TestDurationFromDynamicStringConstruction(t *testing.T) {
	const src = `function at(s: string) { return Temporal.Duration.from(s).years; }
console.log(at("P1Y"));`
	got := renderProgram(t, src)
	if !strings.Contains(got, `value.DurationFromString(s.ToGoString())`) {
		t.Errorf("rendered program missing the dynamic from-string call:\n%s", got)
	}
}

// TestDurationWithConstruction pins Temporal.Duration.prototype.with over a partial bag to a
// value.With call carrying the present fields as value.Some and the absent ones as value.None.
func TestDurationWithConstruction(t *testing.T) {
	const src = `const d = new Temporal.Duration(1, 2, 0, 3);
const e = d.with({ months: 5, days: 10 });
console.log(e.days);`
	got := renderProgram(t, src)
	for _, want := range []string{".With(", "value.Some[float64](5)", "value.Some[float64](10)", "value.None[float64]()"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestDurationFromBagConstruction pins Temporal.Duration.from over an object literal to a
// value.DurationFromFields call over its ten present-or-absent fields.
func TestDurationFromBagConstruction(t *testing.T) {
	const src = `const d = Temporal.Duration.from({ hours: 1, minutes: 30 });
console.log(d.hours);`
	got := renderProgram(t, src)
	for _, want := range []string{"value.DurationFromFields(", "value.Some[float64](1)", "value.Some[float64](30)", "value.None[float64]()"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestDurationAddSubtractConstruction pins Temporal.Duration.prototype.add and subtract over a
// Duration operand to the runtime Add and Subtract calls.
func TestDurationAddSubtractConstruction(t *testing.T) {
	const src = `const a = new Temporal.Duration(0, 0, 0, 2);
const b = new Temporal.Duration(0, 0, 0, 0, 50);
const sum = a.add(b);
const diff = a.subtract(b);
console.log(sum.days, diff.days);`
	got := renderProgram(t, src)
	for _, want := range []string{".Add(", ".Subtract("} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestDurationTotalConstruction pins Temporal.Duration.prototype.total to a runtime Total call:
// a bare-string unit passes the nil relativeTo, and an options object with a PlainDate relativeTo
// passes the lowered date.
func TestDurationTotalConstruction(t *testing.T) {
	const src = `const d = new Temporal.Duration(0, 0, 0, 1, 1);
const rel = Temporal.PlainDate.from("2024-01-01");
console.log(d.total("hour"));
console.log(new Temporal.Duration(0, 18).total({ unit: "year", relativeTo: rel }));`
	got := renderProgram(t, src)
	for _, want := range []string{".Total(", "\"hour\"", "nil", "\"year\""} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestDurationCompareConstruction pins Temporal.Duration.compare to a runtime DurationCompare
// call, passing the nil relativeTo when none is given and the lowered PlainDate when one is.
func TestDurationCompareConstruction(t *testing.T) {
	const src = `const a = new Temporal.Duration(0, 0, 0, 1);
const b = new Temporal.Duration(0, 0, 0, 2);
const rel = Temporal.PlainDate.from("2024-01-01");
console.log(Temporal.Duration.compare(a, b));
console.log(Temporal.Duration.compare(new Temporal.Duration(0, 1), b, { relativeTo: rel }));`
	got := renderProgram(t, src)
	for _, want := range []string{"value.DurationCompare(", "nil"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestDurationRoundConstruction pins Temporal.Duration.prototype.round to a runtime Round call:
// a bare-string smallestUnit passes an empty largestUnit, the default increment and mode, and a
// nil relativeTo; an options object passes the units, increment, mode, and lowered PlainDate.
func TestDurationRoundConstruction(t *testing.T) {
	const src = `const d = new Temporal.Duration(0, 0, 0, 1, 12);
const rel = Temporal.PlainDate.from("2024-01-01");
console.log(d.round("day").toString());
console.log(new Temporal.Duration(0, 5).round({ smallestUnit: "month", roundingIncrement: 2, roundingMode: "ceil", relativeTo: rel }).toString());`
	got := renderProgram(t, src)
	for _, want := range []string{".Round(", "\"day\"", "\"halfExpand\"", "nil", "\"month\"", "\"ceil\""} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestDurationHandBacks pins the honest ceilings for Duration: the balancing and rounding
// methods and compare each hand back with a reason naming where the work belongs, waiting on
// the relativeTo reference and the calendar model.
func TestDurationHandBacks(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "add over a non-Duration argument",
			src:  "const d = new Temporal.Duration(0, 0, 0, 1);\nconst e = d.add(\"P1D\");\nconsole.log(e.days);",
			want: "Temporal.Duration.prototype.add over an argument that is not a Temporal.Duration",
		},
		{
			name: "round with a dynamic smallestUnit",
			src:  "function go(u: any) {\n  const d = new Temporal.Duration(0, 0, 0, 1, 12);\n  return d.round({ smallestUnit: u }).days;\n}\nconsole.log(go(\"day\"));",
			want: "Temporal.Duration.prototype.round with a non-literal smallestUnit is a later slice",
		},
		{
			name: "round with a non-PlainDate relativeTo",
			src:  "const d = new Temporal.Duration(0, 1);\nconst r = d.round({ smallestUnit: \"month\", relativeTo: \"2024-01-01\" });\nconsole.log(r.months);",
			want: "with a relativeTo that is not a Temporal.PlainDate",
		},
		{
			name: "total with a non-PlainDate relativeTo",
			src:  "const d = new Temporal.Duration(1);\nconsole.log(d.total({ unit: \"day\", relativeTo: \"2024-01-01\" }));",
			want: "with a relativeTo that is not a Temporal.PlainDate",
		},
		{
			name: "from a bag with a spread",
			src:  "const base = { years: 1 };\nconst d = Temporal.Duration.from({ ...base, months: 2 });\nconsole.log(d.years);",
			want: "Temporal.Duration.from over a bag with a computed or shorthand key is a later slice",
		},
		{
			name: "with a spread",
			src:  "const base = { months: 1 };\nconst d = new Temporal.Duration(1);\nconst e = d.with({ ...base, days: 2 });\nconsole.log(e.days);",
			want: "Temporal.Duration.prototype.with over a bag with a computed or shorthand key is a later slice",
		},
		{
			name: "compare with a non-PlainDate relativeTo",
			src:  "const a = new Temporal.Duration(1);\nconst b = new Temporal.Duration(0, 0, 0, 365);\nconsole.log(Temporal.Duration.compare(a, b, { relativeTo: \"2024-01-01\" }));",
			want: "with a relativeTo that is not a Temporal.PlainDate",
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
			name: "until over a string argument",
			src:  "const a = new Temporal.PlainTime(12, 30);\nconst d = a.until(\"14:00\");\nconsole.log(d.hours);",
			want: "Temporal.PlainTime.prototype.until over an argument that is not a Temporal.PlainTime is a later slice",
		},
		{
			name: "add over a dynamic string",
			src:  "function go(s: string) {\n  const t = new Temporal.PlainTime(12, 30);\n  return t.add(s).hour;\n}\nconsole.log(go(\"PT1H\"));",
			want: "Temporal.PlainTime.prototype.add over an argument that is not a Duration, a duration-like bag of numbers, or a string literal is a later slice",
		},
		{
			name: "from a bag with a shorthand key",
			src:  "const hour = 12;\nconst t = Temporal.PlainTime.from({ hour });\nconsole.log(t.hour);",
			want: "Temporal.PlainTime.from over a bag with a computed or shorthand key is a later slice",
		},
		{
			name: "round with a dynamic smallestUnit",
			src:  "function go(u: any) {\n  const t = new Temporal.PlainTime(12, 30, 45);\n  return t.round({ smallestUnit: u }).minute;\n}\nconsole.log(go(\"minute\"));",
			want: "Temporal.PlainTime.prototype.round with a non-literal smallestUnit is a later slice",
		},
		{
			name: "round with a dynamic roundingMode",
			src:  "function go(m: any) {\n  const t = new Temporal.PlainTime(12, 30, 45);\n  return t.round({ smallestUnit: \"minute\", roundingMode: m }).minute;\n}\nconsole.log(go(\"ceil\"));",
			want: "Temporal.PlainTime.prototype.round with a non-literal roundingMode is a later slice",
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

// TestPlainTimeFromBag pins Temporal.PlainTime.from over a property bag: each named field
// lowers to value.Some[float64] and each absent one to value.None[float64], with the
// overflow option threaded through as a trailing string. The default overflow is constrain.
func TestPlainTimeFromBag(t *testing.T) {
	const src = `const t = Temporal.PlainTime.from({ hour: 12, minute: 30 });
console.log(t.hour);`
	got := renderProgram(t, src)
	for _, want := range []string{
		"value.PlainTimeFromFields(",
		"value.Some[float64](12)",
		"value.Some[float64](30)",
		"value.None[float64]()",
		`"constrain"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestPlainTimeFromBagReject pins the overflow option: an explicit reject rides through as
// the trailing string literal to the runtime factory.
func TestPlainTimeFromBagReject(t *testing.T) {
	const src = `const t = Temporal.PlainTime.from({ hour: 12 }, { overflow: "reject" });
console.log(t.hour);`
	got := renderProgram(t, src)
	for _, want := range []string{"value.PlainTimeFromFields(", `"reject"`} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestPlainTimeWith pins Temporal.PlainTime.prototype.with: the receiver's With method
// takes the six present-or-absent fields and the overflow string, so an absent field holds
// the receiver's value at run time.
func TestPlainTimeWith(t *testing.T) {
	const src = `const t = new Temporal.PlainTime(12, 30, 15);
const u = t.with({ minute: 45 });
console.log(u.minute);`
	got := renderProgram(t, src)
	for _, want := range []string{
		".With(",
		"value.None[float64]()",
		"value.Some[float64](45)",
		`"constrain"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestPlainTimeAdd pins Temporal.PlainTime.prototype.add: a duration-like bag builds a
// value.NewDuration over the ten padded unit fields and the receiver folds it with
// AddDuration.
func TestPlainTimeAdd(t *testing.T) {
	const src = `const t = new Temporal.PlainTime(12, 30, 15);
const u = t.add({ hours: 1, minutes: 30 });
console.log(u.hour);`
	got := renderProgram(t, src)
	for _, want := range []string{
		".AddDuration(",
		"value.NewDuration(0, 0, 0, 0, 1, 30, 0, 0, 0, 0)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, ".Negated()") {
		t.Errorf("add negated the duration:\n%s", got)
	}
}

// TestPlainTimeSubtract pins Temporal.PlainTime.prototype.subtract: the same duration read
// negated before the fold, so subtract is add over a negated Duration. A Duration receiver
// passes straight through with no bag construction.
func TestPlainTimeSubtract(t *testing.T) {
	const src = `const d = new Temporal.Duration(0, 0, 0, 0, 13);
const t = new Temporal.PlainTime(12, 30, 15);
const u = t.subtract(d);
console.log(u.hour);`
	got := renderProgram(t, src)
	for _, want := range []string{".AddDuration(", ".Negated()"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestPlainTimeRound pins Temporal.PlainTime.prototype.round over an options bag: the
// smallestUnit and roundingMode ride through as string literals and the roundingIncrement
// as a number expression, with the default increment one and the default mode halfExpand.
func TestPlainTimeRound(t *testing.T) {
	const src = `const t = new Temporal.PlainTime(3, 34, 56);
const r = t.round({ smallestUnit: "minute", roundingIncrement: 15, roundingMode: "ceil" });
console.log(r.minute);`
	got := renderProgram(t, src)
	for _, want := range []string{
		".Round(",
		`"minute"`,
		"15",
		`"ceil"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestPlainTimeRoundShorthand pins the smallestUnit string shorthand: t.round("hour")
// carries the default increment one and the default mode halfExpand.
func TestPlainTimeRoundShorthand(t *testing.T) {
	const src = `const t = new Temporal.PlainTime(3, 34, 56);
const r = t.round("hour");
console.log(r.hour);`
	got := renderProgram(t, src)
	for _, want := range []string{
		".Round(",
		`"hour"`,
		`"halfExpand"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestPlainTimeUntil pins the difference emit: t.until(other) lowers to other.Until with the
// four option arguments, defaulting largestUnit to hour, smallestUnit to nanosecond, the
// increment to one, and the mode to trunc.
func TestPlainTimeUntil(t *testing.T) {
	const src = `const a = new Temporal.PlainTime(12, 30);
const b = new Temporal.PlainTime(14, 0);
const d = a.until(b);
console.log(d.hours);`
	got := renderProgram(t, src)
	for _, want := range []string{
		".Until(",
		`"hour"`,
		`"nanosecond"`,
		`"trunc"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestPlainTimeSince pins the reversed difference and its option bag: t.since(other) lowers to
// Since, and largestUnit/smallestUnit/roundingIncrement/roundingMode carry through.
func TestPlainTimeSince(t *testing.T) {
	const src = `const a = new Temporal.PlainTime(12, 30);
const b = new Temporal.PlainTime(14, 0);
const d = a.since(b, { largestUnit: "minute", smallestUnit: "minute", roundingIncrement: 5, roundingMode: "ceil" });
console.log(d.minutes);`
	got := renderProgram(t, src)
	for _, want := range []string{
		".Since(",
		`"minute"`,
		"5",
		`"ceil"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
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

// TestPlainDateTimeArithmetic pins add and subtract to AddDateTime with the overflow rule,
// until and since to Until and Since with the largestUnit, and round to Round with the unit,
// increment, and mode. subtract negates the duration before it folds into the wall clock.
func TestPlainDateTimeArithmetic(t *testing.T) {
	cases := []struct {
		name  string
		src   string
		wants []string
	}{
		{
			name:  "add over a duration bag",
			src:   "const dt = new Temporal.PlainDateTime(2020, 1, 1, 12, 30);\nconst e = dt.add({ hours: 1 });\nconsole.log(e.hour);",
			wants: []string{".AddDateTime(", "value.NewDuration(", `"constrain"`},
		},
		{
			name:  "subtract negates the duration",
			src:   "const dt = new Temporal.PlainDateTime(2020, 1, 1, 12, 30);\nconst e = dt.subtract({ hours: 1 });\nconsole.log(e.hour);",
			wants: []string{".AddDateTime(", ".Negated()", `"constrain"`},
		},
		{
			name:  "add with reject overflow",
			src:   "const dt = new Temporal.PlainDateTime(2020, 1, 31, 12, 30);\nconst e = dt.add({ months: 1 }, { overflow: \"reject\" });\nconsole.log(e.month);",
			wants: []string{".AddDateTime(", `"reject"`},
		},
		{
			name:  "until defaults to day",
			src:   "const a = new Temporal.PlainDateTime(2020, 1, 1, 12, 0);\nconst b = new Temporal.PlainDateTime(2020, 1, 2, 6, 0);\nconsole.log(a.until(b).toString());",
			wants: []string{".Until(", `"day"`},
		},
		{
			name:  "until with a largestUnit year",
			src:   "const a = new Temporal.PlainDateTime(2020, 1, 1, 12, 0);\nconst b = new Temporal.PlainDateTime(2021, 3, 15, 8, 0);\nconsole.log(a.until(b, { largestUnit: \"year\" }).toString());",
			wants: []string{".Until(", `"year"`},
		},
		{
			name:  "since routes to Since",
			src:   "const a = new Temporal.PlainDateTime(2020, 1, 1, 12, 0);\nconst b = new Temporal.PlainDateTime(2020, 1, 2, 6, 0);\nconsole.log(b.since(a).toString());",
			wants: []string{".Since(", `"day"`},
		},
		{
			name:  "round over an options bag",
			src:   "const dt = new Temporal.PlainDateTime(2020, 1, 1, 3, 34, 56);\nconsole.log(dt.round({ smallestUnit: \"hour\" }).toString());",
			wants: []string{".Round(", `"hour"`, `"halfExpand"`},
		},
		{
			name:  "round to day with the string shorthand",
			src:   "const dt = new Temporal.PlainDateTime(2020, 1, 1, 18, 0);\nconsole.log(dt.round(\"day\").toString());",
			wants: []string{".Round(", `"day"`},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderProgram(t, c.src)
			for _, want := range c.wants {
				if !strings.Contains(got, want) {
					t.Errorf("rendered program missing %q:\n%s", want, got)
				}
			}
		})
	}
}

// TestPlainDateTimeHandBacks pins the honest ceilings for PlainDateTime: from over a string,
// and a reshaping or conversion method each hand back with a reason naming where the work
// belongs.
func TestPlainDateTimeHandBacks(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "with an era field",
			src:  "const dt = new Temporal.PlainDateTime(2020, 1, 1, 12, 30);\nconst e = dt.with({ era: \"gregory\", eraYear: 2000 });\nconsole.log(e.month);",
			want: "Temporal.PlainDateTime.prototype.with over a bag with the field era is a later slice",
		},
		{
			name: "from a dynamic string",
			src:  "function at(s: string) { return Temporal.PlainDateTime.from(s).hour; }\nconsole.log(at(\"2020-01-01T12:30:00\"));",
			want: "Temporal.PlainDateTime.from over a dynamic string is a later slice",
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

// TestPlainDateTimeConversions pins the two simple half-conversions: toPlainDate lowers to
// ToPlainDate and toPlainTime lowers to ToPlainTime, each a no-argument delegation whose result
// the checker types as a PlainDate or PlainTime so a chained getter or toString routes on.
func TestPlainDateTimeConversions(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "toPlainDate",
			src:  "const dt = new Temporal.PlainDateTime(2020, 5, 15, 13, 30);\nconst d = dt.toPlainDate();\nconsole.log(d.toString());",
			want: ".ToPlainDate()",
		},
		{
			name: "toPlainTime",
			src:  "const dt = new Temporal.PlainDateTime(2020, 5, 15, 13, 30);\nconst tm = dt.toPlainTime();\nconsole.log(tm.toString());",
			want: ".ToPlainTime()",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderProgram(t, c.src)
			if !strings.Contains(got, c.want) {
				t.Errorf("rendered program missing %q:\n%s", c.want, got)
			}
		})
	}
}

// TestPlainDateTimeWithPlainTime pins withPlainTime: no argument defaults to midnight (a nil
// *PlainTime), a Temporal.PlainTime passes straight through, a time string parses at run time, and
// a time-like bag regulates under constrain.
func TestPlainDateTimeWithPlainTime(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "no argument",
			src:  "const dt = new Temporal.PlainDateTime(2020, 5, 15, 13, 30);\nconsole.log(dt.withPlainTime().toString());",
			want: ".WithPlainTime(nil)",
		},
		{
			name: "plain time",
			src:  "const dt = new Temporal.PlainDateTime(2020, 5, 15, 13, 30);\nconsole.log(dt.withPlainTime(new Temporal.PlainTime(9, 15)).toString());",
			want: ".WithPlainTime(value.NewPlainTime(",
		},
		{
			name: "time string",
			src:  "const dt = new Temporal.PlainDateTime(2020, 5, 15, 13, 30);\nconsole.log(dt.withPlainTime(\"22:45:10\").toString());",
			want: ".WithPlainTime(value.PlainTimeFromString(\"22:45:10\"))",
		},
		{
			name: "time-like bag",
			src:  "const dt = new Temporal.PlainDateTime(2020, 5, 15, 13, 30);\nconsole.log(dt.withPlainTime({ hour: 6, minute: 5 }).toString());",
			want: "value.PlainTimeFromFields(",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderProgram(t, c.src)
			if !strings.Contains(got, c.want) {
				t.Errorf("rendered program missing %q:\n%s", c.want, got)
			}
		})
	}
}

// TestPlainDateTimeWith pins with: a bag of date and time fields lowers to WithFields carrying each
// recognized field as a present or absent optional, and an overflow option threads through.
func TestPlainDateTimeWith(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "time field",
			src:  "const dt = new Temporal.PlainDateTime(2020, 1, 31, 13, 30);\nconsole.log(dt.with({ hour: 9 }).toString());",
			want: ".WithFields(value.None[float64](), value.None[float64](), value.None[float64](), value.Some[float64](9)",
		},
		{
			name: "date and overflow",
			src:  "const dt = new Temporal.PlainDateTime(2020, 1, 31, 13, 30);\nconsole.log(dt.with({ month: 2 }, { overflow: \"reject\" }).toString());",
			want: "value.Some[float64](2), value.None[float64]()",
		},
		{
			name: "overflow reject threads",
			src:  "const dt = new Temporal.PlainDateTime(2020, 1, 31, 13, 30);\nconsole.log(dt.with({ month: 2 }, { overflow: \"reject\" }).toString());",
			want: "\"reject\")",
		},
		{
			name: "a monthCode resolves to its month",
			src:  "const dt = new Temporal.PlainDateTime(2020, 1, 31, 13, 30);\nconsole.log(dt.with({ monthCode: \"M03\" }).toString());",
			want: "value.Some[float64](3), value.None[float64]()",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderProgram(t, c.src)
			if !strings.Contains(got, c.want) {
				t.Errorf("rendered program missing %q:\n%s", c.want, got)
			}
		})
	}
}

// TestPlainDateTimeToZonedDateTime pins toZonedDateTime: a time-zone string lowers to
// ToZonedDateTime with the default compatible disambiguation, and an options bag carries a
// disambiguation string literal through.
func TestPlainDateTimeToZonedDateTime(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "bare time zone",
			src:  "const dt = new Temporal.PlainDateTime(2020, 3, 14, 15, 30);\nconsole.log(dt.toZonedDateTime(\"UTC\").toString());",
			want: ".ToZonedDateTime(\"UTC\", \"compatible\")",
		},
		{
			name: "disambiguation option",
			src:  "const dt = new Temporal.PlainDateTime(2020, 3, 8, 2, 30);\nconsole.log(dt.toZonedDateTime(\"America/New_York\", { disambiguation: \"earlier\" }).toString());",
			want: ".ToZonedDateTime(\"America/New_York\", \"earlier\")",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderProgram(t, c.src)
			if !strings.Contains(got, c.want) {
				t.Errorf("rendered program missing %q:\n%s", c.want, got)
			}
		})
	}
}

// TestPlainDateTimeFromStringConstruction pins Temporal.PlainDateTime.from over a string
// literal to value.PlainDateTimeFromString with the string carried through verbatim.
func TestPlainDateTimeFromStringConstruction(t *testing.T) {
	const src = `const dt = Temporal.PlainDateTime.from("2020-01-01T12:30:00");
console.log(dt.hour);`
	got := renderProgram(t, src)
	if !strings.Contains(got, `value.PlainDateTimeFromString("2020-01-01T12:30:00")`) {
		t.Errorf("rendered program missing the from-string call:\n%s", got)
	}
}

// TestPlainDateTimeFromStringHandsBack pins that a string literal naming a calendar bento
// does not host hands back, since the runtime parser would reject it where the specification
// would succeed.
func TestPlainDateTimeFromStringHandsBack(t *testing.T) {
	const src = `const dt = Temporal.PlainDateTime.from("2020-01-01T12:30:00[u-ca=hebrew]");
console.log(dt.hour);`
	got := renderProgramHandBack(t, src)
	if !strings.Contains(got, "Temporal.PlainDateTime.from over a string naming a calendar bento does not host is a later slice") {
		t.Errorf("hand-back reason = %q, want the unhosted-calendar ceiling", got)
	}
}

// TestPlainDateTimeFromBag pins Temporal.PlainDateTime.from over a property bag to a
// value.PlainDateTimeFromFields call: the required date fields, the optional time fields, the
// calendar, and the overflow option all thread through.
func TestPlainDateTimeFromBag(t *testing.T) {
	cases := []struct {
		name  string
		src   string
		wants []string
	}{
		{
			name:  "the date fields with the constrain default",
			src:   "console.log(Temporal.PlainDateTime.from({ year: 2020, month: 3, day: 14 }).toString());",
			wants: []string{"value.PlainDateTimeFromFields(2020, 3, 14, ", "value.None[float64]()", `"iso8601"`, `"constrain"`},
		},
		{
			name:  "the time fields lower to present optionals",
			src:   "console.log(Temporal.PlainDateTime.from({ year: 2020, month: 3, day: 14, hour: 13, minute: 30 }).toString());",
			wants: []string{"value.PlainDateTimeFromFields(2020, 3, 14, ", "value.Some[float64](13)", "value.Some[float64](30)"},
		},
		{
			name:  "a calendar interprets the year",
			src:   "console.log(Temporal.PlainDateTime.from({ year: 109, month: 5, day: 15, hour: 12, calendar: \"roc\" }).toString());",
			wants: []string{"value.PlainDateTimeFromFields(109, 5, 15, ", `"roc"`},
		},
		{
			name:  "an explicit reject overflow",
			src:   "console.log(Temporal.PlainDateTime.from({ year: 2020, month: 2, day: 31 }, { overflow: \"reject\" }).toString());",
			wants: []string{"value.PlainDateTimeFromFields(", `"reject"`},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderProgram(t, c.src)
			for _, want := range c.wants {
				if !strings.Contains(got, want) {
					t.Errorf("rendered program missing %q:\n%s", want, got)
				}
			}
		})
	}
}

// TestPlainDateTimeFromBagHandsBack pins that a bag missing a required date field or carrying a
// field the runtime does not model hands back rather than dropping to a wrong result.
func TestPlainDateTimeFromBagHandsBack(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "a monthCode field",
			src:  "console.log(Temporal.PlainDateTime.from({ monthCode: \"M02\", day: 29 }).toString());",
			want: "Temporal.PlainDateTime.from over a bag with the field monthCode is a later slice",
		},
		{
			name: "an unhosted calendar",
			src:  "console.log(Temporal.PlainDateTime.from({ year: 2020, month: 3, day: 14, calendar: \"hebrew\" }).toString());",
			want: "Temporal.PlainDateTime.from over a bag whose calendar is dynamic or names a calendar bento does not host is a later slice",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderProgramHandBack(t, c.src)
			if !strings.Contains(got, c.want) {
				t.Errorf("hand-back reason = %q, want %q", got, c.want)
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
		"era":          ".Era()",
		"eraYear":      ".EraYear()",
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

// TestPlainYearMonthArithmetic pins add, subtract, until, and since to their value.PlainYearMonth
// methods with the overflow and largestUnit strings carried through.
func TestPlainYearMonthArithmetic(t *testing.T) {
	const src = `const a = new Temporal.PlainYearMonth(2020, 3);
const b = new Temporal.PlainYearMonth(2021, 8);
console.log(a.add({ months: 1 }).toString());
console.log(a.subtract({ months: 1 }, { overflow: "reject" }).toString());
console.log(a.until(b).toString());
console.log(a.since(b, { largestUnit: "month" }).toString());`
	got := renderProgram(t, src)
	for _, want := range []string{".AddDuration(", ".SubtractDuration(", ".Until(", ".Since(", "\"constrain\"", "\"reject\"", "\"year\"", "\"month\""} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestPlainYearMonthWithAndToPlainDate pins with to value.PlainYearMonth.WithFields, a literal
// monthCode resolving to a numeric month, and toPlainDate to value.PlainYearMonth.ToPlainDate.
func TestPlainYearMonthWithAndToPlainDate(t *testing.T) {
	const src = `const a = new Temporal.PlainYearMonth(2020, 3);
console.log(a.with({ month: 11 }).toString());
console.log(a.with({ year: 1999 }, { overflow: "reject" }).toString());
console.log(a.with({ monthCode: "M07" }).toString());
console.log(a.toPlainDate({ day: 15 }).toString());`
	got := renderProgram(t, src)
	for _, want := range []string{".WithFields(", "value.Some[float64](", "value.None[float64]()", "\"reject\"", ".ToPlainDate("} {
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
			name: "until over a string argument",
			src:  "const ym = new Temporal.PlainYearMonth(2020, 3);\nconst d = ym.until(\"2020-05\");\nconsole.log(d.months);",
			want: "Temporal.PlainYearMonth.prototype.until over an argument that is not a Temporal.PlainYearMonth",
		},
		{
			name: "from a bag with a calendar",
			src:  "const ym = Temporal.PlainYearMonth.from({ year: 2020, month: 3, calendar: \"gregory\" });\nconsole.log(ym.month);",
			want: "Temporal.PlainYearMonth.from over a bag with the field calendar is a later slice",
		},
		{
			name: "from a bag missing the month",
			src:  "const ym = Temporal.PlainYearMonth.from({ year: 2020 });\nconsole.log(ym.month);",
			want: "Temporal.PlainYearMonth.from over a bag missing the month (a TypeError at run time) is a later slice",
		},
		{
			name: "from a non-ISO calendar string",
			src:  "const ym = Temporal.PlainYearMonth.from(\"2020-03-15[u-ca=gregory]\");\nconsole.log(ym.month);",
			want: "Temporal.PlainYearMonth.from over a string naming a non-ISO calendar is a later slice",
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

// TestPlainYearMonthFromStringConstruction pins Temporal.PlainYearMonth.from over a string
// literal to value.PlainYearMonthFromString with the string carried through verbatim.
func TestPlainYearMonthFromStringConstruction(t *testing.T) {
	const src = `const ym = Temporal.PlainYearMonth.from("2020-03");
console.log(ym.month);`
	got := renderProgram(t, src)
	if !strings.Contains(got, `value.PlainYearMonthFromString("2020-03")`) {
		t.Errorf("rendered program missing the from-string call:\n%s", got)
	}
}

// TestPlainYearMonthFromBag pins Temporal.PlainYearMonth.from over a property bag to
// value.PlainYearMonthFromFields, resolving a literal monthCode to its month and carrying the
// overflow option.
func TestPlainYearMonthFromBag(t *testing.T) {
	const src = `const a = Temporal.PlainYearMonth.from({ year: 2020, month: 3 });
console.log(a.toString());
const b = Temporal.PlainYearMonth.from({ year: 2020, monthCode: "M07" }, { overflow: "reject" });
console.log(b.toString());`
	got := renderProgram(t, src)
	for _, want := range []string{"value.PlainYearMonthFromFields(", "\"reject\""} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
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

// TestPlainMonthDayWithAndToPlainDate pins the reshaping and conversion methods: with lowers to
// WithFields over present or absent month and day optionals, resolving a literal monthCode to its
// month and carrying the overflow option, and toPlainDate lowers to ToPlainDate over the year the
// argument bag supplies.
func TestPlainMonthDayWithAndToPlainDate(t *testing.T) {
	const src = `const a = new Temporal.PlainMonthDay(3, 15);
console.log(a.with({ day: 20 }).toString());
console.log(a.with({ month: 4, day: 30 }, { overflow: "reject" }).toString());
console.log(a.with({ monthCode: "M07" }).toString());
console.log(a.toPlainDate({ year: 2020 }).toString());`
	got := renderProgram(t, src)
	for _, want := range []string{".WithFields(", "value.Some[float64](", "value.None[float64]()", "\"reject\"", ".ToPlainDate("} {
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

// TestPlainMonthDayFromBag pins Temporal.PlainMonthDay.from over a property bag to
// value.PlainMonthDayFromFields, resolving a literal monthCode to its month, emitting the optional
// year as a value.Opt, and carrying the overflow option.
func TestPlainMonthDayFromBag(t *testing.T) {
	const src = `const a = Temporal.PlainMonthDay.from({ month: 3, day: 15 });
console.log(a.toString());
const b = Temporal.PlainMonthDay.from({ year: 2021, monthCode: "M02", day: 29 });
console.log(b.toString());`
	got := renderProgram(t, src)
	for _, want := range []string{"value.PlainMonthDayFromFields(", "value.Some[float64](", "value.None[float64]()"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestPlainMonthDayHandBacks pins the honest ceilings: the reshaping and conversion methods, a
// property bag, and a non-ISO calendar string each hand back with a reason naming where the
// work belongs.
func TestPlainMonthDayHandBacks(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "with a year field",
			src:  "const md = new Temporal.PlainMonthDay(3, 15);\nconst e = md.with({ year: 2020 });\nconsole.log(e.day);",
			want: "Temporal.PlainMonthDay.prototype.with over a bag with the field year is a later slice",
		},
		{
			name: "toPlainDate over a computed key",
			src:  "const md = new Temporal.PlainMonthDay(3, 15);\nconst k = \"year\";\nconst d = md.toPlainDate({ [k]: 2020 });\nconsole.log(d.day);",
			want: "Temporal.PlainMonthDay.prototype.toPlainDate over a bag with a computed or shorthand key is a later slice",
		},
		{
			name: "from a bag with a calendar",
			src:  "const md = Temporal.PlainMonthDay.from({ month: 3, day: 15, calendar: \"gregory\" });\nconsole.log(md.day);",
			want: "Temporal.PlainMonthDay.from over a bag with the field calendar is a later slice",
		},
		{
			name: "from a bag with a computed key",
			src:  "const k = \"month\";\nconst md = Temporal.PlainMonthDay.from({ [k]: 3, day: 15 });\nconsole.log(md.day);",
			want: "Temporal.PlainMonthDay.from over a bag with a computed or shorthand key is a later slice",
		},
		{
			name: "from a non-ISO calendar string",
			src:  "const md = Temporal.PlainMonthDay.from(\"2024-06-15[u-ca=gregory]\");\nconsole.log(md.day);",
			want: "Temporal.PlainMonthDay.from over a string naming a non-ISO calendar is a later slice",
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

// TestPlainMonthDayFromStringConstruction pins Temporal.PlainMonthDay.from over a string
// literal to value.PlainMonthDayFromString with the string carried through verbatim.
func TestPlainMonthDayFromStringConstruction(t *testing.T) {
	const src = `const md = Temporal.PlainMonthDay.from("03-15");
console.log(md.day);`
	got := renderProgram(t, src)
	if !strings.Contains(got, `value.PlainMonthDayFromString("03-15")`) {
		t.Errorf("rendered program missing the from-string call:\n%s", got)
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

// TestInstantAddSubtract pins add and subtract to value.Instant.AddDuration, negating the
// duration for subtract, over a bag, a Duration value, and a string literal.
func TestInstantAddSubtract(t *testing.T) {
	const src = `const i = new Temporal.Instant(0n);
const a = i.add({ hours: 1 });
const b = i.subtract({ minutes: 30 });
const c = i.add(Temporal.Duration.from("PT1H"));
const d = i.add("PT15M");
console.log(a.epochMilliseconds);
console.log(b.epochMilliseconds);
console.log(c.epochMilliseconds);
console.log(d.epochMilliseconds);`
	got := renderProgram(t, src)
	for _, want := range []string{".AddDuration(", ".Negated()"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestInstantUntilSince pins until and since to value.Instant.Until and Since, threading the
// default largestUnit, smallestUnit, increment, and mode, and reading an options bag.
func TestInstantUntilSince(t *testing.T) {
	const src = `const a = new Temporal.Instant(0n);
const b = new Temporal.Instant(8130250500000n);
console.log(a.until(b).toString());
console.log(a.since(b).toString());
console.log(a.until(b, { largestUnit: "hour" }).toString());
console.log(a.until(b, { smallestUnit: "second", roundingMode: "ceil" }).toString());`
	got := renderProgram(t, src)
	for _, want := range []string{".Until(", ".Since(", `"second"`, `"hour"`, `"ceil"`} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestInstantRound pins round to value.Instant.Round, reading a string shorthand and an
// options bag with the increment and mode threaded through.
func TestInstantRound(t *testing.T) {
	const src = `const b = new Temporal.Instant(8130250500000n);
console.log(b.round("hour").toString());
console.log(b.round({ smallestUnit: "minute", roundingIncrement: 15 }).toString());
console.log(b.round({ smallestUnit: "second", roundingMode: "ceil" }).toString());`
	got := renderProgram(t, src)
	for _, want := range []string{".Round(", `"hour"`, `"minute"`, `"ceil"`} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestInstantToZonedDateTimeISO pins toZonedDateTimeISO to value.Instant.ToZonedDateTimeISO,
// threading a time-zone string literal and a dynamic string through the shared reader.
func TestInstantToZonedDateTimeISO(t *testing.T) {
	const src = `const i = new Temporal.Instant(0n);
console.log(i.toZonedDateTimeISO("UTC").toString());
console.log(i.toZonedDateTimeISO("America/New_York").toString());`
	got := renderProgram(t, src)
	for _, want := range []string{".ToZonedDateTimeISO(", `"UTC"`, `"America/New_York"`} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestInstantHandBacks pins the honest ceilings: a rounding option that depends on run-time
// data and the locale conversion each hand back with a reason naming where the work belongs.
func TestInstantHandBacks(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "until with a dynamic largestUnit",
			src:  "function diff(u: any) {\n  const i = new Temporal.Instant(0n);\n  const j = new Temporal.Instant(1n);\n  return i.until(j, { largestUnit: u }).nanoseconds;\n}\nconsole.log(diff(\"hour\"));",
			want: "Temporal.Instant.prototype.until with a non-literal largestUnit is a later slice",
		},
		{
			name: "round with a dynamic smallestUnit",
			src:  "function at(u: any) {\n  const i = new Temporal.Instant(1500000000n);\n  return i.round({ smallestUnit: u }).epochMilliseconds;\n}\nconsole.log(at(\"hour\"));",
			want: "Temporal.Instant.prototype.round with a non-literal smallestUnit is a later slice",
		},
		{
			name: "toLocaleString conversion",
			src:  "const i = new Temporal.Instant(1500000000n);\nconsole.log(i.toLocaleString());",
			want: "Temporal.Instant.prototype.toLocaleString is a later slice",
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

// TestInstantFromStringConstruction pins Temporal.Instant.from over a string literal to
// value.InstantFromString with the string carried through verbatim.
func TestInstantFromStringConstruction(t *testing.T) {
	const src = `const i = Temporal.Instant.from("2020-01-01T00:00:00Z");
console.log(i.epochMilliseconds);`
	got := renderProgram(t, src)
	if !strings.Contains(got, `value.InstantFromString("2020-01-01T00:00:00Z")`) {
		t.Errorf("rendered program missing the from-string call:\n%s", got)
	}
}

// TestInstantFromDynamicStringConstruction pins Temporal.Instant.from over a string value not
// known until run time: the argument lowers and reaches value.InstantFromString through its Go
// string. An Instant ignores any calendar the string names, so a dynamic string is safe.
func TestInstantFromDynamicStringConstruction(t *testing.T) {
	const src = `function at(s: string) { return Temporal.Instant.from(s).epochMilliseconds; }
console.log(at("1970-01-01T00:00:00Z"));`
	got := renderProgram(t, src)
	if !strings.Contains(got, `value.InstantFromString(s.ToGoString())`) {
		t.Errorf("rendered program missing the dynamic from-string call:\n%s", got)
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

// TestZonedDateTimeAdd pins add and subtract to value.AddDuration, the duration lowering
// through the shared reader, subtract negating it, and the overflow option carried as a string.
func TestZonedDateTimeAdd(t *testing.T) {
	const src = `const z = new Temporal.ZonedDateTime(0n, "UTC");
console.log(z.add({ days: 1 }).epochMilliseconds);
console.log(z.subtract({ hours: 2 }).epochMilliseconds);
console.log(z.add({ months: 1 }, { overflow: "reject" }).epochMilliseconds);`
	got := renderProgram(t, src)
	for _, want := range []string{".AddDuration(", ".Negated()", `"constrain"`, `"reject"`} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestZonedDateTimeDifference pins until and since over a ZonedDateTime argument to the runtime
// Until and Since, the largestUnit read from the options and defaulting to hour, since gets the
// same difference the runtime negates.
func TestZonedDateTimeDifference(t *testing.T) {
	const src = `const a = new Temporal.ZonedDateTime(0n, "UTC");
const b = new Temporal.ZonedDateTime(1n, "UTC");
console.log(a.until(b).hours);
console.log(a.until(b, { largestUnit: "days" }).days);
console.log(a.since(b, { largestUnit: "months" }).months);`
	got := renderProgram(t, src)
	for _, want := range []string{".Until(", ".Since(", `"hour"`, `"day"`, `"month"`} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestZonedDateTimeRound pins round over a ZonedDateTime to the runtime Round, the smallestUnit,
// increment expression, and mode read through the shared day-and-time round-options reader.
func TestZonedDateTimeRound(t *testing.T) {
	const src = `const z = new Temporal.ZonedDateTime(0n, "UTC");
console.log(z.round("hour").epochMilliseconds);
console.log(z.round({ smallestUnit: "minute", roundingIncrement: 15, roundingMode: "ceil" }).epochMilliseconds);
console.log(z.round({ smallestUnit: "day" }).epochMilliseconds);`
	got := renderProgram(t, src)
	for _, want := range []string{".Round(", `"hour"`, `"minute"`, `"ceil"`, `"day"`, `"halfExpand"`} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestZonedDateTimeWithFamily pins the reshaping methods to their runtime calls: with overlays
// date and time fields through WithFields, withPlainTime replaces the time, withTimeZone re-homes
// the instant onto a new zone, and withCalendar over a literal iso8601 is the identity WithCalendar.
func TestZonedDateTimeWithFamily(t *testing.T) {
	const src = `const z = Temporal.ZonedDateTime.from("2024-06-15T12:30:45[America/New_York]");
console.log(z.with({ hour: 8 }).toString());
console.log(z.withPlainTime(new Temporal.PlainTime(9, 15)).toString());
console.log(z.withTimeZone("Asia/Tokyo").toString());
console.log(z.withCalendar("iso8601").toString());`
	got := renderProgram(t, src)
	for _, want := range []string{".WithFields(", ".WithPlainTime(", ".WithTimeZone(", ".WithCalendar("} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestZonedDateTimeDayQueries pins the day-length queries: startOfDay lowers to the runtime
// StartOfDay and the hoursInDay getter reads through the runtime HoursInDay.
func TestZonedDateTimeDayQueries(t *testing.T) {
	const src = `const z = Temporal.ZonedDateTime.from("2024-03-10T15:00:00[America/New_York]");
console.log(z.startOfDay().toString());
console.log(z.hoursInDay);`
	got := renderProgram(t, src)
	for _, want := range []string{".StartOfDay(", ".HoursInDay("} {
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

// TestZonedDateTimeFromStringConstruction pins Temporal.ZonedDateTime.from over a string
// literal to value.ZonedDateTimeFromString with the string carried through verbatim.
func TestZonedDateTimeFromStringConstruction(t *testing.T) {
	const src = `const z = Temporal.ZonedDateTime.from("2020-06-15T12:30:00[America/New_York]");
console.log(z.epochMilliseconds);`
	got := renderProgram(t, src)
	if !strings.Contains(got, `value.ZonedDateTimeFromString("2020-06-15T12:30:00[America/New_York]")`) {
		t.Errorf("rendered program missing the from-string call:\n%s", got)
	}
}

// TestZonedDateTimeFromBagConstruction pins Temporal.ZonedDateTime.from over a property bag to
// value.ZonedDateTimeFromFields, carrying the date and time fields, the time zone, an optional
// offset field, and the overflow, disambiguation, and offset options through.
func TestZonedDateTimeFromBagConstruction(t *testing.T) {
	const src = `const z = Temporal.ZonedDateTime.from({ year: 2024, month: 11, day: 3, hour: 1, minute: 30, timeZone: "America/New_York", offset: "-05:00" }, { disambiguation: "compatible", offset: "prefer" });
console.log(z.toString());`
	got := renderProgram(t, src)
	for _, want := range []string{"value.ZonedDateTimeFromFields(", `value.Some[string]("-05:00")`, `"America/New_York"`, `"prefer"`} {
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
			name: "until with a rounding option",
			src:  "const a = new Temporal.ZonedDateTime(0n, \"UTC\");\nconst b = new Temporal.ZonedDateTime(1n, \"UTC\");\nconsole.log(a.until(b, { smallestUnit: \"minute\" }).hours);",
			want: "Temporal.ZonedDateTime.prototype.until with the rounding option smallestUnit is a later slice",
		},
		{
			name: "add with a dynamic overflow option",
			src:  "function at(o: any) { return new Temporal.ZonedDateTime(0n, \"UTC\").add({ months: 1 }, { overflow: o }).epochMilliseconds; }\nconsole.log(at(\"constrain\"));",
			want: "Temporal.ZonedDateTime.prototype.add with a non-literal overflow option is a later slice",
		},
		{
			name: "with an offset field",
			src:  "const z = new Temporal.ZonedDateTime(0n, \"UTC\");\nconst j = z.with({ offset: \"+01:00\" });\nconsole.log(j.epochMilliseconds);",
			want: "Temporal.ZonedDateTime.prototype.with over a bag with the field offset is a later slice",
		},
		{
			name: "withCalendar to a calendar bento does not host",
			src:  "const z = new Temporal.ZonedDateTime(0n, \"UTC\");\nconst j = z.withCalendar(\"hebrew\");\nconsole.log(j.epochMilliseconds);",
			want: "Temporal.ZonedDateTime.prototype.withCalendar over a calendar that is dynamic or one bento does not host is a later slice",
		},
		{
			name: "getTimeZoneTransition",
			src:  "const z = new Temporal.ZonedDateTime(0n, \"UTC\");\nz.getTimeZoneTransition(\"next\");",
			want: "Temporal.ZonedDateTime.prototype.getTimeZoneTransition needs a zone transition list the host time package does not expose",
		},
		{
			name: "toLocaleString",
			src:  "const z = new Temporal.ZonedDateTime(0n, \"UTC\");\nconsole.log(z.toLocaleString());",
			want: "Temporal.ZonedDateTime.prototype.toLocaleString is a later slice",
		},
		{
			name: "from a dynamic string",
			src:  "function at(s: string) { return Temporal.ZonedDateTime.from(s).epochMilliseconds; }\nconsole.log(at(\"1970-01-01T00:00:00+00:00[UTC]\"));",
			want: "Temporal.ZonedDateTime.from over a dynamic string is a later slice",
		},
		{
			name: "from a bag naming a non-ISO calendar",
			src:  "const z = Temporal.ZonedDateTime.from({ year: 2024, month: 6, day: 15, timeZone: \"UTC\", calendar: \"gregory\" });\nconsole.log(z.epochMilliseconds);",
			want: "Temporal.ZonedDateTime.from over a bag naming the non-ISO calendar gregory is a later slice",
		},
		{
			name: "from a bag with an out-of-set offset option",
			src:  "function at(o: any) { return Temporal.ZonedDateTime.from({ year: 2024, month: 6, day: 15, timeZone: \"UTC\" }, { offset: o }).epochMilliseconds; }\nconsole.log(at(\"use\"));",
			want: "Temporal.ZonedDateTime.from with a dynamic or out-of-set offset option is a later slice",
		},
		{
			name: "from a string naming a non-ISO calendar",
			src:  "const z = Temporal.ZonedDateTime.from(\"1970-01-01T00:00:00+00:00[UTC][u-ca=gregory]\");\nconsole.log(z.epochMilliseconds);",
			want: "Temporal.ZonedDateTime.from over a string naming a non-ISO calendar is a later slice",
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

// TestNowConstruction checks each Temporal.Now function lowers to its value.Now* constructor:
// instant and timeZoneId with no argument, and the ISO functions with the default zone and with
// a named zone.
func TestNowConstruction(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"instant", "const i = Temporal.Now.instant();\nconsole.log(i.epochMilliseconds);", "value.NowInstant()"},
		{"timeZoneId", "const z = Temporal.Now.timeZoneId();\nconsole.log(z);", "value.NowTimeZoneId()"},
		{"zonedDateTimeISO default", "const z = Temporal.Now.zonedDateTimeISO();\nconsole.log(z.epochMilliseconds);", "value.NowZonedDateTimeISO()"},
		{"zonedDateTimeISO in zone", "const z = Temporal.Now.zonedDateTimeISO(\"America/New_York\");\nconsole.log(z.epochMilliseconds);", "value.NowZonedDateTimeISOIn("},
		{"plainDateTimeISO default", "const d = Temporal.Now.plainDateTimeISO();\nconsole.log(d.year);", "value.NowPlainDateTimeISO()"},
		{"plainDateTimeISO in zone", "const d = Temporal.Now.plainDateTimeISO(\"UTC\");\nconsole.log(d.year);", "value.NowPlainDateTimeISOIn("},
		{"plainDateISO default", "const d = Temporal.Now.plainDateISO();\nconsole.log(d.year);", "value.NowPlainDateISO()"},
		{"plainTimeISO default", "const t2 = Temporal.Now.plainTimeISO();\nconsole.log(t2.hour);", "value.NowPlainTimeISO()"},
		{"plainTimeISO in zone", "const t2 = Temporal.Now.plainTimeISO(\"UTC\");\nconsole.log(t2.hour);", "value.NowPlainTimeISOIn("},
		{"zonedDateTimeISO from a ZonedDateTime", "const ref = new Temporal.ZonedDateTime(0n, \"UTC\");\nconst z = Temporal.Now.zonedDateTimeISO(ref);\nconsole.log(z.epochMilliseconds);", "value.NowZonedDateTimeISOIn(ref.TimeZoneId())"},
		{"plainDateISO from a ZonedDateTime", "const ref = new Temporal.ZonedDateTime(0n, \"America/New_York\");\nconst d = Temporal.Now.plainDateISO(ref);\nconsole.log(d.year);", "value.NowPlainDateISOIn(ref.TimeZoneId())"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderProgram(t, c.src)
			if !strings.Contains(got, c.want) {
				t.Errorf("emitted Go = %q, want it to contain %q", got, c.want)
			}
		})
	}
}

// TestNowHandBacks checks the one boundary Temporal.Now does not carry: a time-zone argument
// typed as the TimeZoneLike union (ZonedDateTime | string) itself. TimeZoneLike mixes an object
// member and a string member, so the argument hands back in the shared union machinery before
// nowCall runs, which is why the reason is the generic union one rather than a Now-specific
// message. A bare string and a bare ZonedDateTime both resolve and lower, covered by
// TestNowConstruction; nowCall's own not-a-string-or-ZonedDateTime branch stays defensive since
// no type-valid TimeZoneLike reaches it.
func TestNowHandBacks(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "union TimeZoneLike argument",
			src:  "function at(tz: Temporal.TimeZoneLike) {\n  const z = Temporal.Now.zonedDateTimeISO(tz);\n  console.log(z.epochMilliseconds);\n}\nat(\"UTC\");",
			want: "union mixing object and non-object members is a later slice",
		},
		{
			name: "plainDateISO union argument",
			src:  "function at(tz: Temporal.TimeZoneLike) {\n  const d = Temporal.Now.plainDateISO(tz);\n  console.log(d.year);\n}\nat(\"UTC\");",
			want: "union mixing object and non-object members is a later slice",
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

// TestGregoryCalendarConstruction pins the gregory calendar routing: a fourth string
// calendar argument on new Temporal.PlainDate routes to value.NewPlainDateCal with the
// canonical id, and the era and eraYear reads lower to their getters.
func TestGregoryCalendarConstruction(t *testing.T) {
	const src = `const d = new Temporal.PlainDate(2024, 6, 30, "gregory");
console.log(d.era, d.eraYear, d.calendarId);`
	got := renderProgram(t, src)
	for _, want := range []string{
		`value.NewPlainDateCal(2024, 6, 30, "gregory")`,
		".Era()",
		".EraYear()",
		".CalendarId()",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestGregoryCalendarCanonicalizes pins the case-insensitive canonicalization: an uppercase
// id lowers to the canonical lowercase form the runtime hosts.
func TestGregoryCalendarCanonicalizes(t *testing.T) {
	const src = `const d = new Temporal.PlainDate(2024, 6, 30, "GREGORY");
console.log(d.calendarId);`
	got := renderProgram(t, src)
	if !strings.Contains(got, `value.NewPlainDateCal(2024, 6, 30, "gregory")`) {
		t.Errorf("uppercase GREGORY did not canonicalize:\n%s", got)
	}
}

// TestGregoryPlainDateTimeConstruction pins the tenth calendar argument on
// new Temporal.PlainDateTime routing to value.NewPlainDateTimeCal.
func TestGregoryPlainDateTimeConstruction(t *testing.T) {
	const src = `const d = new Temporal.PlainDateTime(2024, 6, 30, 12, 34, 56, 0, 0, 0, "gregory");
console.log(d.era, d.toString());`
	got := renderProgram(t, src)
	if !strings.Contains(got, `value.NewPlainDateTimeCal(2024, 6, 30, 12, 34, 56, 0, 0, 0, "gregory")`) {
		t.Errorf("PlainDateTime gregory did not route to NewPlainDateTimeCal:\n%s", got)
	}
}

// TestWithCalendarConstruction pins withCalendar routing to the value helper for a literal id.
func TestWithCalendarConstruction(t *testing.T) {
	const src = `const d = new Temporal.PlainDate(2024, 6, 30).withCalendar("gregory");
console.log(d.calendarId);`
	got := renderProgram(t, src)
	if !strings.Contains(got, `value.PlainDateWithCalendar(`) || !strings.Contains(got, `"gregory"`) {
		t.Errorf("withCalendar did not route to PlainDateWithCalendar:\n%s", got)
	}
}

// TestZonedDateTimeWithCalendar pins withCalendar on a ZonedDateTime routing to the WithCalendar
// method carrying a hosted calendar id, so a non-ISO calendar reinterprets the wall-clock fields
// rather than handing back.
func TestZonedDateTimeWithCalendar(t *testing.T) {
	const src = `const z = new Temporal.ZonedDateTime(0n, "UTC").withCalendar("roc");
console.log(z.calendarId);`
	got := renderProgram(t, src)
	if !strings.Contains(got, `.WithCalendar("roc")`) {
		t.Errorf("withCalendar did not route to WithCalendar with the roc id:\n%s", got)
	}
}

// TestCalendarHandBacks pins the conservative boundary: a three-argument PlainDate stays on
// the ISO constructor, an unhosted calendar hands back rather than mislower, and a dynamic
// calendar hands back since its value is unknown until run time.
func TestCalendarHandBacks(t *testing.T) {
	iso := renderProgram(t, `const d = new Temporal.PlainDate(2024, 6, 30);
console.log(d.day);`)
	if !strings.Contains(iso, "value.NewPlainDate(2024, 6, 30)") || strings.Contains(iso, "NewPlainDateCal") {
		t.Errorf("three-argument PlainDate did not stay on NewPlainDate:\n%s", iso)
	}

	unhosted := renderProgramHandBack(t, `const d = new Temporal.PlainDate(2024, 6, 30, "hebrew");
console.log(d.calendarId);`)
	if !strings.Contains(unhosted, "calendar") {
		t.Errorf("unhosted calendar handback reason = %q, want a calendar reason", unhosted)
	}

	dyn := renderProgramHandBack(t, `function f(c: string): string {
	return new Temporal.PlainDate(2024, 6, 30, c).calendarId;
}
console.log(f("gregory"));`)
	if !strings.Contains(dyn, "calendar") {
		t.Errorf("dynamic calendar handback reason = %q, want a calendar reason", dyn)
	}
}

// TestRocCalendarConstruction pins the roc calendar routing through the same seam gregory
// takes, on the PlainDate constructor, the PlainDateTime constructor, and withCalendar.
func TestRocCalendarConstruction(t *testing.T) {
	got := renderProgram(t, `const d = new Temporal.PlainDate(2024, 6, 30, "roc");
const dt = new Temporal.PlainDateTime(2024, 6, 30, 12, 34, 56, 0, 0, 0, "roc");
const w = new Temporal.PlainDate(2024, 6, 30).withCalendar("ROC");
console.log(d.year, dt.era, w.calendarId);`)
	for _, want := range []string{
		`value.NewPlainDateCal(2024, 6, 30, "roc")`,
		`value.NewPlainDateTimeCal(2024, 6, 30, 12, 34, 56, 0, 0, 0, "roc")`,
		`value.PlainDateWithCalendar(`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
	if !strings.Contains(got, `"roc"`) {
		t.Errorf("uppercase ROC did not canonicalize to roc:\n%s", got)
	}
}

// TestJapaneseCalendarConstruction pins the japanese calendar routing through the same seam
// gregory and roc take, on the PlainDate constructor, the PlainDateTime constructor, and
// withCalendar.
func TestJapaneseCalendarConstruction(t *testing.T) {
	got := renderProgram(t, `const d = new Temporal.PlainDate(2024, 6, 30, "japanese");
const dt = new Temporal.PlainDateTime(2024, 6, 30, 12, 34, 56, 0, 0, 0, "japanese");
const w = new Temporal.PlainDate(2024, 6, 30).withCalendar("JAPANESE");
console.log(d.era, dt.eraYear, w.calendarId);`)
	for _, want := range []string{
		`value.NewPlainDateCal(2024, 6, 30, "japanese")`,
		`value.NewPlainDateTimeCal(2024, 6, 30, 12, 34, 56, 0, 0, 0, "japanese")`,
		`value.PlainDateWithCalendar(`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
	if !strings.Contains(got, `"japanese"`) {
		t.Errorf("uppercase JAPANESE did not canonicalize to japanese:\n%s", got)
	}
}

// TestPlainDateFromStringConstruction pins Temporal.PlainDate.from over a literal ISO
// string routing to value.PlainDateFromString, including a hosted-calendar annotation.
func TestPlainDateFromStringConstruction(t *testing.T) {
	got := renderProgram(t, `const a = Temporal.PlainDate.from("2024-06-30");
const b = Temporal.PlainDate.from("2024-06-30[u-ca=gregory]");
console.log(a.toString(), b.calendarId);`)
	for _, want := range []string{
		`value.PlainDateFromString("2024-06-30")`,
		`value.PlainDateFromString("2024-06-30[u-ca=gregory]")`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestPlainDateFromStringHandsBack pins the two from-string cases the slice hands back: a
// dynamic string, whose calendar cannot be checked at compile time, and a literal naming a
// calendar bento does not host, which the runtime parser would wrongly reject.
func TestPlainDateFromStringHandsBack(t *testing.T) {
	dyn := renderProgramHandBack(t, `function f(s: string) { return Temporal.PlainDate.from(s); }`)
	if !strings.Contains(dyn, "dynamic string") {
		t.Errorf("dynamic-string from did not hand back with the expected reason: %q", dyn)
	}
	unhosted := renderProgramHandBack(t, `const d = Temporal.PlainDate.from("2024-06-30[u-ca=hebrew]");`)
	if !strings.Contains(unhosted, "does not host") {
		t.Errorf("unhosted-calendar from did not hand back with the expected reason: %q", unhosted)
	}
}

// TestPlainTimeFromStringConstruction pins Temporal.PlainTime.from over a literal string
// routing to value.PlainTimeFromString, both a time-only and a full date-time literal. A
// PlainTime carries no calendar, so any calendar annotation lowers without a gate.
func TestPlainTimeFromStringConstruction(t *testing.T) {
	got := renderProgram(t, `const a = Temporal.PlainTime.from("12:30:00");
const b = Temporal.PlainTime.from("2024-06-30T12:30:00[u-ca=gregory]");
console.log(a.toString(), b.minute);`)
	for _, want := range []string{
		`value.PlainTimeFromString("12:30:00")`,
		`value.PlainTimeFromString("2024-06-30T12:30:00[u-ca=gregory]")`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered program missing %q:\n%s", want, got)
		}
	}
}

// TestPlainTimeFromDynamicStringConstruction pins Temporal.PlainTime.from over a string value
// not known until run time: the argument lowers and reaches value.PlainTimeFromString through
// its Go string. A PlainTime carries no calendar, so a dynamic string is always safe.
func TestPlainTimeFromDynamicStringConstruction(t *testing.T) {
	got := renderProgram(t, `function at(s: string) { return Temporal.PlainTime.from(s).minute; }
console.log(at("12:30:00"));`)
	if !strings.Contains(got, `value.PlainTimeFromString(s.ToGoString())`) {
		t.Errorf("rendered program missing the dynamic from-string call:\n%s", got)
	}
}
