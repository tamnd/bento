package lower

import (
	"errors"
	"strings"
	"testing"
)

// TestArrayElemBoxEmits pins the boxer value.ArrayValueOf is handed for each element
// type this slice added. Every one but the optional is a plain name or a method
// expression, so an array of that element type costs nothing at the boxing site; the
// optional closes over the inner element's own boxer, which is what OptToValue takes as
// its second argument.
func TestArrayElemBoxEmits(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []string
	}{
		{
			"dynamicElem",
			"const os = require('os');\nconst a: any[] = [];\nos.arch(a);\n",
			[]string{"value.ArrayValueOf(a, value.Identity)"},
		},
		{
			"bigintElem",
			"const os = require('os');\nconst a: bigint[] = [1n];\nos.arch(a);\n",
			[]string{"value.ArrayValueOf(a, value.BigIntFromBig)"},
		},
		{
			"objectShapeElem",
			"const os = require('os');\nconst a: { x: number }[] = [{ x: 1 }];\nos.arch(a);\n",
			[]string{"value.ArrayValueOf(a, value.StructToValue)"},
		},
		{
			"unionElem",
			"const os = require('os');\nconst a: (number | string)[] = [1, 'a'];\nos.arch(a);\n",
			[]string{"value.ArrayValueOf(a, (NumOrStr).ToValue)", "func (u NumOrStr) ToValue() value.Value"},
		},
		{
			"optionalElem",
			"const os = require('os');\nconst a = [1].map((n): number | undefined => (n > 1 ? n : undefined));\nos.arch(a);\n",
			[]string{"value.OptToValue(o, value.Number)"},
		},
		{
			"loneUnion",
			"const os = require('os');\nfunction pick(n: number): number | string { return n > 1 ? 'b' : n; }\nos.arch(pick(1));\n",
			[]string{"Pick(1).ToValue()"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := renderProgram(t, tc.src)
			for _, want := range tc.want {
				if !strings.Contains(source, want) {
					t.Errorf("the element boxing did not print %q:\n%s", want, source)
				}
			}
		})
	}
}

// TestUnionSentinelJSONArm pins that a sentinel arm hands the JSON walk the singleton
// rather than a bare nil. The nil said neither null nor undefined, and the walk read it
// as no value at all, so an array of unions wrote "[0,,1]" and an object field wrote a
// key with nothing after it.
func TestUnionSentinelJSONArm(t *testing.T) {
	src := "function pick(n: number): number | null { return n > 1 ? null : n; }\nconsole.log(JSON.stringify([pick(0), pick(5)]));\n"
	source := renderProgram(t, src)
	for _, want := range []string{"return value.Null", "func (u NumOrNull) JSONArm() any"} {
		if !strings.Contains(source, want) {
			t.Errorf("the union's JSONArm did not print %q:\n%s", want, source)
		}
	}
}

// TestArrayElemBoxHandsBack pins what is still refused. An element with no dynamic box
// at all keeps the hand-back, and it names a source position now, so the next reader of
// the reason knows which array in the program raised it.
func TestArrayElemBoxHandsBack(t *testing.T) {
	src := "const os = require('os');\nconst a: (() => number)[] = [() => 1];\nos.arch(a);\n"
	prog := compile(t, src)
	r := NewRenderer(prog)
	_, err := r.RenderProgram(entryFile(t, prog))
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
	}
	if !strings.Contains(nyl.Reason, "boxing an array of this element type") {
		t.Fatalf("hand-back reason = %q, want the array element boxing one", nyl.Reason)
	}
	if nyl.Where == "" {
		t.Errorf("hand-back carried no source position: %v", nyl)
	}
}
