package lower

import (
	"errors"
	"strings"
	"testing"

	"github.com/tamnd/bento/pkg/frontend"
)

// loadUntypedBinding loads src like compile but admits the "Binding element 'X'
// implicitly has an 'any' type" report (7031) the AOT front door tolerates once the
// untyped destructured parameter lowers against a dynamic slot. A fully untyped
// destructuring pattern draws 7031 on each element, so compiling it strictly would
// fail before the renderer ran; admitting the code here reaches the renderer on the
// same terms build.Compile does.
func loadUntypedBinding(t *testing.T, src string) *frontend.Program {
	t.Helper()
	prog, err := frontend.Load(frontend.LoadOptions{
		Dir:   "/",
		Roots: []string{"/m.ts"},
		FS:    realFS{files: map[string]string{"/m.ts": src}},
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, d := range prog.Diagnostics() {
		if d.Category != frontend.CategoryError {
			continue
		}
		if d.Code == 7031 {
			continue
		}
		t.Fatalf("unexpected type error in snippet: %s", d.Message)
	}
	return prog
}

// renderUntypedBinding assembles a 7031-admitting program to Go source. A hand-back
// is a test failure: the case is inside the untyped-pattern subset by construction.
func renderUntypedBinding(t *testing.T, src string) string {
	t.Helper()
	prog := loadUntypedBinding(t, src)
	r := NewRenderer(prog)
	r.SetGoSignatures(testGoSignatures())
	r.SetGoConstants(testGoConstants())
	r.SetGoErrorVars(testGoErrorVars())
	p, err := r.RenderProgram(entryFile(t, prog))
	if err != nil {
		t.Fatalf("RenderProgram: %v", err)
	}
	return p.Source
}

// renderUntypedBindingHandBack asserts the assembler hands the whole program back as
// NotYetLowerable and returns the reason, for an untyped pattern shape the dynamic
// path does not serve yet.
func renderUntypedBindingHandBack(t *testing.T, src string) string {
	t.Helper()
	prog := loadUntypedBinding(t, src)
	r := NewRenderer(prog)
	r.SetGoSignatures(testGoSignatures())
	r.SetGoConstants(testGoConstants())
	r.SetGoErrorVars(testGoErrorVars())
	_, err := r.RenderProgram(entryFile(t, prog))
	if err == nil {
		t.Fatalf("expected a hand-back, got a rendered program")
	}
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("expected NotYetLowerable, got %v", err)
	}
	return nyl.Reason
}

// TestUntypedObjectParamLowersToDynamicSlot proves an untyped object-pattern
// parameter takes one boxed value.Value slot and reads its shorthand names through
// the dynamic Get protocol rather than through struct selectors no boxed value carries.
func TestUntypedObjectParamLowersToDynamicSlot(t *testing.T) {
	got := renderUntypedBinding(t, `function f({a, b}) { return a; }
console.log(String(f({ a: 1, b: 2 })));`)
	for _, want := range []string{
		"func F(__0 value.Value) value.Value",
		`a := __0.Get(value.FromGoString("a"))`,
		`b := __0.Get(value.FromGoString("b"))`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("generated Go missing %q\n%s", want, got)
		}
	}
}

// TestUntypedObjectParamRuns compiles and runs the dynamic-slot lowering end to end,
// so the boxed object flows into the parameter and the bound name reads back out.
func TestUntypedObjectParamRuns(t *testing.T) {
	skipIfShort(t)
	got := goRunSource(t, renderUntypedBinding(t, `function f({a, b}) { return String(a) + String(b); }
console.log(f({ a: 1, b: 2 }));`))
	if got != "12\n" {
		t.Fatalf("got %q, want %q", got, "12\n")
	}
}

// TestUntypedObjectParamManyProps runs a wider pattern to prove each shorthand name
// reads its own property off the one boxed slot.
func TestUntypedObjectParamManyProps(t *testing.T) {
	skipIfShort(t)
	got := goRunSource(t, renderUntypedBinding(t, `function f({x, y, z}) { return String(x) + String(y) + String(z); }
console.log(f({ x: 4, y: 5, z: 6 }));`))
	if got != "456\n" {
		t.Fatalf("got %q, want %q", got, "456\n")
	}
}

// TestUntypedArrayParamLowersToDynamicSlot proves an untyped array-pattern parameter
// takes one boxed value.Value slot and reads its positions through the dynamic GetIndex
// protocol, the index analog of the object pattern's Get.
func TestUntypedArrayParamLowersToDynamicSlot(t *testing.T) {
	got := renderUntypedBinding(t, `function f([a, b]) { return a; }
console.log(String(f([1, 2])));`)
	for _, want := range []string{
		"func F(__0 value.Value) value.Value",
		"a := __0.GetIndex(0)",
		"b := __0.GetIndex(1)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("generated Go missing %q\n%s", want, got)
		}
	}
}

// TestUntypedArrayParamRuns compiles and runs the array dynamic-slot lowering end to end,
// so the boxed array flows into the parameter and each position reads back out.
func TestUntypedArrayParamRuns(t *testing.T) {
	skipIfShort(t)
	got := goRunSource(t, renderUntypedBinding(t, `function f([a, b]) { return String(a) + String(b); }
console.log(f([1, 2]));`))
	if got != "12\n" {
		t.Fatalf("got %q, want %q", got, "12\n")
	}
}

// TestUntypedArrayRestParamHandsBack proves a rest on an untyped array pattern, a later
// slice on this path, hands back rather than mislowering.
func TestUntypedArrayRestParamHandsBack(t *testing.T) {
	reason := renderUntypedBindingHandBack(t, `function f([a, ...rest]) { return a; }
console.log(String(f([1, 2, 3])));`)
	if !strings.Contains(reason, "rest") {
		t.Fatalf("reason %q does not name the rest hand-back", reason)
	}
}

// TestUntypedObjectRenameParamHandsBack proves a rename on an untyped object pattern,
// a later slice on this path, hands back rather than mislowering.
func TestUntypedObjectRenameParamHandsBack(t *testing.T) {
	reason := renderUntypedBindingHandBack(t, `function f({a: x}) { return x; }
console.log(String(f({ a: 1 })));`)
	if !strings.Contains(reason, "rename") {
		t.Fatalf("reason %q does not name the rename hand-back", reason)
	}
}
