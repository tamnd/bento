package lower

import (
	"strings"
	"testing"
)

// TestDateHoldsATimeValue is the ordinary JavaScript spelling of the built-in: construct
// from a time value, read it back through both reads that report it, and serialize.
func TestDateHoldsATimeValue(t *testing.T) {
	got := goRunSource(t, renderExpandoJS(t, `const d = new Date(1700000000000);
console.log(d.getTime(), d.valueOf());
console.log(d.toISOString());
console.log(new Date(0).toISOString());
`))
	want := "1700000000000 1700000000000\n2023-11-14T22:13:20.000Z\n1970-01-01T00:00:00.000Z\n"
	if got != want {
		t.Errorf("date time value\n got: %q\nwant: %q", got, want)
	}
}

// TestDateNowIsATimeValue pins the static: Date.now() gives a Number, not a Date, and it
// reads the same clock the bare constructor does.
func TestDateNowIsATimeValue(t *testing.T) {
	got := goRunSource(t, renderExpandoJS(t, `const t = Date.now();
console.log(t > 1700000000000);
const d = new Date();
console.log(d.getTime() >= t);
`))
	want := "true\ntrue\n"
	if got != want {
		t.Errorf("Date.now\n got: %q\nwant: %q", got, want)
	}
}

// TestDateCrossesABinding pins that a Date survives being named and handed around, which
// is what the type rendering is for: without it the binding's type would try to intern
// the date's getters as struct fields.
func TestDateCrossesABinding(t *testing.T) {
	got := goRunSource(t, renderExpandoJS(t, `/**
 * @param {Date} d
 * @returns {string}
 */
function stamp(d) {
  return d.toISOString();
}
const d = new Date(86400000);
const same = d;
console.log(stamp(same));
console.log(same.getTime() - d.getTime());
`))
	want := "1970-01-02T00:00:00.000Z\n0\n"
	if got != want {
		t.Errorf("date across a binding\n got: %q\nwant: %q", got, want)
	}
}

// TestDateBeforeTheEpoch pins the negative time value end to end, the case a truncating
// day split would report as the wrong day.
func TestDateBeforeTheEpoch(t *testing.T) {
	got := goRunSource(t, renderExpandoJS(t, `console.log(new Date(-1).toISOString());
console.log(new Date(-2208988800000).toISOString());
`))
	want := "1969-12-31T23:59:59.999Z\n1900-01-01T00:00:00.000Z\n"
	if got != want {
		t.Errorf("date before the epoch\n got: %q\nwant: %q", got, want)
	}
}

// TestInvalidDateThrowsOnISOString pins the Invalid Date a program observes: an
// out-of-range time value constructs, and serializing it throws a RangeError rather than
// printing something that reads like an instant.
func TestInvalidDateThrowsOnISOString(t *testing.T) {
	got := goRunSource(t, renderExpandoJS(t, `const d = new Date(8.64e15 + 1);
console.log(Number.isNaN(d.getTime()));
try {
  d.toISOString();
  console.log("no throw");
} catch (e) {
  if (e instanceof RangeError) {
    console.log("RangeError");
  }
}
`))
	want := "true\nRangeError\n"
	if got != want {
		t.Errorf("invalid date\n got: %q\nwant: %q", got, want)
	}
}

// TestDateLowersToTheRuntimeType pins the emitted Go: the constructors and the static
// reach value.Date directly rather than through a boxed property bag.
func TestDateLowersToTheRuntimeType(t *testing.T) {
	src := renderExpandoJS(t, `const a = new Date();
const b = new Date(1);
console.log(a.getTime(), b.getTime(), Date.now());
`)
	for _, want := range []string{"value.NewDate()", "value.NewDateFromMillis(", "value.DateNow()"} {
		if !strings.Contains(src, want) {
			t.Errorf("emitted Go does not contain %q:\n%s", want, src)
		}
	}
}

