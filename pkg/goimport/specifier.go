// Package goimport owns bento's Go interoperability: it turns a go: import
// specifier into a Go import path plus an optional module version, and it is the
// home of the .d.ts generation and the value marshaling that let a TypeScript
// program call a pure-Go library as a direct Go call (Spec 2075, document 16).
//
// This file is the front door, the go: specifier parser. Everything downstream, a
// module fetch, a generated declaration, an emitted Go import, starts from a parsed
// Specifier, so the parser is where the rules of section 3 are enforced once and
// for all.
package goimport

import (
	"errors"
	"strings"
	"unicode"
)

// Scheme is the import prefix that marks a specifier as a Go import. A specifier
// that starts with it is a Go import, full stop, which is what keeps the three
// existing resolution paths of 15_module_resolution.md untouched (section 3.1).
const Scheme = "go:"

// Specifier is a parsed go: import: the Go import path exactly as it would appear
// in a Go import statement, and an optional pinned module version (section 3.2).
type Specifier struct {
	// ImportPath is the Go import path, verbatim: a standard-library path like
	// "crypto/sha256" or a module path like
	// "github.com/klauspost/compress/zstd".
	ImportPath string
	// Version is the pinned module version, a semver tag or pseudo-version such as
	// "v1.17.9", or "" when the specifier did not pin one and the manifest decides
	// (section 4.3).
	Version string
}

// Pinned reports whether the specifier named an explicit module version.
func (s Specifier) Pinned() bool { return s.Version != "" }

// IsGoImport reports whether a raw specifier is a Go import, by its scheme alone.
func IsGoImport(specifier string) bool { return strings.HasPrefix(specifier, Scheme) }

// ParseSpecifier parses a full go: specifier into its import path and optional
// version. It is the single place the go: syntax is validated: the scheme must be
// present, the import path must be a syntactically valid Go import path (standard
// library paths included), and a pinned version, when present, must be a Go module
// version. A malformed specifier is an error rather than a best-effort guess,
// because a wrong Go import path is a bug the author wants caught at resolve time.
func ParseSpecifier(specifier string) (Specifier, error) {
	rest, ok := strings.CutPrefix(specifier, Scheme)
	if !ok {
		return Specifier{}, errors.New("not a go: import: " + specifier)
	}
	return parseBody(rest)
}

// ParseBody parses the remainder of a go: specifier after the scheme has already
// been stripped, which is the form the module resolver hands over once it has
// classified the specifier.
func ParseBody(rest string) (Specifier, error) { return parseBody(rest) }

func parseBody(rest string) (Specifier, error) {
	path, version, pinned := strings.Cut(rest, "@")
	if !validImportPath(path) {
		return Specifier{}, errors.New("invalid go: import path " + quote(path))
	}
	if pinned {
		if !validVersion(version) {
			return Specifier{}, errors.New("invalid go: module version " + quote(version))
		}
	}
	return Specifier{ImportPath: path, Version: version}, nil
}

// validImportPath reports whether p is a syntactically valid Go import path. It
// accepts a standard-library path with no dotted host (section 3 imports
// "crypto/sha256" the same way it imports a third-party module), so unlike a bare
// npm name there is no host requirement; the go: scheme has already removed the
// ambiguity that would otherwise need one (section 3.1). Each slash-separated
// element must be non-empty, must not be a relative "." or "..", and must contain
// only characters Go allows in an import path element.
func validImportPath(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || strings.HasSuffix(p, "/") {
		return false
	}
	for elem := range strings.SplitSeq(p, "/") {
		if !validElem(elem) {
			return false
		}
	}
	return true
}

// validElem reports whether one import-path element is well formed: non-empty, not
// a relative segment, not starting or ending with a dot, and built only from
// letters, digits, and the punctuation Go permits in a path element.
func validElem(elem string) bool {
	if elem == "" || elem == "." || elem == ".." {
		return false
	}
	if strings.HasPrefix(elem, ".") || strings.HasSuffix(elem, ".") {
		return false
	}
	for _, r := range elem {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		if strings.ContainsRune("-._~+", r) {
			continue
		}
		return false
	}
	return true
}

// validVersion reports whether v is a Go module version: a semver tag or a
// pseudo-version, both of which begin with "v", carry no whitespace, and hold no
// second "@". This is the same shape "go get pkg@v1.2.3" accepts (section 3.2); the
// exact semver grammar is Go's to enforce when it fetches, so this guards the shape
// rather than reimplementing the parser.
func validVersion(v string) bool {
	if !strings.HasPrefix(v, "v") || len(v) < 2 {
		return false
	}
	if strings.ContainsAny(v, "@ \t\n") {
		return false
	}
	return true
}

func quote(s string) string { return "\"" + s + "\"" }
