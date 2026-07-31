// This file bridges the three byte-buffer objects into the dynamic value.Value world:
// an ArrayBuffer, a SharedArrayBuffer, and the DataView that reads one. It is the same
// seam mapvalue.go and datevalue.go open, a brand field on Object plus a member switch,
// and it reads the same way.
//
// The box is a view, not a copy, and here that matters more than anywhere else: a
// buffer is the storage every typed array and every view over it aliases, so a copy
// would leave a program writing into bytes nobody reads. The box carries the live
// buffer, so a write made through any view of it is visible through the box and the
// other way round, and each buffer caches its own box so one buffer is one object under
// ===.
//
// None of the three has own properties, so the member surface is a switch and the key
// walks stay empty, which is what Node's do. What they do carry, and a Date does not, is
// a real Symbol.toStringTag on the prototype: Object.prototype.toString.call and a plain
// property read both answer "ArrayBuffer", so the tag is a value the symbol hook returns
// rather than a brand ClassTag reads.

package value

import (
	"math/big"
	"strconv"
	"strings"
)

// bufferBacking is the half an ArrayBuffer and a SharedArrayBuffer share: their bytes,
// whether those bytes are gone, and the name they answer to. Everything written against
// this pair (the inspector, the class tag, the deep comparison) is written once, while
// the member surfaces, which differ (one resizes and transfers, the other only grows),
// stay in their own switches.
type bufferBacking interface {
	// jsBufferBytes is the buffer's live bytes, empty for a detached one.
	jsBufferBytes() []byte
	// jsBufferDetached reports whether the bytes have been given away, which the
	// inspector spells "(detached)" rather than as an empty run.
	jsBufferDetached() bool
	// jsBufferName is "ArrayBuffer" or "SharedArrayBuffer", the constructor name that
	// prefixes the rendering and the Symbol.toStringTag the object answers.
	jsBufferName() string
	// jsBufferGet is the kind's own member surface, the half the two do not share.
	jsBufferGet(name string) (Value, bool)
	// jsBufferBox is this buffer's own box, the value a member that answers the
	// receiver hands back.
	jsBufferBox() Value
}

// ToValue boxes a buffer into a dynamic value. The box is built once and kept on the
// buffer, so every crossing of the same buffer hands back the same object: two boxes
// would compare unequal under === and print as two values even though the program has
// one run of bytes.
func (b *ArrayBuffer) ToValue() Value {
	if b.boxed == nil {
		b.boxed = &Object{kind: KindObject, jsBuffer: b}
	}
	return objectValue(b.boxed)
}

// ToValue is the SharedArrayBuffer half of the same crossing.
func (s *SharedArrayBuffer) ToValue() Value {
	if s.boxed == nil {
		s.boxed = &Object{kind: KindObject, jsBuffer: s}
	}
	return objectValue(s.boxed)
}

// ToValue boxes a data view. A view is a window onto a buffer rather than storage of
// its own, so the box carries the view and reaches the bytes through it, which is what
// makes a write through the box show up in every other view of the same buffer.
func (d *DataView) ToValue() Value {
	if d.boxed == nil {
		d.boxed = &Object{kind: KindObject, jsView: d}
	}
	return objectValue(d.boxed)
}

// The bufferBacking implementations. Each is the boxed spelling of a concrete method
// beside it in arraybuffer.go and sharedarraybuffer.go.

func (b *ArrayBuffer) jsBufferBytes() []byte  { return b.data }
func (b *ArrayBuffer) jsBufferDetached() bool { return b.detached }
func (b *ArrayBuffer) jsBufferName() string   { return "ArrayBuffer" }
func (b *ArrayBuffer) jsBufferBox() Value     { return b.ToValue() }

func (s *SharedArrayBuffer) jsBufferBytes() []byte  { return s.buf.data }
func (s *SharedArrayBuffer) jsBufferDetached() bool { return false }
func (s *SharedArrayBuffer) jsBufferName() string   { return "SharedArrayBuffer" }
func (s *SharedArrayBuffer) jsBufferBox() Value     { return s.ToValue() }

