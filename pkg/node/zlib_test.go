package node

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/tamnd/bento/pkg/engine"
)

// zlibRun calls the host primitive the way the zlib module does and returns the
// bytes, failing the test if the envelope came back with an error.
func zlibRun(t *testing.T, format, direction string, in []byte, level int) []byte {
	t.Helper()
	raw, err := hostZlibRun([]any{format, direction, base64.StdEncoding.EncodeToString(in), float64(level)})
	if err != nil {
		t.Fatalf("%s %s: %v", format, direction, err)
	}
	var res zlibResult
	if err := json.Unmarshal([]byte(raw.(string)), &res); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if !res.OK {
		t.Fatalf("%s %s failed: %s %s", format, direction, res.Code, res.Msg)
	}
	out, err := base64.StdEncoding.DecodeString(res.B64)
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	return out
}

// zlibErr is zlibRun for a call expected to fail, returning the envelope.
func zlibErr(t *testing.T, format, direction string, in []byte) zlibResult {
	t.Helper()
	raw, err := hostZlibRun([]any{format, direction, base64.StdEncoding.EncodeToString(in), float64(-1)})
	if err != nil {
		t.Fatalf("%s %s: %v", format, direction, err)
	}
	var res zlibResult
	if err := json.Unmarshal([]byte(raw.(string)), &res); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if res.OK {
		t.Fatalf("%s %s on malformed input succeeded, want a failure", format, direction)
	}
	return res
}

// sample is compressible enough that a round trip through every codec is worth
// checking against a size, and long enough to span more than one deflate block.
var sample = []byte(strings.Repeat("the quick brown fox jumps over the lazy dog\n", 200))

func TestZlibRoundTripsEveryFormat(t *testing.T) {
	for _, format := range []string{formatDeflate, formatDeflateRaw, formatGzip} {
		compressed := zlibRun(t, format, "compress", sample, -1)
		if len(compressed) >= len(sample) {
			t.Errorf("%s: compressed %d bytes to %d, want smaller", format, len(sample), len(compressed))
		}
		if got := zlibRun(t, format, "decompress", compressed, -1); !bytes.Equal(got, sample) {
			t.Errorf("%s: round trip returned %d bytes, want the original %d", format, len(got), len(sample))
		}
	}
}

// TestZlibProducesTheStandardFramings pins that what comes out is the wire
// format the name promises, not merely something bento can read back: each is
// handed to the matching reader from the standard library.
func TestZlibProducesTheStandardFramings(t *testing.T) {
	readers := map[string]func([]byte) (io.ReadCloser, error){
		formatDeflate:    func(b []byte) (io.ReadCloser, error) { return zlib.NewReader(bytes.NewReader(b)) },
		formatDeflateRaw: func(b []byte) (io.ReadCloser, error) { return flate.NewReader(bytes.NewReader(b)), nil },
		formatGzip:       func(b []byte) (io.ReadCloser, error) { return gzip.NewReader(bytes.NewReader(b)) },
	}
	for format, open := range readers {
		r, err := open(zlibRun(t, format, "compress", sample, -1))
		if err != nil {
			t.Fatalf("%s: open: %v", format, err)
		}
		got, err := io.ReadAll(r)
		// ReadAll consumed the stream, so a close error here is about the reader's own
		// teardown and cannot change what was read; the read error below is the one
		// that decides the case.
		_ = r.Close()
		if err != nil {
			t.Fatalf("%s: read: %v", format, err)
		}
		if !bytes.Equal(got, sample) {
			t.Errorf("%s: standard reader got %d bytes, want %d", format, len(got), len(sample))
		}
	}
}

// TestZlibUnzipSniffsTheFraming pins the one mode that is not a format: unzip
// takes a gzip member or a zlib stream and decides by the header.
func TestZlibUnzipSniffsTheFraming(t *testing.T) {
	for _, format := range []string{formatDeflate, formatGzip} {
		compressed := zlibRun(t, format, "compress", sample, -1)
		if got := zlibRun(t, formatUnzip, "decompress", compressed, -1); !bytes.Equal(got, sample) {
			t.Errorf("unzip of a %s stream returned %d bytes, want %d", format, len(got), len(sample))
		}
	}
}

