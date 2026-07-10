package lower

import (
	"strings"
	"testing"
)

// TestParenthesizedBaseRuns pins that a base class wrapped in parentheses,
// extends (A), unwraps to the plain name and embeds A the way an unparenthesized
// extends A does.
func TestParenthesizedBaseRuns(t *testing.T) {
	const src = `class A { x: number = 3; }
class B extends (A) {}
console.log(String(new B().x));
`
	got := runProgramGo(t, src)
	if got != "3\n" {
		t.Errorf("parenthesized base ran wrong\n got: %q\nwant: %q", got, "3\n")
	}
}

// TestNonPlainBaseHandsBack pins the honest split the base-class slice makes: a
// base that is not a plain registered class name hands back with a reason naming
// its shape, not the generic heritage message it used to share.
func TestNonPlainBaseHandsBack(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"extendsNull",
			"class B extends null {}\nnew B();\n",
			"extends null is a later slice",
		},
		{
			"mixinCall",
			"function mk(): any { return class { x: number = 1; }; }\nclass B extends mk() {}\nnew B();\n",
			"a mixin base expression is a later slice",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reason := renderProgramHandBack(t, tc.src)
			if !strings.Contains(reason, tc.want) {
				t.Errorf("hand-back reason %q does not name %q", reason, tc.want)
			}
		})
	}
}
