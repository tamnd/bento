package lower

import "testing"

// TestNewPromiseResolveReachesThen checks that new Promise runs its executor now and
// that a resolve settles the promise so a then callback fires, after the synchronous
// code, with the fulfilled value: the microtask ordering the Promise tests observe.
func TestNewPromiseResolveReachesThen(t *testing.T) {
	src := `
console.log("before");
new Promise<number>((resolve, reject) => {
  resolve(41);
}).then((v) => console.log("then:" + (v + 1)));
console.log("after");
`
	got := runProgramGo(t, src)
	want := "before\nafter\nthen:42\n"
	if got != want {
		t.Fatalf("promise resolve order = %q, want %q", got, want)
	}
}

// TestPromiseFinallyRunsOnSettle checks that a finally callback runs when the
// promise settles, after the then reaction and after the synchronous code, taking no
// argument: the cleanup reaction scheduled as a microtask in settle order.
func TestPromiseFinallyRunsOnSettle(t *testing.T) {
	src := `
new Promise<number>((resolve, reject) => { resolve(1); })
  .then((v) => console.log("then:" + v))
  .finally(() => console.log("finally"));
console.log("sync");
`
	got := runProgramGo(t, src)
	want := "sync\nthen:1\nfinally\n"
	if got != want {
		t.Fatalf("promise finally order = %q, want %q", got, want)
	}
}

// TestNewPromiseRejectReachesCatch checks that a reject settles the promise as
// rejected carrying the arbitrary value it was handed, so a catch callback reads that
// value back, a plain string here rather than an Error.
func TestNewPromiseRejectReachesCatch(t *testing.T) {
	src := `
new Promise<number>((resolve, reject) => {
  reject("boom");
}).catch((e) => console.log("caught:" + e));
`
	got := runProgramGo(t, src)
	want := "caught:boom\n"
	if got != want {
		t.Fatalf("promise reject = %q, want %q", got, want)
	}
}

// TestPromiseResolveAndReject checks the constructor-level factories: Promise.resolve
// mints a settled promise a then reads, Promise.reject one a catch reads, and
// Promise.resolve of a promise hands that promise straight back so its fulfilled value
// flows through unchanged.
func TestPromiseResolveAndReject(t *testing.T) {
	src := `
Promise.resolve(41).then((v) => console.log("r:" + v));
Promise.reject("boom").catch((e) => console.log("j:" + e));
const p: Promise<number> = Promise.resolve(7);
Promise.resolve(p).then((v) => console.log("id:" + v));
console.log("sync");
`
	got := runProgramGo(t, src)
	want := "sync\nr:41\nj:boom\nid:7\n"
	if got != want {
		t.Fatalf("promise resolve/reject = %q, want %q", got, want)
	}
}

// TestPromiseAllFulfillsInOrder checks that Promise.all fulfills with the inputs'
// values in input order once every input has fulfilled, after the synchronous code,
// and that a single rejection among the inputs rejects the combined promise with that
// reason rather than fulfilling.
func TestPromiseAllFulfillsInOrder(t *testing.T) {
	src := `
const ps: Promise<number>[] = [Promise.resolve(1), Promise.resolve(2), Promise.resolve(3)];
Promise.all(ps).then((vs) => console.log("all:" + vs.join(",")));

const mixed: Promise<number>[] = [Promise.resolve(9), Promise.reject("bad")];
Promise.all(mixed).catch((e) => console.log("caught:" + e));
console.log("sync");
`
	got := runProgramGo(t, src)
	want := "sync\nall:1,2,3\ncaught:bad\n"
	if got != want {
		t.Fatalf("promise all = %q, want %q", got, want)
	}
}

// TestPromiseRaceAndAny checks that Promise.race settles the way the first input
// settles, that Promise.any fulfills with the first fulfillment even past a rejection,
// and that an all-rejected any rejects with an AggregateError whose errors array carries
// the reasons in input order.
func TestPromiseRaceAndAny(t *testing.T) {
	src := `
const first: Promise<number>[] = [Promise.resolve(1), Promise.reject("no")];
Promise.race(first).then((v) => console.log("race:" + v));

const slow: Promise<number>[] = [Promise.reject("a"), Promise.reject("b")];
Promise.any(slow).catch((e) => {
  console.log("any-name:" + e.name);
  console.log("any-errors:" + e.errors[0] + "," + e.errors[1]);
});

const win: Promise<number>[] = [Promise.reject("x"), Promise.resolve(5)];
Promise.any(win).then((v) => console.log("any-ok:" + v));
console.log("sync");
`
	got := runProgramGo(t, src)
	want := "sync\nrace:1\nany-name:AggregateError\nany-errors:a,b\nany-ok:5\n"
	if got != want {
		t.Fatalf("promise race/any = %q, want %q", got, want)
	}
}
