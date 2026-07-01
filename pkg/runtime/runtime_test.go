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
