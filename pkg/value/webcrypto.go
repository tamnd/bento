package value

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
)

// This file is the runtime side of the crypto global, the WebCrypto surface Node
// installs on the global object and its own test/common reads at load. Three
// members carry the weight: subtle, getRandomValues and randomUUID, which is the
// whole of Crypto.prototype in Node.
//
// Two of the three are real here. getRandomValues fills an integer typed array
// from the operating system's entropy and hands the same array back, and
// randomUUID mints a version 4 UUID from the same source. Both are synchronous and
// both are exactly what the name promises, so there is nothing to approximate.
//
// subtle is the one that is not. Every method on SubtleCrypto answers a Promise,
// and a promise in this package is a *Promise[T], a static shape with no dynamic
// value form: nothing here can hand a compiled program a boxed promise. An object
// carrying a then would compile and would even work for a .then call, but `await`
// of it would not unwrap, so a program that awaited a digest would get the wrapper
// object rather than the bytes. That is a wrong answer rather than a refusal, so
// the methods are present, which is what keeps typeof crypto.subtle.digest reading
// "function" the way it does in Node, and each throws when it is called. They come
// back one at a time once a promise has a value form.
//
// The object is real storage built once and cached, the same as the process and
// console globals, so crypto === globalThis.crypto holds and a property a program
// puts on it stays there.

// cryptoInstance caches the one crypto object, so every reference reaches the same
// value and the identity a known-globals set is built on holds.
var cryptoInstance Value

// CryptoValue returns the crypto global as an object, building it on first read.
// The members are own properties on the object, where Node carries them on
// Crypto.prototype; that is the same shape the other host classes here take
// (abort.go, event.go), and it changes a read of one only under
// Object.getOwnPropertyNames.
func CryptoValue() Value {
	if cryptoInstance.Kind() != KindUndefined {
		return cryptoInstance
	}
	c := NewObject()
	c.Set(FromGoString("subtle"), subtleCryptoValue())
	c.Set(FromGoString("getRandomValues"), NewFunc(func(args []Value) Value {
		return cryptoGetRandomValues(Arg(args, 0))
	}))
	c.Set(FromGoString("randomUUID"), NewFunc(func(args []Value) Value {
		return StringValue(FromGoString(randomUUIDString()))
	}))
	cryptoInstance = c
	return cryptoInstance
}

// cryptoMaxRandomBytes is the ceiling WebCrypto puts on one getRandomValues call.
// A request past it throws rather than being served, which is the spec's way of
// keeping a single call from draining the entropy pool.
const cryptoMaxRandomBytes = 65536

// cryptoGetRandomValues fills an integer typed array with random bytes and returns
// the same array, so `crypto.getRandomValues(a) === a` holds and a caller that
// ignores the return still sees its array filled.
//
// The two refusals are the spec's. A view that is not an integer-type typed array,
// which means a Float16Array, a Float32Array, a Float64Array, a DataView or
// anything that is not a view at all, is a TypeMismatchError, and a view longer
// than 65,536 bytes is a QuotaExceededError. Both are DOMExceptions in Node and
// name-carrying errors here, the same stand-in newAbortError takes.
func cryptoGetRandomValues(v Value) Value {
	backing := typedArrayBackingOf(v)
	if backing == nil || !isIntegerTypedArrayName(backing.jsTypedName()) {
		Throw(newDOMException("TypeMismatchError",
			"The data argument must be an integer-type TypedArray"))
	}
	bytes := typedArrayBytes(backing)
	if len(bytes) > cryptoMaxRandomBytes {
		Throw(newDOMException("QuotaExceededError",
			"The requested length exceeds 65,536 bytes"))
	}
	fillRandom(bytes)
	return v
}

// typedArrayBackingOf returns the typed-array backing behind a value, or nil when the
// value is not one. It reaches the brand field rather than asking the member
// surface, because what getRandomValues needs is the bytes under the view and not
// anything the prototype answers.
func typedArrayBackingOf(v Value) typedArrayBacking {
	if v.Kind() != KindObject {
		return nil
	}
	return v.object().jsTyped
}

// isIntegerTypedArrayName reports whether a typed array's constructor name is one
// of the integer kinds. Every member of the family is an integer kind except the
// three floating-point ones, so the test names those and admits the rest, which is
// what keeps a kind added later on the accepting side by default the way the spec
// has it.
func isIntegerTypedArrayName(name string) bool {
	switch name {
	case "Float16Array", "Float32Array", "Float64Array":
		return false
	}
	return true
}

// randomUUIDString mints a version 4 UUID, the string crypto.randomUUID answers:
// sixteen random bytes with the version nibble set to 4 and the variant bits to
// the RFC 4122 pattern, rendered lowercase in the 8-4-4-4-12 grouping.
func randomUUIDString() string {
	var b [16]byte
	fillRandom(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b[:])
	var out strings.Builder
	out.Grow(36)
	for i, n := range []int{8, 4, 4, 4, 12} {
		if i > 0 {
			out.WriteByte('-')
		}
		out.WriteString(h[:n])
		h = h[n:]
	}
	return out.String()
}

// fillRandom fills a byte slice from the operating system's entropy source. A read
// from it cannot fail on any platform bento builds for, and Go's own contract is
// that it panics rather than returning short, so there is no error to hand a
// program here and nothing to fall back to.
func fillRandom(b []byte) {
	if len(b) == 0 {
		return
	}
	if _, err := rand.Read(b); err != nil {
		Throw(newDOMException("OperationError", "The random source is unavailable"))
	}
}

// subtleCryptoNames is SubtleCrypto's method surface, the names Node carries on
// SubtleCrypto.prototype. Every one of them answers a Promise, which is why every
// one of them throws here; the list is what makes a typeof read of any of them
// answer "function" the way it does in Node instead of "undefined".
var subtleCryptoNames = []string{
	"encrypt", "decrypt", "sign", "verify", "digest",
			"generateKey", "deriveKey", "deriveBits",
	"importKey", "exportKey", "wrapKey", "unwrapKey",
	"getPublicKey", "encapsulateBits", "encapsulateKey",
	"decapsulateBits", "decapsulateKey",
}

// subtleCryptoInstance caches the one subtle object, so crypto.subtle read twice
// is the same object.
var subtleCryptoInstance Value

// subtleCryptoValue returns the SubtleCrypto object crypto.subtle holds, building
// it on first read. Each member throws when called, naming itself, so a program
// that reaches one fails where it asked rather than somewhere later holding
// something that is not a promise.
func subtleCryptoValue() Value {
	if subtleCryptoInstance.Kind() != KindUndefined {
		return subtleCryptoInstance
	}
	s := NewObject()
	for _, name := range subtleCryptoNames {
		s.Set(FromGoString(name), NewFunc(notModeledSubtleMethod(name)))
	}
	subtleCryptoInstance = s
	return subtleCryptoInstance
}

// notModeledSubtleMethod builds the body of a SubtleCrypto method bento has not
// built. It throws rather than answering, because what it would have to answer is
// a promise and there is no dynamic promise value to answer with.
func notModeledSubtleMethod(name string) func([]Value) Value {
	return func([]Value) Value {
		Throw(NewTypeError(FromGoString("crypto.subtle." + name +
			" answers a promise, which bento does not model as a value yet")))
		return Undefined
	}
}
