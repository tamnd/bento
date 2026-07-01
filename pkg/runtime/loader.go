package runtime

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/tamnd/bento/pkg/frontend"
	"github.com/tamnd/bento/pkg/node"
	"github.com/tamnd/bento/pkg/resolve"
)

// builtinSet adapts a fixed list of core module names to resolve.Builtins. The
// runtime require path checks its native registry first, so this only classifies
// specifiers for the resolver; it never decides what a builtin loads.
type builtinSet map[string]bool

func (b builtinSet) Has(name string) bool { return b[name] }

// newResolver builds the resolver the running program uses. Dev leniency is on
// so `bento run app.ts` gets extension search, directory index, and the
// TypeScript extension rewrite, which is what a TypeScript-first runtime should
// feel like on disk.
func newResolver() *resolve.Resolver {
	set := builtinSet{}
	for _, name := range node.Builtins() {
		set[name] = true
	}
	return resolve.New(resolve.Options{
		FS:       resolve.OSFS{},
		Builtins: set,
		Dev:      true,
	})
}

// loadResult is the JSON envelope __bento_loadModule returns to the prelude. It
// follows the node layer's convention of a small tagged object across the
// bridge. OK false carries a Node error code the prelude rethrows.
type loadResult struct {
	OK      bool   `json:"ok"`
	Kind    string `json:"kind,omitempty"`
	Format  string `json:"format,omitempty"`
	Path    string `json:"path,omitempty"`
	Dir     string `json:"dir,omitempty"`
	Code    string `json:"code,omitempty"`
	Source  string `json:"source,omitempty"`
	ErrCode string `json:"errCode,omitempty"`
	Message string `json:"message,omitempty"`
}

// loadModule resolves a specifier through the resolver and, for on-disk modules,
// reads and transpiles the source so the prelude can wrap and run it. Builtins
// come back as a tag the prelude satisfies from its own native registry.
func (rt *Runtime) loadModule(spec, parentPath, parentFormat string) string {
	res, err := rt.resolver.Resolve(spec, parentModule(parentPath, parentFormat))
	if err != nil {
		return loadError(err, spec)
	}

	switch res.Kind {
	case resolve.KindBuiltin:
		return marshalLoad(loadResult{OK: true, Kind: "builtin", Path: res.Path})
	case resolve.KindFile:
		return rt.loadFile(res)
	case resolve.KindData:
		return rt.loadData(res)
	case resolve.KindGo:
		return marshalLoad(loadResult{
			OK:      false,
			ErrCode: "ERR_UNSUPPORTED_MODULE",
			Message: "go: imports are not enabled yet: " + spec,
		})
	default:
		return marshalLoad(loadResult{
			OK:      false,
			ErrCode: "ERR_MODULE_NOT_FOUND",
			Message: "cannot load " + spec,
		})
	}
}

// loadFile reads an on-disk module and prepares it for execution. JSON is handed
// over as raw source for the prelude to parse into an object; everything else is
// transpiled to CommonJS so the wrapper can run it.
func (rt *Runtime) loadFile(res resolve.Resolved) string {
	data, err := os.ReadFile(res.Path)
	if err != nil {
		return marshalLoad(loadResult{
			OK:      false,
			ErrCode: "ERR_MODULE_NOT_FOUND",
			Message: "cannot read " + res.Path + ": " + err.Error(),
		})
	}

	dir := filepath.Dir(res.Path)
	if res.Format == resolve.FormatJSON {
		return marshalLoad(loadResult{
			OK:     true,
			Kind:   "json",
			Path:   res.Path,
			Dir:    dir,
			Source: string(data),
		})
	}

	out, err := frontend.Transpile(string(data), frontend.Options{Filename: res.Path})
	if err != nil {
		return marshalLoad(loadResult{
			OK:      false,
			ErrCode: "ERR_TRANSPILE_FAILED",
			Message: err.Error(),
		})
	}
	return marshalLoad(loadResult{
		OK:     true,
		Kind:   "file",
		Format: res.Format.String(),
		Path:   res.Path,
		Dir:    dir,
		Code:   out.Code,
	})
}

// loadData turns a data: URL into a runnable module. The resolver already
// decoded the body, so this only routes JSON versus source.
func (rt *Runtime) loadData(res resolve.Resolved) string {
	if res.Format == resolve.FormatJSON {
		return marshalLoad(loadResult{
			OK:     true,
			Kind:   "json",
			Path:   res.Path,
			Source: string(res.Body),
		})
	}
	out, err := frontend.Transpile(string(res.Body), frontend.Options{Filename: "data:.js"})
	if err != nil {
		return marshalLoad(loadResult{
			OK:      false,
			ErrCode: "ERR_TRANSPILE_FAILED",
			Message: err.Error(),
		})
	}
	return marshalLoad(loadResult{
		OK:     true,
		Kind:   "file",
		Format: res.Format.String(),
		Path:   res.Path,
		Code:   out.Code,
	})
}

// parentModule builds the resolver's view of the importing module. An empty path
// means an entry point, resolved as CommonJS from the current directory.
func parentModule(path, format string) *resolve.Module {
	if path == "" {
		return nil
	}
	return &resolve.Module{
		Path:   path,
		Dir:    filepath.Dir(path),
		Format: parseFormat(format),
	}
}

// parseFormat maps the format string the prelude tracks back to a resolve.Format.
// Anything but esm is treated as CommonJS, which is the safe default for the run
// path since the entry and transpiled output are CommonJS.
func parseFormat(s string) resolve.Format {
	switch s {
	case "esm":
		return resolve.FormatESM
	case "json":
		return resolve.FormatJSON
	default:
		return resolve.FormatCommonJS
	}
}

// loadError maps a resolver error to the JSON envelope, preserving the Node
// error code when the resolver produced one so require() throws the right thing.
func loadError(err error, spec string) string {
	res := loadResult{OK: false, ErrCode: "ERR_MODULE_NOT_FOUND", Message: err.Error()}
	if re, ok := errors.AsType[*resolve.ResolveError](err); ok {
		res.ErrCode = re.Code
		res.Message = re.Error()
	}
	if res.Message == "" {
		res.Message = "cannot find module '" + spec + "'"
	}
	return marshalLoad(res)
}

func marshalLoad(r loadResult) string {
	b, err := json.Marshal(r)
	if err != nil {
		return `{"ok":false,"errCode":"ERR_INTERNAL","message":"load marshal failed"}`
	}
	return string(b)
}
