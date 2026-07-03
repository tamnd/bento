package value

import (
	"math"
	"testing"
)

func TestNumberMapSetGetOverwrite(t *testing.T) {
	m := NewNumberMap[float64]()
	if got := m.Size(); got != 0 {
		t.Fatalf("empty map size = %v, want 0", got)
	}
	m.Set(1, 10).Set(2, 20)
	if got := m.Size(); got != 2 {
		t.Fatalf("size after two sets = %v, want 2", got)
	}
	if v := m.Get(1); v.IsUndefined() || v.Get() != 10 {
		t.Fatalf("get(1) = %+v, want present 10", v)
	}
	// An existing key updates in place and does not grow the map.
	m.Set(1, 99)
	if got := m.Size(); got != 2 {
		t.Fatalf("size after overwrite = %v, want 2", got)
	}
	if v := m.Get(1); v.Get() != 99 {
		t.Fatalf("get(1) after overwrite = %v, want 99", v.Get())
	}
	if v := m.Get(3); !v.IsUndefined() {
		t.Fatalf("get of an absent key = %+v, want undefined", v)
	}
}

func TestNumberMapNaNAndZeroKeys(t *testing.T) {
	m := NewNumberMap[float64]()
	nan := math.NaN()
	m.Set(nan, 1)
	m.Set(nan, 2) // SameValueZero: every NaN is the same key, so this overwrites.
	if got := m.Size(); got != 1 {
		t.Fatalf("size after two NaN sets = %v, want 1", got)
	}
	if v := m.Get(nan); v.IsUndefined() || v.Get() != 2 {
		t.Fatalf("get(NaN) = %+v, want present 2", v)
	}
	// +0 and -0 are the same key under SameValueZero.
	m.Set(0, 5)
	m.Set(math.Copysign(0, -1), 6)
	if got := m.Size(); got != 2 {
		t.Fatalf("size after +0 and -0 sets = %v, want 2 (one NaN, one zero)", got)
	}
	if v := m.Get(0); v.Get() != 6 {
		t.Fatalf("get(+0) = %v, want 6 written through -0", v.Get())
	}
}

func TestStringMapKeyIdentity(t *testing.T) {
	m := NewStringMap[float64]()
	// Two strings built differently but equal by code units are the same key.
	m.Set(FromGoString("ab"), 1)
	joined := FromGoString("a").ConcatN(FromGoString("b"))
	m.Set(joined, 2)
	if got := m.Size(); got != 1 {
		t.Fatalf("size after equal-key sets = %v, want 1", got)
	}
	if v := m.Get(FromGoString("ab")); v.Get() != 2 {
		t.Fatalf("get(ab) = %v, want 2", v.Get())
	}
}

func TestMapHasDeleteOrder(t *testing.T) {
	m := NewStringMap[float64]()
	m.Set(FromGoString("a"), 1)
	m.Set(FromGoString("b"), 2)
	m.Set(FromGoString("c"), 3)
	if !m.Has(FromGoString("b")) {
		t.Fatal("has(b) = false, want true")
	}
	if !m.Delete(FromGoString("b")) {
		t.Fatal("delete(b) = false, want true")
	}
	if m.Has(FromGoString("b")) {
		t.Fatal("has(b) after delete = true, want false")
	}
	if m.Delete(FromGoString("b")) {
		t.Fatal("delete(b) twice = true, want false")
	}
	// The remaining entries keep insertion order.
	var keys []string
	var vals []float64
	m.Range(func(k BStr, v float64) {
		keys = append(keys, k.ToGoString())
		vals = append(vals, v)
	})
	if len(keys) != 2 || keys[0] != "a" || keys[1] != "c" {
		t.Fatalf("range keys = %v, want [a c]", keys)
	}
	if vals[0] != 1 || vals[1] != 3 {
		t.Fatalf("range vals = %v, want [1 3]", vals)
	}
}

func TestMapClear(t *testing.T) {
	m := NewBoolMap[float64]()
	m.Set(true, 1)
	m.Set(false, 2)
	m.Clear()
	if got := m.Size(); got != 0 {
		t.Fatalf("size after clear = %v, want 0", got)
	}
	if v := m.Get(true); !v.IsUndefined() {
		t.Fatalf("get after clear = %+v, want undefined", v)
	}
	// A refill after clear works and reuses the storage.
	m.Set(true, 7)
	if v := m.Get(true); v.Get() != 7 {
		t.Fatalf("get after refill = %v, want 7", v.Get())
	}
}
