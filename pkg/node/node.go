// Package node implements bento's Node.js core module layer.
//
// Each core module (path, events, buffer, util, assert, stream, os, fs, ...)
// ships as an embedded CommonJS factory in js/*.js. The factories register
// themselves with the prelude through __bento_defineModule and build their
// exports lazily on first require, exactly like Node loads its own core
// modules. Modules that need real I/O (fs, os) call a small set of Go host
// functions registered here; the pure-logic modules (path, events, buffer,
// util, assert, stream) run entirely in JavaScript.
//
// Install wires both halves into a runtime: it registers the host functions and
// evaluates the bundled module source so require() can resolve every builtin.
package node

import (
	"embed"
	"fmt"
	"io/fs"
	"maps"
	"sort"
	"strings"

	"github.com/tamnd/bento/pkg/engine"
)

//go:embed js/*.js
var jsFiles embed.FS

// bootstrap runs after every module factory is defined. It promotes the globals
// Node and the web platform expose without an explicit require: Buffer, the
// base64 helpers, the WHATWG URL classes, and the text encoders.
const bootstrap = `
(function () {
  var buffer = require("buffer");
  if (typeof globalThis.Buffer === "undefined") globalThis.Buffer = buffer.Buffer;
  if (typeof globalThis.atob === "undefined") globalThis.atob = buffer.atob;
  if (typeof globalThis.btoa === "undefined") globalThis.btoa = buffer.btoa;

  var url = require("url");
  if (typeof globalThis.URL === "undefined") globalThis.URL = url.URL;
  if (typeof globalThis.URLSearchParams === "undefined") globalThis.URLSearchParams = url.URLSearchParams;

  var fetchMod = require("fetch");
  if (typeof globalThis.fetch === "undefined") globalThis.fetch = fetchMod.fetch;
  if (typeof globalThis.Headers === "undefined") globalThis.Headers = fetchMod.Headers;
  if (typeof globalThis.Request === "undefined") globalThis.Request = fetchMod.Request;
  if (typeof globalThis.Response === "undefined") globalThis.Response = fetchMod.Response;

  // TextEncoder/TextDecoder are the canonical utf8 codec. Buffer's utf8 paths
  // delegate here when these globals exist, so the encoders must implement utf8
  // directly rather than routing back through Buffer, or the two recurse into a
  // stack overflow. The non-utf8 decode labels (utf-16le, latin1) do not share
  // that cycle, so those defer to Buffer.
  if (typeof globalThis.TextEncoder === "undefined") {
    var utf8Encode = function (str) {
      var out = [];
      for (var i = 0; i < str.length; i++) {
        var c = str.charCodeAt(i);
        if (c < 0x80) {
          out.push(c);
        } else if (c < 0x800) {
          out.push(0xc0 | (c >> 6), 0x80 | (c & 0x3f));
        } else if (c >= 0xd800 && c <= 0xdbff) {
          var c2 = str.charCodeAt(i + 1);
          if (c2 >= 0xdc00 && c2 <= 0xdfff) {
            var cp = 0x10000 + ((c - 0xd800) << 10) + (c2 - 0xdc00);
            out.push(0xf0 | (cp >> 18), 0x80 | ((cp >> 12) & 0x3f), 0x80 | ((cp >> 6) & 0x3f), 0x80 | (cp & 0x3f));
            i++;
          } else {
            out.push(0xef, 0xbf, 0xbd); // lone high surrogate
          }
        } else if (c >= 0xdc00 && c <= 0xdfff) {
          out.push(0xef, 0xbf, 0xbd); // lone low surrogate
        } else {
          out.push(0xe0 | (c >> 12), 0x80 | ((c >> 6) & 0x3f), 0x80 | (c & 0x3f));
        }
      }
      return out;
    };
    globalThis.TextEncoder = class TextEncoder {
      get encoding() { return "utf-8"; }
      encode(input) {
        return Uint8Array.from(utf8Encode(input == null ? "" : String(input)));
      }
      encodeInto(source, dest) {
        var encoded = this.encode(source);
        var written = Math.min(encoded.length, dest.length);
        dest.set(encoded.subarray(0, written));
        return { read: written, written: written };
      }
    };
  }

  if (typeof globalThis.TextDecoder === "undefined") {
    var normalizeEncoding = function (label) {
      var l = (label == null ? "utf-8" : String(label)).toLowerCase();
      if (l === "utf-16le" || l === "utf-16" || l === "utf16le") return "utf-16le";
      if (l === "latin1" || l === "iso-8859-1" || l === "ascii") return "latin1";
      return "utf-8";
    };
    var utf8Decode = function (bytes) {
      var out = "";
      var i = 0;
      var n = bytes.length;
      while (i < n) {
        var b0 = bytes[i++];
        if (b0 < 0x80) { out += String.fromCharCode(b0); continue; }
        var cp, extra;
        if ((b0 & 0xe0) === 0xc0) { cp = b0 & 0x1f; extra = 1; }
        else if ((b0 & 0xf0) === 0xe0) { cp = b0 & 0x0f; extra = 2; }
        else if ((b0 & 0xf8) === 0xf0) { cp = b0 & 0x07; extra = 3; }
        else { out += "�"; continue; }
        for (var k = 0; k < extra; k++) {
          if (i >= n) { cp = -1; break; }
          var bx = bytes[i];
          if ((bx & 0xc0) !== 0x80) { cp = -1; break; }
          cp = (cp << 6) | (bx & 0x3f);
          i++;
        }
        if (cp < 0) { out += "�"; continue; }
        if (cp > 0xffff) {
          cp -= 0x10000;
          out += String.fromCharCode(0xd800 + (cp >> 10), 0xdc00 + (cp & 0x3ff));
        } else {
          out += String.fromCharCode(cp);
        }
      }
      return out;
    };
    globalThis.TextDecoder = class TextDecoder {
      constructor(label, options) {
        this._encoding = normalizeEncoding(label);
        this.fatal = !!(options && options.fatal);
        this.ignoreBOM = !!(options && options.ignoreBOM);
      }
      get encoding() { return this._encoding; }
      decode(input) {
        if (input == null) return "";
        var bytes = input instanceof Uint8Array
          ? input
          : new Uint8Array(input.buffer || input, input.byteOffset || 0, input.byteLength != null ? input.byteLength : input.length);
        if (this._encoding === "utf-8") return utf8Decode(bytes);
        var enc = this._encoding === "utf-16le" ? "utf16le" : "latin1";
        return globalThis.Buffer.from(bytes).toString(enc);
      }
    };
  }
})();
`

