package value

import "testing"

// A boxed instance carries an instance's fields and its class name and, before this,
// none of its methods, so the two methods the language calls on its own, toString and
// valueOf, had nothing to run. These hold the coercions a box answers now: the class's
// own code when it writes one of the two, and what Object.prototype gives when it does
// not. Every expectation is Node v24's.

// speakerClass writes a toString, the class whose instance has an answer of its own for
// the string hint. The registrations sit in an init rather than on the vars, the way
// classbox_test.go's do, because a test file's vars would run before the runtime's.
type speakerClass struct {
	Y float64 `json:"y"`
}

// weigherClass writes a valueOf and no toString, the pair that reads two ways in one
// program: String(w) is the class tag and 'x' + w is the number.
type weigherClass struct {
	V float64 `json:"v"`
}

// bothClass writes both, so the string hint and the default hint each have their own
// answer and the two differ.
type bothClass struct {
	W float64 `json:"w"`
}

// shadowClass carries a field named toString, which is a real own property of the
// instance and shadows the method the way it does on a live one.
type shadowClass struct {
	ToString float64 `json:"toString"`
}

func init() {
	RegisterClass("Speaker", (*speakerClass)(nil))
	RegisterClass("Weigher", (*weigherClass)(nil))
	RegisterClass("Both", (*bothClass)(nil))
	RegisterClass("Shadow", (*shadowClass)(nil))

	RegisterClassCoercion((*speakerClass)(nil), "toString", func(any) Value {
		return StringValue(FromGoString("Q!"))
	})
	RegisterClassCoercion((*weigherClass)(nil), "valueOf", func(x any) Value {
		return Number(x.(*weigherClass).V)
	})
	RegisterClassCoercion((*bothClass)(nil), "toString", func(any) Value {
		return StringValue(FromGoString("W!"))
	})
	RegisterClassCoercion((*bothClass)(nil), "valueOf", func(x any) Value {
		return Number(x.(*bothClass).W)
	})
}

// TestABoxedInstanceRunsItsOwnToString is the capability. The box holds no methods, so
// the string hint used to fall through to the class tag; now it finds the class's own
// toString and takes its answer, which is what makes String([q]) read the program's
// string rather than a row of tags.
func TestABoxedInstanceRunsItsOwnToString(t *testing.T) {
	box := ObjectFromStruct(&speakerClass{Y: 2})

	if got := ToString(box).ToGoString(); got != "Q!" {
		t.Errorf("String(q) = %q, want \"Q!\"", got)
	}
	if got := ToString(callMember(t, box, "toString")).ToGoString(); got != "Q!" {
		t.Errorf("q.toString() = %q, want \"Q!\"", got)
	}
	// An array of them stringifies element by element, which is Array.prototype.toString
	// being join(','), so each element runs its own toString one level down.
	arr := NewArrayValue([]Value{box, ObjectFromStruct(&speakerClass{Y: 3})})
	if got := ToString(arr).ToGoString(); got != "Q!,Q!" {
		t.Errorf("String([q, q]) = %q, want \"Q!,Q!\"", got)
	}
}

// TestABoxedInstanceReadsBothHints holds the two hints apart. The string hint asks
// toString first and the default hint asks valueOf first, so one instance legitimately
// reads two ways and a box that served only one of them would be wrong in the other.
func TestABoxedInstanceReadsBothHints(t *testing.T) {
	weigher := ObjectFromStruct(&weigherClass{V: 7})
	if got := ToString(weigher).ToGoString(); got != "[object Object]" {
		t.Errorf("String(v) = %q, want \"[object Object]\": the string hint asks toString first", got)
	}
	if got := ToString(Add(StringValue(FromGoString("x")), weigher)).ToGoString(); got != "x7" {
		t.Errorf("'x' + v = %q, want \"x7\": + asks valueOf first", got)
	}
	if got := ToNumber(weigher); got != 7 {
		t.Errorf("Number(v) = %v, want 7", got)
	}

	both := ObjectFromStruct(&bothClass{W: 9})
	if got := ToString(both).ToGoString(); got != "W!" {
		t.Errorf("String(w) = %q, want \"W!\"", got)
	}
	if got := ToString(Add(StringValue(FromGoString("x")), both)).ToGoString(); got != "x9" {
		t.Errorf("'x' + w = %q, want \"x9\"", got)
	}
}

// TestABoxedInstanceFallsBackToObjectPrototype covers the class that writes neither.
// Object.prototype answers both, the class tag for toString and the object itself for
// valueOf, and the identity valueOf is object-like so the default hint falls through it
// to toString rather than stopping there.
func TestABoxedInstanceFallsBackToObjectPrototype(t *testing.T) {
	box := ObjectFromStruct(newPoint())

	if got := ToString(callMember(t, box, "toString")).ToGoString(); got != "[object Object]" {
		t.Errorf("p.toString() = %q, want \"[object Object]\"", got)
	}
	if got := callMember(t, box, "valueOf"); !StrictEquals(got, box) {
		t.Errorf("p.valueOf() did not answer the object itself")
	}
	if got := ToString(Add(StringValue(FromGoString("x")), box)).ToGoString(); got != "x[object Object]" {
		t.Errorf("'x' + p = %q, want \"x[object Object]\"", got)
	}
	for _, name := range []string{"toString", "valueOf"} {
		if !box.HasProperty(FromGoString(name)) {
			t.Errorf("'%s' in p = false, want true", name)
		}
	}
}

// TestAnOwnFieldShadowsTheCoercion holds the ordering. A field named toString is a real
// own property of the instance, so it wins over the method the same way it does on a
// live instance. The number it holds is not callable, so the string hint moves on to
// valueOf, which answers the object, and the coercion runs out of rungs and throws.
// That is Node's answer for the same class, so the box is not falling back to a tag
// here, it is failing the way the engine fails.
func TestAnOwnFieldShadowsTheCoercion(t *testing.T) {
	box := ObjectFromStruct(&shadowClass{ToString: 5})

	if got := box.Get(FromGoString("toString")); got.Kind() != KindNumber || got.AsNumber() != 5 {
		t.Errorf("the own field read = %v, want the number 5", got.Kind())
	}
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("String(s) did not throw")
		}
		if got := ToString(Caught(r).ToValue()).ToGoString(); got != "TypeError: Cannot convert object to primitive value" {
			t.Errorf("String(s) threw %q, want a TypeError about converting an object", got)
		}
	}()
	_ = ToString(box)
}

// TestAnUnregisteredTypeAnswersNoOtherName holds the surface closed: only the two
// coercions are answered, so an ordinary method name still misses and reads undefined
// the way a property that is not there does.
func TestAnUnregisteredTypeAnswersNoOtherName(t *testing.T) {
	box := ObjectFromStruct(&speakerClass{Y: 2})

	if got := box.Get(FromGoString("shout")); !got.IsUndefined() {
		t.Errorf("q.shout = %v, want undefined", got.Kind())
	}
	if box.HasProperty(FromGoString("shout")) {
		t.Error("'shout' in q = true, want false")
	}
}
