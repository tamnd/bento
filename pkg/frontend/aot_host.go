package frontend

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/tamnd/bento/pkg/frontend/adapter"
	"github.com/tamnd/bento/pkg/resolve"
)

// osFileSystem is the default FileSystem for a Load with no FS supplied: it reads
// straight through the operating system.
type osFileSystem struct{}

func (osFileSystem) ReadFile(path string) (string, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return string(b), true
}

func (osFileSystem) FileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func (osFileSystem) DirectoryExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// resolveFS adapts a bento FileSystem to the narrow FS the resolver reads
// through. A virtual FileSystem exposes no directory listing or symlink target,
// so ReadDir and RealPath degrade to the minimum the resolver tolerates: an
// empty listing and an identity realpath. That is enough for relative and
// extension-probed resolution; a virtual tree that needs node_modules directory
// scans supplies its own richer FS through the OS path instead.
type resolveFS struct{ fs FileSystem }

func (r resolveFS) Stat(path string) (resolve.FileInfo, error) {
	if r.fs.DirectoryExists(path) {
		return resolve.FileInfo{IsDir: true}, nil
	}
	if r.fs.FileExists(path) {
		return resolve.FileInfo{IsDir: false}, nil
	}
	return resolve.FileInfo{}, os.ErrNotExist
}

func (r resolveFS) ReadFile(path string) ([]byte, error) {
	s, ok := r.fs.ReadFile(path)
	if !ok {
		return nil, os.ErrNotExist
	}
	return []byte(s), nil
}

func (r resolveFS) ReadDir(string) ([]string, error) { return nil, os.ErrNotExist }

func (r resolveFS) RealPath(path string) (string, error) { return path, nil }

// fsForResolver picks the richest FS available: the real OS filesystem when the
// load reads through the disk, so the resolver gets directory listings and
// symlink canonicalization, or the thin virtual adapter otherwise.
func fsForResolver(fs FileSystem) resolve.FS {
	if _, ok := fs.(osFileSystem); ok {
		return resolve.OSFS{}
	}
	return resolveFS{fs: fs}
}

// loadHost is the adapter.Host that routes the checker's file reads and module
// resolution through bento's own FileSystem and resolver, so the type view and
// the run view describe the same module graph (04_frontend_typescript_go.md
// section 5).
type loadHost struct {
	fs       FileSystem
	resolver *resolve.Resolver
	cwd      string
}

func (h *loadHost) ReadFile(path string) (string, bool) { return h.fs.ReadFile(path) }
func (h *loadHost) FileExists(path string) bool         { return h.fs.FileExists(path) }
func (h *loadHost) DirectoryExists(path string) bool    { return h.fs.DirectoryExists(path) }
func (h *loadHost) GetCurrentDirectory() string         { return h.cwd }

// ResolveModule answers where an import points by delegating to bento's resolver,
// then classifies the result into the ImportKind the loader and interop generator
// bind on. ok is true only when the target is a source file the checker should
// pull into the program; a builtin, a go: import, or a non-source asset resolves
// but is not added as a typed input.
func (h *loadHost) ResolveModule(specifier, containingFile string) (string, adapter.ImportKind, bool) {
	parent := &resolve.Module{
		Path:   containingFile,
		Dir:    filepath.Dir(containingFile),
		Format: resolve.FormatESM,
	}
	res, err := h.resolver.Resolve(specifier, parent)
	if err != nil {
		return "", importKindFromSpecifier(specifier), false
	}
	kind := mapImportKind(specifier, res)
	return res.Path, kind, res.Kind == resolve.KindFile && isSourceFile(res.Path)
}

// importKindFromSpecifier classifies an unresolved specifier by shape alone, so a
// failed resolution still reports whether it looked relative or bare.
func importKindFromSpecifier(specifier string) adapter.ImportKind {
	if strings.HasPrefix(specifier, ".") || strings.HasPrefix(specifier, "/") {
		return adapter.ImportRelative
	}
	if strings.HasPrefix(specifier, "go:") {
		return adapter.ImportGo
	}
	if strings.HasPrefix(specifier, "node:") {
		return adapter.ImportNode
	}
	return adapter.ImportBare
}

// mapImportKind turns a resolved module into bento's import kind, reading the
// resolver's own classification first and falling back to specifier shape for the
// file-vs-file distinction between a relative path and a bare package.
func mapImportKind(specifier string, res resolve.Resolved) adapter.ImportKind {
	switch res.Kind {
	case resolve.KindBuiltin:
		return adapter.ImportNode
	case resolve.KindGo:
		return adapter.ImportGo
	}
	switch res.Format {
	case resolve.FormatJSON:
		return adapter.ImportJSON
	case resolve.FormatText, resolve.FormatBytes:
		return adapter.ImportAsset
	}
	if strings.HasPrefix(specifier, ".") || strings.HasPrefix(specifier, "/") {
		return adapter.ImportRelative
	}
	return adapter.ImportBare
}

// isSourceFile reports whether a resolved path is TypeScript or JavaScript the
// checker parses, as opposed to a JSON or asset import that resolves to a file
// but is not type-checked as a module.
func isSourceFile(path string) bool {
	switch {
	case strings.HasSuffix(path, ".ts"),
		strings.HasSuffix(path, ".tsx"),
		strings.HasSuffix(path, ".mts"),
		strings.HasSuffix(path, ".cts"),
		strings.HasSuffix(path, ".js"),
		strings.HasSuffix(path, ".jsx"),
		strings.HasSuffix(path, ".mjs"),
		strings.HasSuffix(path, ".cjs"):
		return true
	default:
		return false
	}
}
