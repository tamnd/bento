package build

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

// skipWithoutSignals skips a test on a platform with no signal to send. Windows has a
// few signal names and no way to raise one, so a program there registers a listener
// that can never fire, which is what Node does on windows too.
func skipWithoutSignals(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("no deliverable signals on windows")
	}
}

// TestProcessSignalIsDeliveredOnALoopTurn pins where a signal lands: the loop takes it
// at the top of a turn, ahead of the timers that turn would run, so a program that
// signals itself sees the rest of the current statement list first, then the handler,
// then the timer. The handler is called with the signal's name and its number, the two
// arguments Node passes. Every line of this was read off node 24.4.1.
func TestProcessSignalIsDeliveredOnALoopTurn(t *testing.T) {
	skipWithoutSignals(t)
	got := buildAndRunFile(t, "main.js",
		"process.on('SIGUSR2', (name, num) => { console.log('got', name, typeof num); });\n"+
			"setTimeout(() => { console.log('timer'); }, 50);\n"+
			"process.kill(process.pid, 'SIGUSR2');\n"+
			"console.log('after kill');\n")
	if want := "after kill\ngot SIGUSR2 number\ntimer\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestProcessSignalWithNothingScheduledIsNotDelivered pins the other half of that rule,
// which is the surprising half: a signal listener does not keep a program alive, so a
// program whose only remaining reason to run is the listener has already left by the
// time the signal could be delivered. Node prints exactly this, and only this.
func TestProcessSignalWithNothingScheduledIsNotDelivered(t *testing.T) {
	skipWithoutSignals(t)
	got := buildAndRunFile(t, "main.js",
		"process.on('SIGUSR2', () => { console.log('got it'); });\n"+
			"process.kill(process.pid, 'SIGUSR2');\n"+
			"console.log('after kill');\n")
	if want := "after kill\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestProcessRemoveAllListenersRestoresTheDefaultDisposition pins that the disarm is
// real. A listener suppresses what the signal would otherwise do, so taking the last
// one away has to hand the signal back to the host: after the removal the program is
// killed by its own SIGINT rather than printing the line after it.
//
// The status is not checked as a number because a process killed by a signal has no
// exit code of its own, only the signal that ended it; that the line after the kill
// never printed is the same fact and reads the same on every platform.
func TestProcessRemoveAllListenersRestoresTheDefaultDisposition(t *testing.T) {
	skipWithoutSignals(t)
	got, code := buildAndRunFileExit(t, "main.js",
		"process.on('SIGINT', () => { console.log('handled'); });\n"+
			"process.removeAllListeners('SIGINT');\n"+
			"console.log('removed', process.listenerCount('SIGINT'));\n"+
			"process.kill(process.pid, 'SIGINT');\n"+
			"console.log('still here');\n")
	if want := "removed 0\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
	if code == 0 {
		t.Fatal("the program left cleanly, want it killed by its own SIGINT")
	}
}

// TestProcessEmitterSurface pins the emitter methods against what node 24.4.1 prints
// for the same program: once runs one time and is gone, prependListener puts a listener
// at the front, off removes what on registered, emit answers whether anything was
// listening, and listenerCount follows all of it.
func TestProcessEmitterSurface(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const a = () => { console.log('a'); };\n"+
			"const b = () => { console.log('b'); };\n"+
			"process.on('ping', a);\n"+
			"process.once('ping', b);\n"+
			"process.prependListener('ping', () => console.log('first'));\n"+
			"console.log('count', process.listenerCount('ping'));\n"+
			"console.log('emit', process.emit('ping'));\n"+
			"console.log('emit', process.emit('ping'));\n"+
			"process.off('ping', a);\n"+
			"console.log('emit', process.emit('ping'));\n"+
			"console.log('nobody', process.emit('nobody'));\n")
	want := "count 3\nfirst\na\nb\nemit true\nfirst\na\nemit true\nfirst\nemit true\nnobody false\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestProcessOffFindsAListenerBoxedAtAnotherSite pins the identity that makes removal
// work at all. The listener is boxed once where it is registered and again where it is
// removed, and the two have to be the same value for removeListener to find it, which
// is what the shared box gives a module-level binding.
func TestProcessOffFindsAListenerBoxedAtAnotherSite(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const h = () => { console.log('h'); };\n"+
			"process.on('e', h);\n"+
			"process.removeListener('e', h);\n"+
			"console.log('left', process.listenerCount('e'));\n"+
			"console.log('emit', process.emit('e'));\n")
	if want := "left 0\nemit false\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestProcessOnAnEventNamedAtRunTime pins that the event name does not have to be a
// literal. The registration goes through the process object, which reads the name at
// run time, so a computed name and a symbol both reach the same registry.
func TestProcessOnAnEventNamedAtRunTime(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const evName = 'ev' + 1;\n"+
			"process.on(evName, (x) => { console.log('got', x); });\n"+
			"const s = Symbol('tag');\n"+
			"process.on(s, (x) => { console.log('sym', x); });\n"+
			"process.emit('ev1', 'a');\n"+
			"process.emit(s, 'b');\n")
	if want := "got a\nsym b\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestProcessOnRejectsANonFunctionListener pins Node's ERR_INVALID_ARG_TYPE, down to
// the message: a listener that is not callable is refused where it is registered rather
// than failing later in an emit that no longer names the registration.
func TestProcessOnRejectsANonFunctionListener(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"try { process.on('x', 'notafunction'); } catch (e) { console.log(e.code, e.message); }\n")
	want := "ERR_INVALID_ARG_TYPE The \"listener\" argument must be of type function. " +
		"Received type string ('notafunction')\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestProcessOnAnUncatchableSignalThrows pins the other refusal Node makes at
// registration. SIGKILL and SIGSTOP cannot be caught by anyone, so a listener for one
// would be a promise the platform cannot keep, and Node answers with the libuv error
// rather than accepting it.
func TestProcessOnAnUncatchableSignalThrows(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"try { process.on('SIGKILL', () => {}); } catch (e) { console.log(e.code, e.message); }\n"+
			"console.log('count', process.listenerCount('SIGKILL'));\n")
	if want := "EINVAL uv_signal_start EINVAL\ncount 0\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestProcessKillReportsAnUnknownSignal pins that a misspelled signal name is an error
// rather than a call that quietly sends nothing, and that signal 0 sends nothing on
// purpose and answers whether the process is there, which is what a liveness check uses.
func TestProcessKillReportsAnUnknownSignal(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"try { process.kill(process.pid, 'SIGFOO'); } catch (e) { console.log(e.code, e.message); }\n"+
			"console.log('alive', process.kill(process.pid, 0));\n")
	if want := "ERR_UNKNOWN_SIGNAL Unknown signal: SIGFOO\nalive true\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