// TestUncoveredDateFormsHandBack pins that the spellings this slice does not lower say so
// instead of guessing. A string has to be parsed and the component form has to build a
// date from a calendar, each its own slice; the same holds for the getters that read
// components off the time value.
func TestUncoveredDateFormsHandBack(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"const v = Date.now() > 0 ? 1 : \"2023-01-01\";\nconst d = new Date(v);\nconsole.log(d.getTime());", "needs coercion"},
		{`const d = new Date(0); console.log(d.toLocaleDateString());`, "the Date method .toLocaleDateString"},
		{`const d = new Date(0); console.log(d.toLocaleString());`, "the Date method .toLocaleString"},
	} {
		prog := compileJS(t, c.src)
		r := NewRenderer(prog)
		r.SetGoSignatures(testGoSignatures())
		_, err := r.RenderProgram(entryFile(t, prog))
		if err == nil {
			t.Errorf("%s lowered, want a hand-back", c.src)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s\n got: %v\nwant a reason containing %q", c.src, err, c.want)
		}
	}
}

// TestDateGettersReadTheComponents is the ordinary JavaScript spelling of the calendar
// reads. Only the UTC getters are pinned exactly: the local ones depend on the zone the
// compiled program runs in, so they are checked for agreement instead.
func TestDateGettersReadTheComponents(t *testing.T) {
	got := goRunSource(t, renderExpandoJS(t, `const d = new Date(1700000000123);
console.log(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate(), d.getUTCDay());
console.log(d.getUTCHours(), d.getUTCMinutes(), d.getUTCSeconds(), d.getUTCMilliseconds());
console.log(new Date(-1).getUTCFullYear(), new Date(-1).getUTCMonth(), new Date(-1).getUTCDate());
`))
	want := "2023 10 14 2\n22 13 20 123\n1969 11 31\n"
	if got != want {
		t.Errorf("date getters\n got: %q\nwant: %q", got, want)
	}
}

// TestLocalGettersAgreeWithTheOffset pins the local reads without pinning a zone: the
// local wall clock, shifted back by the offset the date reports, has to land on the UTC
// wall clock. That holds in every zone, so the assertion is stable wherever the compiled
// program runs.
func TestLocalGettersAgreeWithTheOffset(t *testing.T) {
	got := goRunSource(t, renderExpandoJS(t, `const d = new Date(1700000000123);
const shifted = new Date(d.getTime() - d.getTimezoneOffset() * 60000);
console.log(d.getHours() === shifted.getUTCHours(), d.getDate() === shifted.getUTCDate());
console.log(d.getMilliseconds() === d.getUTCMilliseconds());
console.log(d.getFullYear() >= 2023);
`))
	want := "true true\ntrue\ntrue\n"
	if got != want {
		t.Errorf("local getters\n got: %q\nwant: %q", got, want)
	}
}

// TestInvalidDateGettersAreNaNInJS pins that a program reading the Invalid Date sees NaN
// from every getter rather than a number that reads like an instant.
func TestInvalidDateGettersAreNaNInJS(t *testing.T) {
	got := goRunSource(t, renderExpandoJS(t, `const d = new Date(NaN);
console.log(Number.isNaN(d.getUTCFullYear()), Number.isNaN(d.getMonth()), Number.isNaN(d.getTimezoneOffset()));
`))
	want := "true true true\n"
	if got != want {
		t.Errorf("invalid date getters\n got: %q\nwant: %q", got, want)
	}
}

// TestDateSettersMutateInPlace pins that a setter moves the date it was called on and
// gives the new time value back, so it reads as both a statement and an expression.
func TestDateSettersMutateInPlace(t *testing.T) {
	got := goRunSource(t, renderExpandoJS(t, `const d = new Date(0);
console.log(d.setUTCFullYear(2000));
console.log(d.toISOString());
`))
	want := "946684800000\n2000-01-01T00:00:00.000Z\n"
	if got != want {
		t.Errorf("setter in place\n got: %q\nwant: %q", got, want)
	}
}

