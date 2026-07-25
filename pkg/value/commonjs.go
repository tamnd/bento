package value

// This file is the runtime side of the CommonJS module-wrapper globals. The
// lowerer models module, exports, __dirname, and __filename inline (they are a
// value object and two compile-time strings), but require is a live function the
// compiled module holds and calls, so it lives here as a callable value.

// RequireFunc returns the CommonJS require function as a callable value, the box a
// module's require binding takes. A require of a specifier the compiler resolved to
// a sibling module lowers to a direct call on that module's loader, not through this
// value; this value is what a bare require reference, a typeof require, or a require
// of a specifier the compiler could not resolve statically evaluates to. Such a call
// throws the error Node raises for a specifier it cannot resolve, so a program that
// probes require works (typeof require is "function", and require can be passed
// around and stored) while a dynamic or missing require fails honestly rather than
// resolving to a silent wrong value. The specifier is coerced to a string the way
// Node coerces its argument, and the message matches Node's exactly so a test that
// asserts on err.message compares equal.
func RequireFunc() Value {
	return NewFunc(func(args []Value) Value {
		specifier := ToString(Arg(args, 0)).ToGoString()
		Throw(NewError(FromGoString("Cannot find module '" + specifier + "'")))
		return Undefined
	})
}

// ModuleSlot is the per-module cache the compiled CommonJS loader guards its body
// with, so a module required more than once runs its body once and every require
// returns the one exports value. Each required module emits one package-level slot
// and one loader function; the loader consults the slot before running the body,
// which is what makes require idempotent the way Node's module cache is.
type ModuleSlot struct {
	loaded  bool
	exports Value
}

// NewModuleSlot returns an empty cache slot, the zero state a module holds before
// its loader first runs.
func NewModuleSlot() *ModuleSlot { return &ModuleSlot{} }

// Loaded reports whether the module's body has already started running, so the
// loader can return the cached exports instead of running the body again. It is
// true from the moment Init runs, before the body finishes, so a circular require
// that re-enters the loader mid-body sees a loaded slot and takes the cached path.
func (s *ModuleSlot) Loaded() bool { return s.loaded }

// Exports returns the module's current exports, the value a repeated or re-entrant
// require resolves to. During the body it is the initial exports object, so a
// circular require observes the exports built so far; after the body it is whatever
// module.exports finally names. This is exactly Node's rule that a cyclic require
// yields the partially populated exports rather than looping.
func (s *ModuleSlot) Exports() Value { return s.exports }

// Init marks the slot loaded and builds the module object with a fresh exports
// object, caching that exports object before the body runs so a circular require
// re-entering the loader returns the partial exports. It returns the module object
// the loader binds its module local to; the exports local reads module.exports off
// it.
func (s *ModuleSlot) Init() Value {
	s.loaded = true
	module := NewObject()
	module.Set(FromGoString("exports"), NewObject())
	s.exports = module.Get(FromGoString("exports"))
	return module
}

// Finish caches the module's final exports, read back off the module object so a
// body that reassigned module.exports wholesale returns the new value rather than
// the initial object, and returns it as the loader's result. A body that only
// mutated exports leaves module.exports naming the initial object, so the cached
// value is unchanged from Init.
func (s *ModuleSlot) Finish(module Value) Value {
	s.exports = module.Get(FromGoString("exports"))
	return s.exports
}
