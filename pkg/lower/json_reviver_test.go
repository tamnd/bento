package lower

import (
	"errors"
	"strings"
	"testing"
)

// TestJSONParseReviverEmits pins the reviver lowering: an inline arrow reviver
// taking the key and value routes JSON.parse to value.JSONParseReviver over the
// lowered function, while a plain one-argument parse stays value.JSONParse.
func TestJSONParseReviverEmits(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"arrowReviver",
			"const v = JSON.parse('{\"a\":1}', (key, value) => {\n  if (typeof value === \"number\") { return value * 2; }\n  return value;\n});\nconsole.log(v);\n",
			"value.JSONParseReviver(",
		},
		{
			"plainParse",
			"const v = JSON.parse('{\"a\":1}');\nconsole.log(v);\n",
			"value.JSONParse(",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := renderProgram(t, tc.src)
			if !strings.Contains(source, tc.want) {
				t.Errorf("reviver lowering did not print %q:\n%s", tc.want, source)
			}
		})
	}
}

// TestJSONParseReviverHandsBack pins the boundary this slice keeps: a reviver that
// is a named function rather than an inline arrow has no static shape the walk can
// call yet, so it hands back with a named reason.
func TestJSONParseReviverHandsBack(t *testing.T) {
	src := "function rev(k: string, v: unknown): unknown { return v; }\nconst v = JSON.parse('{\"a\":1}', rev);\nconsole.log(v);\n"
	prog := compile(t, src)
	r := NewRenderer(prog)
	_, err := r.RenderProgram(entryFile(t, prog))
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
	}
	if !strings.Contains(nyl.Reason, "not an inline arrow function") {
		t.Errorf("hand-back reason = %q, want it to contain %q", nyl.Reason, "not an inline arrow function")
	}
}
