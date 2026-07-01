package resolve

import (
	"path/filepath"
	"strings"
)

// detectFormat decides how a resolved file should be parsed. Extension wins for
// the unambiguous cases; the ambiguous .ts/.js family defers to the nearest
// package.json "type". Content is never sniffed.
func (r *Resolver) detectFormat(path string) Format {
	switch strings.ToLower(filepath.Ext(path)) {
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
	pkg := r.nearestPackageJSON(filepath.Dir(path))
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
		if pkg, err := r.readPackageJSON(filepath.Join(dir, "package.json")); err == nil && pkg != nil {
			return pkg
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil
		}
		dir = parent
	}
}
