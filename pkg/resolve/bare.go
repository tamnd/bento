package resolve

import (
	"path/filepath"
	"strings"
)

// resolveBare resolves a bare specifier (a package name, optionally with a
// subpath) by walking node_modules from the importer up to the filesystem root.
func (r *Resolver) resolveBare(specifier string, parent *Module) (Resolved, error) {
	// A bare name may also be an unshadowed builtin (import fs from "fs"). Only
	// fall back to the builtin when no local package shadows it, which the walk
	// below discovers naturally, so try the walk first and the builtin after.
	dir := parentDir(parent)
	key := resolutionKey{dir: dir, specifier: specifier, conditions: r.conditionKey()}
	if hit, ok := r.cache.get(key); ok {
		return hit, nil
	}

	name, subpath := parseBare(specifier)
	if name == "" {
		return Resolved{}, notFound(specifier, parent, nil)
	}

	searched := []string{}
	for _, nm := range nodeModulesDirs(dir) {
		pkgDir := filepath.Join(nm, name)
		searched = append(searched, nm)
		if !r.dirExists(pkgDir) {
			continue
		}
		resolved, err := r.loadFromPackageDir(pkgDir, subpath, specifier)
		if err != nil {
			return Resolved{}, err
		}
		if resolved.Path != "" {
			r.cache.put(key, resolved)
			return resolved, nil
		}
	}

	// No local package shadowed it, so an unshadowed builtin name resolves to
	// the builtin (import fs from "fs").
	if subpath == "." && r.builtins != nil && r.builtins.Has(name) {
		return r.resolveBuiltin(name, specifier)
	}

	return Resolved{}, notFound(specifier, parent, searched)
}

// loadFromPackageDir resolves a subpath inside a found package directory,
// through exports when present and the legacy fields otherwise.
func (r *Resolver) loadFromPackageDir(pkgDir, subpath, specifier string) (Resolved, error) {
	pkg, err := r.readPackageJSON(filepath.Join(pkgDir, "package.json"))
	if err != nil {
		return Resolved{}, err
	}

	real := func(path string) Resolved {
		canon := r.realPath(path)
		return Resolved{
			Kind:      KindFile,
			Format:    r.detectFormat(canon),
			Path:      canon,
			Specifier: specifier,
		}
	}

	if pkg != nil && pkg.Exports != nil {
		target, err := r.resolveExports(pkg, subpath, specifier)
		if err != nil {
			return Resolved{}, err
		}
		full := filepath.Clean(filepath.Join(pkgDir, target))
		// exports targets are exact: no extension search, no directory index.
		if r.fileExists(full) {
			return real(full), nil
		}
		return Resolved{}, notFound(specifier, nil, nil)
	}

	// No exports: main entry through legacy fields, subpaths as plain files.
	if subpath == "." {
		if pkg != nil {
			if main := pkg.mainEntry(r.conditions); main != "" {
				full := filepath.Clean(filepath.Join(pkgDir, main))
				if p, ok := r.resolveAsFile(full, true); ok {
					return real(p), nil
				}
				if r.dirExists(full) {
					if p, ok := r.resolveIndex(full); ok {
						return real(p), nil
					}
				}
			}
		}
		if p, ok := r.resolveIndex(pkgDir); ok {
			return real(p), nil
		}
		return Resolved{}, notFound(specifier, nil, nil)
	}

	full := filepath.Clean(filepath.Join(pkgDir, subpath))
	if p, ok := r.resolveAsFile(full, true); ok {
		return real(p), nil
	}
	if r.dirExists(full) {
		if p, ok := r.resolveAsDirectory(full); ok {
			return real(p), nil
		}
	}
	return Resolved{}, notFound(specifier, nil, nil)
}

// parseBare splits a bare specifier into a package name and a subpath. The
// subpath is "." for the package root, or "./sub" for a subpath. A scoped name
// keeps its @scope/name together.
func parseBare(specifier string) (name, subpath string) {
	if specifier == "" {
		return "", "."
	}
	parts := strings.Split(specifier, "/")
	if strings.HasPrefix(specifier, "@") {
		if len(parts) < 2 {
			return "", "."
		}
		name = parts[0] + "/" + parts[1]
		if len(parts) == 2 {
			return name, "."
		}
		return name, "./" + strings.Join(parts[2:], "/")
	}
	name = parts[0]
	if len(parts) == 1 {
		return name, "."
	}
	return name, "./" + strings.Join(parts[1:], "/")
}

// nodeModulesDirs returns the node_modules directories to search, from the
// importer's directory up to the filesystem root.
func nodeModulesDirs(start string) []string {
	var dirs []string
	dir := start
	if dir == "" || dir == "." {
		abs, err := filepath.Abs(".")
		if err == nil {
			dir = abs
		}
	}
	for {
		// Do not nest node_modules/node_modules while walking up.
		if filepath.Base(dir) != "node_modules" {
			dirs = append(dirs, filepath.Join(dir, "node_modules"))
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return dirs
}
