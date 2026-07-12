package value

// Atomics is bento's runtime for the JavaScript Atomics namespace, the read, write,
// and read-modify-write operations that run over an integer typed array (25 §25.4). In
// a browser or a worker host the point of Atomics is coordination: two agents race on
// the bytes of one SharedArrayBuffer and each Atomics operation is indivisible, so a
// read-modify-write on one agent is never interleaved with another's.
//
// Ceiling: bento's AOT output is a single Go process with one agent, so no second
// agent races on the buffer. Every Atomics operation is therefore already indivisible
// with respect to the one agent that runs it: a plain read-modify-write observes and
// stores exactly what the spec's atomic step would, because there is no concurrent
// writer to interleave with. load, store, add, sub, and, or, xor, exchange, and
// compareExchange all run with full single-agent fidelity here.
//
// The one place the single-agent model is visible is wait and notify, which exist to
// block one agent until another wakes it: with no second agent to send the wake, a wait
// that would block has no notifier, so it can only report that the value changed
// (not-equal) or that it timed out, and notify never finds a waiter to wake so it wakes
// zero. A program that needs a real second agent to make progress is the multi-agent
// slice this group states as a handback rather than emits.

import "math"

// AtomicView is the integer typed array an Atomics operation runs over: the numeric
// family value.TypedArray[T] and the byte-backed value.Uint8Array both satisfy it, so
// one set of Atomics functions covers every integer element width. It exposes the
// element read and write the operations share and the element coercion compareExchange
// needs to compare a stored element against a coerced operand.
type AtomicView interface {
	// Len is the view's element count, the bound an access index is checked against.
	Len() float64
	// At reads the element at an index, widened to a Number.
	At(i float64) float64
	// SetAt writes an element at an index, coercing the value with the element's store
	// rule, the same write a plain indexed assignment makes.
	SetAt(i float64, v float64)
	// coerceElem returns the value the element store rule would keep, widened back to a
	// Number, so compareExchange can compare a stored element against a coerced operand
	// without a destructive write.
	coerceElem(v float64) float64
}

// coerceElem exposes a numeric typed array's store coercion, widened back to a Number,
// so the Atomics compare step can round an operand to the element's representation the
// same way a store would.
func (a *TypedArray[T]) coerceElem(v float64) float64 { return float64(a.coerce(v)) }

// coerceElem exposes a Uint8Array's byte store coercion, the ToUint8 wrap a Uint8Array
// element write applies, widened back to a Number.
func (a *Uint8Array) coerceElem(v float64) float64 { return float64(toUint8(v)) }

// atomicIndex resolves and bounds-checks an Atomics access index, throwing a
// RangeError when it is out of range, the throw ValidateAtomicAccess raises (25
// §25.4.3.1). A valid index is a canonical non-negative integer inside the view; a
// fractional, negative, not-a-number, or past-the-end index names no element, so the
// operation faults rather than silently reading or dropping the way a plain indexed
// access does.
func atomicIndex(a AtomicView, index float64) float64 {
	n := a.Len()
	if index != index || index != math.Trunc(index) || index < 0 || index >= n {
		Throw(NewRangeError(FromGoString("Atomics access index is out of bounds")))
	}
	return index
}

// AtomicLoad reads the element at the index, the lowering of Atomics.load. In a single
// agent the read is already indivisible, so it is the same widened element At reads,
// after the bounds check the spec runs first.
func AtomicLoad(a AtomicView, index float64) float64 {
	i := atomicIndex(a, index)
	return a.At(i)
}

// AtomicStore writes value at the index and returns it, the lowering of Atomics.store.
// The spec returns the integer value passed in, not the wrapped element the store
// keeps, so a value outside the element's range reads back here as the value given
// while the stored element wraps; the covered subset passes an in-range integer, for
// which the two agree.
func AtomicStore(a AtomicView, index float64, value float64) float64 {
	i := atomicIndex(a, index)
	a.SetAt(i, value)
	return value
}

// AtomicAdd adds value to the element and returns the previous element, the lowering of
// Atomics.add. The read, the add, and the write are one indivisible step in the spec;
// with one agent a plain read-modify-write is that step. The sum is stored through the
// element's coercion, so it wraps into the element's range exactly as a plain store
// would, and the returned previous value is the widened element from before the add.
func AtomicAdd(a AtomicView, index float64, value float64) float64 {
	i := atomicIndex(a, index)
	old := a.At(i)
	a.SetAt(i, old+value)
	return old
}

