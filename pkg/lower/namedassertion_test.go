package lower

import (
	"strings"
	"testing"
)

// TestAssertionBoundToANameTakesABoxedSlot pins the slot. The checker types the binding
// by the asserted shape, so without this the declaration reached typeExpr and asked for
// the Go struct that shape interns to, which the box cannot fill.
func TestAssertionBoundToANameTakesABoxedSlot(t *testing.T) {
	const src = "type Config = { name: string; port: number };\n" +
		"const parsed = JSON.parse('{}') as Config;\n" +
		"console.log(parsed.name);"
	out := renderProgram(t, src)
	if !strings.Contains(out, "var parsed value.Value") {
		t.Fatalf("a name bound to an assertion did not take a boxed slot:\n%s", out)
	}
	if !strings.Contains(out, "parsed.Get(value.FromGoString(\"name\"))") {
		t.Fatalf("a read off the binding did not dispatch dynamically:\n%s", out)
	}
}

// TestReadsOffABoxedBindingRouteAtRunTime pins the three routings that would otherwise
// have emitted Go that does not compile. The checker keeps calling the member number[],
// so each of these paths has to ask the receiver's lowered form rather than its type:
// ranging .Elems, calling .Join, and mapping an option are all methods a value.Value
// does not have.
func TestReadsOffABoxedBindingRouteAtRunTime(t *testing.T) {
	const prelude = "type Cfg = { list: number[]; a?: { b: number } };\n" +
		"const cfg = JSON.parse('{}') as Cfg;\n"
	for name, tc := range map[string]struct{ src, want string }{
		"for of":         {prelude + "for (const x of cfg.list) console.log(x);", "value.Iterate("},
		"method call":    {prelude + "console.log(cfg.list.join('-'));", "Get(value.FromGoString(\"join\")).Call("},
		"optional chain": {prelude + "console.log(cfg.a?.b);", "value.OptionalMember("},
	} {
		t.Run(name, func(t *testing.T) {
			out := renderProgram(t, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Fatalf("a read off a boxed binding did not route through %s:\n%s", tc.want, out)
			}
		})
	}
}

// TestDestructuringABoxBindsEveryNameDynamic pins that a destructuring declaration off a
// boxed source marks every name it binds, not only a rest it gathers. Each name holds
// what an index or member read off the box yielded, which is a box, and the checker
// calls them number and string. Unmarked, the console call below coerced them as a
// float64 and a string, which is Go that does not compile.
func TestDestructuringABoxBindsEveryNameDynamic(t *testing.T) {
	const src = "type P = { a: number; b: string };\n" +
		"const { a, b } = JSON.parse('{}') as P;\n" +
		"console.log(a, b);"
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.ConsoleFormat(a, b)") {
		t.Fatalf("a name bound by destructuring a box was not read as a box:\n%s", out)
	}
}
