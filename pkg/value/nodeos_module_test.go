package value

import (
	"runtime"
	"strings"
	"testing"
)

// require('os') used to hand back the throw-on-use stub every unimplemented
// built-in gets, so a compiled CommonJS program could load the module and then
// fail on the first thing it read off it. These cases pin the real module: the
// members it has, in the order Node has them, and the shape of what each answers.

// nodeOSExports is the list Object.keys(require('os')) reports in Node v24, in
// Node's own order, minus the two bento does not implement yet.
//
// The order is checked and not only the set, because it is part of what the module
// is: a program that enumerates the module or prints it sees what Node's prints.
var nodeOSExports = []string{
	"arch", "availableParallelism", "cpus", "endianness", "freemem", "homedir",
	"hostname", "loadavg", "networkInterfaces", "platform", "release", "tmpdir",
	"totalmem", "type", "userInfo", "uptime", "version", "machine", "constants",
	"EOL", "devNull",
}

// TestOSModuleExportsMatchNode pins the module's members against Node's. The two
// missing from the list, getPriority and setPriority, are the only exports of Node's
// os that change something rather than report it; leaving them out of the module
// means a program that calls one gets an undefined-is-not-a-function error naming
// the call, which is a better failure than a wrong nice value.
func TestOSModuleExportsMatchNode(t *testing.T) {
	mod := RequireBuiltin("os")
	var got []string
	for _, key := range mod.OwnKeys().Elems() {
		got = append(got, key.ToGoString())
	}
	if strings.Join(got, ",") != strings.Join(nodeOSExports, ",") {
		t.Errorf("require('os') has\n%v\nwant Node's order\n%v", got, nodeOSExports)
	}
	for _, name := range []string{"getPriority", "setPriority"} {
		if !mod.Get(FromGoString(name)).IsUndefined() {
			t.Errorf("os.%s is defined; if it is implemented now it belongs in the export list", name)
		}
	}
}

// TestOSModuleIsNotAStub pins the whole point of the slice: the registry answers
// require('os') with a real module. Reading a member off a stub throws, so a read
// that returns a callable is the check.
func TestOSModuleIsNotAStub(t *testing.T) {
	for _, specifier := range []string{"os", "node:os"} {
		mod := RequireBuiltin(specifier)
		if got := mod.Get(FromGoString("platform")); got.Kind() != KindFunc {
			t.Errorf("require(%q).platform is %v, want a function", specifier, got.Kind())
		}
	}
	if RequireBuiltin("os") != RequireBuiltin("node:os") {
		t.Error("require('os') and require('node:os') are different values")
	}
}

