package value

import (
	"os"
	"runtime"
	"strings"
	"time"
)

// This file is the runtime side of the process global. A Node program reads
// process as an object, not just as the receiver of the handful of calls the
// lowerer models statically: it branches on process.platform, slices
// process.argv, reads a variable out of process.env, and prints
// process.version. Those are member reads on a live object, so the AOT path
// needs a real process value behind the name rather than a set of call-shaped
// special cases.
//
// The object is built once and cached, so the identity process === process holds
// and a write like process.foo = 1 is still there on the next read, the single
// process object Node gives a program. Every property is filled from the host at
// first read: the argument vector, the environment, the platform and
// architecture strings Node reports, and the process and parent process ids.
//
// A property whose honest value bento cannot produce yet is left off the object
// rather than filled with a plausible wrong one, so a program that reads it gets
// undefined (the reading-a-missing-property answer) instead of a number that
// looks right and is not. process.exitCode is the notable absence: a compiled
// program's exit status is fixed by main, so a settable exitCode would be
// recorded and never honored.

// nodeCompatVersion is the Node version the AOT runtime reports through
// process.version and process.versions.node. It matches the constant pkg/runtime
// reports on the interpreter path, so a program sees one version whichever path
// ran it; the spec pins Node 22 as the compatibility baseline.
const nodeCompatVersion = "22.11.0"

// processStart is when the program began, the zero point process.uptime counts
// from. It is set at package initialization, which for a compiled bento binary
// runs before main, so the reading is the process lifetime rather than the time
// since the first uptime call.
var processStart = time.Now()

// processObject caches the one process object. A second read of the global
// returns the same value the first built, so the object has identity and holds
// any property a program set on it.
var processObject Value

// ProcessValue returns the process global, building it on first read. The
// lowerer emits one package-level call to it per program, so the compiled
// binary pays for the environment and argument snapshots only when the program
// actually names process.
func ProcessValue() Value {
	if processObject.Kind() != KindUndefined {
		return processObject
	}
	p := NewObject()
	p.Set(FromGoString("argv"), processArgv())
	p.Set(FromGoString("argv0"), StringValue(FromGoString(processArgv0())))
	p.Set(FromGoString("execPath"), StringValue(FromGoString(processExecPath())))
	p.Set(FromGoString("platform"), StringValue(FromGoString(nodePlatform())))
	p.Set(FromGoString("arch"), StringValue(FromGoString(nodeArch())))
	p.Set(FromGoString("pid"), Number(float64(os.Getpid())))
	p.Set(FromGoString("ppid"), Number(float64(os.Getppid())))
	p.Set(FromGoString("version"), StringValue(FromGoString("v"+nodeCompatVersion)))
	p.Set(FromGoString("versions"), processVersions())
	p.Set(FromGoString("env"), processEnv())
	p.Set(FromGoString("stdout"), processStream(WriteStdout))
	p.Set(FromGoString("stderr"), processStream(WriteStderr))
	p.Set(FromGoString("cwd"), NewFunc(func(args []Value) Value {
		dir, err := os.Getwd()
		if err != nil {
			return StringValue(FromGoString(""))
		}
		return StringValue(FromGoString(dir))
	}))
	p.Set(FromGoString("uptime"), NewFunc(func(args []Value) Value {
		return Number(time.Since(processStart).Seconds())
	}))
	p.Set(FromGoString("exit"), NewFunc(func(args []Value) Value {
		processExit(args)
		return Undefined
	}))
	p.Set(FromGoString("kill"), NewFunc(ProcessKill))
	processObject = p
	setProcessEmitter(p)
	return processObject
}

// setProcessEmitter puts the EventEmitter surface on the process object. Node's process
// is an emitter, so a program reaches for the whole method set rather than just on:
// once for a listener that should run one time, off to take one away, listeners to save
// and restore what is registered, and emit to raise an event of its own.
//
// The methods answer process itself where Node's do, so the chained
// process.on(...).on(...) a test writes works. It is set after the cache is filled
// because each answer is the cached object, and building it here would recurse.
func setProcessEmitter(p Value) {
	on := NewFunc(func(args []Value) Value {
		return OnProcessEventNamed(Arg(args, 0), Arg(args, 1))
	})
	off := NewFunc(func(args []Value) Value {
		return RemoveProcessListener(Arg(args, 0), Arg(args, 1))
	})
	p.Set(FromGoString("on"), on)
	p.Set(FromGoString("addListener"), on)
	p.Set(FromGoString("off"), off)
	p.Set(FromGoString("removeListener"), off)
	p.Set(FromGoString("once"), NewFunc(func(args []Value) Value {
		return OnceProcessEvent(Arg(args, 0), Arg(args, 1))
	}))
	p.Set(FromGoString("prependListener"), NewFunc(func(args []Value) Value {
		return PrependProcessListener(Arg(args, 0), Arg(args, 1))
	}))
	p.Set(FromGoString("prependOnceListener"), NewFunc(func(args []Value) Value {
		return PrependOnceProcessListener(Arg(args, 0), Arg(args, 1))
	}))
	p.Set(FromGoString("removeAllListeners"), NewFunc(func(args []Value) Value {
		return RemoveAllProcessListeners(args...)
	}))
	p.Set(FromGoString("listeners"), NewFunc(func(args []Value) Value {
		return ProcessListeners(Arg(args, 0))
	}))
	p.Set(FromGoString("listenerCount"), NewFunc(func(args []Value) Value {
		return Number(ProcessListenerCount(Arg(args, 0)))
	}))
	p.Set(FromGoString("eventNames"), NewFunc(func(args []Value) Value {
		return ProcessEventNames()
	}))
	p.Set(FromGoString("emit"), NewFunc(func(args []Value) Value {
		if len(args) == 0 {
			return False
		}
		return EmitProcessEventNamed(args[0], args[1:]...)
	}))
}

