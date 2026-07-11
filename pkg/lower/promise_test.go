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
