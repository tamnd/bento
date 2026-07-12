package value

import "testing"

// A symbol is unique by identity: two symbols with the same description are never
// equal, and each is equal only to itself.
func TestSymbolIdentity(t *testing.T) {
	a := NewSymbol(FromGoString("k"))
	b := NewSymbol(FromGoString("k"))
	if StrictEquals(a, b) {
		t.Fatal("two symbols with the same description compared equal")
	}
	if !StrictEquals(a, a) {
		t.Fatal("a symbol did not compare equal to itself")
	}
	if a.Kind() != KindSymbol {
		t.Fatalf("Kind = %v, want KindSymbol", a.Kind())
	}
}

// The description reads back as the string it was created with, and a symbol made
// without a description reads back undefined, keeping Symbol() apart from
// Symbol("").
func TestSymbolDescription(t *testing.T) {
	withDesc := NewSymbol(FromGoString("tag"))
	if got := withDesc.SymbolDescription(); got.kind != KindString || got.str().ToGoString() != "tag" {
		t.Fatalf("description = %v, want \"tag\"", got)
	}
	empty := NewSymbol(FromGoString(""))
	if got := empty.SymbolDescription(); got.kind != KindString || got.str().ToGoString() != "" {
		t.Fatalf("Symbol(\"\").description = %v, want empty string", got)
	}
	none := NewSymbolNoDesc()
	if got := none.SymbolDescription(); !got.IsUndefined() {
		t.Fatalf("Symbol().description = %v, want undefined", got)
	}
}

// The global registry interns one symbol per key: Symbol.for returns the same
// reference for an equal key, a different key yields a distinct symbol, a fresh
// Symbol never joins the registry, and the registered symbol's description is its
// key. Symbol.keyFor reads the key back for a registered symbol and reports
// undefined for one that never entered the registry.
func TestSymbolRegistry(t *testing.T) {
	a := SymbolFor(FromGoString("shared"))
	b := SymbolFor(FromGoString("shared"))
	if !StrictEquals(a, b) {
		t.Fatal("Symbol.for returned distinct symbols for the same key")
	}
	other := SymbolFor(FromGoString("other"))
	if StrictEquals(a, other) {
		t.Fatal("Symbol.for shared a symbol across different keys")
	}
	fresh := NewSymbol(FromGoString("shared"))
	if StrictEquals(a, fresh) {
		t.Fatal("a fresh Symbol compared equal to a registered one")
	}
	if got := a.SymbolDescription(); got.kind != KindString || got.str().ToGoString() != "shared" {
		t.Fatalf("registered symbol description = %v, want \"shared\"", got)
	}

	if got := SymbolKeyFor(a); got.kind != KindString || got.str().ToGoString() != "shared" {
		t.Fatalf("Symbol.keyFor(a) = %v, want \"shared\"", got)
	}
	if got := SymbolKeyFor(fresh); !got.IsUndefined() {
		t.Fatalf("Symbol.keyFor(fresh) = %v, want undefined", got)
	}
}

// The property bag keys by symbol identity: a symbol write reads back through the
// same symbol, a different symbol of the same description misses, and a symbol key
// never collides with a string key.
func TestObjectSymbolBag(t *testing.T) {
	o := NewObject()
	s := NewSymbol(FromGoString("k"))
	other := NewSymbol(FromGoString("k"))

	o.SetElem(s, Number(1))
	if got := o.GetElem(s); got.kind != KindNumber || got.AsNumber() != 1 {
		t.Fatalf("o[s] = %v, want 1", got)
	}
	if got := o.GetElem(other); !got.IsUndefined() {
		t.Fatalf("o[other] = %v, want undefined", got)
	}

	// A string key that spells the same thing must not read the symbol slot.
	o.SetKey(FromGoString("k"), Number(2))
	if got := o.GetElem(s); got.AsNumber() != 1 {
		t.Fatalf("symbol slot clobbered by string key: o[s] = %v, want 1", got)
	}
	if got := o.Get(FromGoString("k")); got.AsNumber() != 2 {
		t.Fatalf("o.k = %v, want 2", got)
	}
}

// The symbol bag grows and shrinks at runtime: a delete removes the symbol slot
// and reports true, and a later read misses.
func TestObjectSymbolDelete(t *testing.T) {
	o := NewObject()
	s := NewSymbol(FromGoString("k"))
	o.SetElem(s, Number(1))
	if !o.DeleteElem(s) {
		t.Fatal("DeleteElem returned false for a configurable symbol property")
	}
	if got := o.GetElem(s); !got.IsUndefined() {
		t.Fatalf("o[s] after delete = %v, want undefined", got)
	}
	// Deleting an absent symbol still reports true.
	if !o.DeleteElem(s) {
		t.Fatal("DeleteElem returned false for an absent symbol property")
	}
}