// AtomicSub subtracts value from the element and returns the previous element, the
// lowering of Atomics.sub, the same read-modify-write shape as AtomicAdd.
func AtomicSub(a AtomicView, index float64, value float64) float64 {
	i := atomicIndex(a, index)
	old := a.At(i)
	a.SetAt(i, old-value)
	return old
}

// AtomicAnd stores the bitwise AND of the element and value and returns the previous
// element, the lowering of Atomics.and. The operands are integer elements, so the bit
// op runs on their int64 forms, which hold every covered element width exactly, and the
// result stores through the element's coercion back into range.
func AtomicAnd(a AtomicView, index float64, value float64) float64 {
	i := atomicIndex(a, index)
	old := a.At(i)
	a.SetAt(i, float64(int64(old)&int64(value)))
	return old
}

// AtomicOr stores the bitwise OR of the element and value and returns the previous
// element, the lowering of Atomics.or, the same integer read-modify-write as AtomicAnd.
func AtomicOr(a AtomicView, index float64, value float64) float64 {
	i := atomicIndex(a, index)
	old := a.At(i)
	a.SetAt(i, float64(int64(old)|int64(value)))
	return old
}

// AtomicXor stores the bitwise XOR of the element and value and returns the previous
// element, the lowering of Atomics.xor, the same integer read-modify-write as
// AtomicAnd.
func AtomicXor(a AtomicView, index float64, value float64) float64 {
	i := atomicIndex(a, index)
	old := a.At(i)
	a.SetAt(i, float64(int64(old)^int64(value)))
	return old
}

// AtomicExchange stores value and returns the previous element, the lowering of
// Atomics.exchange: an unconditional swap whose returned value is the element from
// before the store.
func AtomicExchange(a AtomicView, index float64, value float64) float64 {
	i := atomicIndex(a, index)
	old := a.At(i)
	a.SetAt(i, value)
	return old
}

// AtomicCompareExchange stores replacement only if the element equals expected and
// returns the previous element either way, the lowering of Atomics.compareExchange. The
// comparison is against the element representation, so expected is coerced to the
// element's store form before the compare, matching the spec's byte-equal check; the
// returned value is the element from before any store.
func AtomicCompareExchange(a AtomicView, index float64, expected float64, replacement float64) float64 {
	i := atomicIndex(a, index)
	old := a.At(i)
	if old == a.coerceElem(expected) {
		a.SetAt(i, replacement)
	}
	return old
}

// AtomicIsLockFree reports whether an atomic operation on an element of the given byte
// size is lock-free, the lowering of Atomics.isLockFree. A size of 1, 2, 4, or 8 bytes
// is lock-free on the platforms bento targets, matching what a modern engine reports;
// any other size is not.
func AtomicIsLockFree(size float64) bool {
	switch size {
	case 1, 2, 4, 8:
		return true
	default:
		return false
	}
}

// AtomicNotify wakes up to count agents waiting on the index and returns the number
// woken, the lowering of Atomics.notify. In a single agent there is never a waiter to
// wake, since the one agent that could wait is the one calling notify, so it always
// wakes zero. The index is bounds-checked the same way the read-modify-write path
// checks it.
func AtomicNotify(a AtomicView, index float64, count ...float64) float64 {
	atomicIndex(a, index)
	return 0
}

// AtomicWait blocks until the element at the index changes from value or a notify
// arrives, the lowering of Atomics.wait, and returns "ok", "not-equal", or
// "timed-out". In a single agent there is no other agent to change the element or send
// a notify, so a wait that would block cannot make progress: it returns "not-equal"
// when the element already differs from value, and "timed-out" otherwise, since a wait
// with no possible notifier is a wait that has already timed out. The "ok" result,
// which only a real notify from a second agent produces, never occurs single-agent.
func AtomicWait(a AtomicView, index float64, value float64, timeout ...float64) BStr {
	i := atomicIndex(a, index)
	if a.At(i) != value {
		return FromGoString("not-equal")
	}
	return FromGoString("timed-out")
}

// AtomicPause is a hint to the processor that the agent is in a spin-wait loop, the
// lowering of Atomics.pause. It has no observable effect on program state and returns
// undefined; with one agent there is no contention to back off from, so it is a no-op.
func AtomicPause(iterations ...float64) {}
