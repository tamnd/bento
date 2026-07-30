package value

// This file is require('util'), the module whose most-used members bento now
// carries: format (what console.log's specifiers go through), inspect (what
// console.log prints), and isDeepStrictEqual (the deep comparison assert is built
// on). The first two arrived together because they are the same code, and they are
// the members a program reaches util for most often.
//
// The rest of util is not here yet, and the module says so rather than answering
// undefined. util.promisify needs promises, and util.inherits and util.types need
// prototypes and the boxed forms of Map, Set, Date and the typed arrays. Each of
// those is its own slice, and until then a read of one throws the honest-stub error
// naming the member, so a program that needs it fails where it needs it instead of at
// some later line where undefined turned out not to be a function.

// newUtilModule builds require('util') and require('node:util'). The members are set
// in the order Node's own util module defines them, so a program that enumerates the
// module sees them in a familiar order.
func newUtilModule() Value {
	mod := NewObject()
	set := func(name string, fn func([]Value) Value) {
		mod.Set(FromGoString(name), WithName(NewFunc(fn), name))
	}

	set("format", func(args []Value) Value {
		return StringValue(NodeFormat(args...))
	})
	set("formatWithOptions", func(args []Value) Value {
		if len(args) == 0 {
			// The options argument is validated even when it was not passed, which is
			// Node's ERR_INVALID_ARG_TYPE for undefined rather than a format of nothing.
			return StringValue(NodeFormatWithOptions(Undefined))
		}
		return StringValue(NodeFormatWithOptions(args[0], args[1:]...))
	})
	set("inspect", func(args []Value) Value {
		return StringValue(NodeInspectArgs(args...))
	})
	set("isDeepStrictEqual", func(args []Value) Value {
		return Bool(NodeIsDeepStrictEqual(args...))
	})

	return newPartialModule("util", mod)
}
