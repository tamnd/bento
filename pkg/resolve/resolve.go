// Package resolve turns an import specifier into a concrete module on disk.
//
// It is the single answer to the question "what file does this import mean".
// The frontend, the bundler, and the package manager all route through here, so
// bento finds the same file Node finds for the same import, with a few
// documented, DX-driven extensions (TypeScript-first extension order, a bento
// export condition, and dev-mode leniency).
//
// The resolver is pure: it reads through an injected FS and never touches global
// state, so it runs identically over a real disk, an in-memory tree, or a build
// overlay.
package resolve

import "sync"

// Kind is the broad category of a resolved module. It decides which loader the
// runtime hands the module to.
type Kind int

const (
	// KindFile is an ordinary file on the (possibly virtual) filesystem.
	KindFile Kind = iota
	// KindBuiltin is a Node core module such as fs or path.
	KindBuiltin
	// KindGo is a go: import handed off to the Go interop layer.
	KindGo
	// KindData is a data: URL carrying its body inline.
	KindData
	// KindExternal is a module the build marks external and does not inline.
	KindExternal
)

func (k Kind) String() string {
	switch k {
	case KindFile:
		return "file"
	case KindBuiltin:
		return "builtin"
	case KindGo:
		return "go"
	case KindData:
		return "data"
	case KindExternal:
		return "external"
	default:
		return "unknown"
	}
}

// Format is how the module's source should be parsed and linked. It is decided
// by extension and the nearest package.json "type", never by content sniffing.
type Format int

const (
	// FormatUnknown means the format has not been determined yet.
	FormatUnknown Format = iota
	// FormatESM is an ES module with import/export and possible top-level await.
	FormatESM
	// FormatCommonJS is a CommonJS module with require and module.exports.
	FormatCommonJS
	// FormatJSON is a JSON document exposed as a default export.
	FormatJSON
	// FormatText is a UTF-8 string default export.
	FormatText
	// FormatBytes is a Uint8Array default export.
	FormatBytes
	// FormatBuiltin is a bento-provided core module.
	FormatBuiltin
)

func (f Format) String() string {
	switch f {
	case FormatESM:
		return "esm"
	case FormatCommonJS:
		return "commonjs"
	case FormatJSON:
		return "json"
	case FormatText:
		return "text"
	case FormatBytes:
		return "bytes"
	case FormatBuiltin:
		return "builtin"
	default:
		return "unknown"
	}
}

// Resolved is the outcome of resolving one specifier. Path is the realpath for a
// file, the builtin name for a builtin, the import path for a go: import, or the
// full URL for a data: import, so it is always a stable cache identity.
type Resolved struct {
	Kind       Kind
	Format     Format
	Path       string
	Specifier  string
	Attributes map[string]string
	Conditions []string
	// GoVersion is the pinned module version of a go: import (KindGo), or "" when
	// the import did not pin one and go.mod decides. It is empty for every other
	// kind.
	GoVersion string
	// Body carries the decoded payload for data: URLs; it is nil otherwise.
	Body []byte
}

// Module is a resolved module acting as the parent of a nested import. Only the
// fields the resolver reads are kept here; the runtime carries the rest.
type Module struct {
	// Path is the realpath of the importing module, "" for a synthetic entry.
	Path string
	// Dir is the directory the import is resolved relative to.
	Dir string
	// Format is the importing module's format, which chooses ESM vs CJS rules.
	Format Format
}

// Builtins reports whether a bare or node: name is a bento core module. The node
// layer supplies the implementation; the resolver only needs the membership
// test to classify a specifier as a builtin.
type Builtins interface {
	Has(name string) bool
}

// Resolver answers Resolve. It is configured once and then shared, so dev and
// build differ only by the conditions and leniency passed in here.
type Resolver struct {
	fs         FS
	builtins   Builtins
	conditions []string
	extensions []string
	// cjsExtensions is extensions reordered for a CommonJS require context, with
	// ESM-only extensions (.mjs, .mts) moved to the end so require prefers the
	// .js family, as Node does.
	cjsExtensions []string
	// dev turns on extension search and directory index for ESM parents and the
	// TypeScript extension rewrite, matching how a dev server should feel.
	dev bool
	// preserveSymlinks skips realpath canonicalization when set.
	preserveSymlinks bool
	cache            *cache
	// pkgCache memoizes parsed package.json files by path, since the nearest-
	// package walk and directory resolution revisit the same files constantly.
	pkgCache sync.Map
}

// Options configures a Resolver.
type Options struct {
	// FS is the filesystem to read through. Required.
	FS FS
	// Builtins reports Node core module membership. Optional; nil means none.
	Builtins Builtins
	// Conditions is the export condition set. Empty uses DefaultImportConditions.
	Conditions []string
	// Extensions is the extension search order. Empty uses DefaultExtensions.
	Extensions []string
	// Dev enables extension search, directory index, and the TS rewrite for ESM.
	Dev bool
	// PreserveSymlinks skips symlink canonicalization.
	PreserveSymlinks bool
}

