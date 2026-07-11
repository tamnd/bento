package value

import "testing"

func descObj(pairs ...any) Value {
	o := NewObject()
	for i := 0; i+1 < len(pairs); i += 2 {
		o.Set(FromGoString(pairs[i].(string)), pairs[i+1].(Value))
	}
	return o
}

// TestDefinePropertyData proves a data descriptor lands with its value and flags,
// and an attribute the descriptor omits defaults to false on a fresh define.
func TestDefinePropertyData(t *testing.T) {
	o := NewObject()
	o.DefineProperty(StringValue(FromGoString("a")), descObj("value", Number(1), "enumerable", True))

	if got := o.Get(FromGoString("a")); got.scalar != Number(1).scalar {
		t.Fatalf("o.a = %v, want 1", got)
	}
	d, _ := o.object().getOwnDesc(FromGoString("a"))
	if !d.enumerable {
		t.Fatal("enumerable not applied")
	}
	if d.writable || d.configurable {
		t.Fatalf("omitted attributes = w:%v c:%v, want both false", d.writable, d.configurable)
	}
}

// TestDefinePropertyNonEnumerableHidden proves a non-enumerable property is left
// out of Object.keys but kept by Object.getOwnPropertyNames.
func TestDefinePropertyNonEnumerableHidden(t *testing.T) {
	o := NewObject()
	o.DefineProperty(StringValue(FromGoString("hidden")), descObj("value", Number(1)))
	o.DefineProperty(StringValue(FromGoString("shown")), descObj("value", Number(2), "enumerable", True))

	if keys := joinKeys(o.OwnEnumerableKeys()); keys != "shown" {
		t.Fatalf("enumerable keys = %q, want \"shown\"", keys)
	}
	if names := joinKeys(o.OwnKeys()); names != "hidden,shown" {
		t.Fatalf("own names = %q, want \"hidden,shown\"", names)
	}
}

// TestDefinePropertyRedefineKeepsFlags proves a redefine that names only the value
// keeps the property's existing enumerable and configurable flags.
func TestDefinePropertyRedefineKeepsFlags(t *testing.T) {
	o := NewObject()
	o.DefineProperty(StringValue(FromGoString("a")), descObj("value", Number(1), "enumerable", False, "configurable", True))
	o.DefineProperty(StringValue(FromGoString("a")), descObj("value", Number(9)))

	d, _ := o.object().getOwnDesc(FromGoString("a"))
	if d.value.scalar != Number(9).scalar {
		t.Fatalf("redefined value = %v, want 9", d.value)
	}
	if d.enumerable {
		t.Fatal("redefine flipped enumerable, should have kept false")
	}
	if !d.configurable {
		t.Fatal("redefine dropped configurable, should have kept true")
	}
}

// TestDefinePropertyAccessor proves an accessor descriptor runs its getter on read.
func TestDefinePropertyAccessor(t *testing.T) {
	o := NewObject()
	getter := NewFunc(func(args []Value) Value { return Number(7) })
	o.DefineProperty(StringValue(FromGoString("g")), descObj("get", getter, "enumerable", True))

	if got := o.Get(FromGoString("g")); got.scalar != Number(7).scalar {
		t.Fatalf("accessor read = %v, want 7", got)
	}
}

func joinKeys(a *Array[BStr]) string {
	out := ""
	for i := 0.0; i < a.Len(); i++ {
		if i > 0 {
			out += ","
		}
		out += a.At(i).ToGoString()
	}
	return out
}
