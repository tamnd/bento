package lower

import (
	"strings"
	"testing"
)

// TestBtoaLowers pins that btoa over a string lowers to the value.Btoa runtime
// call.
func TestBtoaLowers(t *testing.T) {
	src := `
const s = "hello";
console.log(btoa(s));
`
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.Btoa(") {
		t.Fatalf("expected a Btoa call, got:\n%s", out)
	}
}

// TestAtobLowers pins that atob over a string lowers to the value.Atob runtime
// call.
func TestAtobLowers(t *testing.T) {
	src := `
const s = "aGVsbG8=";
console.log(atob(s));
`
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.Atob(") {
		t.Fatalf("expected an Atob call, got:\n%s", out)
	}
}

// TestBase64RoundTripRuns builds and runs the emitted Go against the Node oracle:
// btoa encodes a string and atob decodes it back, across the padding cases.
func TestBase64RoundTripRuns(t *testing.T) {
	skipIfShort(t)
	src := `
const s = "Many hands make light work.";
const e = btoa(s);
console.log(e);
console.log(atob(e));
console.log(atob(e) === s);
console.log(btoa("M"), btoa("Ma"), btoa("Man"));
`
	got := runProgramGo(t, src)
	want := "TWFueSBoYW5kcyBtYWtlIGxpZ2h0IHdvcmsu\nMany hands make light work.\ntrue\nTQ== TWE= TWFu\n"
	if got != want {
		t.Fatalf("base64 round-trip mismatch:\n got %q\nwant %q", got, want)
	}
}
