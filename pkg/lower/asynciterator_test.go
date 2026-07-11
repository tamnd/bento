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
