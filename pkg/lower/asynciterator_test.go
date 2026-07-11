package lower

import (
	"strings"
	"testing"
)

// TestSymbolAsyncIteratorMethodLowers checks that a user class's [Symbol.asyncIterator]
// method lowers to a Go method under the fixed SymbolAsyncIterator name, the async
// mirror of the SymbolIterator method for...of obtains a sync iterator through, and that
// a manual obj[Symbol.asyncIterator]() reference reads that method. The method returns
// the async iterator, an object whose next() returns a promise of the { value, done }
// result; for await...of drives it in a later slice, but the factory itself resolves here.
func TestSymbolAsyncIteratorMethodLowers(t *testing.T) {
	src := `
class Counter {
  next(): Promise<{ value: number; done: boolean }> {
    return Promise.resolve({ value: 1, done: true });
  }
  [Symbol.asyncIterator]() { return this; }
}
async function run(): Promise<void> {
  const c = new Counter();
  const it = c[Symbol.asyncIterator]();
  console.log("ok");
}
run();
`
	got := renderProgram(t, src)
	if !strings.Contains(got, "func (c *Counter) SymbolAsyncIterator()") {
		t.Errorf("[Symbol.asyncIterator] method did not lower to a SymbolAsyncIterator Go method:\n%s", got)
	}
	if !strings.Contains(got, "c.SymbolAsyncIterator()") {
		t.Errorf("manual obj[Symbol.asyncIterator]() did not lower to a SymbolAsyncIterator call:\n%s", got)
	}
}

// TestAsyncIteratorResultAwaits checks that awaiting a user async iterator's next()
// settles to its { value, done } result and the body reads value and done off it: the
// awaited result crosses the settle path the way any awaited promise does, and the
// object it fulfills with resolves to the struct fields the reads select. This is the
// pull step for await...of drives, exercised here by hand so the result path is proven
// before the loop wraps it.
func TestAsyncIteratorResultAwaits(t *testing.T) {
	src := `
class Counter {
  next(): Promise<{ value: number; done: boolean }> {
    return Promise.resolve({ value: 7, done: false });
  }
  [Symbol.asyncIterator]() { return this; }
}
async function run(): Promise<void> {
  const c = new Counter();
  const it = c[Symbol.asyncIterator]();
  const r = await it.next();
  console.log("value:" + r.value);
  console.log("done:" + r.done);
}
run();
`
	got := runProgramGo(t, src)
	want := "value:7\ndone:false\n"
	if got != want {
		t.Fatalf("async iterator result = %q, want %q", got, want)
	}
}

// TestForAwaitOfAsyncIterable checks that a for await...of over a user async iterable
// awaits each next() result before the body runs, binds its value, and stops when the
// result is done: the async mirror of a for...of over a user iterable, driven through
// the [Symbol.asyncIterator] factory and value.Await on each pull. The loop runs inside
// an async body, so it parks and resumes on the microtask queue, and the synchronous
// code after the call runs first, the ordering Node fixes for the awaited loop.
func TestForAwaitOfAsyncIterable(t *testing.T) {
	src := `
class Counter {
  i = 0;
  next(): Promise<{ value: number; done: boolean }> {
    const cur = this.i;
    this.i = cur + 1;
    return Promise.resolve({ value: cur, done: cur >= 3 });
  }
  [Symbol.asyncIterator]() { return this; }
}
async function run(): Promise<void> {
  console.log("start");
  for await (const x of new Counter()) {
    console.log("x:" + x);
  }
  console.log("end");
}
run();
console.log("sync");
`
	got := runProgramGo(t, src)
	want := "start\nsync\nx:0\nx:1\nx:2\nend\n"
	if got != want {
		t.Fatalf("for await...of = %q, want %q", got, want)
	}
}

