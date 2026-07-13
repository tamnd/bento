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

// TestPlainDateHandBacks pins the honest ceilings: the union getters, the arithmetic
// and conversion methods, from over a dynamic string or a property bag, and the other
// Temporal types each hand back with a reason naming where the work belongs.
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
			name: "with a monthCode field",
			src:  "const d = new Temporal.PlainDate(2020, 1, 31);\nconsole.log(d.with({ monthCode: \"M02\" }).day);",
			want: "Temporal.PlainDate.prototype.with over a bag with the field monthCode is a later slice",
		},
		{
			name: "from a property bag",
			src:  "const d = Temporal.PlainDate.from({ year: 2020, month: 2, day: 29 });\nconsole.log(d.day);",
			want: "Temporal.PlainDate.from over a dynamic string or a property bag is a later slice",
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
			name: "from a property bag",
			src:  "const d = Temporal.Duration.from({ years: 1 });\nconsole.log(d.years);",
			want: "Temporal.Duration.from over a property bag or a value not statically typed as a string is a later slice",
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
			src:  "function go(u: \"minute\" | \"hour\") {\n  const t = new Temporal.PlainTime(12, 30, 45);\n  return t.round({ smallestUnit: u }).minute;\n}\nconsole.log(go(\"minute\"));",
			want: "Temporal.PlainTime.prototype.round with a non-literal smallestUnit is a later slice",
		},
		{
			name: "round with a dynamic roundingMode",
			src:  "function go(m: \"ceil\" | \"floor\") {\n  const t = new Temporal.PlainTime(12, 30, 45);\n  return t.round({ smallestUnit: \"minute\", roundingMode: m }).minute;\n}\nconsole.log(go(\"ceil\"));",
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
			name: "from a property bag",
			src:  "const dt = Temporal.PlainDateTime.from({ year: 2020, month: 1, day: 1, hour: 12 });\nconsole.log(dt.hour);",
			want: "Temporal.PlainDateTime.from over a dynamic string or a property bag is a later slice",
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
			name: "from a property bag",
			src:  "const ym = Temporal.PlainYearMonth.from({ year: 2020, month: 3 });\nconsole.log(ym.month);",
			want: "Temporal.PlainYearMonth.from over a dynamic string or a property bag is a later slice",
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
			name: "from a property bag",
			src:  "const md = Temporal.PlainMonthDay.from({ month: 3, day: 15 });\nconsole.log(md.day);",
			want: "Temporal.PlainMonthDay.from over a dynamic string or a property bag is a later slice",
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

// TestInstantHandBacks pins the honest ceilings: the arithmetic and rounding methods each
// hand back with a reason naming where the work belongs.
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
			name: "from a dynamic string",
			src:  "function at(s: string) { return Temporal.ZonedDateTime.from(s).epochMilliseconds; }\nconsole.log(at(\"1970-01-01T00:00:00+00:00[UTC]\"));",
			want: "Temporal.ZonedDateTime.from over a dynamic string or a property bag is a later slice",
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

// TestNowHandBacks checks the boundaries Temporal.Now does not carry: a non-string time-zone
// argument (a ZonedDateTime is a valid TimeZoneLike the checker accepts but this slice does not
// coerce), and a superfluous argument to a no-argument function.
func TestNowHandBacks(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "zoned time-zone argument",
			src:  "const ref = new Temporal.ZonedDateTime(0n, \"UTC\");\nconst z = Temporal.Now.zonedDateTimeISO(ref);\nconsole.log(z.epochMilliseconds);",
			want: "Temporal.Now.zonedDateTimeISO over a non-string time-zone argument is a later slice",
		},
		{
			name: "plainDateISO zoned argument",
			src:  "const ref = new Temporal.ZonedDateTime(0n, \"UTC\");\nconst d = Temporal.Now.plainDateISO(ref);\nconsole.log(d.year);",
			want: "Temporal.Now.plainDateISO over a non-string time-zone argument is a later slice",
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