// asBuffer returns the live buffer a value boxes, or nil when the value is not a buffer
// box. It is the probe the reads, the inspector and the deep comparison make before
// their ordinary object handling, the same shape asMap and asDate have.
func (v Value) asBuffer() bufferBacking {
	if v.kind != KindObject {
		return nil
	}
	return v.object().jsBuffer
}

// asDataView returns the live view a value boxes, or nil when it is not a view box.
func (v Value) asDataView() *DataView {
	if v.kind != KindObject {
		return nil
	}
	return v.object().jsView
}

// jsBufferGet is the ArrayBuffer member surface: the four accessors, which answer their
// values rather than callables because they are getters on the prototype, and the four
// methods that reshape the storage. slice and the transfer pair each answer a fresh
// buffer, so the result is boxed on the way out and a chained b.slice(0, 2).byteLength
// reads through the box the way it does on the typed side.
func (b *ArrayBuffer) jsBufferGet(name string) (Value, bool) {
	switch name {
	case "byteLength":
		return Number(b.ByteLength()), true
	case "maxByteLength":
		return Number(b.MaxByteLength()), true
	case "resizable":
		return Bool(b.Resizable()), true
	case "detached":
		return Bool(b.Detached()), true
	case "slice":
		return boundMethod("slice", func(args []Value) Value {
			return b.Slice(numberArgs(args)...).ToValue()
		}), true
	case "resize":
		return boundMethod("resize", func(args []Value) Value {
			b.Resize(ToNumber(Arg(args, 0)))
			return Undefined
		}), true
	case "transfer":
		return boundMethod("transfer", func(args []Value) Value {
			return b.Transfer(numberArgs(args)...).ToValue()
		}), true
	case "transferToFixedLength":
		return boundMethod("transferToFixedLength", func(args []Value) Value {
			return b.TransferToFixedLength(numberArgs(args)...).ToValue()
		}), true
	}
	return Undefined, false
}

// jsBufferGet is the SharedArrayBuffer member surface. It is the ArrayBuffer's with the
// three members that give bytes away removed, since shared bytes are never detached, and
// with resizable spelled growable and resize spelled grow, which only ever enlarges.
func (s *SharedArrayBuffer) jsBufferGet(name string) (Value, bool) {
	switch name {
	case "byteLength":
		return Number(s.ByteLength()), true
	case "maxByteLength":
		return Number(s.MaxByteLength()), true
	case "growable":
		return Bool(s.Growable()), true
	case "grow":
		return boundMethod("grow", func(args []Value) Value {
			s.Grow(ToNumber(Arg(args, 0)))
			return Undefined
		}), true
	case "slice":
		return boundMethod("slice", func(args []Value) Value {
			return s.Slice(numberArgs(args)...).ToValue()
		}), true
	}
	return Undefined, false
}

// numberArgs coerces a boxed call's arguments into the numbers a variadic runtime method
// takes. An omitted optional argument is simply absent here rather than a NaN, because
// the runtime methods read their variadic slice by length: b.slice() copies the whole
// buffer, where b.slice(undefined) would have to mean the same thing through a NaN that
// relativeIndex folds to zero. Both spellings land on the same bytes, and this one says
// so directly.
func numberArgs(args []Value) []float64 {
	out := make([]float64, len(args))
	for i, a := range args {
		out[i] = ToNumber(a)
	}
	return out
}

// bufferGet reads a member off a boxed buffer. The shared members are answered here and
// the rest handed to the kind's own switch, so a name that is not a member of either
// reports ok=false and the caller climbs the ordinary chain to undefined.
func bufferGet(b bufferBacking, name string) (Value, bool) {
	return b.jsBufferGet(name)
}

