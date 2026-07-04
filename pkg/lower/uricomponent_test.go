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

// TestEncodeURILowers pins that encodeURI over a string lowers to the
// value.EncodeURI runtime call.
func TestEncodeURILowers(t *testing.T) {
	src := `
const s = "a b";
console.log(encodeURI(s));
`
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.EncodeURI(") {
		t.Fatalf("expected an EncodeURI call, got:\n%s", out)
	}
}

// TestDecodeURILowers pins that decodeURI over a string lowers to the
// value.DecodeURI runtime call.
func TestDecodeURILowers(t *testing.T) {
	src := `
const s = "a%20b";
console.log(decodeURI(s));
`
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.DecodeURI(") {
		t.Fatalf("expected a DecodeURI call, got:\n%s", out)
	}
}

// TestURIWholeRoundTripRuns builds and runs the emitted Go against the Node
// oracle: encodeURI keeps the reserved delimiters and decodeURI reverses it while
// leaving an escaped reserved delimiter as its literal.
func TestURIWholeRoundTripRuns(t *testing.T) {
	skipIfShort(t)
	src := `
const u = "http://a.b/c d?x=café&y=z#f";
const e = encodeURI(u);
console.log(e);
console.log(decodeURI(e));
console.log(decodeURI(e) === u);
console.log(decodeURI("%3Bx%2Fy"));
`
	got := runProgramGo(t, src)
	want := "http://a.b/c%20d?x=caf%C3%A9&y=z#f\nhttp://a.b/c d?x=café&y=z#f\ntrue\n%3Bx%2Fy\n"
	if got != want {
		t.Fatalf("encodeURI round-trip mismatch:\n got %q\nwant %q", got, want)
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
