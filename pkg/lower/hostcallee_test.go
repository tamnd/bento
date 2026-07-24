package lower

import (
	"strings"
	"testing"
)

// TestHostCalleeOSInfoLowers pins that a bare __bento_os_info() call, the host
// callee the os module reaches for to read the platform snapshot, lowers to a
// direct call into pkg/nodehost rather than handing back at the ambient-global
// gate. The emitted Go wraps nodehost.OSInfoJSON in a value.BStr, the string the
// checker types the call as and the JavaScript JSON.parse reads.
func TestHostCalleeOSInfoLowers(t *testing.T) {
	src := `const info = JSON.parse(__bento_os_info()); const p: any = info.platform;`
	out := renderProgram(t, src)
	if !strings.Contains(out, "nodehost.OSInfoJSON()") {
		t.Fatalf("__bento_os_info() did not lower to nodehost.OSInfoJSON():\n%s", out)
	}
	if !strings.Contains(out, `"github.com/tamnd/bento/pkg/nodehost"`) {
		t.Fatalf("emitted program did not import pkg/nodehost:\n%s", out)
	}
}

// TestHostCalleeOSInfoWithArgHandsBack pins that __bento_os_info called with an
// argument hands back rather than lowering: the callee takes no argument, so a
// call with one is a malformed factory, and lowering it anyway would silently
// drop the argument. The mismatched arity draws a 2554 the AOT front door
// tolerates, so the call reaches the renderer through the tolerant path, which is
// where the arity guard hands it back.
func TestHostCalleeOSInfoWithArgHandsBack(t *testing.T) {
	src := `const info: any = __bento_os_info("x");`
	reason := renderProgramTolerantHandBack(t, src)
	if !strings.Contains(reason, "__bento_os_info") {
		t.Fatalf("__bento_os_info(arg) handed back for the wrong reason: %q", reason)
	}
}

// TestHostCalleeOSInfoRuns builds and runs a program that reads the os snapshot
// through the host callee and checks the platform field comes back a string, the
// same value os.platform() reports, so the bridge returns a well-formed object
// JSON.parse can read.
func TestHostCalleeOSInfoRuns(t *testing.T) {
	skipIfShort(t)
	src := `const info = JSON.parse(__bento_os_info()); console.log(typeof info.platform);`
	got := runProgramGo(t, src)
	if got != "string\n" {
		t.Fatalf("os-info host callee run mismatch:\n got %q\nwant %q", got, "string\n")
	}
}

// TestHostCalleeInspectLowers pins that __bento_inspect(value), the callee util
// and assert reach for to render a value, lowers to value.Inspect over the boxed
// argument rather than handing back.
func TestHostCalleeInspectLowers(t *testing.T) {
	src := `const o = { a: 1 }; const s: string = __bento_inspect(o);`
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.Inspect(") {
		t.Fatalf("__bento_inspect() did not lower to value.Inspect:\n%s", out)
	}
}

// TestHostCalleeInspectNoArgHandsBack pins that __bento_inspect with no argument
// hands back: the callee takes exactly one value, and lowering a zero-argument
// call would have nothing to inspect. The missing argument draws a 2554 the AOT
// front door tolerates, so the call reaches the arity guard through the tolerant
// path.
func TestHostCalleeInspectNoArgHandsBack(t *testing.T) {
	src := `const s: any = __bento_inspect();`
	reason := renderProgramTolerantHandBack(t, src)
	if !strings.Contains(reason, "__bento_inspect") {
		t.Fatalf("__bento_inspect() handed back for the wrong reason: %q", reason)
	}
}

// TestHostCalleeInspectRuns builds and runs a program that inspects an object, a
// nested array, and a top-level string, and checks the compact form matches the
// interpreter's prelude inspector: keys unquoted, nested strings quoted, a bare
// string left as its own text.
func TestHostCalleeInspectRuns(t *testing.T) {
	skipIfShort(t)
	src := `
const o = { a: 1, b: "hi", c: [1, 2] };
console.log(__bento_inspect(o));
console.log(__bento_inspect("top"));
`
	got := runProgramGo(t, src)
	want := "{ a: 1, b: \"hi\", c: [ 1, 2 ] }\ntop\n"
	if got != want {
		t.Fatalf("inspect host callee run mismatch:\n got %q\nwant %q", got, want)
	}
}
