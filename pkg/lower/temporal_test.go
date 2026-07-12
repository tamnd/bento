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
			name: "PlainTime construction",
			src:  "function makeTime(): void { new Temporal.PlainTime(12, 30); }",
			want: "new Temporal.PlainTime is a later slice",
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
