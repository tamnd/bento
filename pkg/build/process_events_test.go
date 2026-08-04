package build

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildAndRunFileExit is buildAndRunFile for a program whose exit status is part of
// what is being pinned, so a non-zero status is a result rather than a test failure.
func buildAndRunFileExit(t *testing.T, name, src string) (string, int) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	prog, err := Build(Options{Entry: path, Output: filepath.Join(dir, "prog")})
	if err != nil {
		t.Fatalf("build %s: %v", name, err)
	}
	out, err := exec.Command(prog).CombinedOutput()
	code := 0
	if err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			t.Fatalf("run %s: %v (%s)", name, err, out)
		}
		code = ee.ExitCode()
	}
	return string(out), code
}

// buildFileErr builds an entry and returns the build error, for a shape whose
// hand-back is the point.
func buildFileErr(t *testing.T, name, src string) error {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	_, err := Build(Options{Entry: path, Output: filepath.Join(dir, "prog")})
	return err
}

// Node's process is an EventEmitter and its suite leans on four of its events: exit
// to assert at the end that something ran, beforeExit to do one last thing while the
// loop can still be put back to work, uncaughtException to take over from the crash,
// and unhandledRejection to see a rejection nobody consumed. These pin all four end to
// end, since what each one is worth is entirely in when it fires relative to the body.

// TestProcessExitListenerGetsTheCode pins that the exit listener is called with the
// status the process is leaving with, the one argument Node passes it.
func TestProcessExitListenerGetsTheCode(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"process.on('exit', (code) => { console.log('exit', code); });\n"+
			"console.log('body');\n")
	if want := "body\nexit 0\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestProcessBeforeExitRunsBeforeExit pins the order of the two end-of-run events:
// beforeExit fires while the process could still be put back to work, exit fires when
// it cannot.
func TestProcessBeforeExitRunsBeforeExit(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"process.on('exit', (code) => { console.log('exit', code); });\n"+
			"process.on('beforeExit', (code) => { console.log('beforeExit', code); });\n"+
			"console.log('body');\n")
	if want := "body\nbeforeExit 0\nexit 0\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestProcessBeforeExitFiresAgainAfterScheduledWork is the reason the event exists: a
// listener that schedules something puts the loop back to work, and the event fires
// once more when that work is done. Node prints the first listener line, the timer it
// scheduled, and then the second listener line.
func TestProcessBeforeExitFiresAgainAfterScheduledWork(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"let n = 0;\n"+
			"process.on('beforeExit', () => {\n"+
			"  n = n + 1;\n"+
			"  console.log('beforeExit', n);\n"+
			"  if (n === 1) { setTimeout(() => { console.log('timer'); }, 1); }\n"+
			"});\n"+
			"console.log('body');\n")
	if want := "body\nbeforeExit 1\ntimer\nbeforeExit 2\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestProcessExitSkipsBeforeExit pins that an explicit process.exit does not fire
// beforeExit, which Node reserves for a process that ran out of work rather than one
// that asked to leave, and that the exit listener gets the requested status.
func TestProcessExitSkipsBeforeExit(t *testing.T) {
	got, code := buildAndRunFileExit(t, "main.js",
		"process.on('beforeExit', () => { console.log('beforeExit'); });\n"+
			"process.on('exit', (c) => { console.log('exit', c); });\n"+
			"process.exit(3);\n")
	if want := "exit 3\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
	if code != 3 {
		t.Fatalf("exit status = %d, want 3", code)
	}
}

// TestProcessUncaughtExceptionTakesOverTheCrash pins that a listener suppresses the
// crash: the throw escapes every catch, the listener sees it, the exit listeners still
// run, and the program leaves cleanly. Without a listener the same throw prints an
// uncaught line and exits non-zero.
func TestProcessUncaughtExceptionTakesOverTheCrash(t *testing.T) {
	got, code := buildAndRunFileExit(t, "main.js",
		"process.on('uncaughtException', (err) => { console.log('caught', err.message); });\n"+
			"process.on('exit', (c) => { console.log('exit', c); });\n"+
			"throw new Error('boom');\n")
	if want := "caught boom\nexit 0\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
	if code != 0 {
		t.Fatalf("exit status = %d, want 0", code)
	}
}

// TestProcessUnhandledRejectionListenerSeesTheReason pins that a rejection nobody
// consumed reaches the listener rather than the stderr report, and that the program
// leaves cleanly because the listener handled it.
func TestProcessUnhandledRejectionListenerSeesTheReason(t *testing.T) {
	got, code := buildAndRunFileExit(t, "main.js",
		"process.on('unhandledRejection', (reason) => { console.log('rejected', reason.message); });\n"+
			"Promise.reject(new Error('nope'));\n"+
			"console.log('body');\n")
	if want := "body\nrejected nope\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
	if code != 0 {
		t.Fatalf("exit status = %d, want 0", code)
	}
}

// TestProcessListenerForAnEventNothingRaisesIsQuiet pins the rest of the surface: a
// listener for an event a compiled program has no source for registers and never
// fires, which is what Node does when nothing raises it. A program with no IPC channel
// never sees message, and one that emits no warning never sees warning.
func TestProcessListenerForAnEventNothingRaisesIsQuiet(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"process.on('warning', () => { console.log('warn'); });\n"+
			"process.on('message', () => { console.log('message'); });\n"+
			"console.log('body');\n")
	if want := "body\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestProcessSignalListenerHandsBack pins the one event that is refused rather than
// registered. The host really can deliver a signal, and a listener also suppresses the
// default disposition, so accepting one and never delivering it would change what the
// program does.
func TestProcessSignalListenerHandsBack(t *testing.T) {
	err := buildFileErr(t, "main.js", "process.on('SIGINT', () => {});\n")
	if err == nil {
		t.Fatal("a signal listener built, want a hand-back")
	}
	if want := "SIGINT"; !strings.Contains(err.Error(), want) {
		t.Fatalf("hand-back reason %q does not name %q", err, want)
	}
}
