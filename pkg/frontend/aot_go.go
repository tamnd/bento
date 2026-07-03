package frontend

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/tamnd/bento/pkg/goimport"
)

// This file makes a go: import type-check. The resolver classifies go:crypto/sha256
// as a Go interop import (document 16 section 3), but the checker still needs the
// package's API to bind the names a program imports from it. bento generates that
// API as a .d.ts from the real Go package (section 4) and serves it here, at a
// virtual path the go: import resolves to, so the import checks against the genuine
// signatures without a declaration file living on disk.

// goDeclPrefix is the virtual directory generated Go package declarations are
// served under. A go: import resolves to a .d.ts path beneath it, which the checker
// reads like any other declaration file.
const goDeclPrefix = "/__bento_go__/"

// goDeclPath is the virtual .d.ts path a go: import at importPath and version
// resolves to. The import path nests as directories and the pinned version rides an
// @ suffix, both recoverable later because a Go import path never contains an @.
func goDeclPath(importPath, version string) string {
	p := goDeclPrefix + importPath
	if version != "" {
		p += "@" + version
	}
	return p + ".d.ts"
}

// goImportForDeclPath reverses goDeclPath into the import path and version, so the
// overlay knows which package to generate and a later lowering stage can recover
// the go: target from a resolved import edge. ok is false for any path that is not
// one of these virtual declaration files.
func goImportForDeclPath(path string) (importPath, version string, ok bool) {
	if !strings.HasPrefix(path, goDeclPrefix) || !strings.HasSuffix(path, ".d.ts") {
		return "", "", false
	}
	body := strings.TrimSuffix(strings.TrimPrefix(path, goDeclPrefix), ".d.ts")
	if body == "" {
		return "", "", false
	}
	if at := strings.LastIndexByte(body, '@'); at >= 0 {
		return body[:at], body[at+1:], true
	}
	return body, "", true
}

// goDeclOverlay serves generated Go package declarations for the virtual paths a
// go: import resolves to, layered over a base FileSystem. It generates a package's
// declarations on first read and memoizes the outcome for the rest of the load, so
// a package imported from several files is generated once. Generation goes through
// the goimport cache, so a second load reuses the on-disk entry as well.
type goDeclOverlay struct {
	base  FileSystem
	cache *goimport.Cache
	mu    sync.Mutex
	memo  map[string]goDeclResult
}

// goDeclResult is a memoized generation outcome: the declaration text and whether
// generation succeeded, so a package that fails to generate is a clean miss rather
// than a repeated attempt.
type goDeclResult struct {
	text string
	ok   bool
}

// newGoDeclOverlay wraps base so reads of the virtual Go declaration paths are
// served from generation and every other read falls through unchanged.
func newGoDeclOverlay(base FileSystem, cache *goimport.Cache) *goDeclOverlay {
	return &goDeclOverlay{base: base, cache: cache, memo: map[string]goDeclResult{}}
}

func (o *goDeclOverlay) ReadFile(path string) (string, bool) {
	if importPath, version, ok := goImportForDeclPath(path); ok {
		r := o.declarations(path, importPath, version)
		return r.text, r.ok
	}
	return o.base.ReadFile(path)
}

func (o *goDeclOverlay) FileExists(path string) bool {
	if importPath, version, ok := goImportForDeclPath(path); ok {
		return o.declarations(path, importPath, version).ok
	}
	return o.base.FileExists(path)
}

// DirectoryExists falls through to the base: a go: import resolves straight to a
// declaration file and never probes the virtual tree for directories.
func (o *goDeclOverlay) DirectoryExists(path string) bool {
	return o.base.DirectoryExists(path)
}

// declarations returns the generated declarations for a virtual path, generating
// and memoizing them on first request. A generation failure memoizes as a miss, so
// an unresolvable go: import surfaces as a missing module the checker reports once,
// not a stall that retries the Go toolchain on every read.
func (o *goDeclOverlay) declarations(path, importPath, version string) goDeclResult {
	o.mu.Lock()
	defer o.mu.Unlock()
	if r, seen := o.memo[path]; seen {
		return r
	}
	body, err := o.cache.Load(importPath, version)
	r := goDeclResult{ok: err == nil}
	if err == nil {
		r.text = wrapGoModule(goSpecifier(importPath, version), body)
	}
	o.memo[path] = r
	return r
}

// goSpecifier rebuilds the go: import specifier a package's declarations are served
// for, so the ambient module the checker reads is named exactly as the program
// wrote the import.
func goSpecifier(importPath, version string) string {
	s := "go:" + importPath
	if version != "" {
		s += "@" + version
	}
	return s
}

// wrapGoModule wraps a generated package body as a TypeScript ambient module
// declaration for the go: specifier. This is the form the checker resolves an
// `import ... from "go:crypto/sha256"` against, the same mechanism the Node and
// bento:go declarations rely on. A declare module is already an ambient context, so
// any top-level declare modifier the body carries is dropped.
func wrapGoModule(specifier, body string) string {
	wrapped := "declare module \"" + specifier + "\" {\n" + body + "}\n"
	return strings.ReplaceAll(wrapped, "\nexport declare ", "\nexport ")
}

// goDeclCache builds the on-disk cache generated Go declarations are stored in. It
// lives under the user cache directory so it survives across loads and across
// projects, keyed inside by the module version and toolchain, and falls back to the
// temp directory when no user cache is available.
func goDeclCache() *goimport.Cache {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	return goimport.NewCache(filepath.Join(base, "bento", "go-decls"), "")
}
