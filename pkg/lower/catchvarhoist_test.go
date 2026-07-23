package lower

import (
	"strings"
	"testing"
)

// A catch parameter is its own block-scoped binding, distinct from a hoisted var of
// the same name. `var x; try {} catch (x) { ... }` gives the catch clause its own x
// that the clause's closures capture, while the outer x stays independent. The var
// hoist must not fold the catch binding into the hoisted var, or the emitted Go
// reuses one name for two bindings and the closure reads the wrong one.

// TestCatchParamShadowsHoistedVarRuns builds and runs the shadow case, proving the
// closure captures the catch binding (returns 'inside') while the outer assignment
// after the catch lands on the hoisted var (reads 'outside').
func TestCatchParamShadowsHoistedVarRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
var probe: any, x: any;
try {
  throw 'inside';
} catch (x) {
  probe = function () { return x; };
}
x = 'outside';
console.log(x);
console.log(probe());
`
	if got, want := runProgramGo(t, src), "outside\ninside\n"; got != want {
		t.Fatalf("catch parameter shadowing a hoisted var printed %q, want %q", got, want)
	}
}

// TestDestructuringAnonFunctionDefaultHandsBack pins that a destructuring default
// which is an anonymous function, an arrow, or a parenthesized anonymous function
// hands back rather than bind a value whose NamedEvaluation name the static function
// model cannot host. A named function expression keeps its own name and is not this
// case, so it does not hand back for this reason.
func TestDestructuringAnonFunctionDefaultHandsBack(t *testing.T) {
	cases := map[string]string{
		"anonymous function": "const [fn = function () {}] = [] as any[]; fn;\n",
		"arrow":              "const [fn = () => {}] = [] as any[]; fn;\n",
		"cover":              "const [fn = (function () {})] = [] as any[]; fn;\n",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			reason := renderProgramHandBack(t, src)
			if !strings.Contains(reason, "NamedEvaluation") {
				t.Fatalf("anon-function destructuring default hand-back reason = %q, want a NamedEvaluation reason", reason)
			}
		})
	}
}
