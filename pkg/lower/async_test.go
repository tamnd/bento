package lower

import (
	"strings"
	"testing"
)

// TestAsyncMethodEmitsResolvedPromise pins the shape half of the async method
// lowering: an await-free async method returns its body wrapped in value.Async,
// so a normal completion settles a resolved promise of the element type.
func TestAsyncMethodEmitsResolvedPromise(t *testing.T) {
	const src = `class Calc {
  x: number;
  constructor(x: number) {
    this.x = x;
  }
  async compute(): Promise<number> {
    return this.x * 2;
  }
}
new Calc(21).compute();
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "func (c *Calc) Compute() *value.Promise[float64]") {
		t.Errorf("async method did not return a promise of its element type:\n%s", source)
	}
	if !strings.Contains(source, "return value.Async(func() float64 {") {
		t.Errorf("async body did not wrap in value.Async:\n%s", source)
	}
}

// TestAsyncVoidMethodEmitsUnitPromise pins that an async method with no value
// lowers to a Promise<Unit> through value.AsyncVoid.
func TestAsyncVoidMethodEmitsUnitPromise(t *testing.T) {
	const src = `class Box {
  async log(msg: string): Promise<void> {
    console.log(msg);
  }
}
new Box().log("hi");
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "func (b *Box) Log(msg value.BStr) *value.Promise[value.Unit]") {
		t.Errorf("void async method did not return a unit promise:\n%s", source)
	}
	if !strings.Contains(source, "return value.AsyncVoid(func() {") {
		t.Errorf("void async body did not wrap in value.AsyncVoid:\n%s", source)
	}
}

// TestStaticAsyncMethodEmitsPackageFunc pins that a static async method lowers
// to a package func returning a promise, the same closure wrapping as an
// instance method.
func TestStaticAsyncMethodEmitsPackageFunc(t *testing.T) {
	const src = `class Calc {
  static async of(v: number): Promise<number> {
    return v + 1;
  }
}
Calc.of(4);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "func CalcOf(v float64) *value.Promise[float64]") {
		t.Errorf("static async method did not become a package func:\n%s", source)
	}
	if !strings.Contains(source, "return value.Async(func() float64 {") {
		t.Errorf("static async body did not wrap in value.Async:\n%s", source)
	}
}

// TestAsyncFuncDeclEmitsResolvedPromise pins that a top-level async function
// declaration lowers to a package func returning a promise of its element type,
// its await-free body wrapped in value.Async the same way an async method is.
func TestAsyncFuncDeclEmitsResolvedPromise(t *testing.T) {
	const src = `async function double(n: number): Promise<number> {
  return n * 2;
}
double(21);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "func Double(n float64) *value.Promise[float64]") {
		t.Errorf("async function declaration did not return a promise of its element type:\n%s", source)
	}
	if !strings.Contains(source, "return value.Async(func() float64 {") {
		t.Errorf("async function body did not wrap in value.Async:\n%s", source)
	}
}

// TestAsyncVoidFuncDeclEmitsUnitPromise pins that an async function declaration
// with no value lowers to a Promise<Unit> through value.AsyncVoid.
func TestAsyncVoidFuncDeclEmitsUnitPromise(t *testing.T) {
	const src = `async function shout(msg: string): Promise<void> {
  console.log(msg);
}
shout("hi");
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "func Shout(msg value.BStr) *value.Promise[value.Unit]") {
		t.Errorf("void async function did not return a unit promise:\n%s", source)
	}
	if !strings.Contains(source, "return value.AsyncVoid(func() {") {
		t.Errorf("void async function body did not wrap in value.AsyncVoid:\n%s", source)
	}
}

// TestAsyncArrowEmitsResolvedPromise pins that an async arrow returns a promise,
// with a concise body wrapped directly in value.Async.
func TestAsyncArrowEmitsResolvedPromise(t *testing.T) {
	const src = `const triple = async (n: number): Promise<number> => n * 3;
triple(4);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "func(n float64) *value.Promise[float64]") {
		t.Errorf("async arrow did not return a promise of its element type:\n%s", source)
	}
	if !strings.Contains(source, "return value.Async(func() float64 {") {
		t.Errorf("async arrow body did not wrap in value.Async:\n%s", source)
	}
}

