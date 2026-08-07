package lower

import (
	"strings"
	"testing"
)

// An object literal can name a property with a computed key whose expression is a
// runtime value the checker types outside string, number, symbol, or any: a boolean
// is the common case, which draws the 2464 diagnostic. JavaScript still runs
// ToPropertyKey on the key, so a boolean becomes "true" or "false" and a number its
// canonical string. Such a literal is not statically fixed, so it lowers through the
// dynamic bag: the binding boxes and the computed member emits SetKeyed over the
// boxed key, whose SetElem runs the same ToString the language does. The 2464 gate is
// the only thing that held these back, so tolerating it drives the tolerant path.

func TestComputedBooleanKeyCoercesToString(t *testing.T) {
	skipIfShort(t)
	if got := runProgramGoTolerant(t, `const k: boolean = true; const o: any = { [k]: 1 }; console.log(o.true);`); got != "1\n" {
		t.Fatalf("{ [true]: 1 }.true = %q, want 1", got)
	}
}

func TestComputedBooleanKeyFalseSlot(t *testing.T) {
	skipIfShort(t)
	if got := runProgramGoTolerant(t, `const k: boolean = false; const o: any = { [k]: 7 }; console.log(o.false);`); got != "7\n" {
		t.Fatalf("{ [false]: 7 }.false = %q, want 7", got)
	}
}

// The literal boxes even without an any annotation: a computed runtime key makes the
// shape not fixed, so the variable initializer path boxes it into the dynamic bag and
// marks the binding dynamic, so a later read routes the dynamic way.
func TestComputedKeyBoxesWithoutAnnotation(t *testing.T) {
	skipIfShort(t)
	if got := runProgramGoTolerant(t, `const k: boolean = true; const o = { [k]: 1 }; console.log((o as any).true);`); got != "1\n" {
		t.Fatalf("unannotated { [true]: 1 }.true = %q, want 1", got)
	}
}

// The boxed computed key emits SetKeyed over the boxed key value, not a Go struct
// field, so the runtime coercion the language runs on the key is preserved.
func TestComputedKeyEmitsSetKeyed(t *testing.T) {
	src := `const k: boolean = true; const o: any = { [k]: 1 }; console.log(o.true);`
	got := renderProgramTolerant(t, src)
	if !strings.Contains(got, "SetKeyed") {
		t.Fatalf("emit did not route the computed key through SetKeyed:\n%s", got)
	}
	if strings.Contains(got, "struct {") {
		t.Fatalf("a not-fixed literal must not build a struct:\n%s", got)
	}
}

// An object-typed computed key runs. This test pinned a handback while the `object`
// keyword had no lowering and its binding could not be rendered at all; it takes the
// dynamic box now, so the key reaches SetKeyed like every other runtime key and the
// language's own ToPropertyKey names the slot. A plain object stringifies to
// "[object Object]", so that is the property the write lands on, which is what the
// read back proves.
func TestComputedObjectKeyCoercesToString(t *testing.T) {
	skipIfShort(t)
	src := `const k: object = {}; const o: any = { [k]: 1 }; console.log(o["[object Object]"]);`
	if got := runProgramGoTolerant(t, src); got != "1\n" {
		t.Fatalf("{ [{}]: 1 }[\"[object Object]\"] = %q, want 1", got)
	}
}

// A computed runtime key in an argument position used to hand back, since a literal
// with no boxing around it would have to become a struct and the struct has no field
// the key names. The parameter is any, so the literal boxes on the way in and the key
// reaches SetKeyed like any other runtime key; ToPropertyKey names the slot, so a
// boolean key writes "true" and the read back finds it, which is what node prints.
func TestComputedKeyInArgPositionBoxes(t *testing.T) {
	src := `function foo(x: any): void { console.log(x.true); } foo({ [true as boolean]: 1 });`
	prog := compileTolerant(t, src)
	r := NewRenderer(prog)
	p, err := r.RenderProgram(entryFile(t, prog))
	if err != nil {
		t.Fatalf("RenderProgram: %v", err)
	}
	if !strings.Contains(p.Source, "SetKeyed(value.Bool(true)") {
		t.Fatalf("arg-position computed key did not reach SetKeyed:\n%s", p.Source)
	}
}
