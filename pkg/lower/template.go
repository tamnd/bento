package lower

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers string literals and template expressions to bstr values, and
// hoists the strings.Builder locals a template concatenation chain uses.

// newStrBuilder records that the body being lowered needs one more reusable
// value.StrBuilder and returns the Go variable name for it, unique within the
// body. The caller emits a Reset-to-Done chain on this name; hoistStrBuilders
// later declares it above the body.
func (r *Renderer) newStrBuilder() string {
	name := "_sb" + strconv.Itoa(len(r.strBuilders))
	r.strBuilders = append(r.strBuilders, name)
	r.requireImport(valuePkg)
	return name
}

// hoistStrBuilders prepends a var declaration for each builder the body recorded,
// so every builder is created once above the body's statements and reused on each
// iteration of any loop it sits in. A body that built no template through a builder
// prepends nothing and is returned unchanged.
func (r *Renderer) hoistStrBuilders(stmts []ast.Stmt) []ast.Stmt {
	if len(r.strBuilders) == 0 {
		return stmts
	}
	decls := make([]ast.Stmt, 0, len(r.strBuilders))
	for _, name := range r.strBuilders {
		decls = append(decls, &ast.DeclStmt{Decl: &ast.GenDecl{
			Tok: token.VAR,
			Specs: []ast.Spec{&ast.ValueSpec{
				Names: []*ast.Ident{ident(name)},
				Type:  sel("value", "StrBuilder"),
			}},
		}})
	}
	return append(decls, stmts...)
}

// stringLiteral lowers a string literal to a value.BStr. The literal's runtime
// content is not its source text: the source carries backslash escapes, so it is
// decoded into UTF-16 code units first (decodeJSString). A content that decodes to
// valid UTF-16 becomes a Go string literal wrapped in value.FromGoString, the
// common case. A content that decodes to a lone surrogate, which a \u escape can
// name and which no Go string can hold, is emitted as a raw []uint16 wrapped in
// value.FromUTF16 so the surrogate survives. A content that does not decode (a
// malformed escape) hands back.
// stringLiteralKey reads the property name a string-literal key spells, the value
// between the quotes with its escapes resolved, so o["k"] can select the same Go
// struct field o.k does. It returns false for a node that is not a plain string
// literal, and for a key that decodes to a lone surrogate, which no Go identifier
// could name anyway, so the caller hands such a key back rather than intern it.
func (r *Renderer) stringLiteralKey(n frontend.Node) (string, bool) {
	if n.Kind() != frontend.NodeStringLiteral {
		return "", false
	}
	text := r.prog.Text(n)
	if len(text) < 2 {
		return "", false
	}
	quote := text[0]
	if (quote != '"' && quote != '\'') || text[len(text)-1] != quote {
		return "", false
	}
	units, ok := decodeJSString(text[1 : len(text)-1])
	if !ok || hasLoneSurrogate(units) {
		return "", false
	}
	return string(utf16.Decode(units)), true
}

func (r *Renderer) stringLiteral(n frontend.Node) (ast.Expr, error) {
	text := r.prog.Text(n)
	if len(text) < 2 {
		return nil, &NotYetLowerable{Reason: "string literal source too short to lower"}
	}
	quote := text[0]
	if (quote != '"' && quote != '\'') || text[len(text)-1] != quote {
		return nil, &NotYetLowerable{Reason: "unusual string literal quoting is a later slice"}
	}
	units, ok := decodeJSString(text[1 : len(text)-1])
	if !ok {
		return nil, &NotYetLowerable{Reason: "string literal has a malformed escape sequence"}
	}
	return r.bstrLit(units), nil
}

// bstrLit builds the AST for a value.BStr holding the given UTF-16 code units. A
// content that is valid UTF-16 becomes a Go string literal wrapped in
// value.FromGoString, the common case; a content that carries a lone surrogate,
// which no Go string can hold, is emitted as a raw []uint16 wrapped in
// value.FromUTF16 so the surrogate survives. The string literal and template
// paths share this so both spell a compile-time string the same way.
func (r *Renderer) bstrLit(units []uint16) ast.Expr {
	r.requireImport(valuePkg)
	if hasLoneSurrogate(units) {
		return &ast.CallExpr{Fun: sel("value", "FromUTF16"), Args: []ast.Expr{uint16SliceLit(units)}}
	}
	lit := &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(string(utf16.Decode(units)))}
	return &ast.CallExpr{Fun: sel("value", "FromGoString"), Args: []ast.Expr{lit}}
}

