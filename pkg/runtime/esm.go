package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
	"github.com/tamnd/bento/pkg/resolve"
)

// reqPrefix tags a canonical module name whose source is a re-export shim over a
// CommonJS require. Builtins, JSON, data: URLs, and plain CommonJS files all
// reach the ES module world this way: they load once through require and the
// shim re-exports the result, so an ES module and a require see one instance.
const reqPrefix = "bento:req:"

// entryModuleName is the name the engine gives the entry module passed to
// EvalModule. The quickjs binding hardcodes it, so the runtime recognizes it to
// anchor the entry's own imports at the real entry directory.
const entryModuleName = "<eval>"

// moduleHost routes the engine's native ES module resolution through the bento
// resolver. It is installed once and used only when a program runs as a real
// module, which happens when the entry (or one of its imports) needs top-level
// await or otherwise takes the ES module path.
type moduleHost struct {
	rt *Runtime
}

// Normalize resolves a specifier as written in referrer to a canonical module
// name. An ES module file keeps its realpath as its name; everything the ES
// world reaches through CommonJS interop (builtins, JSON, data:, plain CJS) gets
// the req: tag so Load knows to build a re-export shim.
func (h moduleHost) Normalize(referrer, specifier string) (string, error) {
	// The engine also normalizes the entry's own name; return it unchanged.
	if referrer == "" {
		return specifier, nil
	}
	parent := esmParent(referrer)
	// Imports written in the entry carry the engine's fixed entry name, so anchor
	// them at the real entry directory the runtime recorded.
	if referrer == entryModuleName && h.rt.esmEntry != "" {
		parent = &resolve.Module{Path: h.rt.esmEntry, Dir: filepath.Dir(h.rt.esmEntry), Format: resolve.FormatESM}
	}
	res, err := h.rt.resolver.Resolve(specifier, parent)
	if err != nil {
		return "", err
	}
	switch res.Kind {
	case resolve.KindBuiltin:
		return reqPrefix + res.Path, nil
	case resolve.KindData:
		return reqPrefix + res.Specifier, nil
	case resolve.KindFile:
		if res.Format == resolve.FormatESM {
			return res.Path, nil
		}
		return reqPrefix + res.Path, nil
	case resolve.KindGo:
		return "", fmt.Errorf("go: imports are not enabled yet: %s", specifier)
	default:
		return "", fmt.Errorf("cannot import %s", specifier)
	}
}

// Load returns ES module source for a canonical name. A req: name is turned into
// a re-export shim built in JavaScript, where the CommonJS module is required and
// its keys become named exports. A plain name is an ES module file read from disk
// and transpiled with its import, export, and top-level await intact.
func (h moduleHost) Load(name string) (string, error) {
	if spec, ok := strings.CutPrefix(name, reqPrefix); ok {
		src, err := h.rt.eng.Call("__bento_esmShim", spec)
		if err != nil {
			return "", err
		}
		code, _ := src.(string)
		return code, nil
	}

	data, err := os.ReadFile(name)
	if err != nil {
		return "", fmt.Errorf("bento: read module %s: %w", name, err)
	}
	res, err := frontend.TranspileESM(string(data), frontend.Options{Filename: name})
	if err != nil {
		return "", err
	}
	return res.Code, nil
}

// esmParent builds the resolver's view of the importing module from a canonical
// name. A req: shim carries the real path after its tag; a plain name is the
// ES module's own path.
func esmParent(referrer string) *resolve.Module {
	if spec, ok := strings.CutPrefix(referrer, reqPrefix); ok {
		return &resolve.Module{Path: spec, Dir: filepath.Dir(spec), Format: resolve.FormatCommonJS}
	}
	return &resolve.Module{Path: referrer, Dir: filepath.Dir(referrer), Format: resolve.FormatESM}
}

// runESMEntry runs an entry program as a native ES module, the path taken when
// the source uses top-level await. The entry's absolute path is its module name,
// so its own imports resolve against its directory. The event loop then pumps
// timers and the top-level-await continuations until the program settles.
func (rt *Runtime) runESMEntry(path, source string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	res, err := frontend.TranspileESM(source, frontend.Options{Filename: abs})
	if err != nil {
		return err
	}
	rt.esmEntry = abs
	if err := rt.eng.EvalModule(abs, res.Code); err != nil {
		return err
	}
	return rt.loop.Run()
}
