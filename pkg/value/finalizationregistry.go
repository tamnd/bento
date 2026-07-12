// This file owns FinalizationRegistry, the runtime type behind a JavaScript
// FinalizationRegistry<T> the lowerer reaches for when a program asks to be told after
// an object is collected (05_type_lowering the value model, 25 §26.2). A program
// registers a target object with a held value and an optional unregister token; once
// the target is collected the registry's cleanup callback is called with the held
// value, unless the registration was unregistered first.
//
// The mechanism is Go's runtime.AddCleanup, which runs a cleanup function after a
// pointer becomes unreachable. Register wires the target to AddCleanup with the held
// value as the argument and the registry's callback as the cleanup, and keeps the
// returned Cleanup handle keyed by the unregister token so unregister can Stop it.
// The exact turn a cleanup runs, and whether it runs at all before the process exits,
// is the garbage-collection-timing ceiling the milestone names: JavaScript runs
// cleanup callbacks as a host job on a later turn, and Go runs them on its own cleanup
// goroutine, so the operational surface (register and unregister) lowers exactly while
// the timing a test may pin does not.

package value

import "runtime"

// FinalizationRegistry is bento's runtime representation of a JavaScript
// FinalizationRegistry<T>. It holds the cleanup callback and the live registrations,
// each pairing an unregister token with the Cleanup handle AddCleanup returned, so a
// later unregister can find and stop the pending cleanup by its token.
type FinalizationRegistry[T any] struct {
	cleanup       func(T)
	registrations []frRegistration
}

// frRegistration records one live registration: the unregister token it was given
// (nil when none was passed, which no real token ever matches) and the Cleanup handle
// that unregister stops.
type frRegistration struct {
	token  any
	handle runtime.Cleanup
}

// NewFinalizationRegistry builds a registry with the given cleanup callback, the
// lowering of new FinalizationRegistry(cb). The callback is called with a registration's
// held value after that registration's target is collected.
func NewFinalizationRegistry[T any](cleanup func(T)) *FinalizationRegistry[T] {
	return &FinalizationRegistry[T]{cleanup: cleanup}
}

// FinalizationRegister registers target with the registry, the lowering of
// registry.register(target, held, token). It is a free function rather than a method
// because it is generic over the target's own type: the registry is generic only over
// the held-value type T, while a target may be any object, so the target type arrives
// at the call site the lowerer emits. It wires target to runtime.AddCleanup with held
// as the argument and the registry's callback as the cleanup, then records the returned
// handle under token so unregister can stop it. A nil token records a registration no
// unregister can target, which matches passing no unregister token.
func FinalizationRegister[Target any, T any](r *FinalizationRegistry[T], target *Target, held T, token any) {
	handle := runtime.AddCleanup(target, r.cleanup, held)
	r.registrations = append(r.registrations, frRegistration{token: token, handle: handle})
}

// Unregister removes every registration made under token, stopping its pending
// cleanup, and reports whether it removed any, the lowering of registry.unregister(token).
// Tokens are objects, so they compare by reference identity through ==.
func (r *FinalizationRegistry[T]) Unregister(token any) bool {
	removed := false
	kept := r.registrations[:0]
	for _, reg := range r.registrations {
		if reg.token != nil && reg.token == token {
			reg.handle.Stop()
			removed = true
			continue
		}
		kept = append(kept, reg)
	}
	r.registrations = kept
	return removed
}
