package lower

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
	"github.com/tamnd/bento/pkg/value"
)

// This file lowers a RegExp value: a regexp literal like /abc/g in a value
// position, and a new RegExp(pattern, flags) call over a constant pattern. Both
// lower to a *value.RegExp built by value.NewRegExpLiteral, the runtime object that
// hosts the match on Go's regexp package. A pattern lowers only when it is provably
// faithful on RE2, which value.TranslateRegExpSource decides; a pattern RE2 cannot
// host with ECMAScript semantics (a backreference, a lookaround) or a construct a
// later slice owns (a named group, the u or v flag) hands back with the translator's
// own reason, so the compiler reports the exact ceiling.

// regExpType reports whether a checker type is the global RegExp interface. Like the
// DataView and typed-array checks it is a shape test on the type's declaring symbol:
// an object type, not an array, whose symbol is named RegExp. That is enough to tell
// a RegExp receiver from a plain object, whose class the compiler would otherwise
// intern as struct fields.
func (r *Renderer) regExpType(t frontend.Type) bool {
	if t.Flags&frontend.TypeObject == 0 {
		return false
	}
	if _, isArray := r.prog.ElementType(t); isArray {
		return false
	}
	sym, ok := r.prog.TypeSymbol(t)
	return ok && sym.Name == "RegExp"
}

// isRegExp reports whether the node's static type is a RegExp, the receiver test the
// class-tag and, in later slices, the exec and test paths use to route to the RegExp
// machinery.
func (r *Renderer) isRegExp(n frontend.Node) bool {
	return r.regExpType(r.prog.TypeAt(n))
}

// regExpLiteralParts reports whether n is a regexp literal and returns its pattern
// body and flag text. A regexp literal arrives as the catch-all NodeUnknown whose
// source reads /body/flags, the same shape regexLiteralArg recognizes for the
// replace path; requiring the checker's type to be RegExp keeps a stray NodeUnknown
// that merely starts with a slash from matching here.
func (r *Renderer) regExpLiteralParts(n frontend.Node) (pattern, flags string, ok bool) {
	if n.Kind() != frontend.NodeUnknown {
		return "", "", false
	}
	if !r.regExpType(r.prog.TypeAt(n)) {
		return "", "", false
	}
	return splitRegExpLiteral(r.prog.Text(n))
}

// splitRegExpLiteral splits a regexp literal's source text /body/flags into its
// body and flag run. The closing slash is the last slash in the text, sound because
// a body that would carry an escaped slash reads its backslash first, so the split
// point is unambiguous for the shapes that reach here. It reports ok=false for a text
// that is not a slash-delimited literal or whose flag run is not all ASCII letters.
func splitRegExpLiteral(text string) (pattern, flags string, ok bool) {
	text = strings.TrimSpace(text)
	if len(text) < 2 || text[0] != '/' {
		return "", "", false
	}
	close := strings.LastIndexByte(text, '/')
	if close == 0 {
		return "", "", false
	}
	pattern = text[1:close]
	flags = text[close+1:]
	if !allASCIILetters(flags) {
		return "", "", false
	}
	return pattern, flags, true
}

// lowerRegExpLiteral lowers a regexp literal in a value position to the *value.RegExp
// it constructs. A literal used as a replace or split pattern is intercepted earlier,
// on the string-method path, so this fires only where the regexp itself is the value:
// a binding initializer, an argument, a return.
func (r *Renderer) lowerRegExpLiteral(n frontend.Node) (ast.Expr, error) {
	pattern, flags, ok := r.regExpLiteralParts(n)
	if !ok {
		return nil, &NotYetLowerable{Reason: "a regexp literal in this position is a later slice"}
	}
	return r.buildRegExp(pattern, flags)
}

// newRegExp lowers a new RegExp(pattern, flags) call. Only a constant pattern lowers:
// the pattern must be a string literal so the compiler can prove its faithfulness on
// RE2 the same way it does for a literal, and the optional flags must be a string
// literal too. A pattern built from a runtime string, or a RegExp copied from another
// RegExp, cannot be checked at compile time and hands back. The result is the same
// value.NewRegExpLiteral a literal lowers to, since the two build the identical object.
func (r *Renderer) newRegExp(args []frontend.Node) (ast.Expr, error) {
	if len(args) == 0 || len(args) > 2 {
		return nil, &NotYetLowerable{Reason: "new RegExp with this argument count is a later slice"}
	}
	pattern, ok := r.stringLiteralKey(args[0])
	if !ok {
		return nil, &NotYetLowerable{Reason: "new RegExp with a non-literal pattern is a later slice"}
	}
	flags := ""
	if len(args) == 2 {
		f, ok := r.stringLiteralKey(args[1])
		if !ok {
			return nil, &NotYetLowerable{Reason: "new RegExp with non-literal flags is a later slice"}
		}
		flags = f
	}
	return r.buildRegExp(pattern, flags)
}

// regExpAccessor maps a RegExp accessor name to the value.RegExp method that reads
// it, or reports ok=false for a name that is not a flag getter. The .source and
// .flags getters read the pattern text and the canonical flag run; the rest are the
// single-flag booleans, one per flag the specification defines, spelled with the
// property name the source uses (.unicodeSets for the v flag, .hasIndices for d).
func regExpAccessor(prop string) (method string, ok bool) {
	switch prop {
	case "source":
		return "Source", true
	case "flags":
		return "Flags", true
	case "global":
		return "Global", true
	case "ignoreCase":
		return "IgnoreCase", true
	case "multiline":
		return "Multiline", true
	case "dotAll":
		return "DotAll", true
	case "unicode":
		return "Unicode", true
	case "unicodeSets":
		return "UnicodeSets", true
	case "sticky":
		return "Sticky", true
	case "hasIndices":
		return "HasIndices", true
	}
	return "", false
}

// buildRegExp validates a pattern and flag pair through the runtime translator and,
// on success, emits value.NewRegExpLiteral(pattern, flags). The translator is the
// single gate the runtime constructor also consults, so a pattern lowers exactly
// when the runtime can build it; a pattern RE2 cannot host faithfully hands back with
// the translator's reason, the honest ceiling the RE2 host imposes.
func (r *Renderer) buildRegExp(pattern, flags string) (ast.Expr, error) {
	if _, ok, reason := value.TranslateRegExpSource(pattern, flags); !ok {
		return nil, &NotYetLowerable{Reason: reason}
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{
		Fun: sel("value", "NewRegExpLiteral"),
		Args: []ast.Expr{
			&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(pattern)},
			&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(flags)},
		},
	}, nil
}
