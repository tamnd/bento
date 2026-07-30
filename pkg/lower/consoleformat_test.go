package lower

import (
	"strings"
	"testing"
)

// console.log used to render each argument on its own and join them with a space,
// which prints "%s x" for console.log("%s", "x"). These pin the routing rule: which
// shapes go through value.ConsoleFormat, which keep the per-argument join, and what
// happens when a call that must be formatted has an argument that does not box.

// TestAFormatStringRoutesThroughConsoleFormat is the shape the whole path exists for.
func TestAFormatStringRoutesThroughConsoleFormat(t *testing.T) {
	got := renderProgram(t, `console.log("%s is %d", "x", 2);`)
	want := `value.ConsoleLog(value.ConsoleFormat(value.StringValue(value.FromGoString("%s is %d")), value.StringValue(value.FromGoString("x")), value.Number(2)))`
	if !strings.Contains(got, want) {
		t.Errorf("did not emit %s:\n%s", want, got)
	}
}

// TestConsoleErrorFormatsToo pins that the routing is per-console-method rather than
// bolted onto log: warn and error write to stderr through the same formatting.
func TestConsoleErrorFormatsToo(t *testing.T) {
	for _, method := range []string{"error", "warn"} {
		got := renderProgram(t, `console.`+method+`("%d%%", 50);`)
		if !strings.Contains(got, "value.ConsoleError(value.ConsoleFormat(") {
			t.Errorf("console.%s did not route through ConsoleFormat:\n%s", method, got)
		}
	}
}

// TestARuntimeStringFirstArgumentFormats covers the shape that forces the decision to
// be made on the type rather than on the text: the first argument is a string whose
// contents the compiler cannot see, so it may hold a specifier and must be treated as
// if it does.
func TestARuntimeStringFirstArgumentFormats(t *testing.T) {
	got := renderProgram(t, `const prefix: string = process.argv[2];
console.log(prefix, 41);`)
	if !strings.Contains(got, "value.ConsoleLog(value.ConsoleFormat(") {
		t.Errorf("a runtime string first argument did not format:\n%s", got)
	}
}

// TestADynamicFirstArgumentFormats is the same case one type wider: an any-typed
// first argument may hold a string at run time, and ConsoleFormat is what decides
// that, at run time, correctly.
func TestADynamicFirstArgumentFormats(t *testing.T) {
	got := renderProgram(t, `function f(a: any, b: any): void { console.log(a, b); }
f("%s", 1);`)
	if !strings.Contains(got, "value.ConsoleLog(value.ConsoleFormat(") {
		t.Errorf("a dynamic first argument did not format:\n%s", got)
	}
}

// TestOrdinaryConsoleCallsKeepThePerArgumentPath pins the deliberate limit on the
// rule. Each of these prints the same line either way, and leaving them alone keeps
// the emitted Go of every existing program unchanged.
func TestOrdinaryConsoleCallsKeepThePerArgumentPath(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"a literal with no specifier", `console.log("count", 2);`},
		{"a single argument", `console.log("%s only");`},
		{"a non-string first argument", `console.log(1, 2);`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderProgram(t, tc.src)
			if strings.Contains(got, "value.ConsoleFormat(") {
				t.Errorf("routed through ConsoleFormat, want the per-argument path:\n%s", got)
			}
		})
	}
}

// TestAFormatStringWithAnUnboxableArgumentHandsBack is the rule's failure mode, and it
// fails loudly on purpose. The call must be formatted to print the right thing, an
// argument cannot be boxed to format it, so the build stops rather than emitting a
// line with a bare %s in it.
// The unboxable argument is a union parameter, which the per-argument path renders
// arm by arm through the union's own ToString and the boxing path hands back on, so
// the two halves of this decision differ only in the first argument.
func TestAFormatStringWithAnUnboxableArgumentHandsBack(t *testing.T) {
	prog := compileTolerant(t, `function show(u: string | number): void { console.log("%s", u); }
show(1);
show("x");`)
	r := NewRenderer(prog)
	_, err := r.RenderProgram(entryFile(t, prog))
	if err == nil {
		t.Fatal("lowered, want a hand back")
	}
	want := "console.log with a format string and an argument that does not box yet"
	if !strings.Contains(err.Error(), want) {
		t.Errorf("hand back said %q, want it to mention %q", err.Error(), want)
	}
}

// TestARuntimeStringWithAnUnboxableArgumentFallsBack is the other half of that
// decision. Here the first argument may not hold a specifier at all, so refusing the
// build would reject a program bento used to compile; it falls back to the join it
// printed before: the same union argument the test above refuses is printed here
// through its own ToString, one argument at a time.
func TestARuntimeStringWithAnUnboxableArgumentFallsBack(t *testing.T) {
	got := renderProgram(t, `function show(prefix: string, u: string | number): void { console.log(prefix, u); }
show("p", 1);
show("p", "x");`)
	if strings.Contains(got, "value.ConsoleFormat(") {
		t.Errorf("formatted an unboxable argument, want the per-argument fallback:\n%s", got)
	}
	if !strings.Contains(got, "value.ConsoleLog(prefix, u.ToString())") {
		t.Errorf("did not emit the per-argument join:\n%s", got)
	}
}
