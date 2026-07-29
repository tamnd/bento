package node

import (
	"runtime"
	"testing"
)

// node:os has two data exports a program reads for a platform fact rather than
// calls: EOL, the line ending, and devNull, the path of the null device. The
// compiled path answers them from value.OSEOL and value.OSDevNull, and this pins
// the engine to the same answers, so a program gets one value whether it was run
// or built.
//
// Both sides are checked against Node's own spellings rather than against each
// other, which is what makes the check worth running: two implementations that
// agree on a wrong value would still pass a comparison between themselves.
func TestOSConstantsMatchNode(t *testing.T) {
	eng := harness(t)
	eol, devNull := `"\n"`, "/dev/null"
	if nodePlatform() == "win32" {
		eol, devNull = `"\r\n"`, `\\.\nul`
	}
	if got := evalString(t, eng, `JSON.stringify(require("os").EOL)`); got != eol {
		t.Errorf("os.EOL = %s, want %s on %s", got, eol, nodePlatform())
	}
	if got := evalString(t, eng, `require("os").devNull`); got != devNull {
		t.Errorf("os.devNull = %q, want %q on %s", got, devNull, nodePlatform())
	}
}

// TestOSFactsReachTheModule pins that the host facts the snapshot now measures
// arrive in the module rather than stopping at the bridge. Each one used to be a
// zero or an empty string, which is a shape a program cannot tell from a real
// answer, so what is checked here is that they are no longer that.
//
// The values themselves are checked where they are measured, in pkg/nodehost,
// against uname and the kernel's own counters. Here the question is only whether
// the module reports what was measured.
func TestOSFactsReachTheModule(t *testing.T) {
	switch runtime.GOOS {
	case "linux", "darwin", "windows":
	default:
		t.Skipf("no host facts are measured on %s yet", runtime.GOOS)
	}
	eng := harness(t)
	for _, expr := range []string{
		`require("os").release().length > 0`,
		`require("os").machine().length > 0`,
		`require("os").totalmem() > 0`,
		`require("os").freemem() > 0`,
		`require("os").freemem() <= require("os").totalmem()`,
		`require("os").uptime() > 0`,
		`Array.isArray(require("os").loadavg()) && require("os").loadavg().length === 3`,
	} {
		if got := evalString(t, eng, expr); got != "true" {
			t.Errorf("%s = %q, want true", expr, got)
		}
	}
	// machine is uname's name for the hardware, which is not arch's name for it on a
	// 64-bit Intel machine: uname says x86_64 and Node says x64. The module used to
	// answer arch for both, so this is the case that was wrong.
	if runtime.GOARCH == "amd64" {
		if got := evalString(t, eng, `require("os").machine()`); got != "x86_64" {
			t.Errorf("os.machine() = %q on amd64, want %q", got, "x86_64")
		}
		if got := evalString(t, eng, `require("os").arch()`); got != "x64" {
			t.Errorf("os.arch() = %q on amd64, want %q", got, "x64")
		}
	}
}

// TestCPUsReachTheModule pins that the processor list the snapshot now reads off
// the machine arrives in the module. Every entry used to be an unknown model at
// zero megahertz, which a program that prints os.cpus()[0].model would have
// printed without anything failing.
//
// The values are checked against the kernel where they are measured, in
// pkg/nodehost. What matters here is that the module reports them, and that
// availableParallelism is answered on its own rather than from the length of this
// array, which is a different question with the same answer on most machines.
func TestCPUsReachTheModule(t *testing.T) {
	switch runtime.GOOS {
	case "linux", "darwin", "windows":
	default:
		t.Skipf("no processor list is read on %s yet", runtime.GOOS)
	}
	eng := harness(t)
	for _, expr := range []string{
		`require("os").cpus().length > 0`,
		`require("os").cpus()[0].model.length > 0`,
		`require("os").cpus()[0].model !== "unknown"`,
		`typeof require("os").cpus()[0].speed === "number"`,
		`typeof require("os").cpus()[0].times.idle === "number"`,
		`require("os").availableParallelism() > 0`,
		`require("os").availableParallelism() <= require("os").cpus().length`,
	} {
		if got := evalString(t, eng, expr); got != "true" {
			t.Errorf("%s = %q, want true", expr, got)
		}
	}
	// Every key of the times object is one Node reports, and a program that sums them
	// reads all five, so a missing one is undefined rather than absent.
	for _, key := range []string{"user", "nice", "sys", "idle", "irq"} {
		expr := `typeof require("os").cpus()[0].times.` + key + ` === "number"`
		if got := evalString(t, eng, expr); got != "true" {
			t.Errorf("os.cpus()[0].times.%s is not a number", key)
		}
	}
}