// processArgv builds process.argv, the argument vector a program slices to read
// its own arguments. Node fills index 0 with the executable and index 1 with the
// main script, so a program reads its arguments from index 2 on. A compiled
// bento binary is both the executable and the script, so both leading slots hold
// the binary's path and the user arguments follow at index 2, which keeps the
// argv.slice(2) a Node program is written against meaning the same thing here.
func processArgv() Value {
	exe := processExecPath()
	elems := []Value{
		StringValue(FromGoString(exe)),
		StringValue(FromGoString(exe)),
	}
	if len(os.Args) > 1 {
		for _, a := range os.Args[1:] {
			elems = append(elems, StringValue(FromGoString(a)))
		}
	}
	return NewArrayValue(elems)
}

// processExecPath returns the path of the running binary, the value
// process.execPath reports and the one argv's leading slots hold. It prefers the
// path the operating system resolved, falling back to the invoked name when that
// lookup fails, so the property is always a string.
func processExecPath() string {
	if exe, err := os.Executable(); err == nil {
		return exe
	}
	if len(os.Args) > 0 {
		return os.Args[0]
	}
	return ""
}

// processArgv0 returns process.argv0, the name the process was invoked under.
// Node keeps the original argv[0] here even after execPath resolves elsewhere,
// so this reads the invoked name rather than the resolved binary path.
func processArgv0() string {
	if len(os.Args) > 0 {
		return os.Args[0]
	}
	return ""
}

// processVersions builds process.versions, the map of component versions. It
// carries the node entry a compatibility check reads; bento does not embed V8, so
// no v8 entry is invented for it, and a program that reads one gets undefined
// rather than a version string for an engine that is not there.
func processVersions() Value {
	v := NewObject()
	v.Set(FromGoString("node"), StringValue(FromGoString(nodeCompatVersion)))
	return v
}

// processEnv builds process.env from the environment the process started with.
// It is a snapshot taken once: reads see the environment the program was given,
// which is what a configuration lookup wants. A write lands on the object and is
// visible to later reads in the same program, but does not reach the operating
// system environment, a difference a program can only observe by spawning a child
// process, which the AOT path does not do yet.
func processEnv() Value {
	env := NewObject()
	for _, kv := range os.Environ() {
		name, val, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		env.Set(FromGoString(name), StringValue(FromGoString(val)))
	}
	return env
}

// processStream builds a process.stdout or process.stderr object over the byte
// writer that backs it. Only write is carried, the method a program reaches for
// to print without a trailing newline, and it returns the boolean Node's stream
// write returns. The lowerer also has a static path for process.stdout.write that
// emits the same helper call directly; this object is what a read of the stream
// as a value, or a call through a dynamic receiver, lands on, so the two agree.
func processStream(write func(BStr) bool) Value {
	s := NewObject()
	s.Set(FromGoString("write"), NewFunc(func(args []Value) Value {
		return Bool(write(ToString(Arg(args, 0))))
	}))
	return s
}

// processExit ends the program, the runtime behind process.exit(code). Node runs
// the exit listeners before leaving, so this drains them first and then exits with
// the requested status, defaulting to 0 the way Node does for a bare exit(). The
// code is truncated to its integer part because an exit status is a small integer,
// not a float.
func processExit(args []Value) {
	code := 0
	if c := Arg(args, 0); c.Kind() != KindUndefined {
		code = int(ToNumber(c))
	}
	RunExitCallbacksWithCode(code)
	os.Exit(code)
}

// nodePlatform maps the Go operating system name to the string Node reports
// through process.platform. Only Windows differs, which Node names win32.
func nodePlatform() string {
	if runtime.GOOS == "windows" {
		return "win32"
	}
	return runtime.GOOS
}

// nodeArch maps the Go architecture name to the string Node reports through
// process.arch. Node names the 64-bit and 32-bit Intel architectures x64 and
// ia32; the rest carry the same name in both.
func nodeArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x64"
	case "386":
		return "ia32"
	default:
		return runtime.GOARCH
	}
}
