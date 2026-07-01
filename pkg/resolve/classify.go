package resolve

import "strings"

// class is the category a specifier falls into before any filesystem access.
type class int

const (
	classBare class = iota
	classRelative
	classAbsolute
	classBuiltin
	classData
	classGo
	classImports
	classUnsupported
)

// classify sorts a specifier into a class purely by its shape, with no disk
// access. It returns the class and the remainder after any scheme prefix.
//
// Only a leading ./, ../, or / makes a specifier relative or absolute; a slash
// inside the specifier (lodash/fp) does not, so bare package subpaths classify
// as bare.
func classify(specifier string) (class, string) {
	if specifier == "" {
		return classUnsupported, specifier
	}

	if rest, ok := schemePrefix(specifier, "node:"); ok {
		return classBuiltin, rest
	}
	if rest, ok := schemePrefix(specifier, "data:"); ok {
		return classData, rest
	}
	if rest, ok := schemePrefix(specifier, "go:"); ok {
		return classGo, rest
	}
	if rest, ok := schemePrefix(specifier, "file:"); ok {
		return classAbsolute, fileURLToPath(rest)
	}
	if hasURLScheme(specifier) {
		return classUnsupported, specifier
	}

	switch {
	case specifier == "." || specifier == "..":
		return classRelative, specifier
	case strings.HasPrefix(specifier, "./") || strings.HasPrefix(specifier, "../"):
		return classRelative, specifier
	case strings.HasPrefix(specifier, "/"):
		return classAbsolute, specifier
	case strings.HasPrefix(specifier, "#"):
		return classImports, specifier
	default:
		return classBare, specifier
	}
}

// schemePrefix returns the remainder after a scheme prefix and whether it
// matched, case-sensitively since import schemes are lowercase.
func schemePrefix(specifier, scheme string) (string, bool) {
	if strings.HasPrefix(specifier, scheme) {
		return specifier[len(scheme):], true
	}
	return "", false
}

// hasURLScheme reports whether a specifier begins with an unrecognized URL
// scheme like http: or ftp:, which the resolver rejects by default. It looks for
// a scheme of letters/digits followed by a colon and a slash, to avoid mistaking
// a Windows drive letter (C:\) for a scheme.
func hasURLScheme(specifier string) bool {
	colon := strings.IndexByte(specifier, ':')
	if colon <= 1 {
		return false
	}
	for i := range colon {
		c := specifier[i]
		isLetter := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
		isDigit := c >= '0' && c <= '9'
		if !isLetter && !isDigit && c != '+' && c != '-' && c != '.' {
			return false
		}
	}
	rest := specifier[colon+1:]
	return strings.HasPrefix(rest, "//") || strings.HasPrefix(rest, "/")
}

// fileURLToPath converts the body of a file: URL to a filesystem path. It strips
// a leading // and localhost authority and turns a Windows /C:/x form into C:/x.
func fileURLToPath(rest string) string {
	rest = strings.TrimPrefix(rest, "//")
	rest = strings.TrimPrefix(rest, "localhost")
	if len(rest) >= 3 && rest[0] == '/' && rest[2] == ':' {
		return rest[1:]
	}
	return rest
}