// TestZlibLevelChangesTheOutput pins that the level reaches the compressor: no
// compression has to be bigger than the best, and both have to read back.
func TestZlibLevelChangesTheOutput(t *testing.T) {
	none := zlibRun(t, formatGzip, "compress", sample, 0)
	best := zlibRun(t, formatGzip, "compress", sample, 9)
	if len(none) <= len(best) {
		t.Errorf("level 0 produced %d bytes and level 9 produced %d, want level 0 to be larger", len(none), len(best))
	}
	if got := zlibRun(t, formatGzip, "decompress", none, -1); !bytes.Equal(got, sample) {
		t.Error("level 0 output did not read back")
	}
}

// TestZlibClampsAnOutOfRangeLevel pins that a level no compressor accepts is
// clamped rather than raised as an error. Go rejects anything outside [-2, 9];
// zlib clamps, and Node hands the caller's number straight to zlib.
func TestZlibClampsAnOutOfRangeLevel(t *testing.T) {
	for _, level := range []int{-99, 42} {
		out := zlibRun(t, formatGzip, "compress", sample, level)
		if got := zlibRun(t, formatGzip, "decompress", out, -1); !bytes.Equal(got, sample) {
			t.Errorf("level %d did not round trip", level)
		}
	}
}

// TestZlibReportsMalformedInput pins the two codes a program branches on: a
// stream that is not one at all is a data error, and one cut short is a buffer
// error, which is what zlib says when it runs out of input.
func TestZlibReportsMalformedInput(t *testing.T) {
	if res := zlibErr(t, formatGzip, "decompress", []byte("not a gzip member at all")); res.Code != "Z_DATA_ERROR" {
		t.Errorf("garbage input reported %q, want Z_DATA_ERROR", res.Code)
	}
	full := zlibRun(t, formatGzip, "compress", sample, -1)
	if res := zlibErr(t, formatGzip, "decompress", full[:len(full)/2]); res.Code != "Z_BUF_ERROR" {
		t.Errorf("truncated input reported %q, want Z_BUF_ERROR", res.Code)
	}
}

// TestZlibGunzipsEveryMember pins that concatenated gzip output comes back
// whole, the way Node's gunzip reads it, rather than stopping at the first
// member's trailer.
func TestZlibGunzipsEveryMember(t *testing.T) {
	one := zlibRun(t, formatGzip, "compress", []byte("first\n"), -1)
	two := zlibRun(t, formatGzip, "compress", []byte("second\n"), -1)
	got := zlibRun(t, formatGzip, "decompress", append(append([]byte{}, one...), two...), -1)
	if want := "first\nsecond\n"; string(got) != want {
		t.Errorf("two members decompressed to %q, want %q", got, want)
	}
}

func TestZlibRoundTripsEmptyInput(t *testing.T) {
	for _, format := range []string{formatDeflate, formatDeflateRaw, formatGzip} {
		compressed := zlibRun(t, format, "compress", nil, -1)
		if len(compressed) == 0 && format != formatDeflateRaw {
			t.Errorf("%s: empty input produced no framing at all", format)
		}
		if got := zlibRun(t, format, "decompress", compressed, -1); len(got) != 0 {
			t.Errorf("%s: empty input round tripped to %d bytes", format, len(got))
		}
	}
}

