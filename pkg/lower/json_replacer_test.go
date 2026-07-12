package lower

import (
	"errors"
	"strings"
	"testing"
)

// TestJSONStringifyReplacerEmits pins the replacer lowering: an inline arrow
// replacer routes to value.JSONStringifyReplacerFunc, an array-literal whitelist
// to value.JSONStringifyReplacerArray, and a whitelist with a space argument
// threads the gap through value.JSONGapNum.
func TestJSONStringifyReplacerEmits(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"funcReplacer",
			"const o = { a: 1, b: 2 };\nconsole.log(JSON.stringify(o, (key, value) => {\n  if (key === \"a\") { return undefined; }\n  return value;\n}));\n",
			"value.JSONStringifyReplacerFunc(",
		},
		{
			"arrayWhitelist",
			"const o = { a: 1, b: 2 };\nconsole.log(JSON.stringify(o, [\"a\"]));\n",
			"value.JSONStringifyReplacerArray(o, []value.BStr{value.FromGoString(\"a\")}, \"\")",
		},
		{
			"whitelistWithSpace",
			"const o = { a: 1, b: 2 };\nconsole.log(JSON.stringify(o, [\"a\"], 2));\n",
			"value.JSONGapNum(2)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := renderProgram(t, tc.src)
			if !strings.Contains(source, tc.want) {
				t.Errorf("replacer lowering did not print %q:\n%s", tc.want, source)
			}
		})
	}
}

// TestJSONStringifyReplacerHandsBack pins the boundaries this slice keeps: a
// replacer that is a named function rather than an inline arrow, and an array
// whitelist that lists a non-string literal, each have no static shape the walk
// can honor yet, so each hands back with a named reason.
func TestJSONStringifyReplacerHandsBack(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"namedFunctionReplacer",
			"function rep(k: string, v: unknown): unknown { return v; }\nconst o = { a: 1 };\nconsole.log(JSON.stringify(o, rep));\n",
			"not an inline arrow function or array literal",
		},
		{
			"numericWhitelistEntry",
			"const o = { a: 1 };\nconsole.log(JSON.stringify(o, [\"a\", 2]));\n",
			"anything but string literals",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prog := compile(t, tc.src)
			r := NewRenderer(prog)
			_, err := r.RenderProgram(entryFile(t, prog))
			var nyl *NotYetLowerable
			if !errors.As(err, &nyl) {
				t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
			}
			if !strings.Contains(nyl.Reason, tc.want) {
				t.Errorf("hand-back reason = %q, want it to contain %q", nyl.Reason, tc.want)
			}
		})
	}
}
