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
// Node exposes without an explicit require: Buffer, and the base64 helpers.
const bootstrap = `
(function () {
  var buffer = require("buffer");
  if (typeof globalThis.Buffer === "undefined") globalThis.Buffer = buffer.Buffer;
  if (typeof globalThis.atob === "undefined") globalThis.atob = buffer.atob;
  if (typeof globalThis.btoa === "undefined") globalThis.btoa = buffer.btoa;
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

// HostFuncs returns every Go host function the node layer installs. The fs and
// os functions live in their own files; this gathers them into one map.
func HostFuncs() map[string]HostFunc {
	out := map[string]HostFunc{}
	maps.Copy(out, fsHostFuncs())
	maps.Copy(out, osHostFuncs())
	return out
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
