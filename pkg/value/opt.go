package value

// Opt is bento's runtime representation of the type T | undefined, the exact
// monomorphized optional 05_type_lowering.md reaches for when a value is a
// concrete type or the one missing value undefined. A method whose declared
// return is T | undefined (Array.prototype.pop, at, find) lowers to a function
// returning Opt[T], and a binding of that type is a Go variable of type Opt[T].
//
// It is deliberately not the fully boxed dynamic Value: when the only alternative
// to T is undefined, a present flag beside a T slot captures the whole type with
// no interface boxing and no allocation, so the optional path stays as cheap as
// the value it wraps. A genuine union of unlike types (the tagged sum) is a
// different representation and a later slice.
//
// The zero Opt[T] is the undefined case, which makes None[T]() a plain zero value
// and means a freshly declared optional reads as undefined before assignment,
// matching a JavaScript binding that has not been given a defined value.
type Opt[T any] struct {
	val     T
	present bool
}

// Some wraps a present value, the Opt an expression of type T flows into when the
// context wants T | undefined, and what a producer returns when it has a value.
func Some[T any](v T) Opt[T] {
	return Opt[T]{val: v, present: true}
}

// None is the undefined optional. It is the zero Opt[T], written as a function so
// the lowerer has a spelling for undefined at a known element type; the result is
// identical to the zero value a declared-but-unassigned optional already holds.
func None[T any]() Opt[T] {
	return Opt[T]{}
}

// IsUndefined reports whether the optional holds no value, the lowering of an
// x === undefined test (its negation lowers an x !== undefined test). It is the
// only way the generated code inspects an optional without first narrowing it, so
// a comparison against undefined never has to touch the wrapped slot.
func (o Opt[T]) IsUndefined() bool { return !o.present }

// Get returns the wrapped value, the lowering of a use of an optional binding at
// a point where control-flow narrowing has already proved it is present (past an
// x !== undefined guard). On an undefined optional it returns the zero T, so a
// use the checker has not narrowed does not panic; that path is unreachable in
// code the checker accepted, since it would be a use of a possibly-undefined value
// where T is required.
func (o Opt[T]) Get() T { return o.val }

// Or returns the wrapped value when present, otherwise the fallback, the lowering
// of a ?? b where a is T | undefined and b is T. For an optional the one nullish
// value is undefined, so present is exactly the "not nullish" test ?? runs. The
// fallback is passed by value, so the lowerer only emits this form when b is
// side-effect free; a pure b evaluated eagerly cannot be observed to run early,
// which keeps the short-circuit ?? gives observationally intact.
func (o Opt[T]) Or(fallback T) T {
	if o.present {
		return o.val
	}
	return fallback
}

// OrOpt returns the optional itself when present, otherwise the fallback optional,
// the lowering of a ?? b where both a and b are T | undefined, so the result is
// still optional. The same eager-evaluation rule as Or applies to the fallback.
func (o Opt[T]) OrOpt(fallback Opt[T]) Opt[T] {
	if o.present {
		return o
	}
	return fallback
}

// OptMap reads a member of the wrapped value when present and returns it as a new
// optional, the lowering of a single link of an optional chain a?.b: when a holds
// a value f reads the member off it, and when a is undefined the whole chain
// short-circuits to undefined and f never runs, so the member read is never
// reached on a missing receiver. It is a free function rather than a method
// because a method cannot introduce the second type parameter the mapped element
// needs. Longer chains compose by nesting: a?.b?.c lowers to OptMap over the
// OptMap that produced a?.b, each link mapping only when the one before it was
// present.
func OptMap[T, U any](o Opt[T], f func(T) U) Opt[U] {
	if o.present {
		return Some(f(o.val))
	}
	return None[U]()
}

// OptToValue boxes an optional into the dynamic Value the language sees when a
// T | undefined result flows into an any slot, the lowering of an optional passed
// where a boxed value is wanted (console.log of an array's at or pop, a member read
// the checker types number | undefined). A present value boxes through the element's
// own box constructor, which the caller supplies because the element type T is not
// itself a Value and only the call site knows how to wrap it; an undefined optional
// is the undefined singleton, the box the language already uses for a missing value.
func OptToValue[T any](o Opt[T], box func(T) Value) Value {
	if o.present {
		return box(o.val)
	}
	return Undefined
}
