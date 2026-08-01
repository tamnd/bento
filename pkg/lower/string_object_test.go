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
		{
			// An instance one container down has no call site of its own, so it reads
			// through its box, which answers the class's own toString now.
			"arrayOfInstancesWithToString",
			"class Q { y = 2; toString() { return 'Q!'; } }\nconst qs: Q[] = [new Q()];\nconsole.log(String(qs));\n",
			"value.StringCoerce(value.ArrayValueOf(qs, value.ClassToValue))",
		},
		{
			"concatOfAnArrayOfInstancesWithToString",
			"class Q { y = 2; toString() { return 'Q!'; } }\nconst qs: Q[] = [new Q()];\nconsole.log('x' + qs);\n",
			"value.PlusToString(value.ArrayValueOf(qs, value.ClassToValue))",
		},
		{
			// A toString answering a number is a rung short of a string, so the direct
			// call cannot spell it and the box walks the ladder instead.
			"toStringAnsweringANumber",
			"class N { n = 1; toString() { return 1; } }\nconsole.log(String(new N()));\n",
			"value.StringCoerce(value.ObjectFromStruct(",
		},
		{
			// A valueOf answering an object is not a primitive, so + falls on to toString
			// from there, which is exactly what the box does.
			"concatOfAValueOfAnsweringAnObject",
			"class O { o = 1; valueOf() { return { a: 1 }; } }\nconst o = new O();\nconsole.log('x' + o);\n",
			"value.PlusToString(value.ObjectFromStruct(o))",
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
// different string than the engine produces is worse than no lowering at all, so an
// instance whose own toString the box cannot run still hands back: the thunk a class
// registers takes no arguments and answers a primitive, so a toString wanting an
// argument or answering through a channel has no shape to be called in, and the box
// would read as the class tag where the program has an answer. A function is left out
// for its own reason: it stringifies to its source text, which no box carries.
func TestObjectStringHandsBack(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"aToStringWantingAnArgument",
			"class T { x = 1; toString(pad: string) { return pad; } }\nconst ts: T[] = [new T()];\nconsole.log(String(ts));\n",
			"coercing a value that holds an instance writing its own toString",
		},
		{
			"anAsyncToString",
			"class A { x = 1; async toString() { return 'a'; } }\nconst as: A[] = [new A()];\nconsole.log(String(as));\n",
			"coercing a value that holds an instance writing its own toString",
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