// HostFunc is re-exported so runtime wiring can name the type without importing
// the engine package directly.
type HostFunc = engine.HostFunc

// Install registers the node layer on an engine: the Go-backed host functions
// first, then the module source, then the global bootstrap. The engine must
// already have the prelude evaluated so __bento_defineModule exists.
func Install(eng engine.Engine) error {
	for name, fn := range HostFuncs() {
		if err := eng.Register(name, fn); err != nil {
			return fmt.Errorf("node: register %s: %w", name, err)
		}
	}
	src, err := Source()
	if err != nil {
		return err
	}
	if _, err := eng.Eval("<node>", src); err != nil {
		return fmt.Errorf("node: load core modules: %w", err)
	}
	if _, err := eng.Eval("<node-bootstrap>", bootstrap); err != nil {
		return fmt.Errorf("node: bootstrap globals: %w", err)
	}
	return nil
}

// InstallNet wires the networking modules that need the event loop: their host
// functions do blocking I/O on pool goroutines and post results back to the loop.
// It runs after Install, which has already evaluated the JavaScript factories
// (js/http.js and friends), so this only has to register the Go host functions.
// The loop must be the same one the runtime pumps.
func InstallNet(eng engine.Engine, loop LoopHost) error {
	if err := installNet(eng, loop); err != nil {
		return fmt.Errorf("node: install net: %w", err)
	}
	if err := installHTTP(eng, loop); err != nil {
		return fmt.Errorf("node: install http: %w", err)
	}
	return nil
}

// HostFuncs returns every Go host function the node layer installs. The fs and
// os functions live in their own files; this gathers them into one map.
func HostFuncs() map[string]HostFunc {
	out := map[string]HostFunc{}
	maps.Copy(out, fsHostFuncs())
	maps.Copy(out, osHostFuncs())
	maps.Copy(out, urlHostFuncs())
	return out
}

// Builtins lists the base names of the Node core modules bento ships, derived
// from the embedded module files. The resolver uses this set to classify a bare
// or node: specifier as a builtin, so it stays in lockstep with what actually
// loads: adding a js/<name>.js file makes <name> resolvable with no other edit.
func Builtins() []string {
	entries, err := fs.ReadDir(jsFiles, "js")
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if name := e.Name(); strings.HasSuffix(name, ".js") {
			names = append(names, strings.TrimSuffix(name, ".js"))
		}
	}
	sort.Strings(names)
	return names
}

// Source concatenates the embedded module files in a stable order so the loaded
// program is deterministic. Order does not affect behavior because factories
// are lazy, but a stable bundle keeps stack traces and caching predictable.
func Source() (string, error) {
	entries, err := fs.ReadDir(jsFiles, "js")
	if err != nil {
		return "", fmt.Errorf("node: read embedded modules: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".js") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	var b strings.Builder
	for _, name := range names {
		data, err := jsFiles.ReadFile("js/" + name)
		if err != nil {
			return "", fmt.Errorf("node: read %s: %w", name, err)
		}
		b.WriteString("// module: ")
		b.WriteString(name)
		b.WriteByte('\n')
		b.Write(data)
		b.WriteByte('\n')
	}
	return b.String(), nil
}
