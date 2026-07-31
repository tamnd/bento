package lower

import (
	"strings"
	"testing"
)

// TestWeakBoxEmits pins how the four weakly-holding kinds box. Each has a no-argument
// ToValue of its own, so a top-level box is that call and a nested one is the method
// expression, exactly the shape a Map and a Set take. What separates them is what the
// box shows, which is held in pkg/build against real Node.
func TestWeakBoxEmits(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"weakMap",
			"class P { x = 1; }\nconst wm = new WeakMap<P, number>();\nconsole.log(wm);\n",
			"value.ConsoleValue(wm.ToValue())",
		},
		{
			"weakSet",
			"class P { x = 1; }\nconst ws = new WeakSet<P>();\nconsole.log(ws);\n",
			"value.ConsoleValue(ws.ToValue())",
		},
		{
			"weakRef",
			"class P { x = 1; }\nconst wr = new WeakRef<P>(new P());\nconsole.log(wr);\n",
			"value.ConsoleValue(wr.ToValue())",
		},
		{
			"registry",
			"class P { x = 1; }\nconst fr = new FinalizationRegistry<string>((h: string) => { void h; });\nconsole.log(fr);\n",
			"value.ConsoleValue(fr.ToValue())",
		},
		{
			"weakSetInAnArray",
			"class P { x = 1; }\nconst a: WeakSet<P>[] = [];\nconsole.log(a);\n",
			"value.ArrayValueOf(a, (*value.WeakSet[P]).ToValue)",
		},
		{
			"weakSetInAMap",
			"class P { x = 1; }\nconst m = new Map<string, WeakSet<P>>();\nconsole.log(m);\n",
			"value.NewStringMap[*value.WeakSet[P]]()",
		},
		{
			"weakMapAsAMapKey",
			"class P { x = 1; }\nconst m = new Map<WeakMap<P, number>, string>();\nconsole.log(m);\n",
			"value.ConsoleValue(m.ToValue())",
		},
		{
			"weakMapFromAnOptional",
			"class P { x = 1; }\nconst m = new Map<string, WeakMap<P, number>>();\nconsole.log(m.get('k'));\n",
			"value.OptToValue(m.Get(value.FromGoString(\"k\")), (*value.WeakMap[P, float64]).ToValue)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := renderProgram(t, tc.src)
			if !strings.Contains(source, tc.want) {
				t.Errorf("the weak box did not print %q:\n%s", tc.want, source)
			}
		})
	}
}

// TestWeakBoxHandsBack pins the boundary. The box presents a key, a member and a value
// as boxed values, so a weak collection over something with no dynamic form would emit
// a view whose reads raise. A plain object shape is such a thing: it boxes as a copy of
// its fields with no pointer back, so a key handed to has could never be recognized as
// the key the collection holds, which is a wrong answer rather than a lost write.
func TestWeakBoxHandsBack(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"plainObjectKey",
			"const wm = new WeakMap<{ a: number }, number>();\nconsole.log(wm);\n",
			"boxing a weak collection whose keys, members or values are not",
		},
		{
			"tupleValue",
			"class P { x = 1; }\nconst wm = new WeakMap<P, [number, string]>();\nconsole.log(wm);\n",
			"boxing a weak collection whose keys, members or values are not",
		},
		{
			"plainObjectMember",
			"const ws = new WeakSet<{ a: number }>();\nconsole.log(ws);\n",
			"boxing a weak collection whose keys, members or values are not",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := renderProgramHandBack(t, tc.src); !strings.Contains(got, tc.want) {
				t.Errorf("hand-back reason = %q, want it to contain %q", got, tc.want)
			}
		})
	}
}
