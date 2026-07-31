package value

import "testing"

// A collection's element can itself be a collection. The element boxing is the runtime's
// own dispatch rather than a boxer handed in from the call site, since a boxed Map
// reaches its values with nothing but their Go type in hand.

// TestANestedCollectionBoxesAsAView holds the read half. The inner map's box is the view
// kept on the map itself, so the outer box shows what the inner map holds now rather than
// what it held when the outer one was boxed.
func TestANestedCollectionBoxesAsAView(t *testing.T) {
	inner := NewStringMap[float64]()
	inner.Set(FromGoString("a"), 1)

	outer := NewStringMap[*Map[BStr, float64]]()
	outer.Set(FromGoString("k"), inner)

	if got := NodeInspect(outer.ToValue()).ToGoString(); got != "Map(1) { 'k' => Map(1) { 'a' => 1 } }" {
		t.Errorf("a map of maps renders as %s", got)
	}
	inner.Set(FromGoString("b"), 2)
	want := "Map(1) { 'k' => Map(2) { 'a' => 1, 'b' => 2 } }"
	if got := NodeInspect(outer.ToValue()).ToGoString(); got != want {
		t.Errorf("a map of maps after the inner map grew renders as %s, want %s", got, want)
	}
}

// TestANestedCollectionUnboxesToItself holds the direction a key needs. A Map keyed by
// Maps finds its entry again because the box carries the backing, so the value handed to
// has comes back out as the very map the outer one is keyed by.
func TestANestedCollectionUnboxesToItself(t *testing.T) {
	inner := NewStringMap[float64]()
	box := dynBox(inner)

	got, ok := dynUnbox[*Map[BStr, float64]](box)
	if !ok {
		t.Fatal("a boxed map did not unbox")
	}
	if got != inner {
		t.Error("a boxed map unboxed to another map")
	}

	set := NewRefSet[float64]()
	sbox := dynBox(set)
	if s, ok := dynUnbox[*Set[float64]](sbox); !ok || s != set {
		t.Error("a boxed set did not unbox to itself")
	}
	// A map is not a set and does not unbox as one, the same rule every other element
	// kind follows: a collection holds one kind of member and could never have held this.
	if _, ok := dynUnbox[*Set[float64]](box); ok {
		t.Error("a boxed map unboxed as a set")
	}
}

// TestANestedArrayBoxesAsACopy pins the one element whose box is not a view. An array's
// box holds its own elements, so a write through it does not reach the typed array; that
// is what its box does everywhere and not something nesting changes. It is also why an
// array is kept out of the two positions a collection is found again by, a Map's key and
// a Set's member, where the copy would be a wrong answer rather than a lost write.
func TestANestedArrayBoxesAsACopy(t *testing.T) {
	a := ArrayFrom([]float64{1, 2})
	box := dynBox(a)

	if got := NodeInspect(box).ToGoString(); got != "[ 1, 2 ]" {
		t.Errorf("a boxed array renders as %s", got)
	}
	if _, ok := dynUnbox[*Array[float64]](box); ok {
		t.Error("a boxed array unboxed to the typed array, which it carries no pointer to")
	}
}
