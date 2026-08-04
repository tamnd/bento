//go:build unix

package value

import (
	"os"
	"syscall"
	"testing"
	"time"
)

// The signal a test raises is SIGUSR2 wherever one is raised at all. It is the one
// signal nothing in a Go test binary is already using: SIGURG belongs to the runtime's
// preemption, SIGCHLD and SIGWINCH arrive on their own, and SIGINT would end the run if
// the arming under test did not work. Each test disarms what it armed, so the process
// leaves with the disposition it started with.
//
// The file is unix-only because raising a signal is: windows has a few signal names
// and no way to send one, so there is nothing here to run there.

// TestDrainSignalsRunsTheListener covers the whole delivery path: arming routes the
// signal here, the raise queues it, and the drain the event loop makes at the top of
// each turn runs the listener with the name and number Node passes it.
func TestDrainSignalsRunsTheListener(t *testing.T) {
	var gotName string
	var gotNum float64
	OnProcessEvent("SIGUSR2", NewFunc(func(args []Value) Value {
		gotName = ToString(Arg(args, 0)).ToGoString()
		gotNum = ToNumber(Arg(args, 1))
		return Undefined
	}))
	defer RemoveAllProcessListeners(StringValue(FromGoString("SIGUSR2")))

	if err := syscall.Kill(os.Getpid(), syscall.SIGUSR2); err != nil {
		t.Fatalf("raise SIGUSR2: %v", err)
	}
	// The host hands the signal over on its own thread, so the drain may look before it
	// has arrived. Waiting for it is what a loop turn does with waitForWork.
	deadline := time.Now().Add(2 * time.Second)
	for gotName == "" && time.Now().Before(deadline) {
		waitForWork(10 * time.Millisecond)
		drainSignals()
	}
	if gotName != "SIGUSR2" {
		t.Fatalf("listener saw name %q, want SIGUSR2", gotName)
	}
	if want := float64(syscall.SIGUSR2); gotNum != want {
		t.Fatalf("listener saw number %v, want %v", gotNum, want)
	}
}

// TestDisarmSignalOnTheLastRemoval pins the bookkeeping the default disposition rests
// on: the signal is armed while a listener is registered and handed back when the last
// one goes. What that buys, a program killed by its own SIGINT after the removal, is
// pinned end to end in pkg/build, since it can only be seen from outside the process.
func TestDisarmSignalOnTheLastRemoval(t *testing.T) {
	event := StringValue(FromGoString("SIGUSR2"))
	fn := NewFunc(func(args []Value) Value { return Undefined })
	OnProcessEventNamed(event, fn)
	if _, armed := armedSignals["SIGUSR2"]; !armed {
		t.Fatal("registering a signal listener did not arm the signal")
	}
	OnProcessEventNamed(event, fn)
	RemoveProcessListener(event, fn)
	if _, armed := armedSignals["SIGUSR2"]; !armed {
		t.Fatal("the signal was disarmed while a listener was still registered")
	}
	RemoveProcessListener(event, fn)
	if _, armed := armedSignals["SIGUSR2"]; armed {
		t.Fatal("the signal is still armed after its last listener went")
	}
}

// TestArmUncatchableSignalThrows pins the refusal Node makes for SIGKILL and SIGSTOP,
// which no process can catch, and that the platform answers it as libuv does.
func TestArmUncatchableSignalThrows(t *testing.T) {
	for _, name := range []string{"SIGKILL", "SIGSTOP"} {
		code, msg := catchThrownCode(func() { armSignal(name) })
		if code != "EINVAL" || msg != "uv_signal_start EINVAL" {
			t.Errorf("arming %s threw %q %q, want EINVAL uv_signal_start EINVAL", name, code, msg)
		}
	}
}

// TestHostSignalNamesTheCommonSet pins that the names a Node program writes resolve to
// a real signal on this platform, and that a name no platform defines does not, since
// that is what makes process.on('SIGFOO') an ordinary event rather than an error.
func TestHostSignalNamesTheCommonSet(t *testing.T) {
	for _, name := range []string{"SIGINT", "SIGTERM", "SIGHUP", "SIGKILL"} {
		if _, ok := hostSignal(name); !ok {
			t.Errorf("%s does not resolve to a host signal", name)
		}
	}
	if _, ok := hostSignal("SIGFOO"); ok {
		t.Error("SIGFOO resolves to a host signal")
	}
}

// TestKillSignalReadsTheArgument covers the three shapes process.kill takes for its
// second argument: absent means SIGTERM, a number is the signal itself, and a name is
// looked up. A name nothing defines is Node's ERR_UNKNOWN_SIGNAL rather than a call
// that quietly sends nothing.
func TestKillSignalReadsTheArgument(t *testing.T) {
	if sig, err := killSignal(Undefined); err != nil || sig != syscall.SIGTERM {
		t.Errorf("no argument gave %v (%v), want SIGTERM", sig, err)
	}
	if sig, err := killSignal(Number(0)); err != nil || sig != 0 {
		t.Errorf("0 gave %v (%v), want 0", sig, err)
	}
	if sig, err := killSignal(StringValue(FromGoString("SIGTERM"))); err != nil || sig != syscall.SIGTERM {
		t.Errorf("SIGTERM gave %v (%v), want SIGTERM", sig, err)
	}
	_, err := killSignal(StringValue(FromGoString("SIGFOO")))
	if err == nil {
		t.Fatal("SIGFOO was accepted, want ERR_UNKNOWN_SIGNAL")
	}
	if got := ToString(err.ToValue().Get(FromGoString("code"))).ToGoString(); got != "ERR_UNKNOWN_SIGNAL" {
		t.Errorf("code = %q, want ERR_UNKNOWN_SIGNAL", got)
	}
}
