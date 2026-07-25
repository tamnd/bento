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
		{`const d = new Date("2023-01-01"); console.log(d.getTime());`, "new Date from a string"},
		{`const d = new Date(2023, 0, 1); console.log(d.getTime());`, "year, month, and day components"},
		{`const d = new Date(0); console.log(d.getFullYear());`, "the Date method .getFullYear"},
		{`console.log(Date.parse("2023-01-01"));`, "Date.parse is a later slice"},
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
