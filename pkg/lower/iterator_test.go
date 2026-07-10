package lower

import (
	"strings"
	"testing"
)

// The iterator protocol is JavaScript's one way to walk a value that is not an
// array or a string: a class defines a [Symbol.iterator]() that answers with an
// iterator, and the iterator answers each next() call with a { value, done }
// result until done is true. bento lowers [Symbol.iterator] to a Go method under a
// fixed name, and a for...of over such a class emits the pull-until-done loop a
// developer writes by hand against next(), reading the value and done fields off
// the { value, done } struct the result object lowers to.

const selfIterator = `
class Countdown {
  i: number = 0;
  stop: number;
  constructor(stop: number) { this.stop = stop; }
  [Symbol.iterator]() { return this; }
  next(): { value: number; done: boolean } {
    if (this.i < this.stop) {
      const v = this.i;
      this.i = this.i + 1;
      return { value: v, done: false };
    }
    return { value: 0, done: true };
  }
}
`

// TestForOfUserIterableLowers proves a for...of over a class that is its own
// iterator lowers to the protocol pull loop: it obtains the iterator through the
// SymbolIterator method, calls Next each turn, breaks on the result's Done, and
// binds the loop variable to its Value, rather than handing back the way a
// non-array, non-string iterable did before this slice.
func TestForOfUserIterableLowers(t *testing.T) {
	src := selfIterator + "for (const x of new Countdown(3)) { console.log(x); }\n"
	source := renderProgram(t, src)
	for _, want := range []string{".SymbolIterator()", ".Next()", ".Done", ".Value"} {
		if !strings.Contains(source, want) {
			t.Errorf("for...of did not lower through the iterator protocol, missing %q:\n%s", want, source)
		}
	}
}

// TestForOfUserIterableRuns builds and runs the self-iterating class so the
// lowered protocol loop is proven against the JavaScript result: a Countdown to 3
// yields 0, 1, 2.
func TestForOfUserIterableRuns(t *testing.T) {
	skipIfShort(t)
	src := selfIterator + "for (const x of new Countdown(3)) { console.log(x); }\n"
	if got, want := runProgramGo(t, src), "0\n1\n2\n"; got != want {
		t.Fatalf("self-iterating class printed %q, want %q", got, want)
	}
}

// TestForOfSeparateIteratorRuns proves the protocol drives an iterable whose
// [Symbol.iterator]() returns a separate iterator object, a distinct class, not
// this, so the value walked comes off the iterator's own next() and the two
// classes lower to two Go structs the loop threads together.
func TestForOfSeparateIteratorRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
class SpanIter {
  n: number;
  hi: number;
  constructor(lo: number, hi: number) { this.n = lo; this.hi = hi; }
  next(): { value: number; done: boolean } {
    if (this.n < this.hi) {
      const v = this.n;
      this.n = this.n + 1;
      return { value: v, done: false };
    }
    return { value: 0, done: true };
  }
}
class Span {
  lo: number;
  hi: number;
  constructor(lo: number, hi: number) { this.lo = lo; this.hi = hi; }
  [Symbol.iterator]() { return new SpanIter(this.lo, this.hi); }
}
for (const x of new Span(2, 5)) { console.log(x); }
`
	if got, want := runProgramGo(t, src), "2\n3\n4\n"; got != want {
		t.Fatalf("separate-iterator class printed %q, want %q", got, want)
	}
}

// TestForOfUnusedBindingRuns proves the counting idiom, a for...of whose loop
// variable the body never reads, still drives the iterator: the binding is
// dropped rather than bound, since Go rejects an unused variable, but the loop
// still pulls the iterator to its end.
func TestForOfUnusedBindingRuns(t *testing.T) {
	skipIfShort(t)
	src := selfIterator + "let n = 0;\nfor (const x of new Countdown(4)) { n = n + 1; }\nconsole.log(n);\n"
	if got, want := runProgramGo(t, src), "4\n"; got != want {
		t.Fatalf("counting for...of printed %q, want %q", got, want)
	}
}

// TestForOfObjectLiteralIteratorHandsBack proves that an iterable whose
// [Symbol.iterator]() returns an inline object literal with a next() method hands
// back rather than mislower: an object literal that carries a method is a later
// slice, so the whole class declines and the unit routes to the engine, keeping
// the zero-fail invariant while the object-literal-with-a-method form waits for
// its own slice.
func TestForOfObjectLiteralIteratorHandsBack(t *testing.T) {
	const src = `
class Seq {
  [Symbol.iterator]() {
    let n = 0;
    return {
      next(): { value: number; done: boolean } {
        return n < 3 ? { value: n++, done: false } : { value: 0, done: true };
      },
    };
  }
}
for (const x of new Seq()) { console.log(x); }
`
	renderProgramHandBack(t, src)
}
