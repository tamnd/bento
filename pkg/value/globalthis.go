package value

// This file is the runtime side of the globalThis global, the object JavaScript
// names the global scope by. A Node program reads it three ways: as a namespace
// for a global it wants unambiguously (globalThis.process, the first thing
// test/common does), as a dictionary keyed at run time (globalThis[name], the
// shape a leak check over a list of names takes), and as a bag it writes its own
// entries into (globalThis.foo = 1, a test making a value visible to a module it
// loads later).
//
// So the object is real storage, built once and cached, and the lowerer emits one
// package-level call to it per program that names it.
//
// What the object holds is what bento models. A global bento hosts is on it, and
// the identity is the one the bare name reads: globalThis.process === process
// holds because both reach the one object ProcessValue caches. A global bento
// does not host is not on it, and the lowerer refuses at compile time rather than
// let the read answer undefined for something Node has, so nothing here stands in
// for an engine surface bento has not built.
//
// The entries bento puts on it are non-enumerable, which is how a host installs a
// global and what makes `for (const k in globalThis)` list only what the program
// itself assigned. test/common's leak check is exactly that loop, so the flag is
// the difference between a clean run and every test reporting bento's own globals
// as leaks.

// globalObject caches the one global object. A second read of the name returns
// the same value the first built, so the identity globalThis === globalThis holds
// and a property a program sets on it is still there on the next read.
var globalObject Value

// GlobalThisValue returns the globalThis global, building it on first read.
func GlobalThisValue() Value {
	if globalObject.Kind() != KindUndefined {
		return globalObject
	}
	g := NewObject()
	// globalThis names itself, so the property is installed on the object it points
	// at, after the object exists. process and console are the Node globals bento
	// hosts as values today; the list grows as the lowerer learns to host another.
	defineGlobal(g, "globalThis", g)
	defineGlobal(g, "process", ProcessValue())
	defineGlobal(g, "console", ConsoleObject())
	globalObject = g
	return globalObject
}

// defineGlobal installs a host global on the global object: writable and
// configurable, the way a host property can be replaced or deleted, but not
// enumerable, so a for...in over globalThis lists only what the program assigned.
// A plain Set would make it enumerable, which is why this reaches the descriptor
// storage rather than going through the ordinary write path.
func defineGlobal(g Value, name string, val Value) {
	o := g.object()
	o.keys = append(o.keys, FromGoString(name))
	o.descs = append(o.descs, dataProperty(val, true, false, true))
}
