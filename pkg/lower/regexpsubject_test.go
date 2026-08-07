package lower

import (
	"strings"
	"testing"
)

// exec and test begin with ToString(string) in the specification, so the subject is
// coerced rather than required. These pin that the coercion happens and that it is the
// same one String(x) and a template substitution run, since a subject the checker does
// not call a string still has to reach the match with the bytes the engine would build.

// TestARegExpSubjectThatIsAlreadyAStringPassesThrough is the shape that has always
// worked, kept here so growing the coercion cannot quietly wrap it.
func TestARegExpSubjectThatIsAlreadyAStringPassesThrough(t *testing.T) {
	got := renderProgram(t, `const s = "abc";
console.log(/b/.test(s));`)
	if !strings.Contains(got, ".Test(s)") {
		t.Errorf("a string subject did not pass through:\n%s", got)
	}
}

// TestADynamicRegExpSubjectCoerces is the shape the Node compat suite is written in. A
// CommonJS require('fs') gives readFileSync an any return, so the subject arrives as a
// box, and requiring a string here was where a third of the suite stopped.
func TestADynamicRegExpSubjectCoerces(t *testing.T) {
	for _, method := range []string{"exec", "test"} {
		t.Run(method, func(t *testing.T) {
			got := renderProgram(t, `const d: any = "abc";
console.log(/b/.`+method+`(d));`)
			if !strings.Contains(got, "value.ToString(d)") {
				t.Errorf("a dynamic subject did not coerce:\n%s", got)
			}
		})
	}
}

// TestABoxedRegExpSubjectCoercesAtRunTime covers the subjects that are boxes without
// being written as any: a parse result and an element of a mixed array. What each holds
// is decided at run time, so the coercion is the value model's, which is exactly what
// makes re.test(12) match against "12" rather than refuse.
func TestABoxedRegExpSubjectCoercesAtRunTime(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"a parse result", `const p = JSON.parse("\"ab\"");
console.log(/b/.test(p));`},
		{"a mixed array element", `const xs: any[] = [12, "ab"];
console.log(/1/.test(xs[0]));`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderProgram(t, tc.src)
			if !strings.Contains(got, "value.ToString(") {
				t.Errorf("a boxed subject did not coerce:\n%s", got)
			}
		})
	}
}

// TestARegExpSubjectWithNoCoercionHandsBack keeps the limit honest. The requirement
// moved rather than disappeared: a subject with no coercion to run is still refused, and
// refused for what is actually missing rather than for not being a string.
func TestARegExpSubjectWithNoCoercionHandsBack(t *testing.T) {
	reason := renderProgramHandBack(t, `const fns = [() => 1];
console.log(/1/.test(fns as any));
`)
	if !strings.Contains(reason, "later slice") {
		t.Errorf("hand back said %q, want a later-slice reason", reason)
	}
	if strings.Contains(reason, "non-string") {
		t.Errorf("hand back still blamed the subject for not being a string: %q", reason)
	}
}
