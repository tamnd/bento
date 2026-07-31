package lower

import (
	"errors"
	"strings"
	"testing"
)

// TestOptionalBoxEmits pins the two calls this slice routes an optional through. A
// console argument the checker types T | undefined boxes with value.OptToValue and the
// element's own boxer, the same boxer an array of that element type is built with, and
// then reads through the console inspector rather than through a string coercion.
// JSON.stringify of one unwraps the option instead of walking the value.Opt struct.
func TestOptionalBoxEmits(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []string
	}{
		{
			"instanceValue",
			"class P { x = 1; }\nconst m = new Map<string, P>();\nm.set('k', new P());\nconsole.log(m.get('k'));\n",
			[]string{"value.OptToValue(", "value.ClassToValue)", "value.ConsoleValue("},
		},
		{
			"dateValue",
			"const m = new Map<string, Date>();\nconsole.log(m.get('k'));\n",
			[]string{"value.OptToValue(", "(*value.Date).ToValue)"},
		},
		{
			"jsonStringify",
			"class P { x = 1; }\nconst m = new Map<string, P>();\nconsole.log(JSON.stringify(m.get('k')));\n",
			[]string{"value.JSONStringifyOpt("},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := renderProgram(t, tc.src)
			for _, want := range tc.want {
				if !strings.Contains(source, want) {
					t.Errorf("the optional lowering did not print %q:\n%s", want, source)
				}
			}
		})
	}
}

// TestOptionalBoxHandsBack pins the boundaries this slice keeps. An element type with no
// dynamic box of its own has none through an optional either, and JSON.stringify of an
// optional answers undefined when the option is absent, so binding that result into a
// clean string slot has no sound spelling and hands back rather than shipping the text
// "undefined" where Node keeps a value whose typeof is not "string".
func TestOptionalBoxHandsBack(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"collectionElement",
			"const outer = new Map<string, Map<string, number>>();\nconsole.log(outer.get('a'));\n",
			"boxing an optional of this type into a dynamic value is a later slice",
		},
		{
			"jsonStringifyIntoStringSlot",
			"const m = new Map<string, number>();\nconst s: string = JSON.stringify(m.get('k'));\nconsole.log(s);\n",
			"an absent optional serializes as undefined",
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