// DefaultExtensions is the extension search order. TypeScript comes first for a
// TypeScript-first runtime, JSON is late so a stray data file does not shadow
// code, and .node is last. This is a documented bento choice, not Node parity.
var DefaultExtensions = []string{".ts", ".tsx", ".mjs", ".cjs", ".js", ".jsx", ".json", ".node"}

// DefaultImportConditions is the condition set for an import context. "bento"
// comes first so a package can ship a bento-specific build.
var DefaultImportConditions = []string{"bento", "node", "import", "default"}

// DefaultRequireConditions is the condition set for a require context.
var DefaultRequireConditions = []string{"bento", "node", "require", "default"}

// New builds a Resolver from Options.
func New(opts Options) *Resolver {
	conditions := opts.Conditions
	if len(conditions) == 0 {
		conditions = DefaultImportConditions
	}
	extensions := opts.Extensions
	if len(extensions) == 0 {
		extensions = DefaultExtensions
	}
	return &Resolver{
		fs:               opts.FS,
		builtins:         opts.Builtins,
		conditions:       conditions,
		extensions:       extensions,
		cjsExtensions:    commonJSOrder(extensions),
		dev:              opts.Dev,
		preserveSymlinks: opts.PreserveSymlinks,
		cache:            newCache(),
	}
}

// esmOnlyExtensions are the extensions that only ever hold an ES module. Node's
// CommonJS loader never resolves a bare specifier or a directory index to one of
// these, so the require-context search order pushes them to the back.
var esmOnlyExtensions = map[string]bool{".mjs": true, ".mts": true}

// commonJSOrder returns the extension search order for a CommonJS (require)
// context: the configured order with the ESM-only extensions moved to the end,
// keeping every other extension's relative position. This makes require('./x')
// and require of a directory prefer index.js over index.mjs, matching Node,
// while the ESM order stays exactly as configured.
func commonJSOrder(exts []string) []string {
	head := make([]string, 0, len(exts))
	tail := make([]string, 0, len(exts))
	for _, ext := range exts {
		if esmOnlyExtensions[ext] {
			tail = append(tail, ext)
		} else {
			head = append(head, ext)
		}
	}
	return append(head, tail...)
}

// searchExtensions returns the extension search order for the importer's format.
// A CommonJS require prefers the .js family; an ESM import keeps the configured,
// TypeScript-first order.
func (r *Resolver) searchExtensions(esm bool) []string {
	if esm {
		return r.extensions
	}
	return r.cjsExtensions
}

// Resolve turns a specifier into a Resolved, using parent to choose relative-vs-
// bare rules and the CommonJS-vs-ESM resolution algorithm. A nil parent is an
// entry point resolved from the current directory as CommonJS.
func (r *Resolver) Resolve(specifier string, parent *Module) (Resolved, error) {
	class, rest := classify(specifier)

	switch class {
	case classBuiltin:
		return r.resolveBuiltin(rest, specifier)
	case classData:
		return resolveData(rest, specifier)
	case classGo:
		return resolveGo(rest, specifier)
	case classImports:
		return r.resolveImports(specifier, parent)
	case classUnsupported:
		return Resolved{}, &ResolveError{
			Code:      "ERR_UNSUPPORTED_ESM_URL_SCHEME",
			Specifier: specifier,
			Importer:  importerPath(parent),
			Message:   "unsupported URL scheme in " + specifier,
		}
	case classRelative, classAbsolute:
		return r.resolveFileSpecifier(specifier, parent)
	case classBare:
		return r.resolveBare(specifier, parent)
	default:
		return Resolved{}, &ResolveError{
			Code:      "ERR_MODULE_NOT_FOUND",
			Specifier: specifier,
			Importer:  importerPath(parent),
			Message:   "cannot resolve " + specifier,
		}
	}
}

// resolveBuiltin resolves a node: or bare builtin name, unless a local package
// of the same name shadows a bare form. node: prefixed names are always builtin.
func (r *Resolver) resolveBuiltin(name, specifier string) (Resolved, error) {
	if r.builtins != nil && r.builtins.Has(name) {
		return Resolved{
			Kind:      KindBuiltin,
			Format:    FormatBuiltin,
			Path:      name,
			Specifier: specifier,
		}, nil
	}
	return Resolved{}, &ResolveError{
		Code:      "ERR_MODULE_NOT_FOUND",
		Specifier: specifier,
		Message:   "no builtin module " + name,
	}
}

func importerPath(parent *Module) string {
	if parent == nil {
		return ""
	}
	return parent.Path
}
