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

// TestManualIteratorDriveLowers proves a test can drive an iterable by hand:
// obj[Symbol.iterator]() reads the iterator factory as the Go SymbolIterator
// method, and the iterator it returns answers a direct .next() through Next, so the
// manual walk lowers to the same two methods the for...of loop calls.
func TestManualIteratorDriveLowers(t *testing.T) {
	src := selfIterator + "const it = new Countdown(2)[Symbol.iterator]();\nconst r = it.next();\nconsole.log(r.value);\n"
	source := renderProgram(t, src)
	for _, want := range []string{".SymbolIterator()", ".Next()", ".Value"} {
		if !strings.Contains(source, want) {
			t.Errorf("manual iterator drive did not lower through the protocol methods, missing %q:\n%s", want, source)
		}
	}
}

// TestManualIteratorDriveRuns builds and runs a by-hand iterator walk: obtain the
// iterator, pull once, and read value off the result, which for a Countdown to 2 is
// its first value, 0.
func TestManualIteratorDriveRuns(t *testing.T) {
	skipIfShort(t)
	src := selfIterator + "const it = new Countdown(2)[Symbol.iterator]();\nconst r = it.next();\nconsole.log(r.value);\n"
	if got, want := runProgramGo(t, src), "0\n"; got != want {
		t.Fatalf("manual iterator drive printed %q, want %q", got, want)
	}
}

// TestSpreadUserIterableRuns proves a spread of a user iterable in an array
// literal walks the iterator protocol: [head, ...iterable, tail] drains the
// iterable between the fixed elements, so a Countdown to 3 splices 0, 1, 2 and the
// literal is [99, 0, 1, 2, 88], length 5.
func TestSpreadUserIterableRuns(t *testing.T) {
	skipIfShort(t)
	src := selfIterator + "const a = [99, ...new Countdown(3), 88];\nconsole.log(a.length);\nfor (const x of a) { console.log(x); }\n"
	if got, want := runProgramGo(t, src), "5\n99\n0\n1\n2\n88\n"; got != want {
		t.Fatalf("spread of a user iterable printed %q, want %q", got, want)
	}
}

// TestSpreadUserIterableIntoRestRuns proves a spread of a user iterable into a
// rest parameter walks the same protocol: sum(head, ...iterable, tail) collects the
// drained values into the rest array, so sum(100, ...Countdown(3), 200) adds 100, 0,
// 1, 2, 200 to 303.
func TestSpreadUserIterableIntoRestRuns(t *testing.T) {
	skipIfShort(t)
	src := selfIterator + `function sum(...xs: number[]): number {
  let s = 0;
  for (const x of xs) { s = s + x; }
  return s;
}
console.log(sum(100, ...new Countdown(3), 200));
`
	if got, want := runProgramGo(t, src), "303\n"; got != want {
		t.Fatalf("spread of a user iterable into a rest parameter printed %q, want %q", got, want)
	}
}

// TestDestructureUserIterableRuns proves array destructuring off a user iterable
// walks the iterator protocol: const [a, b] = iterable drains the iterable into an
// array once and binds a and b to its first two values, so a Countdown to 5 binds 0
// and 1.
func TestDestructureUserIterableRuns(t *testing.T) {
	skipIfShort(t)
	src := selfIterator + "const [a, b] = new Countdown(5);\nconsole.log(a);\nconsole.log(b);\n"
	if got, want := runProgramGo(t, src), "0\n1\n"; got != want {
		t.Fatalf("destructuring off a user iterable printed %q, want %q", got, want)
	}
}

// TestDestructureUserIterableLowers proves the destructuring drains the iterable
// into a value.Array once and reads each binding off it by index, rather than
// handing back the way a non-array source did before this slice.
func TestDestructureUserIterableLowers(t *testing.T) {
	src := selfIterator + "const [a, b] = new Countdown(5);\nconsole.log(a);\n"
	source := renderProgram(t, src)
	for _, want := range []string{"value.ArrayFrom(", ".SymbolIterator()", ".AtI(0)", ".AtI(1)"} {
		if !strings.Contains(source, want) {
			t.Errorf("destructuring did not drain the iterable through the protocol, missing %q:\n%s", want, source)
		}
	}
}

// closingIterator is an endless iterable that records its close: it never reports
// done on its own, so a for...of over it ends only by breaking, and its return()
// prints a marker, which is how a test observes the close fired.
const closingIterator = `
class Guarded {
  i: number = 0;
  [Symbol.iterator]() { return this; }
  next(): { value: number; done: boolean } {
    const v = this.i;
    this.i = this.i + 1;
    return { value: v, done: false };
  }
  return(): { value: number; done: boolean } {
    console.log("closed");
    return { value: 0, done: true };
  }
}
`

// TestForOfIteratorCloseOnBreakRuns proves a for...of that breaks out of a user
// iterable calls the iterator's return() to close it: the loop prints 0 and 1, then
// breaks at 2, which fires return() (printing "closed"), and control falls through
// to the line after the loop.
func TestForOfIteratorCloseOnBreakRuns(t *testing.T) {
	skipIfShort(t)
	src := closingIterator + "for (const x of new Guarded()) { if (x >= 2) { break; } console.log(x); }\nconsole.log(\"after\");\n"
	if got, want := runProgramGo(t, src), "0\n1\nclosed\nafter\n"; got != want {
		t.Fatalf("closing for...of printed %q, want %q", got, want)
	}
}

// TestForOfIteratorCloseNormalCompletionRuns proves the close does not fire when
// the loop ends normally: a bounded iterable that reports done closes itself
// through the done branch, so return() is not called and no marker prints.
func TestForOfIteratorCloseNormalCompletionRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
class Bounded {
  i: number = 0;
  stop: number;
  constructor(stop: number) { this.stop = stop; }
  [Symbol.iterator]() { return this; }
  next(): { value: number; done: boolean } {
    if (this.i < this.stop) { const v = this.i; this.i = this.i + 1; return { value: v, done: false }; }
    return { value: 0, done: true };
  }
  return(): { value: number; done: boolean } {
    console.log("closed");
    return { value: 0, done: true };
  }
}
for (const x of new Bounded(2)) { console.log(x); }
console.log("done");
`
	if got, want := runProgramGo(t, src), "0\n1\ndone\n"; got != want {
		t.Fatalf("normally-completing for...of printed %q, want %q", got, want)
	}
}

// TestForOfIteratorCloseOnReturnHandsBack proves the honest leftover: when the
// iterator defines return() and the loop body exits by a return that would jump
// past the after-loop close, the unit hands back rather than skip the close
// silently, keeping the zero-fail invariant.
func TestForOfIteratorCloseOnReturnHandsBack(t *testing.T) {
	src := closingIterator + `function f(): number {
  for (const x of new Guarded()) { if (x >= 2) { return x; } }
  return -1;
}
f();
`
	reason := renderProgramHandBack(t, src)
	if want := "iterator close on a return, throw, or labeled exit from for...of is a later slice"; reason != want {
		t.Fatalf("handback reason = %q, want %q", reason, want)
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