// noSubTemplate lowers a template literal with no substitutions, `like this`,
// which denotes exactly one string. Its cooked content is the source between the
// backticks with escapes resolved, so it lowers to the same value.BStr a string
// literal of that content would, only the delimiters differ.
func (r *Renderer) noSubTemplate(n frontend.Node) (ast.Expr, error) {
	units, ok := templateCooked(r.prog.Text(n))
	if !ok {
		return nil, &NotYetLowerable{Reason: "template literal has a malformed escape sequence"}
	}
	return r.bstrLit(units), nil
}

// templateExpression lowers a template literal with substitutions, `a${x}b`, to a
// single string. The frontend exposes it as a head literal followed by one span
// per substitution, each span holding the interpolated expression and the literal
// text that follows it (a middle, or the tail at the end), so `a${x}b${y}c` is head
// "a", then x with "b", then y with "c".
//
// Two lowerings are possible. When at least one substitution is a number or a
// boolean and every substitution is a primitive (string, number, or boolean), the
// template builds through a reused value.StrBuilder hoisted above the enclosing
// loop: each part appends straight into the builder's buffer, so the number and
// boolean coercions the join form otherwise materializes as their own String(x)
// strings never allocate. Otherwise the template joins with one ConcatN on the
// head: that covers a string-only template, where a builder would fold no coercion
// and save nothing, and any substitution whose coercion the builder does not carry.
// The join coerces through stringify, the same ToString String(x) uses, so a
// template and an explicit String() call agree. An expression whose type does not
// coerce (an object) hands the whole template back.
func (r *Renderer) templateExpression(n frontend.Node) (ast.Expr, error) {
	kids := r.prog.Children(n)
	if len(kids) < 2 {
		return nil, &NotYetLowerable{Reason: "template expression did not expose a head and at least one span"}
	}
	headUnits, ok := templateCooked(r.prog.Text(kids[0]))
	if !ok {
		return nil, &NotYetLowerable{Reason: "template head has a malformed escape sequence"}
	}
	// Gather the substitution nodes and the literal units that follow each, in
	// source order, so the type scan and both lowerings read the same parts.
	type tmplSpan struct {
		sub frontend.Node
		lit []uint16
	}
	spans := make([]tmplSpan, 0, len(kids)-1)
	for _, span := range kids[1:] {
		parts := r.prog.Children(span)
		if len(parts) != 2 {
			return nil, &NotYetLowerable{Reason: "template span did not expose an expression and a literal"}
		}
		litUnits, ok := templateCooked(r.prog.Text(parts[1]))
		if !ok {
			return nil, &NotYetLowerable{Reason: "template literal part has a malformed escape sequence"}
		}
		spans = append(spans, tmplSpan{sub: parts[0], lit: litUnits})
	}

	// Decide the lowering. The builder pays off only when a substitution needs a
	// number or boolean coercion, and it can carry a substitution only when every
	// one is a primitive it appends directly; a non-primitive forces the join.
	needsCoercion := false
	allPrimitive := true
	for i := range spans {
		switch {
		case r.isString(spans[i].sub):
		case r.isNumber(spans[i].sub), r.isBool(spans[i].sub):
			needsCoercion = true
		default:
			allPrimitive = false
		}
	}

	if needsCoercion && allPrimitive {
		name := r.newStrBuilder()
		var chain ast.Expr = &ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(name), Sel: ident("Reset")}}
		chain = r.appendTemplateLit(chain, headUnits)
		for i := range spans {
			appended, err := r.appendTemplateSub(chain, spans[i].sub)
			if err != nil {
				return nil, err
			}
			chain = r.appendTemplateLit(appended, spans[i].lit)
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: chain, Sel: ident("Done")}}, nil
	}

	// Fallback join: the head, then for each span the coerced expression and the
	// literal that follows it, materialized once by one ConcatN.
	pieces := []ast.Expr{r.bstrLit(headUnits)}
	for i := range spans {
		strExpr, err := r.stringify(spans[i].sub)
		if err != nil {
			return nil, err
		}
		pieces = append(pieces, strExpr, r.bstrLit(spans[i].lit))
	}
	return &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: pieces[0], Sel: ident("ConcatN")},
		Args: pieces[1:],
	}, nil
}

