package value

import (
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// This file is the runtime side of a signal listener. A signal is the one process
// event the host raises rather than the program, so it is the one that needs
// machinery: something has to ask the operating system to route the signal here, hold
// what arrived until the program can be interrupted, and hand the signal back when the
// last listener goes.
//
// The delivery point is the event loop. Node delivers a signal on a loop turn, so a
// program with nothing scheduled never sees one, not even a signal it sent itself:
//
//	process.on('SIGINT', () => console.log('got it'))
//	process.kill(process.pid, 'SIGINT')      // prints nothing, the program has left
//
// That is not a gap to paper over. A compiled bento program runs the loop when it
// scheduled something, the same condition, so a signal is delivered exactly where Node
// delivers one and dropped exactly where Node drops one.
//
// Arming is what changes the default disposition. Until a program registers a
// listener, SIGINT terminates it; from the first registration the signal is routed to
// the listener instead, and when the last listener is removed the default comes back.
// A test that removes its handler and kills itself to check the exit status is
// asserting exactly that, and it only works because the disarm is real.

// signalNotify is the channel the host writes signals into, created when the first
// signal listener arms one. queuedSignals holds what arrived but has not been
// delivered yet, which is what lets a signal be taken off the channel during a sleep
// and run at the top of the next loop turn rather than inside the sleep.
var (
	signalNotify  chan os.Signal
	queuedSignals []syscall.Signal
	armedSignals  = map[string]syscall.Signal{}
)

// signalBuffer is how many signals the host channel holds before it starts dropping.
// The runtime never blocks on a send, so a burst larger than this loses the overflow,
// which is what a signal already is: one pending instance per signal, not a queue.
const signalBuffer = 32

// armSignal starts routing a signal to this program, called when its first listener
// registers. A name the platform does not define is not an error: Node treats
// process.on('SIGFOO', fn) as an ordinary event that nothing raises, so there is
// nothing to arm and the registration stands. SIGKILL and SIGSTOP cannot be caught by
// anyone, and Node refuses the registration rather than accepting one it knows will
// never fire.
func armSignal(name string) {
	if name == "SIGKILL" || name == "SIGSTOP" {
		Throw(NewNodeError("Error", "EINVAL", FromGoString("uv_signal_start EINVAL")))
		return
	}
	sig, ok := hostSignal(name)
	if !ok {
		return
	}
	if _, already := armedSignals[name]; already {
		return
	}
	if signalNotify == nil {
		signalNotify = make(chan os.Signal, signalBuffer)
	}
	armedSignals[name] = sig
	signal.Notify(signalNotify, sig)
}

// disarmSignal hands a signal back to its default disposition, called when its last
// listener goes. signal.Reset rather than signal.Stop, because Stop only detaches the
// channel and leaves the host handler installed, which would go on swallowing the
// signal after the program stopped listening for it.
func disarmSignal(name string) {
	sig, ok := armedSignals[name]
	if !ok {
		return
	}
	delete(armedSignals, name)
	signal.Reset(sig)
}

// drainSignals delivers every signal that has arrived, the call the event loop makes at
// the top of each turn. It takes what the host channel holds first, so a signal
// delivered during the previous turn's callbacks is seen in this one, then runs the
// listeners for each. Emitting is the ordinary process-event emit, so a signal listener
// is a listener like any other and sees the arguments Node passes: the signal's name
// and its number.
func drainSignals() {
	for {
		select {
		case s := <-signalNotify:
			queueSignal(s)
			continue
		default:
		}
		break
	}
	if len(queuedSignals) == 0 {
		return
	}
	batch := queuedSignals
	queuedSignals = nil
	for _, sig := range batch {
		name, ok := signalName(sig)
		if !ok {
			continue
		}
		EmitProcessEvent(name, StringValue(FromGoString(name)), Number(float64(sig)))
	}
}

// queueSignal records a signal the host handed over. A value the channel carries is an
// os.Signal, which every platform backs with a syscall.Signal, and anything else is
// nothing this runtime asked for.
func queueSignal(s os.Signal) {
	if sig, ok := s.(syscall.Signal); ok {
		queuedSignals = append(queuedSignals, sig)
	}
}

// signalName reads back the name a signal was armed under, so a listener sees the name
// it registered with. The armed set is small, a handful of entries at most, so the walk
// is cheaper than a second map kept in step with the first.
func signalName(sig syscall.Signal) (string, bool) {
	for name, armed := range armedSignals {
		if armed == sig {
			return name, true
		}
	}
	return "", false
}

// waitForWork sleeps until the next timer is due or a signal arrives, whichever comes
// first, the wait the event loop makes when nothing is ready to run. A plain sleep
// would hold a signal until the next timer fired, so a program with a listener and a
// long timeout would take its SIGINT a second late; taking the signal off the channel
// here and queueing it lets the loop deliver it on the very next turn.
func waitForWork(d time.Duration) {
	if signalNotify == nil {
		time.Sleep(d)
		return
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case s := <-signalNotify:
		queueSignal(s)
	case <-t.C:
	}
}

// ProcessKill sends a signal to a process, the runtime behind process.kill(pid, sig).
// The signal defaults to SIGTERM and may be named or numbered; signal 0 sends nothing
// and reports whether the process exists, which is what a liveness check uses. A
// program most often sends to itself, which is how a test drives its own handler.
func ProcessKill(args []Value) Value {
	target := Arg(args, 0)
	if target.Kind() != KindNumber {
		Throw(invalidArgType("pid", "number", target))
	}
	sig, err := killSignal(Arg(args, 1))
	if err != nil {
		Throw(err)
	}
	p, ferr := os.FindProcess(int(ToNumber(target)))
	if ferr != nil {
		Throw(NewNodeError("Error", "ESRCH", FromGoString("kill ESRCH")))
		return Undefined
	}
	if serr := sendSignal(p, sig); serr != nil {
		if errors.Is(serr, errSignalUnsupported) {
			Throw(NewNodeError("Error", "ENOSYS", FromGoString("kill ENOSYS")))
			return Undefined
		}
		Throw(NewNodeError("Error", "ESRCH", FromGoString("kill ESRCH")))
		return Undefined
	}
	waitForOwnDeath(int(ToNumber(target)), sig)
	return True
}

// errSignalUnsupported is what sendSignal answers for a signal the platform cannot
// send at all, which is a different thing from a process that is not there. Node
// reports it as ENOSYS, and libuv answers the same way for the signals windows has a
// name for and no way to raise.
var errSignalUnsupported = errors.New("the platform cannot send this signal")

// signalSelfWait is how long a program waits after signalling itself with a signal it
// does not listen for. Under Node the kill lands before the call returns, so the
// statement after it never runs; here the host handler runs on another thread and takes
// a few hundred microseconds to bring the process down, which is long enough for the
// program to print another line and leave with status 0 where Node leaves killed. A
// short wait closes that window. It is only ever paid by a program that signalled
// itself with something it is not listening for, which is the case that was about to
// end anyway.
const signalSelfWait = 5 * time.Millisecond

// waitForOwnDeath gives the host time to act on a signal a program just sent itself.
// A signal with a listener is delivered by the loop instead and needs no wait, a signal
// whose default action is to ignore it will not end anything, and signal 0 was never
// sent at all.
func waitForOwnDeath(pid int, sig syscall.Signal) {
	if sig == 0 || pid != os.Getpid() {
		return
	}
	if _, listening := signalName(sig); listening {
		return
	}
	if defaultIgnores(sig) {
		return
	}
	time.Sleep(signalSelfWait)
}

// defaultIgnores reports whether a signal's default action is to do nothing, which is
// true of the four a process is expected to survive: a child changing state, urgent
// socket data, a terminal resize, and a continue for a process that is not stopped.
func defaultIgnores(sig syscall.Signal) bool {
	for _, name := range []string{"SIGCHLD", "SIGURG", "SIGWINCH", "SIGCONT"} {
		if s, ok := hostSignal(name); ok && s == sig {
			return true
		}
	}
	return false
}

// killSignal reads the signal argument of process.kill. Node takes a name, a number,
// or nothing at all, and refuses a name no platform signal answers to, since sending
// a signal the caller misspelled would silently do nothing.
func killSignal(v Value) (syscall.Signal, *Error) {
	switch v.Kind() {
	case KindUndefined:
		return syscall.SIGTERM, nil
	case KindNumber:
		return syscall.Signal(int(ToNumber(v))), nil
	}
	name := ToString(v).ToGoString()
	if sig, ok := hostSignal(name); ok {
		return sig, nil
	}
	return 0, NewNodeError("Error", "ERR_UNKNOWN_SIGNAL", FromGoString("Unknown signal: "+name))
}
