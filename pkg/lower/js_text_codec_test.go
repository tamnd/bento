package lower

import (
	"strings"
	"testing"
)

// TestTextCodecRoundTrips is the ordinary JavaScript spelling of the codec: build the
// pair, cross a string to bytes and back, and read each one's encoding.
func TestTextCodecRoundTrips(t *testing.T) {
	got := goRunSource(t, renderExpandoJS(t, `const enc = new TextEncoder();
const dec = new TextDecoder();
const bytes = enc.encode("héllo 😀");
console.log(bytes.length, enc.encoding, dec.encoding);
console.log(dec.decode(bytes));
`))
	want := "11 utf-8 utf-8\nhéllo 😀\n"
	if got != want {
		t.Errorf("codec round trip\n got: %q\nwant: %q", got, want)
	}
}

// TestEncodedBytesAreAUint8Array pins that encode hands back the typed view, not a
// boxed value, so the array surface a caller reaches for afterwards still works.
func TestEncodedBytesAreAUint8Array(t *testing.T) {
	got := goRunSource(t, renderExpandoJS(t, `const bytes = new TextEncoder().encode("é");
console.log(bytes[0], bytes[1], bytes.byteLength);
`))
	want := "195 169 2\n"
	if got != want {
		t.Errorf("encoded view\n got: %q\nwant: %q", got, want)
	}
}

// TestDecoderReadsItsOptions pins the options dictionary: the two flags reach the
// constructor and read back off the decoder.
func TestDecoderReadsItsOptions(t *testing.T) {
	got := goRunSource(t, renderExpandoJS(t, `const dec = new TextDecoder("utf-8", { fatal: true, ignoreBOM: true });
console.log(dec.encoding, dec.fatal, dec.ignoreBOM);
const plain = new TextDecoder();
console.log(plain.fatal, plain.ignoreBOM);
`))
	want := "utf-8 true true\nfalse false\n"
	if got != want {
		t.Errorf("decoder options\n got: %q\nwant: %q", got, want)
	}
}

// TestDecoderReplacesMalformedBytes pins the recovery a program observes: a bad byte
// becomes one replacement character and the rest of the input still decodes.
func TestDecoderReplacesMalformedBytes(t *testing.T) {
	got := goRunSource(t, renderExpandoJS(t, `const bad = new Uint8Array([97, 255, 98]);
const s = new TextDecoder().decode(bad);
console.log(s, s.length);
`))
	want := "a\uFFFDb 3\n"
	if got != want {
		t.Errorf("malformed decode\n got: %q\nwant: %q", got, want)
	}
}

// TestCodecLowersToTheRuntimeTypes pins the emitted shape: the pair are Go types the
// compiler proved, not boxed property bags, which is what lets encode hand back a
// Uint8Array at all.
func TestCodecLowersToTheRuntimeTypes(t *testing.T) {
	source := renderExpandoJS(t, `const enc = new TextEncoder();
const dec = new TextDecoder("utf-16le");
console.log(dec.decode(enc.encode("x")));
`)
	for _, want := range []string{"value.NewTextEncoder()", "value.NewTextDecoder("} {
		if !strings.Contains(source, want) {
			t.Errorf("emitted Go does not call %s:\n%s", want, source)
		}
	}
}

// TestUnsupportedEncodingThrows pins the honest answer for a label no hosted encoding
// claims. A quiet fall back to utf-8 would hand the program bytes read as the wrong
// text, so the constructor throws the way Node does.
func TestUnsupportedEncodingThrows(t *testing.T) {
	got := goRunSource(t, renderExpandoJS(t, `try {
  new TextDecoder("shift_jis");
  console.log("no throw");
} catch (e) {
  if (e instanceof RangeError) {
    console.log(e.name);
  }
}
`))
	if got != "RangeError\n" {
		t.Errorf("unsupported label\n got: %q\nwant: %q", got, "RangeError\n")
	}
}
