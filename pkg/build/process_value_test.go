package build

import "testing"

// This file covers slice G1.2: the process global read as a value. A Node program
// branches on process.platform, slices process.argv, and reads a variable out of
// process.env, none of which is a call the lowerer models statically. Before this
// slice a bare process reference handed back to the interpreter, so none of these
// compiled; now it lowers to a live runtime object and the member reads resolve
// against it. Each test builds a .js entry through the AOT path and runs the
// binary, so it pins what a compiled program actually prints.

// TestProcessPlatformAndArchAreStrings pins the platform strings: process.platform
// and process.arch are the names Node reports for the host, which the runtime maps
// from the Go build target. The values differ per machine, so the test asserts
// their type and that they are non-empty rather than a fixed pair, which is what
// keeps it honest across the platforms CI runs. Node prints "string" twice, then
// "true" twice.
func TestProcessPlatformAndArchAreStrings(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"console.log(typeof process.platform);\n"+
			"console.log(typeof process.arch);\n"+
			"console.log(process.platform.length > 0);\n"+
			"console.log(process.arch.length > 0);\n")
	if want := "string\nstring\ntrue\ntrue\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestProcessArgvIsTheArgumentVector pins process.argv: it is an array whose first
// two slots are filled the way Node fills them, with the executable and the script,
// so a program reading its own arguments from index 2 sees the same thing it would
// under Node. A compiled binary is both the executable and the script, so both
// leading slots hold its path. Node prints "true", "true", then "string".
func TestProcessArgvIsTheArgumentVector(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"console.log(Array.isArray(process.argv));\n"+
			"console.log(process.argv.length >= 2);\n"+
			"console.log(typeof process.argv[0]);\n")
	if want := "true\ntrue\nstring\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestProcessEnvReadsTheEnvironment pins process.env: it is an object carrying the
// environment the process started with, so a named variable reads its value and an
// absent one reads undefined, the missing-property answer rather than an error. The
// test sets no variable of its own and instead reads PATH, which every platform CI
// runs on defines. Node prints "object", "true", then "undefined".
func TestProcessEnvReadsTheEnvironment(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"console.log(typeof process.env);\n"+
			"console.log(typeof process.env.PATH === 'string');\n"+
			"console.log(process.env.BENTO_DEFINITELY_NOT_SET_12345);\n")
	if want := "object\ntrue\nundefined\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestProcessVersionReportsNodeCompat pins process.version and process.versions:
// bento reports the Node version it targets for compatibility, in the leading-v
// form Node uses for process.version and the bare form for process.versions.node,
// so a program that parses either reads a well-formed version. Node prints its own
// version here, so the value is bento's compatibility baseline rather than a
// comparison against the host Node.
func TestProcessVersionReportsNodeCompat(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"console.log(process.version);\n"+
			"console.log(process.versions.node);\n")
	if want := "v22.11.0\n22.11.0\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestProcessIdentityAndMissingMember pins the object shape: process is one cached
// object, so a property written on it is there on the next read, and a member the
// object does not carry reads undefined rather than throwing. That is what makes
// the any typing honest, since every member read resolves against the live object
// instead of a fixed declared member list. Node prints "object", "true", then
// "undefined".
func TestProcessIdentityAndMissingMember(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"console.log(typeof process);\n"+
			"process.bentoMarker = 1;\n"+
			"console.log(process.bentoMarker === 1);\n"+
			"console.log(process.nosuchmember);\n")
	if want := "object\ntrue\nundefined\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestProcessCwdAndUptimeDispatchThroughTheObject pins the calls the lowerer does
// not special-case: process.cwd() and process.uptime() are members of the process
// object, so they dispatch through the dynamic path and run, where before this
// slice they handed back as a later slice. Node prints "string" then "number".
func TestProcessCwdAndUptimeDispatchThroughTheObject(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"console.log(typeof process.cwd());\n"+
			"console.log(typeof process.uptime());\n")
	if want := "string\nnumber\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestProcessStaticPathsStillClaimTheirShapes pins that giving process a value form
// did not move the two call shapes the lowerer lowers directly. process.on('exit',
// fn) must still register through the static path, because the end-of-main drain it
// arranges cannot be set up by a runtime call, and process.stdout.write must still
// reach the direct write helper. The body line prints before the exit line, which a
// registration that never ran or ran eagerly would get wrong. Node prints "out",
// "body", then "bye".
func TestProcessStaticPathsStillClaimTheirShapes(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"process.stdout.write('out\\n');\n"+
			"process.on('exit', function () { console.log('bye'); });\n"+
			"console.log('body');\n")
	if want := "out\nbody\nbye\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
