package resolve

import (
	"path/filepath"
	"strings"
)

// resolveFileSpecifier resolves a relative or absolute specifier against its
// parent, choosing the CommonJS or ESM file algorithm by the parent's format.
func (r *Resolver) resolveFileSpecifier(specifier string, parent *Module) (Resolved, error) {
	dir := parentDir(parent)
	key := resolutionKey{dir: dir, specifier: specifier, conditions: r.conditionKey()}
	if hit, ok := r.cache.get(key); ok {
		return hit, nil
	}

	target := specifier
	if !filepath.IsAbs(target) {
		target = filepath.Join(dir, specifier)
	}
	target = filepath.Clean(target)

	esm := parent != nil && parent.Format == FormatESM
	path, err := r.resolveFile(target, specifier, parent, esm)
	if err != nil {
		return Resolved{}, err
	}

	real := r.realPath(path)
	resolved := Resolved{
		Kind:      KindFile,
		Format:    r.detectFormat(real),
		Path:      real,
		Specifier: specifier,
	}
	r.cache.put(key, resolved)
	return resolved, nil
}

// resolveFile turns an absolute target path into a concrete file. In CommonJS
// and dev mode it searches extensions and directory indexes; strict ESM tries
// the exact path first and only searches when leniency is enabled.
func (r *Resolver) resolveFile(target, specifier string, parent *Module, esm bool) (string, error) {
	allowSearch := !esm || r.dev
	allowIndex := !esm || r.dev

	if p, ok := r.resolveAsFile(target, allowSearch); ok {
		return p, nil
	}
	if allowIndex {
		if p, ok := r.resolveAsDirectory(target); ok {
			return p, nil
		}
	}
	return "", r.fileNotFound(target, specifier, parent)
}

// resolveAsFile tries a path as a file: exactly as given, then via the TS
// extension rewrite, then via extension search when allowed.
func (r *Resolver) resolveAsFile(target string, allowSearch bool) (string, bool) {
	if r.fileExists(target) {
		return target, true
	}
	if rewritten, ok := r.tsExtensionRewrite(target); ok {
		return rewritten, true
	}
	if allowSearch {
		for _, ext := range r.extensions {
			candidate := target + ext
			if r.fileExists(candidate) {
				return candidate, true
			}
		}
	}
	return "", false
}

// tsExtensionRewrite handles an import written as ./util.js from a .ts file that
// actually lives at ./util.ts on disk. It swaps a JavaScript extension for its
// TypeScript sibling and reports the first that exists.
func (r *Resolver) tsExtensionRewrite(target string) (string, bool) {
	ext := strings.ToLower(filepath.Ext(target))
	siblings, ok := tsSiblings[ext]
	if !ok {
		return "", false
	}
	base := strings.TrimSuffix(target, filepath.Ext(target))
	for _, tsExt := range siblings {
		candidate := base + tsExt
		if r.fileExists(candidate) {
			return candidate, true
		}
	}
	return "", false
}

// tsSiblings maps a JavaScript extension to the TypeScript extensions that may
// stand in for it, since TypeScript's nodenext mode imports .js but the source
// is .ts.
var tsSiblings = map[string][]string{
	".js":  {".ts", ".tsx"},
	".jsx": {".tsx"},
	".mjs": {".mts"},
	".cjs": {".cts"},
}

// resolveAsDirectory resolves a directory to its entry: the package.json main
// first, then an index file in extension order.
func (r *Resolver) resolveAsDirectory(dir string) (string, bool) {
	if !r.dirExists(dir) {
		return "", false
	}
	if pkg, err := r.readPackageJSON(filepath.Join(dir, "package.json")); err == nil && pkg != nil {
		if main := pkg.mainEntry(r.conditions); main != "" {
			target := filepath.Clean(filepath.Join(dir, main))
			if p, ok := r.resolveAsFile(target, true); ok {
				return p, true
			}
			// A main pointing at a directory recurses to its index.
			if r.dirExists(target) {
				if p, ok := r.resolveIndex(target); ok {
					return p, true
				}
			}
		}
	}
	return r.resolveIndex(dir)
}

// resolveIndex tries index.<ext> in a directory in extension order.
func (r *Resolver) resolveIndex(dir string) (string, bool) {
	for _, ext := range r.extensions {
		candidate := filepath.Join(dir, "index"+ext)
		if r.fileExists(candidate) {
			return candidate, true
		}
	}
	return "", false
}

// fileNotFound builds a not-found error, suggesting the TypeScript rewrite when
// an import of ./x.js would have resolved to ./x.ts.
func (r *Resolver) fileNotFound(target, specifier string, parent *Module) *ResolveError {
	err := notFound(specifier, parent, nil)
	ext := strings.ToLower(filepath.Ext(target))
	if siblings, ok := tsSiblings[ext]; ok {
		base := strings.TrimSuffix(target, filepath.Ext(target))
		for _, tsExt := range siblings {
			if r.fileExists(base + tsExt) {
				err.Message = "cannot find module " + specifier +
					"; did you mean the TypeScript source " + filepath.Base(base) + tsExt + "?"
				break
			}
		}
	}
	return err
}

// conditionKey joins the active conditions into a stable cache-key fragment.
func (r *Resolver) conditionKey() string { return strings.Join(r.conditions, ",") }

// parentDir returns the directory a specifier resolves against.
func parentDir(parent *Module) string {
	if parent == nil {
		return "."
	}
	if parent.Dir != "" {
		return parent.Dir
	}
	if parent.Path != "" {
		return filepath.Dir(parent.Path)
	}
	return "."
}
