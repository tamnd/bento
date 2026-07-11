package lower

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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

// TestPromiseThenChaining checks that a then whose callback returns a value chains the
// value to the next then, that a callback returning a promise flattens so the next then
// reads the inner value, and that a rejection passes through a value-returning then with
// no rejection handler to the catch further down the chain.
func TestPromiseThenChaining(t *testing.T) {
	src := `
Promise.resolve(1)
  .then((v) => v + 1)
  .then((v) => "n:" + v)
  .then((s) => console.log(s));

Promise.resolve(10)
  .then((v) => Promise.resolve(v * 2))
  .then((v) => console.log("flat:" + v));

const failing: Promise<number> = Promise.reject("boom");
failing
  .then((v) => v + 100)
  .catch((e) => console.log("caught:" + e));

console.log("sync");
`
	got := runProgramGo(t, src)
	want := "sync\ncaught:boom\nn:2\nflat:20\n"
	if got != want {
		t.Fatalf("promise then chaining = %q, want %q", got, want)
	}
}

// TestUnhandledRejectionReportsAndExits proves the unhandled-rejection path end to end:
// a program that rejects a promise and never observes it runs to the end of main, drains
// the microtask queue, then reports the rejection to standard error and exits non-zero,
// the way a runtime surfaces an unhandledrejection once the checkpoint is clear. Building
// and running is the oracle: the whole point is that a test asserting a rejection can
// observe it through the crash rather than have it vanish into a false pass. The
// synchronous log still reaches stdout, since the report runs only after the run finishes.
func TestUnhandledRejectionReportsAndExits(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the unhandled-rejection test builds and runs generated Go")
	}
	source := renderProgram(t, "const failing: Promise<number> = Promise.reject(\"boom\");\nconsole.log(\"sync\");\n")
	dir, err := os.MkdirTemp(repoRoot(t), "rejectrun-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err == nil {
		t.Fatalf("an unhandled rejection exited zero:\n--- program ---\n%s", source)
	}
	if _, ok := err.(*exec.ExitError); !ok {
		t.Fatalf("go run failed to launch: %v\n--- program ---\n%s\n--- stderr ---\n%s", err, source, stderr.String())
	}
	if got := stderr.String(); !strings.Contains(got, "Uncaught (in promise) boom") {
		t.Errorf("unhandled rejection printed %q to stderr, want the unhandled-rejection line", got)
	}
	if got := stdout.String(); got != "sync\n" {
		t.Errorf("unhandled rejection wrote %q to stdout, want the synchronous log only", got)
	}
}
