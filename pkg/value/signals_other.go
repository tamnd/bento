//go:build !unix

package value

import "syscall"

// hostSignal on the platforms without a unix signal set, which for bento means windows.
// Go defines a handful of signal values there, and only a couple of them are ever
// raised, but a program compiled for windows still has to answer process.on and
// process.kill rather than fail to build.
//
// The rest of a program's signal names fall through to the ordinary-event answer, which
// is what Node does on windows as well: a listener for SIGUSR1 registers and never
// fires, since nothing on the platform raises one.
var otherSignals = map[string]syscall.Signal{
	"SIGABRT": syscall.SIGABRT,
	"SIGALRM": syscall.SIGALRM,
	"SIGBUS":  syscall.SIGBUS,
	"SIGFPE":  syscall.SIGFPE,
	"SIGHUP":  syscall.SIGHUP,
	"SIGILL":  syscall.SIGILL,
	"SIGINT":  syscall.SIGINT,
	"SIGKILL": syscall.SIGKILL,
	"SIGPIPE": syscall.SIGPIPE,
	"SIGQUIT": syscall.SIGQUIT,
	"SIGSEGV": syscall.SIGSEGV,
	"SIGTERM": syscall.SIGTERM,
	"SIGTRAP": syscall.SIGTRAP,
}

func hostSignal(name string) (syscall.Signal, bool) {
	sig, ok := otherSignals[name]
	return sig, ok
}