// TestAsyncFuncExprEmitsResolvedPromise pins that an async function expression
// returns a promise, its block body wrapped in value.Async.
func TestAsyncFuncExprEmitsResolvedPromise(t *testing.T) {
	const src = `const inc = async function (n: number): Promise<number> {
  return n + 1;
};
inc(9);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "func(n float64) *value.Promise[float64]") {
		t.Errorf("async function expression did not return a promise:\n%s", source)
	}
	if !strings.Contains(source, "return value.Async(func() float64 {") {
		t.Errorf("async function expression body did not wrap in value.Async:\n%s", source)
	}
}

// TestAsyncAwaitEmitsCoroutine pins the shape of a body that awaits: it lowers
// through value.RunAsync over an *value.AsyncCo handle instead of value.Async, and
// each await becomes a value.Await on that handle.
func TestAsyncAwaitEmitsCoroutine(t *testing.T) {
	const src = `async function base(): Promise<number> {
  return 1;
}
async function load(): Promise<number> {
  const a = await base();
  return a + 1;
}
load();
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "return value.RunAsync[float64](func(") {
		t.Errorf("awaiting async body did not wrap in value.RunAsync:\n%s", source)
	}
	if !strings.Contains(source, "*value.AsyncCo") {
		t.Errorf("coroutine func did not take an *value.AsyncCo handle:\n%s", source)
	}
	if !strings.Contains(source, "value.Await(") {
		t.Errorf("await did not lower to value.Await:\n%s", source)
	}
	// The await-free base still keeps the synchronous value.Async wrapping, so only a
	// body that actually awaits pays for the coroutine.
	if !strings.Contains(source, "return value.Async(func() float64 {") {
		t.Errorf("await-free async body lost its synchronous value.Async wrapping:\n%s", source)
	}
}

// TestAsyncVoidAwaitEmitsCoroutine pins that a void body that awaits lowers through
// value.RunAsyncVoid, the unit-promise counterpart of value.RunAsync.
func TestAsyncVoidAwaitEmitsCoroutine(t *testing.T) {
	const src = `async function base(): Promise<number> {
  return 1;
}
async function run(): Promise<void> {
  const a = await base();
  console.log(a);
}
run();
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "return value.RunAsyncVoid(func(") {
		t.Errorf("awaiting void async body did not wrap in value.RunAsyncVoid:\n%s", source)
	}
}

// TestAsyncAwaitValueEmitsAwaitValue pins that awaiting a plain non-promise primitive
// lowers to value.AwaitValue with the operand's element type pinned, so an untyped
// operand crosses to the value's Go type.
func TestAsyncAwaitValueEmitsAwaitValue(t *testing.T) {
	const src = `async function run(): Promise<number> {
  const a = await 40;
  return a + 2;
}
run();
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.AwaitValue[float64](") {
		t.Errorf("await on a plain value did not lower to value.AwaitValue:\n%s", source)
	}
}

// TestAsyncAwaitRunsInOrder runs the emitted Go and pins the suspend-and-resume: the
// body runs synchronously up to its first await, parks, and the code after the await
// runs in a later microtask turn, after the synchronous run has finished.
func TestAsyncAwaitRunsInOrder(t *testing.T) {
	const src = `async function base(): Promise<number> {
  return 1;
}
async function load(): Promise<number> {
  console.log("body-start");
  const a = await base();
  console.log("after-await:" + a);
  return a + 1;
}
console.log("before");
load().then(v => console.log("result:" + v));
console.log("after-call");
`
	got := runProgramGo(t, src)
	want := "before\nbody-start\nafter-call\nafter-await:1\nresult:2\n"
	if got != want {
		t.Errorf("await ordering wrong\n got: %q\nwant: %q", got, want)
	}
}

