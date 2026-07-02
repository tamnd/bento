package lower

import (
	"go/ast"
	"strings"
	"unicode/utf16"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers the one regexp shape the compiled subset covers: a
// String.prototype.replace or replaceAll whose pattern is a regexp literal with a
// plain-literal body and a flag set bento models. A plain pattern (one with no
// regexp metacharacter) matches exactly the byte sequence it spells, so replacing
// it is the same search value.BStr.Replace and ReplaceAll already do on a string
// pattern; the global flag selects between them. A pattern that uses any regexp
// feature (a class, a quantifier, an anchor, an escape, a group, an alternation)
// needs a real regexp engine and hands back so the unit routes to the engine.
// This is enough for the strings benchmark, whose only regexp is /word/g, without
// pretending to a regexp engine bento does not have.

// regexLiteralArg reports whether the node is a regexp literal and, if so, returns
// its pattern body and flags. The frontend leaves a regexp literal as an unnamed
// node whose source text is the literal itself (/body/flags), so the check is on
// the source shape: a leading slash, a closing slash, and a flag run of ASCII
// letters after it. The parser has already decided this position is a regexp and
// not a division, so a node that reads as one is one.
func (r *Renderer) regexLiteralArg(n frontend.Node) (pattern, flags string, ok bool) {
	if n.Kind() != frontend.NodeUnknown {
		return "", "", false
	}
	text := strings.TrimSpace(r.prog.Text(n))
	if len(text) < 2 || text[0] != '/' {
		return "", "", false
	}
	// The closing slash is the last slash in the source, since a plain pattern
	// carries no escaped slash; a body that would need an escaped slash contains a
	// backslash, which isPlainRegexPattern rejects anyway.
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

// regexReplaceCall lowers a replace or replaceAll whose pattern is a regexp
// literal. It admits only a plain-literal pattern and a flag set of at most the
// global flag, so the call reduces to the string search the value methods do: the
// global flag maps replace and replaceAll alike to ReplaceAll, and a non-global
// replace to Replace. replaceAll requires a global regexp in JavaScript, so a
// non-global one is not a shape to compile. The replacement must be a string.
func (r *Renderer) regexReplaceCall(recvNode frontend.Node, method, pattern, flags string, argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 2 {
		return nil, &NotYetLowerable{Reason: "string ." + method + " with a regexp and this argument count is a later slice"}
	}
	if !isPlainRegexPattern(pattern) {
		return nil, &NotYetLowerable{Reason: "string ." + method + " with a non-literal regexp pattern needs the regexp engine, a later slice"}
	}
	global := false
	for _, f := range flags {
		if f != 'g' {
			return nil, &NotYetLowerable{Reason: "string ." + method + " with the regexp flag " + string(f) + " is a later slice"}
		}
		global = true
	}
	if method == "replaceAll" && !global {
		return nil, &NotYetLowerable{Reason: "String.prototype.replaceAll needs a global regexp"}
	}
	if !r.isString(argNodes[1]) {
		return nil, &NotYetLowerable{Reason: "string ." + method + " with a non-string replacement is a later slice"}
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	repl, err := r.lowerExpr(argNodes[1])
	if err != nil {
		return nil, err
	}
	goName := "Replace"
	if global {
		goName = "ReplaceAll"
	}
	search := r.bstrLit(utf16.Encode([]rune(pattern)))
	return &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: recv, Sel: ident(goName)},
		Args: []ast.Expr{search, repl},
	}, nil
}

// isPlainRegexPattern reports whether a regexp body is a plain literal, one whose
// every character matches only itself so the pattern is the same as a string
// search. Any regexp metacharacter, and the backslash that starts an escape,
// makes the pattern more than a literal and is rejected, so only a body that a
// string search reproduces exactly lowers. An empty body is not plain: an empty
// regexp matches at every position, which is not the empty-string search the
// value methods would do, so it hands back.
func isPlainRegexPattern(body string) bool {
	if body == "" {
		return false
	}
	return !strings.ContainsAny(body, `\^$.|?*+()[]{}`)
}

// allASCIILetters reports whether every byte of s is an ASCII letter, the shape a
// regexp flag run has. An empty run is all letters vacuously, the no-flags case.
func allASCIILetters(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') {
			return false
		}
	}
	return true
}
