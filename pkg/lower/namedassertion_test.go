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
// boxed source accounts for every name it binds, not only a rest it gathers. Each name
// holds what a member read off the box yielded, and unaccounted for, the console call
// below coerced them as a float64 and a string, which is Go that does not compile.
//
// What each name holds is the leaf's own answer. A leaf the checker types number or
// string or boolean comes down to its Go primitive at the bind, the same rule a read off
// a box follows, so these two are a float64 and a string and the console call is right
// to say so. A leaf the checker gives a shape keeps the box, because there is nothing to
// come down to; the second case is that one, and the read off it dispatches.
func TestDestructuringABoxBindsEveryNameDynamic(t *testing.T) {
	const src = "type P = { a: number; b: string };\n" +
		"const { a, b } = JSON.parse('{}') as P;\n" +
		"console.log(a, b);"
	out := renderProgram(t, src)
	if !strings.Contains(out, "a := value.ToNumber(") || !strings.Contains(out, "b := value.ToString(") {
		t.Fatalf("a primitive name bound by destructuring a box did not come down:\n%s", out)
	}
	if !strings.Contains(out, "value.NumberToConsole(a), b") {
		t.Fatalf("a name bound by destructuring a box was not read as what it holds:\n%s", out)
	}

	const nested = "type Q = { o: { z: number } };\n" +
		"const { o } = JSON.parse('{}') as Q;\n" +
		"console.log(o.z);"
	out = renderProgram(t, nested)
	if !strings.Contains(out, "o.Get(value.FromGoString(\"z\"))") {
		t.Fatalf("a shaped name bound by destructuring a box did not keep it:\n%s", out)
	}
}