// TestDateFromAString is the ordinary JavaScript spelling of parsing: the constructor and
// the static both read a string, and both agree with the serialization it came from.
func TestDateFromAString(t *testing.T) {
	got := goRunSource(t, renderExpandoJS(t, `const d = new Date("2023-11-14T22:13:20.123Z");
console.log(d.getTime(), d.toISOString());
console.log(Date.parse("2023-11-14T22:13:20Z"));
console.log(new Date("Tue, 14 Nov 2023 22:13:20 GMT").getTime());
console.log(new Date(new Date(5)).getTime());
`))
	want := "1700000000123 2023-11-14T22:13:20.123Z\n1700000000000\n1700000000000\n5\n"
	if got != want {
		t.Errorf("date from a string\n got: %q\nwant: %q", got, want)
	}
}

// TestUnparsableStringIsTheInvalidDate pins that a bad date constructs rather than
// throwing, so a program checks for it with isNaN the way it would in Node.
func TestUnparsableStringIsTheInvalidDate(t *testing.T) {
	got := goRunSource(t, renderExpandoJS(t, `const d = new Date("nonsense");
console.log(Number.isNaN(d.getTime()), Number.isNaN(Date.parse("nope")));
`))
	want := "true true\n"
	if got != want {
		t.Errorf("unparsable string\n got: %q\nwant: %q", got, want)
	}
}

// TestDateOnlyStringIsUTC pins the rule a program is most likely to get wrong on its own:
// a date-only string is UTC, so its UTC components read back exactly as written no matter
// what zone the compiled program runs in.
func TestDateOnlyStringIsUTC(t *testing.T) {
	got := goRunSource(t, renderExpandoJS(t, `const d = new Date("2023-11-14");
console.log(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate(), d.getUTCHours());
console.log(d.toISOString());
`))
	want := "2023 10 14 0\n2023-11-14T00:00:00.000Z\n"
	if got != want {
		t.Errorf("date-only string\n got: %q\nwant: %q", got, want)
	}
}

// TestDateParseLowersToTheRuntime pins the emitted Go: the string paths reach the runtime
// parser rather than going through any boxed value.
func TestDateParseLowersToTheRuntime(t *testing.T) {
	src := renderExpandoJS(t, `const d = new Date("2023-01-01");
console.log(d.getTime(), Date.parse("2023-01-01"));
`)
	for _, want := range []string{"value.NewDateFromString(", "value.ParseDate("} {
		if !strings.Contains(src, want) {
			t.Errorf("emitted Go does not contain %q:\n%s", want, src)
		}
	}
}

// TestDateFromComponents is the ordinary JavaScript spelling of the calendar
// constructor and of Date.UTC. Only the UTC side is pinned exactly, since the component
// constructor reads local time.
func TestDateFromComponents(t *testing.T) {
	got := goRunSource(t, renderExpandoJS(t, `console.log(Date.UTC(2023, 10, 14, 22, 13, 20, 123));
console.log(new Date(Date.UTC(2023, 10, 14)).toISOString());
console.log(new Date(Date.UTC(2023, 10)).toISOString());
console.log(new Date(Date.UTC(99, 0, 1)).getUTCFullYear());
`))
	want := "1700000000123\n2023-11-14T00:00:00.000Z\n2023-11-01T00:00:00.000Z\n1999\n"
	if got != want {
		t.Errorf("date from components\n got: %q\nwant: %q", got, want)
	}
}

// TestComponentsOverflowInJS pins the carrying rule a program relies on, which is the
// whole reason the constructor accepts out-of-range values.
func TestComponentsOverflowInJS(t *testing.T) {
	got := goRunSource(t, renderExpandoJS(t, `console.log(new Date(Date.UTC(2023, 12, 1)).toISOString());
console.log(new Date(Date.UTC(2023, 10, 0)).toISOString());
console.log(new Date(Date.UTC(2023, 0, 1, 25)).toISOString());
`))
	want := "2024-01-01T00:00:00.000Z\n2023-10-31T00:00:00.000Z\n2023-01-02T01:00:00.000Z\n"
	if got != want {
		t.Errorf("component overflow\n got: %q\nwant: %q", got, want)
	}
}