// bufferSymGet reads a symbol-keyed member off a boxed buffer. There is one, and unlike
// a Date's it is a real property in JavaScript rather than an internal slot: a buffer
// carries Symbol.toStringTag on its prototype, so both a plain read of it and
// Object.prototype.toString.call answer the constructor's name.
func bufferSymGet(b bufferBacking, key *Symbol) (Value, bool) {
	if key != symbolToStringTag {
		return Undefined, false
	}
	return StringValue(FromGoString(b.jsBufferName())), true
}

// dataViewNumberReads are the get methods that answer a Number, keyed by name and
// carrying whether the read takes an endianness flag. The single-byte reads do not: one
// byte has no byte order, so getInt8 takes an offset and nothing else, and passing it a
// flag has no meaning to reject or honor.
var dataViewNumberReads = map[string]struct {
	read   func(*DataView, float64, ...bool) float64
	endian bool
}{
	"getInt16":   {(*DataView).GetInt16, true},
	"getUint16":  {(*DataView).GetUint16, true},
	"getInt32":   {(*DataView).GetInt32, true},
	"getUint32":  {(*DataView).GetUint32, true},
	"getFloat16": {(*DataView).GetFloat16, true},
	"getFloat32": {(*DataView).GetFloat32, true},
	"getFloat64": {(*DataView).GetFloat64, true},
}

// dataViewNumberWrites are the set methods that take a Number, the mirror of the reads
// above.
var dataViewNumberWrites = map[string]struct {
	write  func(*DataView, float64, float64, ...bool)
	endian bool
}{
	"setInt16":   {(*DataView).SetInt16, true},
	"setUint16":  {(*DataView).SetUint16, true},
	"setInt32":   {(*DataView).SetInt32, true},
	"setUint32":  {(*DataView).SetUint32, true},
	"setFloat16": {(*DataView).SetFloat16, true},
	"setFloat32": {(*DataView).SetFloat32, true},
	"setFloat64": {(*DataView).SetFloat64, true},
}

// dataViewBigReads and dataViewBigWrites are the eight-byte integer pair, kept apart
// because they are the only members that speak bigint rather than Number: a 64-bit
// integer does not fit a float64 without losing its low bits, which is the whole reason
// the spec gives them their own type.
var dataViewBigReads = map[string]func(*DataView, float64, ...bool) *big.Int{
	"getBigInt64":  (*DataView).GetBigInt64,
	"getBigUint64": (*DataView).GetBigUint64,
}

var dataViewBigWrites = map[string]func(*DataView, float64, *big.Int, ...bool){
	"setBigInt64":  (*DataView).SetBigInt64,
	"setBigUint64": (*DataView).SetBigUint64,
}

// dataViewGet reads a member off a boxed data view: the three accessors, which answer
// their values because they are prototype getters, and the get and set pairs, each bound
// to the live view so a write through the box lands in the bytes every other view of the
// same buffer reads.
func dataViewGet(d *DataView, name string) (Value, bool) {
	if r, ok := dataViewNumberReads[name]; ok {
		return boundMethod(name, func(args []Value) Value {
			return Number(r.read(d, ToNumber(Arg(args, 0)), dataViewEndian(args, 1)...))
		}), true
	}
	if w, ok := dataViewNumberWrites[name]; ok {
		return boundMethod(name, func(args []Value) Value {
			w.write(d, ToNumber(Arg(args, 0)), ToNumber(Arg(args, 1)), dataViewEndian(args, 2)...)
			return Undefined
		}), true
	}
	if r, ok := dataViewBigReads[name]; ok {
		return boundMethod(name, func(args []Value) Value {
			return BigIntFromBig(r(d, ToNumber(Arg(args, 0)), dataViewEndian(args, 1)...))
		}), true
	}
	if w, ok := dataViewBigWrites[name]; ok {
		return boundMethod(name, func(args []Value) Value {
			w(d, ToNumber(Arg(args, 0)), dataViewBigArg(Arg(args, 1)), dataViewEndian(args, 2)...)
			return Undefined
		}), true
	}
	switch name {
	case "buffer":
		return d.Buffer().ToValue(), true
	case "byteLength":
		return Number(d.ByteLength()), true
	case "byteOffset":
		return Number(d.ByteOffset()), true
	case "getInt8":
		return boundMethod("getInt8", func(args []Value) Value {
			return Number(d.GetInt8(ToNumber(Arg(args, 0))))
		}), true
	case "getUint8":
		return boundMethod("getUint8", func(args []Value) Value {
			return Number(d.GetUint8(ToNumber(Arg(args, 0))))
		}), true
	case "setInt8":
		return boundMethod("setInt8", func(args []Value) Value {
			d.SetInt8(ToNumber(Arg(args, 0)), ToNumber(Arg(args, 1)))
			return Undefined
		}), true
	case "setUint8":
		return boundMethod("setUint8", func(args []Value) Value {
			d.SetUint8(ToNumber(Arg(args, 0)), ToNumber(Arg(args, 1)))
			return Undefined
		}), true
	}
	return Undefined, false
}

