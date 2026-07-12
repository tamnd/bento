package lower

import (
	"errors"
	"strings"
	"testing"
)

// TestJSONStringifySpaceEmits pins the three-argument JSON.stringify lowering: a
// numeric space routes to value.JSONStringifyIndentNum, a string space to
// value.JSONStringifyIndentStr, and a null space collapses to the compact
// value.JSONStringify since the specification treats it as no indentation.
func TestJSONStringifySpaceEmits(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"numeric",
			"const a = { x: 1 };\nconsole.log(JSON.stringify(a, null, 2));\n",
			"value.JSONStringifyIndentNum(",
		},
		{
			"string",
			"const a = { x: 1 };\nconsole.log(JSON.stringify(a, null, \"\\t\"));\n",
			"value.JSONStringifyIndentStr(",
		},
		{
			"undefinedSpace",
			"const a = { x: 1 };\nconsole.log(JSON.stringify(a, null, undefined));\n",
			"value.JSONStringify(a)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := renderProgram(t, tc.src)
			if !strings.Contains(source, tc.want) {
				t.Errorf("JSON.stringify space lowering did not print %q:\n%s", tc.want, source)
			}
		})
	}
}

// TestJSONStringifySpaceHandsBack pins the boundary this slice keeps: a space that
// is neither a number nor a string has no indented form yet, so the call hands
// back with a named reason rather than emitting a call the runtime cannot honor.
func TestJSONStringifySpaceHandsBack(t *testing.T) {
	const src = "function f(s: string | number): string {\n  return JSON.stringify({ x: 1 }, null, s);\n}\nconsole.log(f(2));\n"
	prog := compile(t, src)
	r := NewRenderer(prog)
	_, err := r.RenderProgram(entryFile(t, prog))
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
	}
	if !strings.Contains(nyl.Reason, "space that is not a number or string") {
		t.Errorf("hand-back reason = %q, want it to name the unsupported space", nyl.Reason)
	}
}
