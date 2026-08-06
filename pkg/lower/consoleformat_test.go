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

// TestAFormatStringWithAUnionArgumentFormats covers the argument that used to be the
// rule's failure mode. A union parameter had no dynamic form, so a call that had to be
// formatted to print the right thing stopped the build rather than emit a line with a
// bare %s in it. It boxes through the union's own ToValue now, so the call formats.
func TestAFormatStringWithAUnionArgumentFormats(t *testing.T) {
	skipIfShort(t)
	const src = `function show(u: string | number): void { console.log("%s", u); }
show(1);
show("x");
`
	if got := runProgramGo(t, src); got != "1\nx\n" {
		t.Fatalf("got %q, want %q", got, "1\nx\n")
	}
}

// TestARuntimeStringWithAUnionArgumentFormats is the other half of that decision, and
// the half that was quietly wrong. A first argument that may or may not hold a
// specifier could not refuse the build, so it fell back to a join through the union's
// own ToString and printed "v=%s! 2" where Node prints "v=2!". The union boxes now, so
// the call reaches the format path and the specifier is honoured.
func TestARuntimeStringWithAUnionArgumentFormats(t *testing.T) {
	skipIfShort(t)
	const src = `function show(prefix: string, u: string | number): void { console.log(prefix, u); }
show("p", 1);
show("v=%s!", 2);
show("%d items", "no");
`
	want := "p 1\nv=2!\nNaN items\n"
	if got := runProgramGo(t, src); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestAFormatStringWithAnUnboxableArgumentHandsBack keeps the rule's failure mode
// pinned on an argument that still has no dynamic form. A function-valued element reads
// as undefined to the reflection walk, so an array of them does not box, and a call
// that must be formatted stops the build rather than print the wrong line.
func TestAFormatStringWithAnUnboxableArgumentHandsBack(t *testing.T) {
	prog := compileTolerant(t, `const fns = [() => 1];
console.log("%s", fns);`)
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

// TestARuntimeStringWithAnUnboxableArgumentFallsBack pins the other half of the rule on
// the same argument: a first argument whose text is unknown may not hold a specifier at
// all, so the format path lets the call go rather than refuse it, and whatever the
// per-argument path says about the argument is what the reader is told.
//
// The per-argument path boxes an array too, so this still stops the build today. What
// it must not do is stop it in the format path's name, since that would tell a reader
// the specifier was the problem when the first argument may spell none.
func TestARuntimeStringWithAnUnboxableArgumentFallsBack(t *testing.T) {
	prog := compileTolerant(t, `const fns = [() => 1];
function show(prefix: string): void { console.log(prefix, fns); }
show("p");`)
	r := NewRenderer(prog)
	_, err := r.RenderProgram(entryFile(t, prog))
	if err == nil {
		return
	}
	if strings.Contains(err.Error(), "with a format string") {
		t.Errorf("a runtime first argument claimed the format path: %q", err.Error())
	}
}
