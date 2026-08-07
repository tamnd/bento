package value

import "testing"

// drainIter pulls an iterator to exhaustion and joins what it yielded with a pipe, so a
// whole walk is one comparison rather than a step at a time.
func drainIter(it *IterHelper) string {
	out := ""
	for {
		r := it.Next()
		if r.Done {
			return out
		}
		if out != "" {
			out += "|"
		}
		out += ToString(r.Value).ToGoString()
	}
}

func TestIterateWalksAnArray(t *testing.T) {
	if got := drainIter(Iterate(arr(1, 2, 3), "a")); got != "1|2|3" {
		t.Errorf("walked %q, want %q", got, "1|2|3")
	}
}

// TestIterateSeesALiveLength pins that the walk re-reads length at each step rather
// than capturing it, which is what the array iterator does: a push during the loop
// extends what it visits and a pop ends it early.
func TestIterateSeesALiveLength(t *testing.T) {
	a := arr(1, 2, 3)
	it := Iterate(a, "a")
	if r := it.Next(); ToNumber(r.Value) != 1 {
		t.Fatalf("first step gave %v", ToString(r.Value).ToGoString())
	}
	GenericPush(a, Number(4))
	if got := drainIter(it); got != "2|3|4" {
		t.Errorf("after a push the rest was %q, want %q", got, "2|3|4")
	}
}

func TestIterateWalksAStringByCodePoint(t *testing.T) {
	// The emoji is one astral code point, two UTF-16 units. A walk that counted units
	// would yield two halves of a surrogate pair instead of one character.
	if got := drainIter(Iterate(StringValue(FromGoString("a\U0001F600b")), "s")); got != "a|\U0001F600|b" {
		t.Errorf("walked %q, want %q", got, "a|\U0001F600|b")
	}
}

// TestIterateWalksAnArrayLike pins the object case: a plain object carrying a length
// and integer keys walks like an array, the shape a built-in that answers an
// arguments-like object hands back.
func TestIterateWalksAnArrayLike(t *testing.T) {
	o := NewObject()
	o.Set(FromGoString("length"), Number(2))
	o.Set(FromGoString("0"), Number(7))
	o.Set(FromGoString("1"), Number(8))
	if got := drainIter(Iterate(o, "o")); got != "7|8" {
		t.Errorf("walked %q, want %q", got, "7|8")
	}
}

// TestIterateThrowsForAnObjectWithNoLength pins the boundary the array-like walk draws.
// An object carrying a length is the arguments-like shape a built-in hands back and it
// walks; a plain object is not iterable and throws the way JavaScript does. Reading
// length off it would answer undefined, which ToLength turns into 0, so without the
// probe the walk would quietly yield nothing where the language raises a TypeError.
func TestIterateThrowsForAnObjectWithNoLength(t *testing.T) {
	o := NewObject()
	o.Set(FromGoString("x"), Number(1))
	defer func() {
		rec := recover()
		if rec == nil {
			t.Fatalf("iterating a plain object did not throw")
		}
		e, ok := rec.(Thrown)
		if !ok {
			t.Fatalf("panicked with %T, want a thrown JavaScript value", rec)
		}
		if e.ErrorName() != "TypeError" || e.ErrorMessage() != "o is not iterable" {
			t.Errorf("threw %s: %s, want TypeError: o is not iterable", e.ErrorName(), e.ErrorMessage())
		}
	}()
	Iterate(o, "o")
}

// TestIterateWalksAnEmptyArrayLike pins that the probe asks whether length is there
// rather than whether it is non-zero, so an array-like holding nothing still iterates
// and answers no elements instead of throwing.
func TestIterateWalksAnEmptyArrayLike(t *testing.T) {
	o := NewObject()
	o.Set(FromGoString("length"), Number(0))
	if got := drainIter(Iterate(o, "o")); got != "" {
		t.Errorf("walked %q, want no elements", got)
	}
}

