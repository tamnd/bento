package runtime

import (
	"bytes"
	"strings"
	"testing"

	// Pull in the default engine backend for the end-to-end tests.
	_ "github.com/tamnd/bento/pkg/engine/quickjs"
)

func run(t *testing.T, source string) (string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	rt, err := New(Config{
		Argv:         []string{"bento", "test.ts"},
		BentoVersion: "test",
		Stdout:       &out,
		Stderr:       &errb,
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	defer func() { _ = rt.Close() }()
	if err := rt.RunString("test.ts", source); err != nil {
		t.Fatalf("run: %v", err)
	}
	return out.String(), errb.String()
}

func TestConsoleLog(t *testing.T) {
	out, _ := run(t, `console.log("hello", 42, [1,2,3]);`)
	if !strings.Contains(out, "hello 42") {
		t.Errorf("unexpected stdout: %q", out)
	}
	if !strings.Contains(out, "[ 1, 2, 3 ]") {
		t.Errorf("array formatting off: %q", out)
	}
}

func TestConsoleErrorGoesToStderr(t *testing.T) {
	out, errb := run(t, `console.error("boom");`)
	if strings.Contains(out, "boom") {
		t.Errorf("stderr content leaked to stdout: %q", out)
	}
	if !strings.Contains(errb, "boom") {
		t.Errorf("expected boom on stderr, got %q", errb)
	}
}

func TestProcessGlobals(t *testing.T) {
	out, _ := run(t, `
		console.log(typeof process.pid === "number");
		console.log(process.version.startsWith("v"));
		console.log(process.versions.bento);
		console.log(process.platform.length > 0);
	`)
	lines := strings.Fields(out)
	if len(lines) < 4 || lines[0] != "true" || lines[1] != "true" || lines[2] != "test" {
		t.Errorf("process globals wrong: %q", out)
	}
}

func TestMicrotaskAndTimerOrder(t *testing.T) {
	out, _ := run(t, `
		console.log("sync");
		Promise.resolve().then(() => console.log("micro"));
		setTimeout(() => console.log("timer"), 5);
	`)
	// Sync first, then the microtask, then the timer once the loop pumps.
	iSync := strings.Index(out, "sync")
	iMicro := strings.Index(out, "micro")
	iTimer := strings.Index(out, "timer")
	if iSync >= iMicro || iMicro >= iTimer {
		t.Errorf("ordering wrong, want sync<micro<timer, got:\n%s", out)
	}
}

func TestTimersComplete(t *testing.T) {
	out, _ := run(t, `
		let n = 0;
		const iv = setInterval(() => {
			n++;
			console.log("tick", n);
			if (n === 3) clearInterval(iv);
		}, 1);
	`)
	if strings.Count(out, "tick") != 3 {
		t.Errorf("expected 3 ticks, got:\n%s", out)
	}
}

func TestRequireUnknownThrows(t *testing.T) {
	var out, errb bytes.Buffer
	rt, err := New(Config{Argv: []string{"bento"}, Stdout: &out, Stderr: &errb})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	defer func() { _ = rt.Close() }()
	err = rt.RunString("t.ts", `require("nope-not-real")`)
	if err == nil {
		t.Fatal("expected require of unknown module to throw")
	}
}

// TestCaptureStackTraceWritesTheV8Shape pins the header V8 writes and the frames
// under it: a name and message on the first line, then the trace indented.
func TestCaptureStackTraceWritesTheV8Shape(t *testing.T) {
	out, _ := run(t, `
		const target = { name: "AppError", message: "went wrong" };
		function inner() { Error.captureStackTrace(target); }
		inner();
		const lines = target.stack.split("\n");
		console.log(lines[0]);
		console.log(lines[1].startsWith("    at "));
		console.log(lines[1].includes("inner"));
	`)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 3 || lines[0] != "AppError: went wrong" {
		t.Fatalf("header line wrong: %q", out)
	}
	if lines[1] != "true" || lines[2] != "true" {
		t.Errorf("frames wrong: %q", out)
	}
}

// TestCaptureStackTraceHidesTheConstructor pins the reason a custom error class
// passes its own constructor: that frame, and everything above it, is dropped.
func TestCaptureStackTraceHidesTheConstructor(t *testing.T) {
	out, _ := run(t, `
		class AppError extends Error {
			constructor(msg) { super(msg); this.name = "AppError"; Error.captureStackTrace(this, AppError); }
		}
		function build() { return new AppError("boom"); }
		const stack = build().stack;
		console.log(stack.split("\n")[0]);
		console.log(stack.includes("at AppError"));
		console.log(stack.split("\n")[1].includes("build"));
	`)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 3 || lines[0] != "AppError: boom" {
		t.Fatalf("header line wrong: %q", out)
	}
	if lines[1] != "false" {
		t.Errorf("the constructor frame survived: %q", out)
	}
	if lines[2] != "true" {
		t.Errorf("the caller's frame is not first: %q", out)
	}
}

// TestCaptureStackTraceHonorsTheLimit pins that stackTraceLimit trims the frame
// count and that zero leaves the header alone.
func TestCaptureStackTraceHonorsTheLimit(t *testing.T) {
	out, _ := run(t, `
		const target = { name: "E", message: "m" };
		function a() { Error.captureStackTrace(target); }
		function b() { a(); }
		function c() { b(); }
		console.log(Error.stackTraceLimit);
		Error.stackTraceLimit = 2;
		c();
		console.log(target.stack.split("\n").length);
		Error.stackTraceLimit = 0;
		c();
		console.log(JSON.stringify(target.stack));
	`)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 3 || lines[0] != "10" {
		t.Fatalf("default limit wrong: %q", out)
	}
	if lines[1] != "3" {
		t.Errorf("a limit of 2 gave %s lines, want 3 (header plus two frames)", lines[1])
	}
	if lines[2] != `"E: m"` {
		t.Errorf("a limit of 0 gave %s, want the header alone", lines[2])
	}
}

// TestCaptureStackTraceDoesNotEnumerate pins that a stack put on a plain object
// stays out of Object.keys and so out of JSON, the way V8 installs it.
func TestCaptureStackTraceDoesNotEnumerate(t *testing.T) {
	out, _ := run(t, `
		const target = { code: "E_THING" };
		Error.captureStackTrace(target);
		console.log(Object.keys(target).join(","));
		console.log(JSON.stringify(target));
		console.log(typeof target.stack);
	`)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 3 || lines[0] != "code" {
		t.Errorf("stack is enumerable: %q", out)
	}
	if lines[1] != `{"code":"E_THING"}` {
		t.Errorf("stack leaked into JSON: %q", out)
	}
	if lines[2] != "string" {
		t.Errorf("no stack was written: %q", out)
	}
}

// TestCaptureStackTraceRejectsAPrimitive pins the TypeError V8 raises rather
// than silently doing nothing.
func TestCaptureStackTraceRejectsAPrimitive(t *testing.T) {
	out, _ := run(t, `
		for (const bad of [null, undefined, 1, "s"]) {
			try { Error.captureStackTrace(bad); console.log("no throw"); }
			catch (e) { console.log(e.constructor.name); }
		}
	`)
	if got := strings.Fields(out); len(got) != 4 || got[0] != "TypeError" || got[3] != "TypeError" {
		t.Errorf("want four TypeErrors, got %q", out)
	}
}
