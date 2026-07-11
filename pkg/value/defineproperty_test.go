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

// throws runs fn and reports whether it threw a JavaScript value, the recovery a
// catch block performs, so a test can assert Object.defineProperty rejected a
// define with a TypeError.
func throws(fn func()) (thrown bool) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(Thrown); ok {
				thrown = true
				return
			}
			panic(r)
		}
	}()
	fn()
	return false
}

// TestDefinePropertyRejectsReconfigure proves redefining a non-configurable
// property to be configurable, to flip its enumerable flag, or to change its kind
// throws, while a redefine that changes nothing is allowed.
func TestDefinePropertyRejectsReconfigure(t *testing.T) {
	o := NewObject()
	o.DefineProperty(StringValue(FromGoString("a")), descObj("value", Number(1)))

	if !throws(func() {
		o.DefineProperty(StringValue(FromGoString("a")), descObj("configurable", True))
	}) {
		t.Fatal("making a non-configurable property configurable did not throw")
	}
	if !throws(func() {
		o.DefineProperty(StringValue(FromGoString("a")), descObj("enumerable", True))
	}) {
		t.Fatal("flipping a non-configurable property's enumerable did not throw")
	}
	if !throws(func() {
		o.DefineProperty(StringValue(FromGoString("a")), descObj("get", NewFunc(func(args []Value) Value { return Undefined })))
	}) {
		t.Fatal("turning a non-configurable data property into an accessor did not throw")
	}
	// Redefining with the identical descriptor is a no-op, not a rejection.
	if throws(func() {
		o.DefineProperty(StringValue(FromGoString("a")), descObj("value", Number(1)))
	}) {
		t.Fatal("redefining a non-configurable property with its own value threw")
	}
}

// TestDefinePropertyNonWritableValue proves a non-writable, non-configurable data
// property rejects a value change but accepts rewriting the same value.
func TestDefinePropertyNonWritableValue(t *testing.T) {
	o := NewObject()
	o.DefineProperty(StringValue(FromGoString("a")), descObj("value", Number(1)))

	if !throws(func() {
		o.DefineProperty(StringValue(FromGoString("a")), descObj("value", Number(2)))
	}) {
		t.Fatal("changing a non-writable property's value did not throw")
	}
	if throws(func() {
		o.DefineProperty(StringValue(FromGoString("a")), descObj("value", Number(1)))
	}) {
		t.Fatal("rewriting a non-writable property's own value threw")
	}
}

// TestDefinePropertyWritableAllowsValue proves a writable but non-configurable
// data property still accepts a value change and even a downgrade to non-writable.
func TestDefinePropertyWritableAllowsValue(t *testing.T) {
	o := NewObject()
	o.DefineProperty(StringValue(FromGoString("a")), descObj("value", Number(1), "writable", True))

	if throws(func() {
		o.DefineProperty(StringValue(FromGoString("a")), descObj("value", Number(2)))
	}) {
		t.Fatal("changing a writable property's value threw")
	}
	if throws(func() {
		o.DefineProperty(StringValue(FromGoString("a")), descObj("writable", False))
	}) {
		t.Fatal("downgrading a writable property to non-writable threw")
	}
	if got := o.Get(FromGoString("a")); got.scalar != Number(2).scalar {
		t.Fatalf("value after writable change = %v, want 2", got)
	}
}

// TestGetOwnPropertyDescriptorData proves the descriptor read returns an object
// carrying the property's value and its writable, enumerable, and configurable
// flags, the shape verifyProperty reads its assertions from.
func TestGetOwnPropertyDescriptorData(t *testing.T) {
	o := NewObject()
	o.DefineProperty(StringValue(FromGoString("a")), descObj("value", Number(42), "enumerable", True, "configurable", True))

	d := o.GetOwnPropertyDescriptor(StringValue(FromGoString("a")))
	if got := d.Get(FromGoString("value")); got.scalar != Number(42).scalar {
		t.Fatalf("descriptor value = %v, want 42", got)
	}
	if d.Get(FromGoString("writable")).scalar != False.scalar {
		t.Fatal("writable = true, want false")
	}
	if d.Get(FromGoString("enumerable")).scalar != True.scalar {
		t.Fatal("enumerable = false, want true")
	}
	if d.Get(FromGoString("configurable")).scalar != True.scalar {
		t.Fatal("configurable = false, want true")
	}
}

// TestGetOwnPropertyDescriptorAbsent proves the descriptor read returns undefined
// for a key the object does not carry, the value verifyProperty treats as absence.
func TestGetOwnPropertyDescriptorAbsent(t *testing.T) {
	o := NewObject()
	if got := o.GetOwnPropertyDescriptor(StringValue(FromGoString("missing"))); got.kind != KindUndefined {
		t.Fatalf("descriptor of a missing key = %v, want undefined", got)
	}
}

// TestGetOwnPropertyDescriptorAccessor proves an accessor property reports its get
// and set rather than a value and writable pair.
func TestGetOwnPropertyDescriptorAccessor(t *testing.T) {
	o := NewObject()
	getter := NewFunc(func(args []Value) Value { return Number(7) })
	o.DefineProperty(StringValue(FromGoString("g")), descObj("get", getter, "enumerable", True))

	d := o.GetOwnPropertyDescriptor(StringValue(FromGoString("g")))
	if d.Get(FromGoString("get")).kind != KindFunc {
		t.Fatal("accessor descriptor is missing its getter")
	}
	if d.HasProperty(FromGoString("value")) {
		t.Fatal("accessor descriptor carries a value field, want none")
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