// TestAsyncAwaitThenThrowRejects runs the emitted Go and pins that a throw after an
// await rejects the returned promise: the coroutine unwinds the throw and settles its
// promise rejected, which a .catch observes at the microtask checkpoint.
func TestAsyncAwaitThenThrowRejects(t *testing.T) {
	const src = `async function base(): Promise<number> {
  return 1;
}
async function boom(): Promise<number> {
  const a = await base();
  if (a > 0) {
    throw new Error("bang:" + a);
  }
  return a;
}
boom().catch((e) => console.log("caught:" + e.message));
console.log("sync");
`
	got := runProgramGo(t, src)
	want := "sync\ncaught:bang:1\n"
	if got != want {
		t.Errorf("reject-after-await ordering wrong\n got: %q\nwant: %q", got, want)
	}
}

// TestAwaitRejectedThrowsIntoBody runs the emitted Go and pins that awaiting a rejected
// promise raises the rejection at the await, where a try/catch around it recovers: the
// body returns a fallback and the returned promise fulfills with it.
func TestAwaitRejectedThrowsIntoBody(t *testing.T) {
	const src = `async function rejects(): Promise<number> {
  throw new Error("inner");
  return 0;
}
async function outer(): Promise<number> {
  try {
    return await rejects();
  } catch (e: any) {
    console.log("caught-in-body:" + e.message);
    return -1;
  }
}
outer().then((v) => console.log("result:" + v));
console.log("sync");
`
	got := runProgramGo(t, src)
	want := "sync\ncaught-in-body:inner\nresult:-1\n"
	if got != want {
		t.Errorf("await-on-rejected ordering wrong\n got: %q\nwant: %q", got, want)
	}
}

// TestAsyncMethodResolvesAfterSyncCode runs the emitted Go and pins the
// microtask ordering: a .then callback registered during the synchronous run
// fires only after that run completes, at the end-of-main drain.
func TestAsyncMethodResolvesAfterSyncCode(t *testing.T) {
	const src = `class Calc {
  x: number;
  constructor(x: number) {
    this.x = x;
  }
  async compute(): Promise<number> {
    return this.x * 2;
  }
}
console.log("sync-1");
new Calc(21).compute().then(v => console.log(v));
console.log("sync-2");
`
	got := runProgramGo(t, src)
	want := "sync-1\nsync-2\n42\n"
	if got != want {
		t.Errorf("microtask ordering wrong\n got: %q\nwant: %q", got, want)
	}
}

// TestAsyncMethodThrowRejects runs the emitted Go and pins that a synchronous
// throw inside an async body becomes a rejected promise a .catch observes,
// after the synchronous run.
func TestAsyncMethodThrowRejects(t *testing.T) {
	const src = `class Box {
  async boom(): Promise<number> {
    throw new Error("bang");
    return 0;
  }
}
new Box().boom().catch(e => console.log("caught:" + e.message));
console.log("sync");
`
	got := runProgramGo(t, src)
	want := "sync\ncaught:bang\n"
	if got != want {
		t.Errorf("rejection ordering wrong\n got: %q\nwant: %q", got, want)
	}
}

// TestAsyncHandsBack pins the boundary: each async construct outside the
// await-free subset hands the unit back with its own honest reason rather than
// mislowering the suspension or propagation away.
func TestAsyncHandsBack(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			// A block-bodied await lowers through the coroutine now; a concise-bodied
			// arrow keeps the synchronous path and has no handle to park on, so its
			// await still hands back.
			"conciseAwait",
			"const f = async (): Promise<number> => await Promise.resolve(1);\nf();\n",
			"an await outside a lowered async body is a later slice",
		},
		{
			"chainingThen",
			"class C { async n(): Promise<number> { return 1; } }\nnew C().n().then(v => v + 1);\n",
			"later slice",
		},
		{
			"asyncGenerator",
			"class C { async *g(): AsyncGenerator<number> { yield 1; } }\nconsole.log(\"x\");\n",
			"async generator",
		},
		{
			"staticAsyncGenerator",
			"class C { static async *g(): AsyncGenerator<number> { yield 1; } }\nconsole.log(\"x\");\n",
			"async generator",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reason := renderProgramHandBack(t, tc.src)
			if !strings.Contains(reason, tc.want) {
				t.Errorf("hand-back reason %q does not name %q", reason, tc.want)
			}
		})
	}
}
