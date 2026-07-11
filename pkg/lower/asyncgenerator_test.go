package lower

import (
	"strings"
	"testing"
)

// TestAsyncGeneratorFuncLowers checks that an async generator function lowers to a Go
// function returning the *value.AsyncGen coroutine: the async plus generator pair routes
// to the async generator form rather than handing back, the body wraps in
// value.NewAsyncGen, and a yield in it lowers to a call on the coroutine handle. The
// coroutine is the runtime the manual next() and the for await...of both drive.
func TestAsyncGeneratorFuncLowers(t *testing.T) {
	src := `
async function* ag(): AsyncGenerator<number> {
  yield 1;
  yield 2;
}
ag();
`
	got := renderProgram(t, src)
	if !strings.Contains(got, "*value.AsyncGen[float64]") {
		t.Errorf("async generator did not return a *value.AsyncGen coroutine:\n%s", got)
	}
	if !strings.Contains(got, "value.NewAsyncGen[float64]") {
		t.Errorf("async generator body did not wrap in value.NewAsyncGen:\n%s", got)
	}
}

// TestAsyncGeneratorDrivesAwaitAndYield runs an async generator that yields, awaits
// between yields, and completes, pulled by hand through await g.next() the way a manual
// consumer drives it. Each pull resolves to the { value, done } result on the microtask
// queue, so the synchronous tail runs before the first value and the loop reads each
// yielded value in order, the ordering Node fixes for the awaited pulls.
func TestAsyncGeneratorDrivesAwaitAndYield(t *testing.T) {
	src := `
async function* ag(): AsyncGenerator<number> {
  yield 1;
  await Promise.resolve(0);
  yield 2;
  yield 3;
}
async function run(): Promise<void> {
  console.log("start");
  const g = ag();
  let r = await g.next();
  while (!r.done) {
    console.log("v:" + r.value);
    r = await g.next();
  }
  console.log("end");
}
run();
console.log("sync");
`
	got := runProgramGo(t, src)
	want := "start\nsync\nv:1\nv:2\nv:3\nend\n"
	if got != want {
		t.Fatalf("async generator drive = %q, want %q", got, want)
	}
}