// TestIterateUsesAnOwnSymbolIterator pins the precedence the spec sets: Symbol.iterator
// is looked up first and unconditionally, so a receiver that defines one overrides the
// index walk it would otherwise take.
func TestIterateUsesAnOwnSymbolIterator(t *testing.T) {
	a := arr(1, 2, 3)
	n := 0
	a.setSymKey(symbolIterator, NewFunc(func(args []Value) Value {
		it := NewObject()
		it.Set(FromGoString("next"), NewFunc(func(args []Value) Value {
			res := NewObject()
			n++
			res.Set(FromGoString("done"), Bool(n > 2))
			res.Set(FromGoString("value"), Number(float64(n*10)))
			return res
		}))
		return it
	}))
	if got := drainIter(Iterate(a, "a")); got != "10|20" {
		t.Errorf("walked %q, want %q, the own iterator should have won over the index walk", got, "10|20")
	}
}

// TestIterateWalksAProtocolIterator pins the general case: anything answering an object
// with a next() that reports done is walked, which is what a user class defining
// [Symbol.iterator] and a built-in iterator both look like from outside.
func TestIterateWalksAProtocolIterator(t *testing.T) {
	src := NewObject()
	i := 0
	src.setSymKey(symbolIterator, NewFunc(func(args []Value) Value {
		it := NewObject()
		it.Set(FromGoString("next"), NewFunc(func(args []Value) Value {
			res := NewObject()
			res.Set(FromGoString("done"), Bool(i >= 3))
			res.Set(FromGoString("value"), Number(float64(i)))
			i++
			return res
		}))
		return it
	}))
	if got := drainIter(Iterate(src, "src")); got != "0|1|2" {
		t.Errorf("walked %q, want %q", got, "0|1|2")
	}
}

// TestIterateKeepsReportingDone pins that a caller which pulls past the end reads done
// forever rather than restarting the source or panicking.
func TestIterateKeepsReportingDone(t *testing.T) {
	it := Iterate(arr(1), "a")
	it.Next()
	for i := 0; i < 3; i++ {
		if r := it.Next(); !r.Done || !r.Value.IsUndefined() {
			t.Fatalf("pull %d past the end gave %v done=%v", i, ToString(r.Value).ToGoString(), r.Done)
		}
	}
}

// TestIterateThrowsOnANonIterable pins the message. The source text is passed in from
// the lowerer precisely so the error names what the program wrote, the way Node's does,
// rather than describing the box.
func TestIterateThrowsOnANonIterable(t *testing.T) {
	for name, v := range map[string]Value{
		"number":    Number(5),
		"undefined": Undefined,
		"null":      Null,
		"bool":      True,
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				rec := recover()
				if rec == nil {
					t.Fatalf("iterating a %s did not throw", name)
				}
				e, ok := rec.(Thrown)
				if !ok {
					t.Fatalf("panicked with %T, want a thrown JavaScript value", rec)
				}
				if e.ErrorName() != "TypeError" || e.ErrorMessage() != "os.cpus is not iterable" {
					t.Errorf("threw %s: %s, want TypeError: os.cpus is not iterable", e.ErrorName(), e.ErrorMessage())
				}
			}()
			Iterate(v, "os.cpus")
		})
	}
}

// TestIterateToSliceDrainsEverything pins the eager form a spread uses, and that it
// agrees with the lazy one about what an iterable holds.
func TestIterateToSliceDrainsEverything(t *testing.T) {
	got := IterateToSlice(arr(1, 2, 3), "a")
	if len(got) != 3 {
		t.Fatalf("drained %d elements, want 3", len(got))
	}
	for i, v := range got {
		if ToNumber(v) != float64(i+1) {
			t.Errorf("element %d is %v, want %d", i, ToNumber(v), i+1)
		}
	}
}

// TestIterateToSliceOfAnEmptySourceIsEmptyNotNil pins that a spread of an empty
// iterable appends nothing rather than a nil the caller would have to guard.
func TestIterateToSliceOfAnEmptySourceIsEmptyNotNil(t *testing.T) {
	if got := IterateToSlice(NewArrayValue(nil), "a"); got == nil || len(got) != 0 {
		t.Errorf("drained %v, want an empty slice", got)
	}
}
