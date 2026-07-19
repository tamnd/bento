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

// TestAsyncGeneratorInterleavesAwaitAndYield runs an async generator that awaits inside a
// loop and yields the running accumulator each pass, then returns it, checking the two
// kinds of suspend point stay ordered: each await settles before the value it feeds is
// yielded, so the consumer reads the accumulator after every step and the final return
// value once the loop ends. The await between yields keeps its pull pending until the
// awaited promise settles, so the yields still arrive one at a time in order.
func TestAsyncGeneratorInterleavesAwaitAndYield(t *testing.T) {
	src := `
async function* running(n: number): AsyncGenerator<number> {
  let acc = 0;
  for (let i = 0; i < n; i++) {
    const step = await Promise.resolve(i + 1);
    acc = acc + step;
    yield acc;
  }
  return acc;
}
async function run(): Promise<void> {
  const g = running(4);
  let r = await g.next();
  while (!r.done) {
    console.log("acc:" + r.value);
    r = await g.next();
  }
  console.log("final:" + r.value);
}
run();
console.log("sync");
`
	got := runProgramGo(t, src)
	want := "sync\nacc:1\nacc:3\nacc:6\nacc:10\nfinal:10\n"
	if got != want {
		t.Fatalf("interleaved await/yield = %q, want %q", got, want)
	}
}

// TestAsyncGeneratorDelegatesYieldStar runs an async generator that delegates part of its
// sequence to another async generator with yield*, checking the delegate's values flow out
// through the outer generator's pulls in order and the delegate's completion value becomes
// the value of the yield* expression. The delegate awaits between its own yields, so the
// outer drive stays parked while the delegate runs and the yields still arrive one at a
// time, and the outer generator resumes with its own yield once the delegate completes.
func TestAsyncGeneratorDelegatesYieldStar(t *testing.T) {
	src := `
async function* inner(): AsyncGenerator<number, number> {
  yield await Promise.resolve(1);
  yield await Promise.resolve(2);
  return 99;
}
async function* outer(): AsyncGenerator<number> {
  yield 0;
  const r = yield* inner();
  console.log("delegate returned:" + r);
  yield 3;
}
async function run(): Promise<void> {
  const g = outer();
  let res = await g.next();
  while (!res.done) {
    console.log("v:" + res.value);
    res = await g.next();
  }
}
run();
console.log("sync");
`
	got := runProgramGo(t, src)
	want := "sync\nv:0\nv:1\nv:2\ndelegate returned:99\nv:3\n"
	if got != want {
		t.Fatalf("yield* delegation = %q, want %q", got, want)
	}
}

func TestAsyncGenExprDestructuredParamRuns(t *testing.T) {
	// An async generator function expression with a destructured parameter injects the
	// entry bindings at the top of the coroutine body, so the pattern names read inside.
	const src = `const g = async function*([a, b]: number[]) { yield a; yield b; };
async function main(): Promise<void> {
  const it = g([10, 20]);
  console.log((await it.next()).value);
  console.log((await it.next()).value);
}
main();`
	if got, want := runProgramGo(t, src), "10\n20\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestAsyncGenObjectPatternWithAwaitRuns(t *testing.T) {
	// An object-pattern parameter and an await between yields share the one coroutine
	// handle, so the destructure bindings coexist with AsyncGenAwait.
	const src = `const g = async function*({ p, q }: { p: number; q: number }) {
  yield p;
  await Promise.resolve(0);
  yield q;
};
async function main(): Promise<void> {
  const it = g({ p: 5, q: 6 });
  console.log((await it.next()).value);
  console.log((await it.next()).value);
}
main();`
	if got, want := runProgramGo(t, src), "5\n6\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestAsyncGenDeclDestructuredParamRuns(t *testing.T) {
	// The declaration form shares the same coroutine builder, so the entry bindings land
	// there too.
	const src = `async function* g([a, b]: number[]) { yield a; yield b; }
async function main(): Promise<void> {
  const it = g([7, 8]);
  console.log((await it.next()).value);
  console.log((await it.next()).value);
}
main();`
	if got, want := runProgramGo(t, src), "7\n8\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