// TestForAwaitOfClosesOnBreak checks that breaking out of a for await...of over a user
// async iterable that defines return() closes the iterator, awaiting the promise return()
// hands back. The loop pulls three values, breaks on the third, then calls return() once,
// so the close runs after the last body pass and before the after-loop statement. A loop
// that runs to completion never calls return(), so the close is reached only on the early
// break, the same broke-flag gate the sync iterator close uses.
func TestForAwaitOfClosesOnBreak(t *testing.T) {
	src := `
class Counter {
  n: number;
  i: number;
  constructor(n: number) { this.n = n; this.i = 0; }
  [Symbol.asyncIterator](): Counter { return this; }
  async next(): Promise<{ value: number; done: boolean }> {
    if (this.i < this.n) { const v = this.i; this.i++; return { value: v, done: false }; }
    return { value: 0, done: true };
  }
  async return(): Promise<{ value: number; done: boolean }> {
    console.log("closed at " + this.i);
    return { value: 0, done: true };
  }
}
async function run(): Promise<void> {
  for await (const x of new Counter(5)) {
    console.log("x:" + x);
    if (x === 2) break;
  }
  console.log("after");
}
run();
console.log("sync");
`
	got := runProgramGo(t, src)
	want := "sync\nx:0\nx:1\nx:2\nclosed at 3\nafter\n"
	if got != want {
		t.Fatalf("for await...of close on break = %q, want %q", got, want)
	}
}

// TestForAwaitOfNoCloseOnCompletion checks that a for await...of that runs to completion
// over an async iterable with return() never calls return(): the loop stops when next()
// reports done, which clears the broke flag, so the after-loop close is skipped. Only an
// early exit closes the iterator, matching the protocol.
func TestForAwaitOfNoCloseOnCompletion(t *testing.T) {
	src := `
class Counter {
  n: number;
  i: number;
  constructor(n: number) { this.n = n; this.i = 0; }
  [Symbol.asyncIterator](): Counter { return this; }
  async next(): Promise<{ value: number; done: boolean }> {
    if (this.i < this.n) { const v = this.i; this.i++; return { value: v, done: false }; }
    return { value: 0, done: true };
  }
  async return(): Promise<{ value: number; done: boolean }> {
    console.log("closed");
    return { value: 0, done: true };
  }
}
async function run(): Promise<void> {
  for await (const x of new Counter(3)) {
    console.log("x:" + x);
  }
  console.log("after");
}
run();
console.log("sync");
`
	got := runProgramGo(t, src)
	want := "sync\nx:0\nx:1\nx:2\nafter\n"
	if got != want {
		t.Fatalf("for await...of no close on completion = %q, want %q", got, want)
	}
}

// TestForAwaitOfSyncArrayOfPromises checks that a for await...of over an array of
// promises awaits each element before the body runs, the fallback the spec takes for a
// sync iterable with no [Symbol.asyncIterator]: the array yields its promises
// synchronously and for await settles each one, so the body binds the fulfilled value.
// The loop parks inside the async body, so the synchronous tail runs before the first
// element, the ordering Node fixes for the awaited loop.
func TestForAwaitOfSyncArrayOfPromises(t *testing.T) {
	src := `
async function run(): Promise<void> {
  console.log("start");
  for await (const x of [Promise.resolve(1), Promise.resolve(2), Promise.resolve(3)]) {
    console.log("x:" + x);
  }
  console.log("end");
}
run();
console.log("sync");
`
	got := runProgramGo(t, src)
	want := "start\nsync\nx:1\nx:2\nx:3\nend\n"
	if got != want {
		t.Fatalf("for await...of array of promises = %q, want %q", got, want)
	}
}

// TestForAwaitOfSyncArrayOfValues checks that a for await...of over an array of plain
// values awaits each one, which JavaScript wraps in a resolved promise: the value comes
// straight back after the one-turn delay await imposes, so the body binds each number in
// order. Same fallback as the promise case, routed through value.AwaitValue rather than
// value.Await because the element is a definite non-thenable.
func TestForAwaitOfSyncArrayOfValues(t *testing.T) {
	src := `
async function run(): Promise<void> {
  console.log("start");
  for await (const n of [10, 20, 30]) {
    console.log("n:" + n);
  }
  console.log("end");
}
run();
console.log("sync");
`
	got := runProgramGo(t, src)
	want := "start\nsync\nn:10\nn:20\nn:30\nend\n"
	if got != want {
		t.Fatalf("for await...of array of values = %q, want %q", got, want)
	}
}
