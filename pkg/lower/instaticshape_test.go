package lower

import (
	"errors"
	"strings"
	"testing"
)

// renderOrHandBack assembles a module and reports the Go source, or reports that the
// assembler handed it back. Either is a pass for a case that only needs to pin that the
// answer was not folded to a constant, which is what the unsealed shapes below check:
// whether such a receiver reaches the runtime check or hands back is a separate question
// from whether the fold wrongly claimed to know.
func renderOrHandBack(t *testing.T, src string) (string, bool) {
	t.Helper()
	prog := compile(t, src)
	r := NewRenderer(prog)
	r.SetGoSignatures(testGoSignatures())
	r.SetGoConstants(testGoConstants())
	r.SetGoErrorVars(testGoErrorVars())
	p, err := r.RenderProgram(entryFile(t, prog))
	var nyl *NotYetLowerable
	if errors.As(err, &nyl) {
		return "", true
	}
	if err != nil {
		t.Fatalf("RenderProgram: %v", err)
	}
	return p.Source, false
}

// TestInStaticShapeFoldsBothWays pins the capability. A static receiver has no box for
// the runtime InOperator to read, and until this the whole unit handed back the moment a
// program wrote `"a" in o` against one. A sealed shape names every key an object of that
// type carries, so the checker decides the membership on its own and the test folds to a
// Go constant with no receiver read at all.
//
// The false half is the half that makes it usable. A program asking whether a key is
// present almost always asks about a miss beside the hit, and a fold that answered only
// the hit would take the whole unit back with it on the next line.
func TestInStaticShapeFoldsBothWays(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "required own property",
			src:  "class P { x = 1; }\nconst p = new P();\nconsole.log(String(\"x\" in p));",
			want: "true",
		},
		{
			name: "undeclared name on a class",
			src:  "class P { x = 1; }\nconst p = new P();\nconsole.log(String(\"z\" in p));",
			want: "false",
		},
		{
			name: "prototype member on a class",
			src:  "class P { x = 1; }\nconst p = new P();\nconsole.log(String(\"toString\" in p));",
			want: "true",
		},
		{
			name: "method on a class",
			src:  "class P { m() { return 1; } }\nconst p = new P();\nconsole.log(String(\"m\" in p));",
			want: "true",
		},
		{
			name: "required own property on a shape binding",
			src:  "const o: { a: number } = { a: 1 };\nconsole.log(String(\"a\" in o));",
			want: "true",
		},
		{
			name: "undeclared name on a shape binding",
			src:  "const o: { a: number } = { a: 1 };\nconsole.log(String(\"c\" in o));",
			want: "false",
		},
		{
			name: "constructor on a static shape",
			src:  "const o: { a: number } = { a: 1 };\nconsole.log(String(\"constructor\" in o));",
			want: "true",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := renderProgram(t, c.src)
			if !strings.Contains(out, c.want) {
				t.Fatalf("in on a static shape did not fold to %s:\n%s", c.want, out)
			}
			if strings.Contains(out, "value.InOperator(") {
				t.Fatalf("in on a static shape reached the runtime check instead of folding:\n%s", out)
			}
		})
	}
}

// TestInStaticShapeElidesTheReceiver pins that a receiver read only by a folded test is
// blanked rather than left declared and unused, which Go rejects outright. The fold emits
// no read of it, so the binding has to lose the one the source wrote.
func TestInStaticShapeElidesTheReceiver(t *testing.T) {
	const src = `const o: { a: number } = { a: 1 };
console.log(String("a" in o), String("c" in o));`
	out := renderProgram(t, src)
	if strings.Contains(out, "value.InOperator(") {
		t.Fatalf("the folded test still read the receiver:\n%s", out)
	}
	if !strings.Contains(out, "true") || !strings.Contains(out, "false") {
		t.Fatalf("both halves of the fold did not land:\n%s", out)
	}
}

// TestInArrayBoxesRatherThanFolds pins the one static receiver that must not fold. An
// array's own keys are its live indices, so `2 in a` and `9 in a` differ by the length at
// that moment and no compile-time answer holds. The box carries the same elements and the
// same length and the value model answers an index, a length, and the Array.prototype and
// Object.prototype names off it.
func TestInArrayBoxesRatherThanFolds(t *testing.T) {
	for _, src := range []string{
		"const a = [1, 2];\nconsole.log(String(0 in a));",
		"const a = [1, 2];\nconsole.log(String(5 in a));",
		"const a = [1, 2];\nconsole.log(String(\"length\" in a));",
		"const a = [1, 2];\nconsole.log(String(\"toString\" in a));",
		"const t: [number, string] = [1, \"a\"];\nconsole.log(String(1 in t));",
	} {
		t.Run(src, func(t *testing.T) {
			out := renderProgram(t, src)
			if !strings.Contains(out, "value.InOperator(") {
				t.Fatalf("in on an array did not reach the runtime check:\n%s", out)
			}
		})
	}
}

// TestInOptionalMemberHandsBack pins what the fold still declines. An optional member is
// genuinely present or absent at run time and the shape cannot say which, so neither half
// of the fold holds and the honest handback stays. Answering it needs the object to carry
// which of its optional slots were written, which is a later slice.
func TestInOptionalMemberHandsBack(t *testing.T) {
	const src = `const o: { a?: number } = {};
console.log(String("a" in o));`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "in operator") {
		t.Fatalf("in on an optional member did not hand back for that reason: %q", reason)
	}
}

// TestInTupleFoldsNothing pins that a tuple stays off the fold. Its keys are positions,
// so `2 in t` is decided by what the box holds rather than by the type's name list, the
// same reason an array boxes. It handed back until the tuple box landed; now it reaches
// the same runtime check an array does.
func TestInTupleFoldsNothing(t *testing.T) {
	const src = `const t: [number, string] = [1, "a"];
console.log(String(1 in t), String(5 in t));`
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.InOperator(") {
		t.Fatalf("in on a tuple did not reach the runtime check:\n%s", out)
	}
}

// TestInUnsealedShapeHandsBack pins the soundness gate. A shape whose checker property
// list is not the whole story cannot fold either way: an index signature admits any name
// at all, and a type parameter's list is only what its constraint guarantees. Folding
// false against one would print an absence the run time contradicts.
func TestInUnsealedShapeHandsBack(t *testing.T) {
	cases := map[string]string{
		"index signature": "const o: { [k: string]: number } = {};\nconsole.log(String(\"a\" in o));",
		"type parameter":  "function f<T extends object>(o: T): boolean { return \"a\" in o; }\nconsole.log(String(f({})));",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			out, handedBack := renderOrHandBack(t, src)
			if handedBack {
				return
			}
			if strings.Contains(out, "value.InOperator(") {
				return
			}
			t.Fatalf("in on an unsealed shape folded instead of asking the run time:\n%s", out)
		})
	}
}
