// Package frontend turns TypeScript and modern JavaScript source into the
// JavaScript dialect the engine executes.
//
// For the run path bento uses esbuild's single-file transform, which is pure Go
// and fast. It strips TypeScript types, lowers recent syntax to a stable target,
// and leaves module semantics intact. The heavier typescript-go frontend is used
// for type checking and for the ahead-of-time compile path, not here.
package frontend

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
)

// ErrTopLevelAwait reports that a source uses top-level await, which cannot be
// represented in the CommonJS output format. The runtime catches this and runs
// the module through the native ES module path instead, where top-level await is
// legal. It is returned by Transpile, wrapped, so callers test with errors.Is.
var ErrTopLevelAwait = errors.New("top-level await requires the ES module format")

// Result is the outcome of transpiling one source file.
type Result struct {
	// Code is the emitted JavaScript.
	Code string
	// SourceMap is the inline or external source map, empty when disabled.
	SourceMap string
}

// Options controls a transpile.
type Options struct {
	// Filename is the original path, used for diagnostics and loader detection.
	Filename string
	// SourceMap requests an inline source map appended to Code.
	SourceMap bool
}

// Transpile converts one TypeScript or JavaScript file to CommonJS JavaScript
// targeting a stable syntax level the engine supports. The loader is chosen from
// the file extension so .ts, .tsx, .jsx, .mts, and .cts all work.
func Transpile(source string, opts Options) (Result, error) {
	loader := loaderFor(opts.Filename)

	sourcemap := api.SourceMapNone
	if opts.SourceMap {
		sourcemap = api.SourceMapInline
	}

	res := api.Transform(source, api.TransformOptions{
		Loader:     loader,
		Format:     api.FormatCommonJS,
		Target:     api.ES2022,
		Platform:   api.PlatformNeutral,
		Sourcefile: displayName(opts.Filename),
		Sourcemap:  sourcemap,
	})

	if len(res.Errors) > 0 {
		if hasTopLevelAwait(res.Errors) {
			return Result{}, fmt.Errorf("transpile %s: %w", displayName(opts.Filename), ErrTopLevelAwait)
		}
		return Result{}, fmt.Errorf("transpile %s:\n%s", displayName(opts.Filename), formatMessages(res.Errors))
	}

	return Result{Code: string(res.Code)}, nil
}

// TranspileESM converts one TypeScript or JavaScript file to ES module output,
// preserving import and export statements and top-level await so the engine can
// link and run it natively. It is the path for modules the CommonJS format
// cannot express.
func TranspileESM(source string, opts Options) (Result, error) {
	loader := loaderFor(opts.Filename)

	sourcemap := api.SourceMapNone
	if opts.SourceMap {
		sourcemap = api.SourceMapInline
	}

	res := api.Transform(source, api.TransformOptions{
		Loader:     loader,
		Format:     api.FormatESModule,
		Target:     api.ES2022,
		Platform:   api.PlatformNeutral,
		Sourcefile: displayName(opts.Filename),
		Sourcemap:  sourcemap,
	})

	if len(res.Errors) > 0 {
		return Result{}, fmt.Errorf("transpile %s:\n%s", displayName(opts.Filename), formatMessages(res.Errors))
	}

	return Result{Code: string(res.Code)}, nil
}

// hasTopLevelAwait reports whether the transform failed specifically because the
// source uses top-level await, which esbuild cannot lower to CommonJS. The text
// is stable across esbuild versions ("Top-level await is currently not supported
// with the \"cjs\" output format").
func hasTopLevelAwait(msgs []api.Message) bool {
	for _, m := range msgs {
		if strings.Contains(m.Text, "Top-level await") {
			return true
		}
	}
	return false
}

// loaderFor picks the esbuild loader from a file extension. Unknown extensions
// fall back to the TypeScript loader, which is a strict superset of JavaScript.
func loaderFor(name string) api.Loader {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".tsx":
		return api.LoaderTSX
	case ".jsx":
		return api.LoaderJSX
	case ".js", ".mjs", ".cjs":
		return api.LoaderJS
	case ".json":
		return api.LoaderJSON
	default:
		return api.LoaderTS
	}
}

func displayName(name string) string {
	if name == "" {
		return "<input>"
	}
	return name
}

func formatMessages(msgs []api.Message) string {
	var b strings.Builder
	for i, m := range msgs {
		if i > 0 {
			b.WriteByte('\n')
		}
		if m.Location != nil {
			fmt.Fprintf(&b, "  %s:%d:%d: %s", m.Location.File, m.Location.Line, m.Location.Column, m.Text)
		} else {
			fmt.Fprintf(&b, "  %s", m.Text)
		}
	}
	return b.String()
}
