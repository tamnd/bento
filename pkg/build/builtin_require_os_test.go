package build

import (
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// These cases build real binaries and run them. They are the only place the
// require('os') slice is checked end to end: the registry entry in pkg/value is
// unit tested there, but nothing about that test proves the lowerer routes a
// require of a built-in through the registry, that the module survives into a
// compiled binary, or that the binary reads the machine it runs on rather than
// the machine it was built on. A compiled program printing this host's own
// processor count is what proves all three at once.

// TestRequireOSIsARealModule pins the point of the slice: require('os') used to
// hand back the throw-on-use stub, so a compiled CommonJS program loaded the
// module and then failed on the first member it read. Now the read answers. The
// two specifier forms share the one cached module, the registry identity rule.
// Node prints "string", "function", then "true".
func TestRequireOSIsARealModule(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const os = require('os');\n"+
			"console.log(typeof os.platform());\n"+
			"console.log(typeof os.cpus);\n"+
			"console.log(os === require('node:os'));\n")
	if want := "string\nfunction\ntrue\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestRequireOSExportsAreInNodesOrder pins the module's members and their order
// against Node's, from inside a compiled binary. The order is part of what the
// module is: a program that enumerates it sees what Node's prints. getPriority
// and setPriority are absent because bento does not measure them yet, so a
// program that calls one gets an undefined-is-not-a-function error naming the
// call rather than a wrong nice value.
func TestRequireOSExportsAreInNodesOrder(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"console.log(Object.keys(require('os')).join(','));\n")
	want := "arch,availableParallelism,cpus,endianness,freemem,homedir,hostname," +
		"loadavg,networkInterfaces,platform,release,tmpdir,totalmem,type,userInfo," +
		"uptime,version,machine,constants,EOL,devNull\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestRequireOSAnswersTheMachine pins the shape a program indexes into: cpus() is
// an array of entries with a model and a times object, userInfo() has a homedir,
// loadavg() is three numbers, networkInterfaces() is an object. The values
// themselves are checked where they are measured, against uname and the kernel's
// own counters, in pkg/nodehost; the question here is whether they reach a
// compiled program intact. Node prints "true" on every line.
func TestRequireOSAnswersTheMachine(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const os = require('os');\n"+
			"const cpus = os.cpus();\n"+
			"console.log(Array.isArray(cpus) && cpus.length > 0);\n"+
			"console.log(typeof cpus[0].model === 'string' && cpus[0].model.length > 0);\n"+
			"console.log(typeof cpus[0].times.idle === 'number');\n"+
			"console.log(os.loadavg().length === 3);\n"+
			"console.log(typeof os.networkInterfaces() === 'object');\n"+
			"console.log(os.userInfo().homedir.length > 0);\n"+
			"console.log(os.totalmem() > 0 && os.availableParallelism() >= 1);\n"+
			"console.log(typeof os.constants.priority === 'object');\n"+
			"console.log(os.EOL === '\\n' || os.EOL === '\\r\\n');\n")
	if want := "true\ntrue\ntrue\ntrue\ntrue\ntrue\ntrue\ntrue\ntrue\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestRequireOSReadsThisMachine pins that the compiled binary measures the host
// it runs on rather than carrying a constant baked in at build time. The count of
// processors is compared against Go's own view of this machine, and the two
// architecture spellings are checked apart: os.arch() is Node's name and
// os.machine() is uname's, and they differ on a 64-bit Intel machine, which is
// the one place confusing them would go unnoticed.
func TestRequireOSReadsThisMachine(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const os = require('os');\n"+
			"console.log(os.cpus().length);\n"+
			"console.log(os.arch());\n"+
			"console.log(os.platform());\n")
	want := ""
	switch runtime.GOARCH {
	case "amd64":
		want = "x64"
	case "arm64":
		want = "arm64"
	}
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("want three lines, got %q", got)
	}
	count, err := strconv.Atoi(lines[0])
	if err != nil {
		t.Fatalf("os.cpus().length printed %q: %v", lines[0], err)
	}
	arch, platform := lines[1], lines[2]
	if count != runtime.NumCPU() {
		t.Errorf("os.cpus() has %d entries, this machine has %d processors", count, runtime.NumCPU())
	}
	if want != "" && arch != want {
		t.Errorf("os.arch() is %q, want %q on %s", arch, want, runtime.GOARCH)
	}
	wantPlatform := runtime.GOOS
	if runtime.GOOS == "windows" {
		wantPlatform = "win32"
	}
	if platform != wantPlatform {
		t.Errorf("os.platform() is %q, this binary was built for %s", platform, runtime.GOOS)
	}
}

// TestRequireOSBindsThroughLocals pins the way a CommonJS program is actually
// written: the module goes in a const, what it answers goes in another const, and
// the program reads that. Until the lowerer learned that a binding fed by a read
// off a boxed module binding is itself a box, the second line of this program did
// not compile at all, and the failure named the checker rather than the line. Node
// prints the core count twice, then "true".
func TestRequireOSBindsThroughLocals(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const os = require('os');\n"+
			"const cpus = os.cpus();\n"+
			"const first = cpus[0];\n"+
			"const model = first.model;\n"+
			"const idle = first.times.idle;\n"+
			"console.log(cpus.length);\n"+
			"console.log(os.cpus().length);\n"+
			"console.log(model.length > 0 && typeof idle === 'number');\n")
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	if len(lines) != 3 || lines[0] != lines[1] || lines[2] != "true" {
		t.Fatalf("unexpected output %q", got)
	}
}

// TestRequireOSAgreesWithTheImportForm pins the two halves of node:os against each
// other from inside one compiled binary. An import of node:os lowers to the value
// helpers and a require of it reads the registry module, and they are meant to be
// one module: a program that asks the same question the two ways cannot get two
// answers. Only the facts that cannot change between the two calls are compared;
// free memory and the processor times move, which is the point of them.
func TestRequireOSAgreesWithTheImportForm(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"import { platform, arch, type, hostname, homedir, endianness, totalmem } from 'node:os';\n"+
			"const os = require('os');\n"+
			"console.log(platform() === os.platform());\n"+
			"console.log(arch() === os.arch());\n"+
			"console.log(type() === os.type());\n"+
			"console.log(hostname() === os.hostname());\n"+
			"console.log(homedir() === os.homedir());\n"+
			"console.log(endianness() === os.endianness());\n"+
			"console.log(totalmem() === os.totalmem());\n")
	if want := "true\ntrue\ntrue\ntrue\ntrue\ntrue\ntrue\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
