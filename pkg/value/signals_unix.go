//go:build unix

package value

import (
	"os"
	"syscall"
)

// hostSignal reads a signal name the way Node's constants do, on the platforms that
// have a real signal set. Missing is not an error: a name this platform does not
// define is an ordinary event nothing raises, which is what process.on('SIGFOO', fn)
// is in Node too.
//
// The table holds the signals every unix bento builds for defines, which is the POSIX
// set. The platform-specific ones are deliberately out: SIGINFO exists on the BSDs and
// not on linux, SIGPWR and SIGSTKFLT the other way around, and a table that named them
// would fail to build on the platform that lacks them rather than fall through to the
// ordinary-event answer, which is the same answer a program gets from Node on a
// platform without that signal.
var unixSignals = map[string]syscall.Signal{
	"SIGABRT":   syscall.SIGABRT,
	"SIGALRM":   syscall.SIGALRM,
	"SIGBUS":    syscall.SIGBUS,
	"SIGCHLD":   syscall.SIGCHLD,
	"SIGCONT":   syscall.SIGCONT,
	"SIGFPE":    syscall.SIGFPE,
	"SIGHUP":    syscall.SIGHUP,
	"SIGILL":    syscall.SIGILL,
	"SIGINT":    syscall.SIGINT,
	"SIGIO":     syscall.SIGIO,
	"SIGIOT":    syscall.SIGIOT,
	"SIGKILL":   syscall.SIGKILL,
	"SIGPIPE":   syscall.SIGPIPE,
	"SIGPROF":   syscall.SIGPROF,
	"SIGQUIT":   syscall.SIGQUIT,
	"SIGSEGV":   syscall.SIGSEGV,
	"SIGSTOP":   syscall.SIGSTOP,
	"SIGSYS":    syscall.SIGSYS,
	"SIGTERM":   syscall.SIGTERM,
	"SIGTRAP":   syscall.SIGTRAP,
	"SIGTSTP":   syscall.SIGTSTP,
	"SIGTTIN":   syscall.SIGTTIN,
	"SIGTTOU":   syscall.SIGTTOU,
	"SIGURG":    syscall.SIGURG,
	"SIGUSR1":   syscall.SIGUSR1,
	"SIGUSR2":   syscall.SIGUSR2,
	"SIGVTALRM": syscall.SIGVTALRM,
	"SIGWINCH":  syscall.SIGWINCH,
	"SIGXCPU":   syscall.SIGXCPU,
	"SIGXFSZ":   syscall.SIGXFSZ,
}

func hostSignal(name string) (syscall.Signal, bool) {
	sig, ok := unixSignals[name]
	return sig, ok
}

// sendSignal delivers the signal. Every unix signal number is deliverable, including
// zero, which sends nothing and reports whether the process is there, so the platform
// call needs no help.
func sendSignal(p *os.Process, sig syscall.Signal) error {
	return p.Signal(sig)
}
