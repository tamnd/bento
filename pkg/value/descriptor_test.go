package value

import "testing"

// TestDataProperty proves a data descriptor carries its value and flags and reads
// its value back regardless of the receiver.
func TestDataProperty(t *testing.T) {
	d := dataProperty(Number(42), true, false, true)
	if d.isAccessor() {
		t.Fatal("data descriptor reported as accessor")
	}
	if !d.isData() {
		t.Fatal("data descriptor did not report as data")
	}
	if !d.writable || d.enumerable || !d.configurable {
		t.Fatalf("flags = w:%v e:%v c:%v, want w:true e:false c:true", d.writable, d.enumerable, d.configurable)
	}
	if got := d.read(Undefined); got.kind != KindNumber || got.scalar != Number(42).scalar {
		t.Fatalf("read = %v, want 42", got)
	}
}

// TestDefaultDataProperty proves a plain write's descriptor is writable,
// enumerable, and configurable, the attributes an ordinary assignment gives.
func TestDefaultDataProperty(t *testing.T) {
	d := defaultDataProperty(StringValue(FromGoString("x")))
	if !d.writable || !d.enumerable || !d.configurable {
		t.Fatalf("default flags = w:%v e:%v c:%v, want all true", d.writable, d.enumerable, d.configurable)
	}
	if d.isAccessor() {
		t.Fatal("default data descriptor reported as accessor")
	}
}

// TestAccessorProperty proves an accessor descriptor runs its getter on read and
// reports undefined when it has none.
func TestAccessorProperty(t *testing.T) {
	getter := NewFunc(func(args []Value) Value { return Number(7) })
	d := accessorProperty(getter, Undefined, true, true)
	if !d.isAccessor() {
		t.Fatal("accessor descriptor did not report as accessor")
	}
	if got := d.read(Undefined); got.kind != KindNumber || got.scalar != Number(7).scalar {
		t.Fatalf("accessor read = %v, want 7", got)
	}

	none := accessorProperty(Undefined, Undefined, true, true)
	if got := none.read(Undefined); !got.IsUndefined() {
		t.Fatalf("getter-less accessor read = %v, want undefined", got)
	}
}
