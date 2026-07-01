package resolve

import (
	"path/filepath"
	"slices"
	"strings"
)

// dirOf returns the directory containing a file path.
func dirOf(path string) string { return filepath.Dir(path) }

// hasCondition reports whether name is in the active condition set.
func hasCondition(conditions []string, name string) bool {
	return slices.Contains(conditions, name)
}

// mainEntry returns the legacy entry point for a package with no exports field.
// In an import context it prefers the bundler "module" field, then "browser"
// when browser conditions are active, then "main". An empty string means fall
// through to an index file.
func (p *packageJSON) mainEntry(conditions []string) string {
	if p.Exports != nil {
		// exports, when present, fully governs entry resolution, so the legacy
		// fields are dead. The caller resolves through exports instead.
		return ""
	}
	if hasCondition(conditions, "import") && p.Module != "" {
		return p.Module
	}
	if hasCondition(conditions, "browser") && p.Browser != "" {
		return p.Browser
	}
	return p.Main
}

// resolveExports resolves a subpath against a package's exports field. subpath
// is "." for the main entry or "./sub" for a subpath. It returns the target
// path relative to the package dir, or a not-exported error.
func (r *Resolver) resolveExports(pkg *packageJSON, subpath, specifier string) (string, error) {
	target, err := packageExportsResolve(pkg.Exports, subpath, r.conditions)
	if err != nil {
		return "", &ResolveError{
			Code:      "ERR_PACKAGE_PATH_NOT_EXPORTED",
			Specifier: specifier,
			Message:   "package " + pkg.Name + " does not export " + subpath,
		}
	}
	return target, nil
}

// packageExportsResolve implements PACKAGE_EXPORTS_RESOLVE over the ordered
// exports tree. It returns the resolved relative target for the subpath.
func packageExportsResolve(exports *exportsNode, subpath string, conditions []string) (string, error) {
	if exports == nil {
		return "", errNotExported
	}

	// A bare "." request against a string or conditions-only exports maps to the
	// main entry directly.
	if subpath == "." {
		if exports.kind == nodeString || exports.kind == nodeArray || isConditionsMap(exports) {
			return resolveTarget(exports, "", conditions)
		}
	}

	if exports.kind == nodeMap && isSubpathMap(exports) {
		return resolveSubpathMap(exports, subpath, conditions)
	}

	// Non-map (or conditions map) exports only define the main entry.
	if subpath == "." {
		return resolveTarget(exports, "", conditions)
	}
	return "", errNotExported
}

// resolveSubpathMap resolves subpath against a map whose keys are subpaths,
// matching an exact key before falling back to the longest matching pattern.
func resolveSubpathMap(exports *exportsNode, subpath string, conditions []string) (string, error) {
	// Exact match wins.
	for _, entry := range exports.entries {
		if entry.key == subpath {
			return resolveTarget(entry.value, "", conditions)
		}
	}
	// Longest literal-prefix single-star pattern wins.
	best := ""
	var bestTarget *exportsNode
	var bestRest string
	for _, entry := range exports.entries {
		prefix, suffix, ok := patternParts(entry.key)
		if !ok {
			continue
		}
		if rest, matched := matchPattern(subpath, prefix, suffix); matched {
			if len(prefix) >= len(best) {
				best = prefix
				bestTarget = entry.value
				bestRest = rest
			}
		}
	}
	if bestTarget != nil {
		return resolveTarget(bestTarget, bestRest, conditions)
	}
	return "", errNotExported
}

// resolveTarget implements RESOLVE_TARGET: a string is exact (with a single-star
// substitution), an array is tried in order, and a conditions map takes the
// first key that is "default" or an active condition, in author order.
func resolveTarget(target *exportsNode, star string, conditions []string) (string, error) {
	if target == nil {
		return "", errNotExported
	}
	switch target.kind {
	case nodeNull:
		return "", errNotExported
	case nodeString:
		return resolveStringTarget(target.str, star)
	case nodeArray:
		for _, item := range target.array {
			if res, err := resolveTarget(item, star, conditions); err == nil {
				return res, nil
			}
		}
		return "", errNotExported
	case nodeMap:
		for _, entry := range target.entries {
			if entry.key == "default" || hasCondition(conditions, entry.key) {
				if res, err := resolveTarget(entry.value, star, conditions); err == nil {
					return res, nil
				}
			}
		}
		return "", errNotExported
	default:
		return "", errNotExported
	}
}

// resolveStringTarget validates and completes a string target. Targets must be
// package-relative (start with "./") and get the single-star substitution.
func resolveStringTarget(target, star string) (string, error) {
	if !strings.HasPrefix(target, "./") {
		return "", errNotExported
	}
	if strings.Contains(target, "*") {
		return strings.Replace(target, "*", star, 1), nil
	}
	return target, nil
}

// patternParts splits a single-star key into its prefix and suffix, reporting
// whether it contains exactly one star.
func patternParts(key string) (prefix, suffix string, ok bool) {
	prefix, suffix, found := strings.Cut(key, "*")
	if !found || strings.Contains(suffix, "*") {
		return "", "", false
	}
	return prefix, suffix, true
}

// matchPattern reports whether subpath matches prefix+*+suffix and returns the
// text the star captured.
func matchPattern(subpath, prefix, suffix string) (string, bool) {
	if !strings.HasPrefix(subpath, prefix) || !strings.HasSuffix(subpath, suffix) {
		return "", false
	}
	middle := subpath[len(prefix) : len(subpath)-len(suffix)]
	return middle, true
}

// isSubpathMap reports whether every key in a map node is a subpath (starts with
// "."), which distinguishes a subpath map from a conditions map.
func isSubpathMap(node *exportsNode) bool {
	if node.kind != nodeMap || len(node.entries) == 0 {
		return false
	}
	for _, entry := range node.entries {
		if !strings.HasPrefix(entry.key, ".") {
			return false
		}
	}
	return true
}

// isConditionsMap reports whether a map node's keys are condition names rather
// than subpaths.
func isConditionsMap(node *exportsNode) bool {
	if node.kind != nodeMap || len(node.entries) == 0 {
		return false
	}
	for _, entry := range node.entries {
		if strings.HasPrefix(entry.key, ".") {
			return false
		}
	}
	return true
}

// errNotExported is the sentinel for a subpath that exports does not cover; the
// caller wraps it in a Node-shaped ResolveError with context.
var errNotExported = &packageError{"not exported"}

type packageError struct{ msg string }

func (e *packageError) Error() string { return e.msg }