// dataViewEndian reads the optional littleEndian flag at the given position into the
// variadic bool the runtime methods take. An absent flag is absent rather than false,
// because the runtime reads the slice by length and defaults to big-endian, which is
// what the spec's omitted argument means; a present one is coerced to a boolean the way
// ToBoolean does, so a truthy value of any kind reads little-endian.
func dataViewEndian(args []Value, at int) []bool {
	if len(args) <= at {
		return nil
	}
	return []bool{ToBoolean(args[at])}
}

// dataViewBigArg reads the value argument of a bigint write. The spec runs it through
// ToBigInt, which takes a bigint, a boolean and a string of digits but rejects a Number:
// a 64-bit slot is exactly where a silent float64 rounding would corrupt data, so the
// language makes it an error rather than a conversion.
func dataViewBigArg(v Value) *big.Int {
	switch v.kind {
	case KindBigInt:
		return v.bigint().Int()
	case KindBool:
		return BoolToBigInt(v.AsBool())
	case KindString:
		return StringToBigInt(v.str())
	}
	Throw(NewTypeError(FromGoString("Cannot convert " + ToString(v).ToGoString() + " to a BigInt")))
	return nil
}

// dataViewSymGet answers the view's Symbol.toStringTag, which a DataView carries on its
// prototype the way a buffer does.
func dataViewSymGet(key *Symbol) (Value, bool) {
	if key != symbolToStringTag {
		return Undefined, false
	}
	return StringValue(FromGoString("DataView")), true
}

// dataViewWindow is the run of bytes a view actually reads: its own span of the buffer
// rather than the whole of it. It is what the deep comparison compares, so two views of
// the same bytes at different offsets in different buffers are equal, which is what node
// says of them.
func dataViewWindow(d *DataView) []byte {
	n, outOfBounds := d.liveByteLength()
	if outOfBounds {
		return nil
	}
	return d.buffer.data[d.byteOffset : d.byteOffset+n]
}

// bufferHexContents renders a run of bytes the way node's [Uint8Contents] entry does:
// two lowercase hex digits per byte, space separated, cut off at the inspector's array
// limit with a count of what was left out. The limit is the same one that caps an array's
// elements, so a hundred-byte buffer prints in full and a longer one says how much more
// there is rather than filling the terminal.
func bufferHexContents(data []byte, maxLength int) string {
	shown := data
	if maxLength >= 0 && len(shown) > maxLength {
		shown = shown[:maxLength]
	}
	var b strings.Builder
	for i, c := range shown {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteByte(hexLowerDigits[c>>4])
		b.WriteByte(hexLowerDigits[c&0xf])
	}
	if remaining := len(data) - len(shown); remaining > 0 {
		if len(shown) > 0 {
			b.WriteByte(' ')
		}
		b.WriteString("... " + strconv.Itoa(remaining) + " more byte" + plural(remaining))
	}
	return b.String()
}

// hexLowerDigits is the alphabet the byte contents are spelled in. It is its own
// constant rather than the uri encoder's hexDigits because that one is uppercase, which
// is what a percent-escape wants and not what a buffer's contents print as.
const hexLowerDigits = "0123456789abcdef"
