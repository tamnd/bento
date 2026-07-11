package value

import "unsafe"

// This file is the value-model side of a built-in error constructor named as a
// value: TypeError passed as an argument, RangeError compared for identity, or
// Error.name read for its string. A constructor is a first-class object in
// JavaScript, so the dynamic world needs a value for it that answers .name, reports
// typeof "function", and compares equal to itself. The prelude's assert.throws is
// the driving case: it takes an error constructor as an argument, compares it
// against a caught error's own constructor, and reads .name off both for the
// failure message.
//
// The family is modeled as one interned value per built-in name, mirroring how the
// Error value models the whole error family by a name field rather than distinct Go
// types. Interning is what makes identity hold: two references to TypeError box to
// the same object pointer, so TypeError === TypeError is true, and a caught
// TypeError's .constructor is the same value the program compares it against.

// errorCtorNames is the set of standard error constructors the value form covers.
// It is broader than the errorCtors set new lowers, because boxing a constructor as
// a value and reading its name work off the name alone and need no construction
// path. The names are the ECMAScript standard error constructors plus the two the
// runtime raises from the host boundary (URIError, and the DOM InvalidCharacterError
// a caught base64 failure reports), so a program that names any of them as a value
// gets a matching constructor.
var errorCtorNames = []string{
	"Error",
	"TypeError",
	"RangeError",
	"SyntaxError",
	"ReferenceError",
	"EvalError",
	"URIError",
	"AggregateError",
}

// errorCtorByName interns one constructor value per built-in error name. It is
// built once at package initialization and never written after, so the concurrent
// reads a running program makes need no lock. The value is a KindFunc carrying a
// single own property, name, so typeof reports "function" and a .name read finds
// the constructor's name string.
var errorCtorByName = buildErrorCtors()

func buildErrorCtors() map[string]Value {
	m := make(map[string]Value, len(errorCtorNames))
	for _, name := range errorCtorNames {
		m[name] = newErrorCtorValue(name)
	}
	return m
}

// newErrorCtorValue builds one constructor value: a function-kind box over an
// object that holds the name property, the one own property a caught-error test
// reads. The property is stored directly rather than through Set so the value is
// ready to intern with no further mutation.
func newErrorCtorValue(name string) Value {
	o := &Object{
		kind:  KindFunc,
		keys:  []BStr{FromGoString("name")},
		descs: []descriptor{defaultDataProperty(StringValue(FromGoString(name)))},
	}
	return Value{kind: KindFunc, ref: unsafe.Pointer(o)}
}

// ErrorConstructor returns the constructor value for a built-in error name, the
// lowering of naming TypeError or one of its siblings as a value. A known name
// returns the interned singleton, so repeated references share identity the way a
// single global constructor does. An unknown name (a custom error class, which the
// class slice will own) returns a fresh constructor carrying that name, so .name
// still reads correctly; the identity of a fresh value is per call, a deviation the
// class slice removes once it interns user constructors.
func ErrorConstructor(name string) Value {
	if v, ok := errorCtorByName[name]; ok {
		return v
	}
	return newErrorCtorValue(name)
}

// Constructor reports the caught error's constructor as a value, the lowering of a
// caught error's .constructor. The runtime models the error family by name, so the
// constructor is the interned value for that name: a caught TypeError answers the
// same TypeError value the program compares it against, which is what makes
// thrown.constructor === TypeError hold.
func (e *Error) Constructor() Value {
	return ErrorConstructor(e.name.ToGoString())
}
