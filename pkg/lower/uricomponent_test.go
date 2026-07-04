package lower

import (
	"strings"
	"testing"
)

// TestEncodeURIComponentLowers pins that encodeURIComponent over a string lowers
// to the value.EncodeURIComponent runtime call.
func TestEncodeURIComponentLowers(t *testing.T) {
	src := `
const s = "a b";
console.log(encodeURIComponent(s));
`
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.EncodeURIComponent(") {
		t.Fatalf("expected an EncodeURIComponent call, got:\n%s", out)
	}
}

// TestDecodeURIComponentLowers pins that decodeURIComponent over a string lowers
// to the value.DecodeURIComponent runtime call.
func TestDecodeURIComponentLowers(t *testing.T) {
	src := `
const s = "a%20b";
console.log(decodeURIComponent(s));
`
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.DecodeURIComponent(") {
		t.Fatalf("expected a DecodeURIComponent call, got:\n%s", out)
	}
}

// TestEncodeURIComponentNonStringHandsBack pins that encodeURIComponent on a
// non-string argument hands back, since the global would run a string coercion
// first.
func TestEncodeURIComponentNonStringHandsBack(t *testing.T) {
	src := `
const n = 42;
console.log(encodeURIComponent(n));
`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "non-string argument") {
		t.Fatalf("expected a non-string handback, got: %q", reason)
	}
}

// TestURIComponentRoundTripRuns builds and runs the emitted Go against the Node
// oracle: the encoder escapes reserved and multibyte bytes and the decoder
// reverses it.
func TestURIComponentRoundTripRuns(t *testing.T) {
	skipIfShort(t)
	src := `
const s = "a b/c?d=café";
const e = encodeURIComponent(s);
console.log(e);
console.log(decodeURIComponent(e));
console.log(decodeURIComponent(e) === s);
`
	got := runProgramGo(t, src)
	want := "a%20b%2Fc%3Fd%3Dcaf%C3%A9\na b/c?d=café\ntrue\n"
	if got != want {
		t.Fatalf("URI component round-trip mismatch:\n got %q\nwant %q", got, want)
	}
}
