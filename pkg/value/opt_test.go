package value

import "testing"

// TestSome pins that a present optional reports defined and yields its value.
func TestSome(t *testing.T) {
	o := Some[float64](42)
	if o.IsUndefined() {
		t.Error("Some(42).IsUndefined() = true, want false")
	}
	if o.Get() != 42 {
		t.Errorf("Some(42).Get() = %v, want 42", o.Get())
	}
}

// TestNone pins that the undefined optional reports undefined and that Get
// returns the zero value rather than panicking on it.
func TestNone(t *testing.T) {
	o := None[float64]()
	if !o.IsUndefined() {
		t.Error("None().IsUndefined() = false, want true")
	}
	if o.Get() != 0 {
		t.Errorf("None().Get() = %v, want 0 (zero value)", o.Get())
	}
}

// TestOptZeroIsUndefined pins that a freshly declared optional, the Go zero
// Opt[T], reads as undefined, matching a binding not yet given a defined value.
func TestOptZeroIsUndefined(t *testing.T) {
	var o Opt[BStr]
	if !o.IsUndefined() {
		t.Error("zero Opt[BStr].IsUndefined() = false, want true")
	}
}

// TestSomeString pins the optional at a non-numeric element type, the string
// case the lowerer emits as Opt[BStr].
func TestSomeString(t *testing.T) {
	o := Some(FromGoString("hi"))
	if o.IsUndefined() {
		t.Fatal("Some(\"hi\").IsUndefined() = true, want false")
	}
	if got := o.Get().ToGoString(); got != "hi" {
		t.Errorf("Some(\"hi\").Get() = %q, want \"hi\"", got)
	}
}

// TestOptToValuePresent pins that a present optional boxes through the supplied
// element constructor: a present number becomes a number Value the dynamic slot
// can carry.
func TestOptToValuePresent(t *testing.T) {
	got := OptToValue(Some(53.0), Number)
	if got.IsUndefined() {
		t.Fatal("OptToValue(Some(53)) = undefined, want a number")
	}
	if s := ToString(got).ToGoString(); s != "53" {
		t.Errorf("OptToValue(Some(53)) renders %q, want \"53\"", s)
	}
}

// TestOptToValueUndefined pins that an undefined optional boxes to the undefined
// singleton rather than the element's zero, so a missing value reads as undefined
// in the dynamic slot.
func TestOptToValueUndefined(t *testing.T) {
	got := OptToValue(None[float64](), Number)
	if !got.IsUndefined() {
		t.Errorf("OptToValue(None) = %v, want undefined", got)
	}
}
