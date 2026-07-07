package lower

import (
	"strings"
	"testing"
)

// TestDynamicElementAccessLowers pins that a bracket read on a dynamic receiver
// lowers to the runtime index dispatch rather than handing back: a number index
// takes GetIndex, another dynamic value takes GetElem, and a string-literal key
// takes Get.
func TestDynamicElementAccessLowers(t *testing.T) {
	cases := map[string]struct{ src, want string }{
		"number":  {"let s: any = \"abc\"; let i: number = 1; let c: any = s[i];", ".GetIndex("},
		"dynamic": {"let s: any = \"abc\"; let k: any = 1; let c: any = s[k];", ".GetElem("},
		"string":  {"let s: any = \"abc\"; let c: any = s[\"length\"];", ".Get("},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			out := renderProgram(t, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Fatalf("dynamic element access did not lower to %s:\n%s", tc.want, out)
			}
		})
	}
}

// TestDynamicElementAccessRuns builds and runs a dynamic index over a boxed string
// and checks each read answers the code unit at that index and the length, the way
// JavaScript indexes a string.
func TestDynamicElementAccessRuns(t *testing.T) {
	skipIfShort(t)
	src := `
let s: any = "hello";
let i: number = 1;
console.log(s[i]);
console.log(s[0]);
let k: any = 4;
console.log(s[k]);
console.log(s["length"]);
`
	got := runProgramGo(t, src)
	want := "e\nh\no\n5\n"
	if got != want {
		t.Fatalf("dynamic element access run mismatch:\n got %q\nwant %q", got, want)
	}
}
