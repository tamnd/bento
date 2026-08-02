package lower

import (
	"strings"
	"testing"
)

// TestModuleBoxedBindingDeclaresAPackageBox pins the choice this slice makes. A module
// binding a function reads moves out of main and onto a package-level var, and that move
// must not change what the compiler thinks is in the slot. The declaration is now taken
// from boxedChainBinding, the same rule the in-main declaration reads and the same one
// every use of the name reads, so all three agree the slot holds a box.
func TestModuleBoxedBindingDeclaresAPackageBox(t *testing.T) {
	const src = "const ns = JSON.parse('[3,1,2]') as number[];\n" +
		"function mk(): number { return ns.length; }\n" +
		"console.log(mk());\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "var ns value.Value") {
		t.Errorf("the hoisted module binding was not declared as a box:\n%s", got)
	}
	if strings.Contains(got, "var ns *value.Array") {
		t.Errorf("the hoisted module binding kept the checker's array type:\n%s", got)
	}
}

// TestModuleBoxedBindingAgreesWithItsReaders pins the disagreement that made this a build
// break rather than a hand-back. The initializer lowers to a box and the reads dispatch
// through the value model whether they are in main or in the function that made the
// binding hoist, so the package var has to be the one that gives way.
func TestModuleBoxedBindingAgreesWithItsReaders(t *testing.T) {
	const src = "const ns = JSON.parse('[3,1,2]') as number[];\n" +
		"function idx(): number { return ns[2]; }\n" +
		"console.log(idx(), ns.length);\n"
	got := renderProgram(t, src)
	for _, want := range []string{
		"var ns value.Value",
		"ns = value.JSONParse(",
		"value.ToNumber(ns.GetIndex(2))",
		"ns.Get(value.FromGoString(\"length\"))",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// TestModuleUnboxedBindingKeepsItsType pins the other side of the rule. A module binding
// whose slot holds an ordinary Go value is untouched by this, so a plain array a function
// reads still hoists at the type the checker gave the name.
func TestModuleUnboxedBindingKeepsItsType(t *testing.T) {
	const src = "const ns: number[] = [3, 1, 2];\n" +
		"function mk(): number { return ns.length; }\n" +
		"console.log(mk());\n"
	got := renderProgram(t, src)
	if strings.Contains(got, "var ns value.Value") {
		t.Errorf("an unboxed module binding was declared as a box:\n%s", got)
	}
	if !strings.Contains(got, "var ns *value.Array[float64]") {
		t.Errorf("the unboxed module binding did not keep its array type:\n%s", got)
	}
}

// TestConciseArrowBringsABoxDownToItsResult pins the second half of the slice. A concise
// arrow spells its result from the checker's type for the body, so a body that dispatches
// through the runtime and answers a box has to come down to that type at the return, the
// same ToNumber or ToString a read off a box takes anywhere else. Without it the arrow
// returned a value.Value into a value.BStr result and the Go did not build.
func TestConciseArrowBringsABoxDownToItsResult(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"string",
			"const ss = JSON.parse('[]') as string[];\nconst j = (): string => ss.join('|');\nconsole.log(j());\n",
			"return value.ToString(",
		},
		{
			"number",
			"const ns = JSON.parse('[]') as number[];\nconst l = (): number => ns[0];\nconsole.log(l());\n",
			"return value.ToNumber(",
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			got := renderProgram(t, c.src)
			if !strings.Contains(got, c.want) {
				t.Errorf("the concise arrow did not bring its box down, want %s:\n%s", c.want, got)
			}
		})
	}
}
