package value

import "testing"

// A boxed arguments snapshot is an array value: typeof "object", it reports the
// snapshot's length, answers each index, and shares the store's backing so an index
// read off the box sees the same slot the store holds.
func TestArgumentsValue(t *testing.T) {
	store := NewArray[Value](Number(10), Number(20), Number(30))
	box := ArgumentsValue(store)

	if got := box.TypeOf().ToGoString(); got != "object" {
		t.Fatalf("typeof boxed arguments = %q, want object", got)
	}
	if got := ToNumber(box.Get(FromGoString("length"))); got != 3 {
		t.Fatalf(".length = %v, want 3", got)
	}
	for i, want := range []float64{10, 20, 30} {
		if got := ToNumber(box.GetIndex(float64(i))); got != want {
			t.Fatalf("[%d] = %v, want %v", i, got, want)
		}
	}
}

// The box shares the store's backing: a write through the store is visible off the box,
// the aliasing the snapshot's element path and a bare value read must agree on.
func TestArgumentsValueSharesBacking(t *testing.T) {
	store := NewArray[Value](Number(1), Number(2))
	box := ArgumentsValue(store)
	store.Set(0, Number(99))
	if got := ToNumber(box.GetIndex(0)); got != 99 {
		t.Fatalf("box did not see the store write: [0] = %v, want 99", got)
	}
}