// TestSettersInJS pins the mutation surface a program actually writes: replace a field,
// and add to one to move the date.
func TestSettersInJS(t *testing.T) {
	got := goRunSource(t, renderExpandoJS(t, `const d = new Date(Date.UTC(2023, 0, 20, 12, 0, 0, 0));
d.setUTCHours(9, 30);
console.log(d.toISOString());
d.setUTCDate(d.getUTCDate() + 45);
console.log(d.toISOString());
console.log(d.setTime(0), d.toISOString());
`))
	want := "2023-01-20T09:30:00.000Z\n2023-03-06T09:30:00.000Z\n0 1970-01-01T00:00:00.000Z\n"
	if got != want {
		t.Errorf("setters\n got: %q\nwant: %q", got, want)
	}
}

// TestComponentConstructorIsLocalInJS pins that the calendar constructor reads local
// time, without pinning a zone: what it built has to read back as what was written.
func TestComponentConstructorIsLocalInJS(t *testing.T) {
	got := goRunSource(t, renderExpandoJS(t, `const d = new Date(2023, 10, 14, 9, 30, 0, 0);
console.log(d.getFullYear(), d.getMonth(), d.getDate(), d.getHours(), d.getMinutes());
console.log(d.getTime() === new Date(2023, 10, 14, 9, 30, 0, 0).getTime());
`))
	want := "2023 10 14 9 30\ntrue\n"
	if got != want {
		t.Errorf("local component constructor\n got: %q\nwant: %q", got, want)
	}
}

// TestComponentsLowerToTheRuntime pins the emitted Go for the two construction paths and
// for a setter.
func TestComponentsLowerToTheRuntime(t *testing.T) {
	src := renderExpandoJS(t, `const d = new Date(2023, 0, 1);
d.setUTCMonth(5);
console.log(d.getTime(), Date.UTC(2023, 0, 1));
`)
	for _, want := range []string{"value.NewDateFromComponents(", "value.DateUTC(", ".SetUTCMonth("} {
		if !strings.Contains(src, want) {
			t.Errorf("emitted Go does not contain %q:\n%s", want, src)
		}
	}
}

// TestDateFormatsLowerToTheRuntime pins the emission for the five formats this slice
// brings. The four that answer a string are straight method calls on the *value.Date, so
// they read the instant with no boxing; toJSON is the one that is not, since it answers
// null for an invalid date and so goes through the function form that gives a Value.
func TestDateFormatsLowerToTheRuntime(t *testing.T) {
	src := renderExpandoJS(t, `const d = new Date(0);
console.log(d.toString(), d.toDateString(), d.toTimeString(), d.toUTCString());
console.log(d.toJSON());
console.log(String(d), `+"`${d}`"+`);
`)
	for _, want := range []string{".ToString()", ".ToDateString()", ".ToTimeString()", ".ToUTCString()", "value.DateToJSON("} {
		if !strings.Contains(src, want) {
			t.Errorf("emitted Go does not contain %q:\n%s", want, src)
		}
	}
}

// TestDateFormatsAgreeWithEachOther is the run rather than the emission. It cannot pin
// the local reading, which depends on the zone the compiled program runs in, so it pins
// the relationships that hold in every zone: the two coercions reach toString, the two
// halves of the reading compose back into it, and the UTC format does not move.
func TestDateFormatsAgreeWithEachOther(t *testing.T) {
	got := goRunSource(t, renderExpandoJS(t, `const d = new Date(1673751845006);
console.log(String(d) === d.toString(), `+"`${d}`"+` === d.toString());
console.log(d.toDateString() + " " + d.toTimeString() === d.toString());
console.log(d.toUTCString());
console.log(d.toISOString());
const bad = new Date(NaN);
console.log(bad.toString(), bad.toDateString(), bad.toTimeString(), bad.toUTCString());
`))
	want := "true true\ntrue\nSun, 15 Jan 2023 03:04:05 GMT\n2023-01-15T03:04:05.006Z\n" +
		"Invalid Date Invalid Date Invalid Date Invalid Date\n"
	if got != want {
		t.Errorf("date formats\n got: %q\nwant: %q", got, want)
	}
}

