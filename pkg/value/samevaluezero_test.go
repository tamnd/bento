package value

import (
	"math"
	"testing"
)

// TestSameValueZeroDiffersFromStrictOnlyOnNaN pins the one place the two comparisons
// part. A Map keyed by SameValueZero holds a single NaN key, so a set with NaN and a
// later get with NaN find each other, which === would not give.
func TestSameValueZeroDiffersFromStrictOnlyOnNaN(t *testing.T) {
	if !SameValueZero(Number(math.NaN()), Number(math.NaN())) {
		t.Error("SameValueZero(NaN, NaN) = false, want true")
	}
	if !SameValueZero(Number(0), Number(math.Copysign(0, -1))) {
		t.Error("SameValueZero(0, -0) = false, want true")
	}
	if SameValueZero(Number(1), StringValue(FromGoString("1"))) {
		t.Error("SameValueZero(1, \"1\") = true, want false")
	}
	if SameValueZero(Undefined, Null) {
		t.Error("SameValueZero(undefined, null) = true, want false")
	}
}

// TestSameValueZeroComparesObjectsByIdentity pins the reference kinds: two objects
// with the same contents are two different keys, and one object is itself.
func TestSameValueZeroComparesObjectsByIdentity(t *testing.T) {
	a, b := NewObject(), NewObject()
	a.Set(FromGoString("x"), Number(1))
	b.Set(FromGoString("x"), Number(1))
	if !SameValueZero(a, a) {
		t.Error("an object is not the same value as itself")
	}
	if SameValueZero(a, b) {
		t.Error("two objects with equal contents compared the same")
	}
}

// TestDynMapHoldsEveryKeyKind pins the constructor a `new Map()` with no key type
// lowers to: one map, keys of different kinds, each found again.
func TestDynMapHoldsEveryKeyKind(t *testing.T) {
	m := NewDynMap[Value]()
	obj := NewObject()
	m.Set(StringValue(FromGoString("a")), Number(1))
	m.Set(Number(2), StringValue(FromGoString("two")))
	m.Set(obj, Bool(true))
	m.Set(Number(math.NaN()), Number(9))
	m.Set(Number(math.NaN()), Number(10))

	if got := m.Size(); got != 4 {
		t.Fatalf("size = %v, want 4 (the two NaN keys are one key)", got)
	}
	if got := m.Get(StringValue(FromGoString("a"))); got.IsUndefined() || got.Get().AsNumber() != 1 {
		t.Error("string key did not read back")
	}
	if got := m.Get(obj); got.IsUndefined() || !got.Get().AsBool() {
		t.Error("object key did not read back by identity")
	}
	if got := m.Get(Number(math.NaN())); got.IsUndefined() || got.Get().AsNumber() != 10 {
		t.Error("the NaN key did not take the second write")
	}
	if !m.Get(StringValue(FromGoString("missing"))).IsUndefined() {
		t.Error("an absent key reported present")
	}
}

// TestDynSetComparesBySameValueZero pins the Set half: 1 and "1" are two members, a
// repeat is not added twice, and NaN is a single member.
func TestDynSetComparesBySameValueZero(t *testing.T) {
	s := NewDynSet()
	s.Add(Number(1))
	s.Add(StringValue(FromGoString("1")))
	s.Add(Number(1))
	s.Add(Number(math.NaN()))
	s.Add(Number(math.NaN()))
	if got := s.Size(); got != 3 {
		t.Fatalf("size = %v, want 3", got)
	}
	if !s.Has(Number(math.NaN())) {
		t.Error("the NaN member was not found")
	}
}
