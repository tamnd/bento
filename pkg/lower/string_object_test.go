package lower

import (
	"strings"
	"testing"
)

// TestObjectStringEmits pins how an object becomes a string. Every shape but one goes
// through its dynamic box and lets the value model run the coercion, since ToString
// over an object is ToPrimitive with the string hint and that ladder lives in the value
// model rather than in any one Go runtime type. The one exception is a class that
// writes its own toString, which is called directly, because the box carries an
// instance's fields and its class name but not its methods.
func TestObjectStringEmits(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"map",
			"const m = new Map<string, number>();\nconsole.log(String(m));\n",
			"value.StringCoerce(m.ToValue())",
		},
		{
			"mapInATemplate",
			"const m = new Map<string, number>();\nconsole.log(`${m}`);\n",
			"value.ToString(m.ToValue())",
		},
		{
			"set",
			"const s = new Set<number>();\nconsole.log(String(s));\n",
			"value.StringCoerce(s.ToValue())",
		},
		{
			"weakMap",
			"class P { x = 1; }\nconst wm = new WeakMap<P, number>();\nconsole.log(String(wm));\n",
			"value.StringCoerce(wm.ToValue())",
		},
		{
			"array",
			"const a: number[] = [1, 2];\nconsole.log(String(a));\n",
			"value.StringCoerce(value.ArrayValueOf(a, value.Number))",
		},
		{
			"fixedShape",
			"const o = { a: 1 };\nconst s: string = String(o);\nconsole.log(s);\n",
			"value.StringCoerce(value.ObjectFromStruct(o))",
		},
		{
			"instance",
			"class P { x = 1; }\nconst p = new P();\nconsole.log(String(p));\n",
			"value.StringCoerce(value.ObjectFromStruct(p))",
		},
		{
			"instanceWithToString",
			"class Q { y = 2; toString() { return 'Q!'; } }\nconsole.log(String(new Q()));\n",
			".ToString()",
		},
		{
			"concatWithToString",
			"class Q { y = 2; toString() { return 'Q!'; } }\nconst q = new Q();\nconsole.log('x' + q);\n",
			"q.ToString()",
		},
		{
			"concatWithValueOf",
			"class V { v = 1; valueOf() { return 7; } }\nconst v = new V();\nconsole.log('x' + v);\n",
			"value.NumberToString(v.ValueOf())",
		},
		{
			"joinsAnObjectElement",
			"class P { x = 1; }\nconst ps: P[] = [new P()];\nconsole.log(ps.join(','));\n",
			"value.JoinString(value.ClassToValue(x))",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := renderProgram(t, tc.src)
			if !strings.Contains(source, tc.want) {
				t.Errorf("the string coercion did not print %q:\n%s", tc.want, source)
			}
		})
	}
}

// TestObjectStringHandsBack pins the boundary. A box that would read back as a
// different string than the engine produces is worse than no lowering at all, so the
// two shapes where that would happen still hand back: an instance writing its own
// toString reached through a container, where there is no call site to run the method,
// and a valueOf answering something that is not a primitive, which sends the engine on
// down a ladder this slice does not walk. A function is left out for its own reason: it
// stringifies to its source text, which no box carries.
func TestObjectStringHandsBack(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"arrayOfInstancesWithToString",
			"class Q { y = 2; toString() { return 'Q!'; } }\nconst qs: Q[] = [new Q()];\nconsole.log(String(qs));\n",
			"coercing a value that holds an instance writing its own toString",
		},
		{
			"concatOfAnArrayOfInstancesWithToString",
			"class Q { y = 2; toString() { return 'Q!'; } }\nconst qs: Q[] = [new Q()];\nconsole.log('x' + qs);\n",
			"coercing a value that holds an instance writing its own toString",
		},
		{
			"valueOfAnsweringAnObject",
			"class V { v = 1; valueOf() { return { a: 1 }; } }\nconst v = new V();\nconsole.log('x' + v);\n",
			"whose valueOf does not answer a primitive",
		},
		{
			"toStringAnsweringANumber",
			"class N { n = 1; toString() { return 1; } }\nconsole.log(String(new N()));\n",
			"whose toString does not return a string",
		},
		{
			"aFunction",
			"const f = (a: number): number => a;\nconsole.log(String(f));\n",
			"coercing this type to a string is a later slice",
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
