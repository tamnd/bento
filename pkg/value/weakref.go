// This file owns WeakRef, the runtime type behind a JavaScript WeakRef<T> the lowerer
// reaches for when a program holds a single object weakly and reads it back with deref
// (05_type_lowering the value model, 25 §26.1). A WeakRef does not keep its target
// alive: once the last strong reference elsewhere is gone the runtime may collect the
// target, and deref then returns undefined instead of the object. Before that point
// deref returns the same object every time, so a live target is stable across reads.
//
// The target is an object, which lowers to a Go struct pointer, so the type is generic
// over the pointee T and holds the target as a weak.Pointer[T]. weak.Pointer.Value
// reads back the strong pointer while the object lives and nil after it is collected,
// which is exactly the object-or-undefined deref result. The exact turn the target
// becomes collectable is the garbage-collection-timing ceiling the milestone names.

package value

import "weak"

// WeakRef is bento's runtime representation of a JavaScript WeakRef<*T>. It holds a
// single weak.Pointer[T] to the target, which does not extend the target's lifetime.
type WeakRef[T any] struct {
	p weak.Pointer[T]
}

// NewWeakRef builds a WeakRef to target, the lowering of new WeakRef(target). The
// target is wrapped weakly, so the reference alone does not keep it alive.
func NewWeakRef[T any](target *T) *WeakRef[T] {
	return &WeakRef[T]{p: weak.Make(target)}
}

// Deref returns the target as an optional, the object while it lives and undefined
// once it has been collected, the lowering of weakRef.deref() whose declared type is
// T | undefined.
func (w *WeakRef[T]) Deref() Opt[*T] {
	if v := w.p.Value(); v != nil {
		return Some(v)
	}
	return None[*T]()
}