// TestZlibModule drives the JavaScript surface: the synchronous calls, the
// callback forms, the streams and the constants, through the same require the
// rest of the node layer is tested with.
func TestZlibModule(t *testing.T) {
	eng := harness(t)
	cases := map[string]string{
		// Every codec round trips through its own inverse.
		`require("zlib").inflateSync(require("zlib").deflateSync("hello")).toString()`:         "hello",
		`require("zlib").inflateRawSync(require("zlib").deflateRawSync("hello")).toString()`:   "hello",
		`require("zlib").gunzipSync(require("zlib").gzipSync("hello")).toString()`:             "hello",
		`require("zlib").unzipSync(require("zlib").gzipSync("hello")).toString()`:              "hello",
		`require("zlib").unzipSync(require("zlib").deflateSync("hello")).toString()`:           "hello",
		`require("node:zlib").gunzipSync(require("zlib").gzipSync("via node: prefix")).length`: "16",
		`Buffer.isBuffer(require("zlib").gzipSync("hello"))`:                                   "true",
		// A Buffer and a view over an ArrayBuffer are inputs too, not only strings.
		`require("zlib").gunzipSync(require("zlib").gzipSync(Buffer.from("buf"))).toString()`:        "buf",
		`require("zlib").gunzipSync(require("zlib").gzipSync(new Uint8Array([104,105]))).toString()`: "hi",
		// The level reaches the compressor through the options object.
		`require("zlib").gzipSync("x".repeat(500), { level: 0 }).length > require("zlib").gzipSync("x".repeat(500), { level: 9 }).length`: "true",
		// The constants live in both places Node puts them.
		`require("zlib").constants.Z_BEST_COMPRESSION`: "9",
		`require("zlib").Z_DEFAULT_COMPRESSION`:        "-1",
		// The classes are named and the factories build them.
		`require("zlib").createGzip() instanceof require("zlib").Gzip`: "true",
		`require("zlib").Gunzip.name`:                                  "Gunzip",
		// Brotli is deliberately absent rather than present and throwing.
		`typeof require("zlib").brotliCompressSync`: "undefined",
	}
	for expr, want := range cases {
		if got := evalString(t, eng, expr); got != want {
			t.Errorf("%s = %q, want %q", expr, got, want)
		}
	}
}

// TestZlibModuleReportsErrors pins that a failed decompression arrives as an
// Error carrying the code and errno a program reads, not as a rejected envelope.
func TestZlibModuleReportsErrors(t *testing.T) {
	eng := harness(t)
	cases := map[string]string{
		`(() => { try { require("zlib").gunzipSync("not gzip"); return "no throw"; } catch (e) { return e.code; } })()`:  "Z_DATA_ERROR",
		`(() => { try { require("zlib").gunzipSync("not gzip"); return "no throw"; } catch (e) { return e.errno; } })()`: "-3",
	}
	for expr, want := range cases {
		if got := evalString(t, eng, expr); got != want {
			t.Errorf("%s = %q, want %q", expr, got, want)
		}
	}
}

// TestZlibModuleCallbackForm pins that the callback shape works and that it is
// asynchronous, the way Node's is, even though the codec underneath is not: the
// callback must not have run by the time the call returns.
func TestZlibModuleCallbackForm(t *testing.T) {
	eng := harness(t)
	script := `
	globalThis.order = [];
	const zlib = require("zlib");
	zlib.gzip("payload", function (err, buf) {
	  order.push(err ? "err:" + err.code : zlib.gunzipSync(buf).toString());
	});
	order.push("after-call");
	`
	if _, err := eng.Eval("<zlib-callback>", script); err != nil {
		t.Fatalf("eval: %v", err)
	}
	drainMicrotasks(t, eng)
	if got := evalString(t, eng, `order.join(",")`); got != "after-call,payload" {
		t.Errorf("callback order was %q, want %q", got, "after-call,payload")
	}
}

// TestZlibModuleStream pins the Transform path: chunks written across several
// calls come back as one round trip through the codec.
func TestZlibModuleStream(t *testing.T) {
	eng := harness(t)
	script := `
	const zlib = require("zlib");
	globalThis.streamed = null;
	const gzip = zlib.createGzip();
	const parts = [];
	gzip.on("data", (chunk) => parts.push(chunk));
	gzip.on("end", () => { streamed = zlib.gunzipSync(Buffer.concat(parts)).toString(); });
	gzip.write("one ");
	gzip.write("two ");
	gzip.end("three");
	`
	if _, err := eng.Eval("<zlib-stream>", script); err != nil {
		t.Fatalf("eval: %v", err)
	}
	drainMicrotasks(t, eng)
	if got := evalString(t, eng, `streamed`); got != "one two three" {
		t.Errorf("stream produced %q, want %q", got, "one two three")
	}
}

// drainMicrotasks runs the queued jobs the callback form schedules.
func drainMicrotasks(t *testing.T, eng engine.Engine) {
	t.Helper()
	if _, err := eng.DrainMicrotasks(); err != nil {
		t.Fatalf("drain: %v", err)
	}
}