// appendTemplateLit chains a compile-time literal part onto a StrBuilder build. An
// empty part (adjacent substitutions, or an empty head or tail) contributes nothing
// and is skipped. A part that carries a lone surrogate goes through Units, which no
// Go string literal can hold; every other part takes the cheaper Lit byte path with
// its precomputed code-unit length.
func (r *Renderer) appendTemplateLit(recv ast.Expr, units []uint16) ast.Expr {
	if len(units) == 0 {
		return recv
	}
	if hasLoneSurrogate(units) {
		return &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: recv, Sel: ident("Units")},
			Args: []ast.Expr{uint16SliceLit(units)},
		}
	}
	return &ast.CallExpr{
		Fun: &ast.SelectorExpr{X: recv, Sel: ident("Lit")},
		Args: []ast.Expr{
			&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(string(utf16.Decode(units)))},
			&ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(len(units))},
		},
	}
}

// appendTemplateSub chains one substitution onto a StrBuilder build, dispatched on
// the checker's type: a string appends its code units through Str, a number formats
// through Num, and a boolean writes its word through Bool. A number substitution
// lowers through lowerExpr, which already yields the float64 the value model prints
// (including the float64(i) of an int32-specialized local), so Num prints the full
// number and never a wrapped int32. The caller has proven every substitution is one
// of these three, so the default only guards against a type slipping through.
func (r *Renderer) appendTemplateSub(recv ast.Expr, sub frontend.Node) (ast.Expr, error) {
	lowered, err := r.lowerExpr(sub)
	if err != nil {
		return nil, err
	}
	var method string
	switch {
	case r.isString(sub):
		method = "Str"
	case r.isNumber(sub):
		method = "Num"
	case r.isBool(sub):
		method = "Bool"
	default:
		return nil, &NotYetLowerable{Reason: "template substitution is not a primitive the builder appends"}
	}
	return &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: recv, Sel: ident(method)},
		Args: []ast.Expr{lowered},
	}, nil
}

// templateCooked decodes the cooked value of one template literal token: the head,
// a middle, the tail, or a whole no-substitution literal. The raw source carries
// the delimiters the parser matched, so they are stripped first, a leading
// backtick (a head or whole literal) or close brace (a middle or the tail), and a
// trailing "${" before a substitution or a backtick at the end. What remains is
// the same escaped content a string literal holds between its quotes, so
// decodeJSString resolves it, including \` and \$ which stand for themselves. It
// returns false when the delimiters are not the expected shape or an escape is
// malformed, so the caller hands the template back rather than guessing.
func templateCooked(text string) ([]uint16, bool) {
	if len(text) < 2 {
		return nil, false
	}
	if text[0] != '`' && text[0] != '}' {
		return nil, false
	}
	inner := text[1:]
	switch {
	case strings.HasSuffix(inner, "${"):
		inner = inner[:len(inner)-2]
	case strings.HasSuffix(inner, "`"):
		inner = inner[:len(inner)-1]
	default:
		return nil, false
	}
	return decodeJSString(inner)
}

// uint16SliceLit builds the AST for a []uint16{...} composite literal of the given
// code units, each written as a hex constant so a reader sees the code units the
// way the string tables do.
func uint16SliceLit(units []uint16) ast.Expr {
	elts := make([]ast.Expr, len(units))
	for i, u := range units {
		elts[i] = &ast.BasicLit{Kind: token.INT, Value: "0x" + strconv.FormatUint(uint64(u), 16)}
	}
	return &ast.CompositeLit{
		Type: &ast.ArrayType{Elt: ident("uint16")},
		Elts: elts,
	}
}