// TestDateToJSONStaysDynamic pins the reason toJSON is lowered the way it is. It answers
// null for a date with no representable instant, which no string can hold, so the call
// keeps its box: it prints as null and hands the build back where a program binds it into
// a string slot rather than shipping the empty string.
func TestDateToJSONStaysDynamic(t *testing.T) {
	got := goRunSource(t, renderExpandoJS(t, `const d = new Date(0);
console.log(d.toJSON());
console.log(new Date(NaN).toJSON());
`))
	want := "1970-01-01T00:00:00.000Z\nnull\n"
	if got != want {
		t.Errorf("toJSON\n got: %q\nwant: %q", got, want)
	}

	prog := compile(t, `const d = new Date(0);
const s: string = d.toJSON();
console.log(s);
`)
	r := NewRenderer(prog)
	r.SetGoSignatures(testGoSignatures())
	if _, err := r.RenderProgram(entryFile(t, prog)); err == nil {
		t.Fatal("toJSON bound into a string slot lowered, want a hand-back")
	}
}

// TestDateCoercionsSplitByHint pins the split that makes a date unlike every other
// object. ToPrimitive takes the string hint by default on a date and the number hint
// only when an operator asks for one, so + concatenates the local reading while -, *
// and the relationals read the time value. Both sides read off the concrete
// *value.Date, so neither coercion needs a box.
func TestDateCoercionsSplitByHint(t *testing.T) {
	src := renderUncheckedJS(t, `const a = new Date(1000);
const b = new Date(2000);
console.log("" + a);
console.log(b - a, a < b, a * 2, +b);
`)
	for _, want := range []string{".ToString()", ".ValueOf()"} {
		if !strings.Contains(src, want) {
			t.Errorf("emitted Go does not contain %q:\n%s", want, src)
		}
	}
	got := goRunSource(t, src)
	if !strings.HasSuffix(got, "1000 true 2000 2000\n") {
		t.Errorf("date coercions\n got: %q", got)
	}
}

// TestDateArithmeticMatchesTheEngine runs the coercions rather than reading them. Every
// line here is zone independent, since a time value is the same number everywhere, which
// is what lets the expected output be written down.
func TestDateArithmeticMatchesTheEngine(t *testing.T) {
	got := goRunSource(t, renderUncheckedJS(t, `const a = new Date(1000);
const b = new Date(2000);
console.log(b - a, a - b, b / a, b % a, a <= b, b > a, a >= b);
console.log(+a, -a, a * 2, a - "500", a - true);
const bad = new Date(NaN);
console.log(+bad, bad - a, bad < a, bad > a);
`))
	want := "1000 -1000 2 0 true true false\n" +
		"1000 -1000 2000 500 999\n" +
		"NaN NaN false false\n"
	if got != want {
		t.Errorf("date arithmetic\n got: %q\nwant: %q", got, want)
	}
}

// TestDatePlusIsAConcatenation pins the other half of the split: + on a date is never
// addition, so a date and a number join as text. The checker has no type for that
// operator, so the concatenation boxes on its way out, which is what lets it print.
func TestDatePlusIsAConcatenation(t *testing.T) {
	got := goRunSource(t, renderUncheckedJS(t, `const d = new Date(0);
console.log((d + 1) === (d.toString() + "1"));
console.log(("x" + d) === ("x" + d.toString()));
console.log((d + d) === (d.toString() + d.toString()));
`))
	want := "true\ntrue\ntrue\n"
	if got != want {
		t.Errorf("date concatenation\n got: %q\nwant: %q", got, want)
	}
}
