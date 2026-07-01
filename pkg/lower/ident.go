package lower

import "unicode"

// This file implements the name-mangling rules of 05_type_lowering.md section
// 29. TypeScript identifiers are a superset of Go identifiers, so a property or
// type name from the source may not be a legal Go identifier, and the mapping
// has to be fixed and reproducible: the same TypeScript name always mangles to
// the same Go name, independent of emission order, so separately compiled
// modules agree on struct field names and layout.

// isGoIdent reports whether s is spelled as a legal Go identifier: a non-empty
// run of letters, digits, and underscores that does not start with a digit. It
// does not reject keywords, because a keyword is a legal identifier spelling
// that the caller resolves with a trailing underscore.
func isGoIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_':
		case unicode.IsLetter(r):
		case unicode.IsDigit(r):
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// exportedField turns a TypeScript property name into an exported Go struct
// field name: the first letter is uppercased and the rest is preserved, so "x"
// becomes "X" and "count" becomes "Count" (section 12). An exported name can
// never collide with a Go keyword, because every keyword is lowercase, so the
// keyword rule does not fire here; it is kept in mangleUnexported for names that
// must stay lowercase. A name that is not a legal Go identifier is not a struct
// field at all: it belongs in the object's symbol-property side table, and this
// function reports ok=false so the caller routes it there rather than inventing
// an unsound field.
func exportedField(name string) (string, bool) {
	if !isGoIdent(name) {
		return "", false
	}
	runes := []rune(name)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes), true
}

// goKeywords is the set of reserved words a local name cannot spell in Go. A
// TypeScript parameter or local named "type" or "range" is a legal identifier
// there but not here, so localName appends an underscore to keep the same name
// unexported without colliding with the grammar.
var goKeywords = map[string]bool{
	"break": true, "case": true, "chan": true, "const": true, "continue": true,
	"default": true, "defer": true, "else": true, "fallthrough": true, "for": true,
	"func": true, "go": true, "goto": true, "if": true, "import": true,
	"interface": true, "map": true, "package": true, "range": true, "return": true,
	"select": true, "struct": true, "switch": true, "type": true, "var": true,
}

// localName maps a TypeScript parameter or local identifier to its Go spelling.
// A local stays unexported, so the name is preserved verbatim except when it
// spells a Go keyword, which takes a trailing underscore. A name that is not a
// legal Go identifier (a string-keyed binding, say) reports ok=false so the
// caller hands the construct back rather than emitting an unsound reference. The
// mapping is a pure function of the name, so a parameter and every reference to
// it mangle identically without threading any shared table.
func localName(name string) (string, bool) {
	if !isGoIdent(name) {
		return "", false
	}
	if goKeywords[name] {
		return name + "_", true
	}
	return name, true
}
