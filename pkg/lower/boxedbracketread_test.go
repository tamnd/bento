package lower

import (
	"strings"
	"testing"
)

// A binding initialized from an empty object literal lowers to a live
// value.NewObject bag, so a later expando write obj.k = v stores through the
// runtime Set and the object really carries the key. A bracket read obj["k"] of
// that key must dispatch through the same runtime Get the dotted read obj.k
// already takes, not fold to value.MissingProperty on the empty static shape the
// checker still gives the binding: the shape does not obey the expando write, so
// folding answers undefined where the language answers the written value. The
// escaped spelling obj.if denotes the same property "if", so the write and
// the read agree on the cooked name.
func TestBoxedBagBracketReadHitsExpando(t *testing.T) {
	skipIfShort(t)
	cases := map[string]struct{ src, want string }{
		"plain expando": {
			src:  "var obj: any = {};\nobj.foo = 42;\nconsole.log(obj[\"foo\"]);\n",
			want: "42\n",
		},
		"escaped identifier name": {
			src:  "var obj: any = {};\nobj.i\\u0066 = 42;\nconsole.log(obj[\"if\"]);\n",
			want: "42\n",
		},
		"absent key still undefined": {
			src:  "var obj: any = {};\nobj.foo = 1;\nconsole.log(obj[\"bar\"]);\n",
			want: "undefined\n",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := runProgramGo(t, tc.src); got != tc.want {
				t.Fatalf("boxed-bag bracket read printed %q, want %q", got, tc.want)
			}
		})
	}
}

// TestFixedShapeBracketReadStillSelectsField pins that the boxed-bag fix does not
// disturb a genuine fixed-shape read: a typed object literal interns to a Go
// struct, and a bracket read of a declared key selects that struct field rather
// than routing through a runtime Get the struct value does not carry.
func TestFixedShapeBracketReadStillSelectsField(t *testing.T) {
	src := "var o = {a: 1};\nlet n: number = o[\"a\"];\nconsole.log(n);\n"
	out := renderProgram(t, src)
	if !strings.Contains(out, "o.A") || strings.Contains(out, "MissingProperty") {
		t.Fatalf("fixed-shape bracket read did not select the struct field:\n%s", out)
	}
}
