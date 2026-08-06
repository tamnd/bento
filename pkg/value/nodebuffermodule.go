// This file is require('buffer'), the module half of the Buffer slice. The global and
// the module export the same constructor, so `globalThis.Buffer === require('buffer')
// .Buffer` holds, which is a thing test/common checks directly.
//
// What the module carries beyond the constructor is small and mostly constants: the two
// size limits a program checks an allocation against, the base64 pair the web platform
// spells atob and btoa, and the two validators that answer whether a byte run is
// well-formed text. Everything else in Node's buffer module (Blob, File, transcode,
// resolveObjectURL, SlowBuffer's deprecated pooling behaviour) is not built, so it is
// left off and the partial-module wrapper raises the honest-stub error naming it rather
// than answering undefined.

package value

import (
	"unicode/utf8"
)

// newBufferModule builds require('buffer'). It is a partial module: the members below
// are real, and a read of any other member raises rather than reading undefined, which
// is the rule the rest of the built-in registry follows.
func newBufferModule() Value {
	mod := NewObject()
	mod.Set(FromGoString("Buffer"), BufferConstructor())
	// SlowBuffer is Node's un-pooled allocator, deprecated and still exported. bento never
	// pools, so every buffer is already a "slow" one and the export is allocUnsafeSlow
	// under its old name.
	mod.Set(FromGoString("SlowBuffer"), WithName(NewFunc(func(args []Value) Value {
		return NewNodeBuffer(bufferSizeArg(Arg(args, 0))).ToValue()
	}), "SlowBuffer"))
	mod.Set(FromGoString("kMaxLength"), Number(bufferMaxLength))
	mod.Set(FromGoString("kStringMaxLength"), Number(bufferMaxStringLength))

	// INSPECT_MAX_BYTES is how many bytes console.log of a buffer prints before it says how
	// many it left out, and it is the one member here a program writes rather than reads: a
	// program dumping packets raises it to see whole frames. So it is an accessor pair over
	// the variable the renderer reads rather than a copy of its value, which would answer
	// the right number and change nothing when set.
	mod.object().defineOwn(FromGoString("INSPECT_MAX_BYTES"), accessorProperty(
		NewFunc(func(args []Value) Value { return Number(float64(bufferInspectMaxBytes)) }),
		NewFunc(func(args []Value) Value {
			bufferInspectMaxBytes = int(ToInt32(ToNumber(Arg(args, 0))))
			return Undefined
		}),
		true, true))

	constants := NewObject()
	constants.Set(FromGoString("MAX_LENGTH"), Number(bufferMaxLength))
	constants.Set(FromGoString("MAX_STRING_LENGTH"), Number(bufferMaxStringLength))
	mod.Set(FromGoString("constants"), constants)

	// atob and btoa are the same pair the globals carry, so the module's and the global's
	// are one function and a program comparing them finds them equal.
	mod.Set(FromGoString("atob"), GlobalValue("atob"))
	mod.Set(FromGoString("btoa"), GlobalValue("btoa"))

	mod.Set(FromGoString("isAscii"), WithName(NewFunc(func(args []Value) Value {
		b := requireBufferArg(Arg(args, 0), "input")
		for _, c := range b.live() {
			if c > 0x7F {
				return Bool(false)
			}
		}
		return Bool(true)
	}), "isAscii"))
	mod.Set(FromGoString("isUtf8"), WithName(NewFunc(func(args []Value) Value {
		b := requireBufferArg(Arg(args, 0), "input")
		return Bool(utf8.Valid(b.live()))
	}), "isUtf8"))
	return newPartialModule("buffer", mod)
}
