package resolve

import (
	"strings"

	"github.com/tamnd/bento/pkg/cpath"
)

// FormatOf reports how a file on disk should be parsed, so a caller holding a
// path rather than a Resolved can ask the same question the resolver answers
// for itself. A loader needs this to describe the file an import is written in:
// a require in a .js file and an import in a .mjs file are different questions
// and get different answers, and a loader that guesses one for both asks the
// wrong one half the time.
func (r *Resolver) FormatOf(path string) Format { return r.detectFormat(path) }

// detectFormat decides how a resolved file should be parsed. Extension wins for
// the unambiguous cases; the ambiguous .ts/.js family defers to the nearest
// package.json "type". Content is never sniffed.
func (r *Resolver) detectFormat(path string) Format {
	switch strings.ToLower(cpath.Ext(path)) {
	case ".mjs", ".mts":
		return FormatESM
	case ".cjs", ".cts":
		return FormatCommonJS
	case ".json":
		return FormatJSON
	case ".node":
		return FormatCommonJS
	case ".ts", ".tsx":
		return r.ambiguousFormat(path, FormatESM)
	case ".js", ".jsx":
		return r.ambiguousFormat(path, FormatCommonJS)
	default:
		return FormatCommonJS
	}
}

// ambiguousFormat resolves a .ts/.js file's format from the nearest package.json
// "type". A bare .ts with no governing type defaults to ESM (a documented,
// DX-correct divergence from Node); a bare .js defaults to CommonJS like Node.
func (r *Resolver) ambiguousFormat(path string, fallback Format) Format {
	pkg := r.nearestPackageJSON(cpath.Dir(path))
	if pkg == nil {
		return fallback
	}
	switch pkg.Type {
	case "module":
		return FormatESM
	case "commonjs":
		return FormatCommonJS
	default:
		return fallback
	}
}

// nearestPackageJSON walks up from dir to the filesystem root and returns the
// first package.json it can parse, or nil if none governs the file.
func (r *Resolver) nearestPackageJSON(dir string) *packageJSON {
	for {
		if pkg, err := r.readPackageJSON(cpath.Join(dir, "package.json")); err == nil && pkg != nil {
			return pkg
		}
		parent := cpath.Dir(dir)
		if parent == dir {
			return nil
		}
		dir = parent
	}
}
