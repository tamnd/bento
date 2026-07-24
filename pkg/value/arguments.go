package value

// This file boxes the parameter snapshot a function body materializes for its
// arguments object into a dynamic value. A body that reads arguments builds a
// *Array[Value] store from its parameters at entry (the lowerer's argumentsPlan); the
// .length, arguments[i], and for..of arguments forms read that store directly, but a
// bare `arguments` used as a whole value (passed to a call, spread, assigned) needs a
// Value the dynamic model can carry. ArgumentsValue is that box.

// ArgumentsValue boxes the arguments snapshot store into a dynamic value, sharing the
// store's backing so an index read off the box sees the same slot the element path
// reads and the box reports the same .length. The box is an ordinary array value, so
// spread, indexing, .length, and Array.prototype.slice.call over it all read the same
// elements.
//
// The box is an array, so Array.isArray(arguments) and `arguments instanceof Array`
// report true where the real arguments exotic object reports false, and a property of
// the arguments object other than an index or length (callee, caller) is not modeled
// here; the lowerer hands those back rather than route them through this box. Closing
// that array-like-not-Array distinction is a later slice.
func ArgumentsValue(a *Array[Value]) Value {
	return objectValue(&Object{kind: KindArray, elems: a.elems})
}
