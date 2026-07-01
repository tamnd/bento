package resolve

import (
	"path/filepath"
	"strings"
)

// resolveImports resolves a "#"-prefixed subpath import against the nearest
// package's imports field. These are a package's private aliases: an import of
// "#internal/x" means whatever the governing package.json maps it to, which is
// either a package-relative file or a bare specifier routed through the normal
// node_modules walk. The conditions are the same set exports uses, so a package
// can ship a "bento" or "node" variant of an internal alias.
func (r *Resolver) resolveImports(specifier string, parent *Module) (Resolved, error) {
	// A bare "#" or a "#/..." is never a valid internal specifier.
	if specifier == "#" || strings.HasPrefix(specifier, "#/") {
		return Resolved{}, &ResolveError{
			Code:      "ERR_INVALID_MODULE_SPECIFIER",
			Specifier: specifier,
			Importer:  importerPath(parent),
			Message:   specifier + " is not a valid internal import specifier",
		}
	}

	pkg := r.nearestPackageJSON(parentDir(parent))
	if pkg == nil || pkg.Imports == nil {
		return Resolved{}, importNotDefined(specifier, parent)
	}

	target, ok := packageImportsResolve(pkg.Imports, specifier, r.conditions)
	if !ok {
		return Resolved{}, importNotDefined(specifier, parent)
	}

	// A package-relative target resolves as an exact file inside the package, with
	// no extension search, matching how exports targets resolve.
	if strings.HasPrefix(target, "./") {
		full := filepath.Clean(filepath.Join(pkg.dir, filepath.FromSlash(target)))
		if r.fileExists(full) {
			real := r.realPath(full)
			return Resolved{
				Kind:      KindFile,
				Format:    r.detectFormat(real),
				Path:      real,
				Specifier: specifier,
			}, nil
		}
		return Resolved{}, notFound(specifier, parent, nil)
	}

	// Any other target is a bare specifier resolved through the node_modules walk,
	// with the importing module as the parent so the walk starts in the right
	// place.
	return r.resolveBare(target, parent)
}

// packageImportsResolve matches a "#" specifier against an imports map and
// returns the resolved target string with any single-star capture substituted.
// An exact key wins over a pattern, and among patterns the longest literal
// prefix wins, mirroring exports subpath resolution.
func packageImportsResolve(imports *exportsNode, specifier string, conditions []string) (string, bool) {
	if imports == nil || imports.kind != nodeMap {
		return "", false
	}

	// Exact key wins.
	for _, entry := range imports.entries {
		if entry.key == specifier {
			return resolveImportTarget(entry.value, "", conditions)
		}
	}

	// Longest literal-prefix single-star pattern wins.
	best := ""
	var bestTarget *exportsNode
	var bestRest string
	matched := false
	for _, entry := range imports.entries {
		prefix, suffix, ok := patternParts(entry.key)
		if !ok {
			continue
		}
		if rest, m := matchPattern(specifier, prefix, suffix); m {
			if !matched || len(prefix) >= len(best) {
				best = prefix
				bestTarget = entry.value
				bestRest = rest
				matched = true
			}
		}
	}
	if matched {
		return resolveImportTarget(bestTarget, bestRest, conditions)
	}
	return "", false
}

// resolveImportTarget resolves an imports target to a specifier string. It is
// like exports' resolveTarget but a string target may be a bare specifier, not
// only a package-relative "./" path, because an internal import can alias to a
// package. A conditions object takes the first key that is "default" or an
// active condition, in author order.
func resolveImportTarget(target *exportsNode, star string, conditions []string) (string, bool) {
	if target == nil {
		return "", false
	}
	switch target.kind {
	case nodeNull:
		return "", false
	case nodeString:
		// A star in the target is substituted with the pattern capture; with no
		// star this is a no-op, so the replace is unconditional.
		return strings.Replace(target.str, "*", star, 1), true
	case nodeArray:
		for _, item := range target.array {
			if s, ok := resolveImportTarget(item, star, conditions); ok {
				return s, true
			}
		}
		return "", false
	case nodeMap:
		for _, entry := range target.entries {
			if entry.key == "default" || hasCondition(conditions, entry.key) {
				if s, ok := resolveImportTarget(entry.value, star, conditions); ok {
					return s, true
				}
			}
		}
		return "", false
	default:
		return "", false
	}
}

// importNotDefined is the error for a "#" specifier no imports map covers.
func importNotDefined(specifier string, parent *Module) *ResolveError {
	return &ResolveError{
		Code:      "ERR_PACKAGE_IMPORT_NOT_DEFINED",
		Specifier: specifier,
		Importer:  importerPath(parent),
		Message:   "no imports field entry defines " + specifier,
	}
}
