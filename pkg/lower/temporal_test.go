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
			name: "PlainDateTime construction",
			src:  "function makeDateTime(): void { new Temporal.PlainDateTime(2020, 2, 29); }",
			want: "new Temporal.PlainDateTime is a later slice",
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
