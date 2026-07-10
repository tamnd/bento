package lower

import (
	"fmt"
	"strings"
	"unicode"
)

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

// mangleIdent maps a TypeScript identifier to a legal Go identifier spelling.
// A name that is already legal passes through untouched, which keeps today's
// output byte-identical for every program that lowered before mangling
// existed. Otherwise the name is escaped rune by rune with a small mnemonic
// table: `$` becomes `D_` (dollar, the overwhelmingly common case, test262
// spells $DONE and $DONOTEVALUATE everywhere), and any other rune outside
// Go's letter/digit/underscore set becomes `U` plus its uppercase hex code
// point plus `_`, so a name carrying U+2118 spells it U2118_. A name left
// starting with a digit takes an `N_` prefix. The escape decodes back
// unambiguously, so two different names can never mangle to the same output;
// the one possible clash, a mangled spelling against the same name written
// verbatim elsewhere in the module, is declined by the module-level guard in
// RenderProgram rather than resolved with a context-dependent rename, which
// would break the pure-function rule. Only the empty string reports ok=false.
func mangleIdent(name string) (string, bool) {
	if name == "" {
		return "", false
	}
	// The lone underscore is spelled like a legal Go identifier but names the
	// blank identifier, which discards its value and cannot be read. A JavaScript
	// binding or property named _ is an ordinary readable name (var _ = 1; use(_)),
	// so it must not pass through to Go's _. Escape it through the same U + hex code
	// point form any other unusable rune takes, so it decodes back unambiguously
	// and a declaration and every reference agree on U5F_.
	if name == "_" {
		return "U5F_", true
	}
	if isGoIdent(name) {
		return name, true
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r == '$':
			b.WriteString("D_")
		case r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		default:
			fmt.Fprintf(&b, "U%X_", r)
		}
	}
	s := b.String()
	if r := []rune(s)[0]; unicode.IsDigit(r) {
		s = "N_" + s
	}
	return s, true
}

// exportedField turns a TypeScript property name into an exported Go struct
// field name: the name is mangled into a legal identifier if it needs it, then
// the first letter is uppercased and the rest is preserved, so "x" becomes
// "X", "count" becomes "Count" (section 12), and "$DONE" becomes "D_DONE". An
// exported name can never collide with a Go keyword, because every keyword is
// lowercase, so the keyword rule does not fire here. Only the empty string
// reports ok=false.
func exportedField(name string) (string, bool) {
	s, ok := mangleIdent(name)
	if !ok {
		return "", false
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes), true
}

// privateGoName maps a JavaScript private member name (#x, #inc) to its Go
// spelling. The leading # escapes to a p_ prefix, the mnemonic being private,
// applied before the case rule, and the rest mangles the way any member name
// does but without exportedField's uppercasing, because a private name never
// participates in the class's public Go shape. The mapping is a pure function of
// the name, so #m and m coexist on one class the way JavaScript allows, their Go
// spellings differing (p_m and M). A name that does not start with # or whose
// remainder is empty reports ok=false.
func privateGoName(name string) (string, bool) {
	inner, ok := strings.CutPrefix(name, "#")
	if !ok || inner == "" {
		return "", false
	}
	s, ok := mangleIdent(inner)
	if !ok {
		return "", false
	}
	return "p_" + s, true
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

// emitterPkgs is the set of package identifiers the emitted code can qualify
// through: everything requireImport is ever called with. A local spelling one
// of these would shadow the package inside its function, and test262's own
// harness hits it head on with parameters named value, so localName reserves
// them the same way it reserves keywords. The set is fixed rather than
// computed from the imports a module actually uses, because localName is a
// pure function of the name and the rename must not depend on emission order.
var emitterPkgs = map[string]bool{
	"value": true, "math": true, "big": true, "unsafe": true, "bridge": true,
}

// localName maps a TypeScript parameter or local identifier to its Go spelling.
// A local stays unexported, so a legal name is preserved verbatim except when
// it spells a Go keyword or one of the emitter's package identifiers, which
// takes a trailing underscore; a name that is not a legal Go identifier
// mangles through the same escape exportedField uses, minus the uppercasing,
// so "$DONE" declares and reads as D_DONE everywhere. A mangled name can never
// spell a keyword (it always carries an uppercase rune or an underscore from
// the escape), so the two rules do not interact. The mapping is a pure
// function of the name, so a parameter and every reference to it mangle
// identically without threading any shared table.
func localName(name string) (string, bool) {
	s, ok := mangleIdent(name)
	if !ok {
		return "", false
	}
	if goKeywords[s] || emitterPkgs[s] {
		return s + "_", true
	}
	return s, true
}