// TestOSModuleAnswersTheMachine calls every member and checks what comes back
// against what it means. The values themselves are checked where they are
// measured, in pkg/nodehost, against uname and the kernel's own counters; the
// question here is whether the module hands them on in the shape a program indexes
// into.
func TestOSModuleAnswersTheMachine(t *testing.T) {
	mod := RequireBuiltin("os")
	call := func(name string) Value {
		t.Helper()
		fn := mod.Get(FromGoString(name))
		if fn.Kind() != KindFunc {
			t.Fatalf("os.%s is %v, want a function", name, fn.Kind())
		}
		return fn.Call()
	}
	for _, name := range []string{"platform", "arch", "type", "endianness", "tmpdir", "homedir"} {
		if got := ToString(call(name)).ToGoString(); got == "" {
			t.Errorf("os.%s() is empty", name)
		}
	}
	for _, name := range []string{"totalmem", "availableParallelism"} {
		if got := ToNumber(call(name)); got <= 0 {
			t.Errorf("os.%s() is %v, want a positive number", name, got)
		}
	}
	if got := call("loadavg"); got.Kind() != KindArray || got.Get(FromGoString("length")).AsNumber() != 3 {
		t.Errorf("os.loadavg() is %v, want an array of three", got.Kind())
	}
	cpus := call("cpus")
	if cpus.Kind() != KindArray {
		t.Fatalf("os.cpus() is %v, want an array", cpus.Kind())
	}
	n := int(cpus.Get(FromGoString("length")).AsNumber())
	if n <= 0 {
		t.Fatal("os.cpus() is empty")
	}
	first := cpus.GetIndex(0)
	if model := ToString(first.Get(FromGoString("model"))).ToGoString(); model == "" {
		t.Error("the first cpu has no model")
	}
	times := first.Get(FromGoString("times"))
	for _, key := range []string{"user", "nice", "sys", "idle", "irq"} {
		if got := times.Get(FromGoString(key)); got.Kind() != KindNumber {
			t.Errorf("os.cpus()[0].times.%s is %v, want a number", key, got.Kind())
		}
	}
	user := call("userInfo")
	if got := ToString(user.Get(FromGoString("homedir"))).ToGoString(); got == "" {
		t.Error("os.userInfo().homedir is empty")
	}
	ifaces := call("networkInterfaces")
	if ifaces.Kind() != KindObject {
		t.Errorf("os.networkInterfaces() is %v, want an object", ifaces.Kind())
	}
	consts := mod.Get(FromGoString("constants"))
	for _, group := range []string{"signals", "errno", "priority", "dlopen"} {
		if got := consts.Get(FromGoString(group)); got.Kind() != KindObject {
			t.Errorf("os.constants.%s is %v, want an object", group, got.Kind())
		}
	}
}

// TestOSModuleAgreesWithTheLoweredHelpers pins the two halves of node:os against
// each other. An import of node:os lowers to the helpers in nodefs.go and a
// require of it reads this module, and both are meant to be one module: a program
// that asks the same question the two ways cannot get two answers.
//
// Only the answers that cannot change between the two calls are compared. Free
// memory and the processor times move, which is the point of them.
func TestOSModuleAgreesWithTheLoweredHelpers(t *testing.T) {
	mod := RequireBuiltin("os")
	call := func(name string) string {
		t.Helper()
		return ToString(mod.Get(FromGoString(name)).Call()).ToGoString()
	}
	for _, tc := range []struct {
		name    string
		lowered BStr
	}{
		{"platform", OSPlatform()},
		{"arch", OSArch()},
		{"type", OSType()},
		{"release", OSRelease()},
		{"version", OSVersion()},
		{"machine", OSMachine()},
		{"hostname", OSHostname()},
		{"homedir", OSHomedir()},
		{"endianness", OSEndianness()},
		{"tmpdir", Tmpdir()},
	} {
		if got, want := call(tc.name), tc.lowered.ToGoString(); got != want {
			t.Errorf("require('os').%s() is %q, the lowered helper says %q", tc.name, got, want)
		}
	}
	if got, want := ToString(mod.Get(FromGoString("EOL"))).ToGoString(), OSEOL().ToGoString(); got != want {
		t.Errorf("require('os').EOL is %q, the lowered helper says %q", got, want)
	}
	if got, want := ToString(mod.Get(FromGoString("devNull"))).ToGoString(), OSDevNull().ToGoString(); got != want {
		t.Errorf("require('os').devNull is %q, the lowered helper says %q", got, want)
	}
	if got, want := ToNumber(mod.Get(FromGoString("totalmem")).Call()), OSTotalmem(); got != want {
		t.Errorf("require('os').totalmem() is %v, the lowered helper says %v", got, want)
	}
	// arch is the one whose two spellings are both real: os.arch() is Node's name
	// for the architecture and os.machine() is uname's, and they differ on a 64-bit
	// Intel machine, which is where confusing them would go unnoticed everywhere else.
	if runtime.GOARCH == "amd64" {
		if got := call("arch"); got != "x64" {
			t.Errorf("os.arch() is %q on amd64, want x64", got)
		}
		if got := call("machine"); got != "x86_64" {
			t.Errorf("os.machine() is %q on amd64, want x86_64", got)
		}
	}
}
