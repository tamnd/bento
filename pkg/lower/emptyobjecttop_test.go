package lower

import (
	"strings"
	"testing"
)

// TestEmptyObjectTopTypeLowersToDynamicValue pins that the empty object top type { },
// which accepts any non-null value and carries no declared members, lowers to the
// dynamic value.Value box rather than an interned empty struct. A parameter typed
// { } | undefined collapses to the bare box, since value.Value already holds undefined.
func TestEmptyObjectTopTypeLowersToDynamicValue(t *testing.T) {
	src := `type Obj = {} | undefined;
function isUser(obj: Obj): obj is { name?: string } {
    return true;
}
function getUserName(obj: Obj) {
    if (isUser(obj)) {
        return obj.name;
    }
    return '';
}
`
	out := renderProgramTolerant(t, src)
	if !strings.Contains(out, "func IsUser(obj value.Value)") {
		t.Fatalf("empty object top type parameter did not collapse to value.Value:\n%s", out)
	}
	if strings.Contains(out, "ObjEmpty") {
		t.Fatalf("empty object top type interned a struct instead of boxing:\n%s", out)
	}
	// The type guard narrows obj to a shape, but the box-backed variable still reads its
	// member through the dynamic Get and coerces into the optional return slot.
	if !strings.Contains(out, `obj.Get(value.FromGoString("name"))`) {
		t.Fatalf("narrowed member read did not dispatch through the box:\n%s", out)
	}
	if !strings.Contains(out, "value.ToOptString(") {
		t.Fatalf("dynamic member read did not coerce into the optional string return slot:\n%s", out)
	}
}

// TestEmptyObjectLiteralBoxesToNewObject pins that a bare { } literal typed as the
// empty top type lowers to value.NewObject, the runtime property bag, not an empty
// interned struct.
func TestEmptyObjectLiteralBoxesToNewObject(t *testing.T) {
	src := `function f(x: { [s: string]: string }) { }
f({});
`
	out := renderProgramTolerant(t, src)
	if !strings.Contains(out, "value.NewObject()") {
		t.Fatalf("empty object literal did not box to value.NewObject:\n%s", out)
	}
}

// TestConstructorTypeNotBoxedAsEmptyTop guards the narrowing: a construct-signature
// type { new(): T } also has no declared property, but it is a constructor value, not
// the { } top type, so it must not lower to the dynamic box.
func TestConstructorTypeNotBoxedAsEmptyTop(t *testing.T) {
	src := `class Foo {
    constructor(x: number) {}
}
const foo: { new(): Foo } = Foo;
`
	out := renderProgramTolerant(t, src)
	if strings.Contains(out, "var foo value.Value") || strings.Contains(out, "foo value.Value") {
		t.Fatalf("constructor type was wrongly boxed as the empty top type:\n%s", out)
	}
}
