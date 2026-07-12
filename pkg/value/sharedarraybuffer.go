package value

// SharedArrayBuffer is bento's runtime representation of a JavaScript
// SharedArrayBuffer, the backing store Atomics operates over and a typed array or a
// DataView views the same as it views an ArrayBuffer (25 §25.2). In a browser or a
// worker host a SharedArrayBuffer is the one buffer kind whose bytes are visible to a
// second agent, which is the whole reason Atomics exists: it coordinates two agents
// racing on those bytes.
//
// Ceiling: bento's AOT output is a single Go process with one agent, so there is no
// second agent to share bytes with. A SharedArrayBuffer here is therefore an ordinary
// shared backing store that behaves exactly as the spec requires within one agent: its
// bytes alias into every view over it, it cannot be detached, and when growable it
// only grows. The cross-agent visibility a real second agent would observe, and the
// wait and notify that block on it, are the multi-agent slice this group states as a
// handback rather than emits. Every single-agent SharedArrayBuffer program, which is
// what the built-in tests exercise, runs with full fidelity.
//
// The bytes live in an ArrayBuffer so a view over a SharedArrayBuffer aliases the same
// run the ArrayBuffer path already aliases, with no second storage model: a typed
// array or DataView constructed over one takes the underlying buffer and shares its
// bytes. A SharedArrayBuffer differs from an ArrayBuffer only in the surface it
// carries, so the distinctions the spec draws (grow rather than resize, growable
// rather than resizable, no detach or transfer) live in this wrapper's methods rather
// than in the shared storage.
type SharedArrayBuffer struct {
	buf *ArrayBuffer
}

// NewSharedArrayBuffer builds a zeroed fixed-length shared buffer of the given byte
// length, the lowering of new SharedArrayBuffer(n). The length is a Number truncated
// toward zero like ToIndex, and a negative or not-a-number length clamps to zero, the
// same covered subset the ArrayBuffer constructor takes.
func NewSharedArrayBuffer(byteLength float64) *SharedArrayBuffer {
	return &SharedArrayBuffer{buf: NewArrayBuffer(byteLength)}
}

// NewGrowableSharedArrayBuffer builds a growable shared buffer of the given byte
// length that may later grow to maxByteLength, the lowering of new
// SharedArrayBuffer(n, { maxByteLength }). Both arguments are Numbers truncated toward
// zero like ToIndex. A max below the initial length is a RangeError, the throw the
// spec raises. The backing run is sized to the initial length and grow reallocates it,
// so an unused max costs no storage.
func NewGrowableSharedArrayBuffer(byteLength float64, maxByteLength float64) *SharedArrayBuffer {
	return &SharedArrayBuffer{buf: NewResizableArrayBuffer(byteLength, maxByteLength)}
}

// Buffer is the ArrayBuffer holding the shared bytes, the storage every view over the
// SharedArrayBuffer aliases. The view path takes an *ArrayBuffer, so a typed array or
// DataView over a SharedArrayBuffer binds this buffer and observes writes made through
// any other view of the same shared bytes.
func (s *SharedArrayBuffer) Buffer() *ArrayBuffer { return s.buf }

// ByteLength is the shared buffer's size in bytes, the .byteLength accessor, a Number
// to match the type the checker gives the property.
func (s *SharedArrayBuffer) ByteLength() float64 { return s.buf.ByteLength() }

// MaxByteLength is the largest byte length the shared buffer may hold, the
// .maxByteLength accessor. A growable buffer reports the maximum it was built with; a
// fixed-length one reports its current length, matching the spec's getter.
func (s *SharedArrayBuffer) MaxByteLength() float64 { return s.buf.MaxByteLength() }

// Growable reports whether the shared buffer may grow, the .growable accessor, true
// only for a buffer built with a maxByteLength. It is the SharedArrayBuffer spelling
// of the resizable flag an ArrayBuffer carries.
func (s *SharedArrayBuffer) Growable() bool { return s.buf.Resizable() }

// Grow enlarges the shared buffer to newLength, the lowering of
// SharedArrayBuffer.prototype.grow (25 §25.2.4.4). A grow on a non-growable buffer, a
// length past the maximum, or a length below the current byte length is a RangeError,
// the throws the spec raises: unlike an ArrayBuffer resize, a shared buffer only grows,
// so a shorter length is rejected rather than shrinking the run. The retained bytes are
// kept and the growth zeroed, so every view over the buffer sees the new size on its
// next access.
func (s *SharedArrayBuffer) Grow(newLength float64) {
	if !s.buf.resizable {
		Throw(NewTypeError(FromGoString("Cannot grow a non-growable SharedArrayBuffer")))
	}
	if typedLen(newLength) < len(s.buf.data) {
		Throw(NewRangeError(FromGoString("SharedArrayBuffer grow length is smaller than the current byte length")))
	}
	s.buf.Resize(newLength)
}

// Slice copies the bytes in [start, end) into a fresh fixed-length shared buffer, the
// lowering of SharedArrayBuffer.prototype.slice (25 §25.2.4.3). start and end are
// optional Numbers; a negative index counts from the end and an omitted end runs to the
// current byte length, the same relative-index rule Array.prototype.slice takes. The
// result is a new SharedArrayBuffer that owns its bytes and does not alias the
// receiver, so a later write through either shows only in that one.
func (s *SharedArrayBuffer) Slice(bounds ...float64) *SharedArrayBuffer {
	n := len(s.buf.data)
	start := 0
	if len(bounds) > 0 {
		start = relativeIndex(bounds[0], n)
	}
	end := n
	if len(bounds) > 1 {
		end = relativeIndex(bounds[1], n)
	}
	if end < start {
		end = start
	}
	out := NewSharedArrayBuffer(float64(end - start))
	copy(out.buf.data, s.buf.data[start:end])
	return out
}
