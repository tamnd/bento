package lower

import (
	"strings"
	"testing"
)

// TestClassCoercionRegisterEmits pins the registration a boxed class emits. A box
// carries an instance's fields and its class name and none of its methods, so the two
// the language calls on its own, toString and valueOf, are handed to the runtime as
// thunks closing over the ordinary typed call. The assertion inside each thunk names the
// class the box holds rather than the one that declared the method, which is what makes
// an override and an inherited method both resolve the way a written-out call does.
func TestClassCoercionRegisterEmits(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []string
	}{
		{
			"toString",
			"class Q { y = 2; toString() { return 'Q!'; } }\nconst qs: Q[] = [new Q()];\nconsole.log(String(qs));\n",
			[]string{`value.RegisterClassCoercion((*Q)(nil), "toString"`, "value.StringValue(x.(*Q).ToString())"},
		},
		{
			"valueOf",
			"class V { v = 1; valueOf() { return 7; } }\nconst vs: V[] = [new V()];\nconsole.log(String(vs));\n",
			[]string{`value.RegisterClassCoercion((*V)(nil), "valueOf"`, "value.Number(x.(*V).ValueOf())"},
		},
		{
			"both",
			"class W { w = 1; toString() { return 'W!'; } valueOf() { return 9; } }\nconst ws: W[] = [new W()];\nconsole.log(String(ws));\n",
			[]string{"value.StringValue(x.(*W).ToString())", "value.Number(x.(*W).ValueOf())"},
		},
		{
			// A base-typed array holds the embedded base of each instance, so the box is a
			// *Q whatever the instance was constructed as and the one registration through
			// *Q serves the whole hierarchy: the call it wraps is the virtual entry, so an
			// override runs through the vtable exactly as a written-out q.toString() does.
			"aHierarchyRegistersThroughItsBase",
			"class Q { y = 2; toString() { return 'Q!'; } }\nclass R extends Q { toString() { return 'R!'; } }\nconst qs: Q[] = [new Q(), new R()];\nconsole.log(String(qs));\n",
			[]string{"value.StringValue(x.(*Q).ToString())"},
		},
		{
			// A subclass-typed array boxes the subclass itself, so it registers through its
			// own type and reaches the method by Go promotion.
			"aSubclassTypedArrayRegistersTheSubclass",
			"class Q { y = 2; toString() { return 'Q!'; } }\nclass R extends Q { toString() { return 'R!'; } }\nconst rs: R[] = [new R()];\nconsole.log(String(rs));\n",
			[]string{"value.StringValue(x.(*R).ToString())"},
		},
		{
			"anInheritedMethodRegistersThroughTheSubclass",
			"class Q { y = 2; toString() { return 'Q!'; } }\nclass S extends Q { z = 3; }\nconst ss: S[] = [new S()];\nconsole.log(String(ss));\n",
			[]string{"value.StringValue(x.(*S).ToString())"},
		},
		{
			"aToStringAnsweringANumber",
			"class N { n = 1; toString() { return 1; } }\nconst ns: N[] = [new N()];\nconsole.log(String(ns));\n",
			[]string{"value.Number(x.(*N).ToString())"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := renderProgram(t, tc.src)
			for _, want := range tc.want {
				if !strings.Contains(source, want) {
					t.Errorf("the class registration did not print %q:\n%s", want, source)
				}
			}
		})
	}
}

// TestClassCoercionRegisterSkips pins what stays unregistered. The thunk takes no
// arguments and hands back a boxed result, so a method wanting an argument, one
// answering through a channel, and one answering something other than a primitive have
// no shape to be called in. A class the program never boxes registers nothing at all,
// which is the same rule the name registration follows.
func TestClassCoercionRegisterSkips(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{
			"aToStringWantingAnArgument",
			"class T { x = 1; toString(pad: string) { return pad; } }\nconsole.log(new T());\n",
		},
		{
			"anAsyncToString",
			"class A { x = 1; async toString() { return 'a'; } }\nconsole.log(new A());\n",
		},
		{
			"aValueOfAnsweringAnObject",
			"class O { o = 1; valueOf() { return { a: 1 }; } }\nconsole.log(new O());\n",
		},
		{
			"aClassTheProgramNeverBoxes",
			"class Q { y = 2; toString() { return 'Q!'; } }\nconst q = new Q();\nconsole.log(q.toString());\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := renderProgram(t, tc.src)
			if strings.Contains(source, "RegisterClassCoercion") {
				t.Errorf("the class registered a coercion it cannot call:\n%s", source)
			}
		})
	}
}
