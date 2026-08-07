package lower

import (
	"go/ast"
	"go/token"
	"strconv"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers the tagged template, tag`a${x}b`. Despite its spelling it is a
// call and not a string: the tag is invoked with the template's literal parts and
// its substitution values, and whatever the tag answers is the expression's value,
// which need not be a string at all.
//
// The first argument is the template strings object, an array of the cooked literal
// parts carrying a raw property that holds the same parts undecoded. Its identity
// belongs to the call site rather than to the call, so it is emitted as one
// package-level var per site (value.TemplateObject) and every evaluation of that
// template hands the tag the same frozen object. The substitutions follow as the
// remaining arguments, in source order.

// taggedTemplate lowers tag`a${x}b` to the call it is. Three tags are covered: the
// built-in String.raw, which is called directly on the runtime helper; a tag whose
// slot holds a box, which dispatches through the runtime the way any dynamic call
// does; and a tag naming a top-level function, which is a static Go call with the
// strings object riding in as the first argument.
func (r *Renderer) taggedTemplate(n frontend.Node) (ast.Expr, error) {
	kids := r.prog.Children(n)
	// A tagged template exposes the tag and the template it wraps. An explicit type
	// argument list, tag<T>`...`, puts a third child between them and fixes a generic
	// the monomorphizer would have to specialize, so it is named rather than lowered as
	// if the type arguments were not there.
	if len(kids) != 2 {
		return nil, r.notLowerable(n, "a tagged template with explicit type arguments is a later slice")
	}
	tagNode, tmpl := kids[0], kids[1]
	subs, strsObj, err := r.templateStringsArg(n, tmpl)
	if err != nil {
		return nil, err
	}

	// String.raw is the one tag the language itself ships. It reads the raw array and
	// splices the substitutions between its parts, which the runtime helper does, so it
	// needs no user function and no boxing wrapper around one.
	if r.isStringRawTag(tagNode) {
		args, err := r.boxedSubs(strsObj, subs)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "StringRaw"), Args: args}, nil
	}

	// A tag whose slot holds a box is called through the runtime, which reads the
	// callable out of the value and binds a receiver when the tag was read off one. A
	// member tag, obj.tag`...`, takes its `this` the same way obj.tag(...) does.
	if r.isDynamic(tagNode) {
		callee, err := r.lowerExpr(tagNode)
		if err != nil {
			return nil, err
		}
		args, err := r.boxedSubs(strsObj, subs)
		if err != nil {
			return nil, err
		}
		return r.dynamicCallOn(callee, args), nil
	}

	if tagNode.Kind() != frontend.NodeIdentifier {
		return nil, r.notLowerable(n, "a tag that is not a plain function name is a later slice")
	}
	sym, ok := r.prog.SymbolAt(tagNode)
	if !ok {
		return nil, r.notLowerable(n, "a tag that does not resolve to a declaration is a later slice")
	}
	sym = r.derefAlias(sym)
	if sym.Flags&frontend.SymbolFunction == 0 {
		return nil, r.notLowerable(n, "a tag that is not a declared function is a later slice")
	}
	// The paths a plain call takes for an overload set and for a callee that reads its
	// own arguments object both work from the call's own argument nodes, which a tagged
	// template's leading argument has none of. Each is named rather than lowered through
	// a path that would bind the wrong slot.
	if _, ok := r.overloadedFuncImpl(sym); ok {
		return nil, r.notLowerable(n, "an overloaded tag function is a later slice")
	}
	if r.funcSymThreadsArgs(sym) || r.funcSymReadsArguments(sym) {
		return nil, r.notLowerable(n, "a tag that reads its own arguments object is a later slice")
	}
	name, ok := exportedField(sym.Name)
	if !ok {
		return nil, r.notLowerable(n, "a tag whose name is not a Go identifier is a later slice")
	}

	sig, ok := r.prog.SignatureAt(n)
	if !ok {
		return nil, r.notLowerable(n, "a tag whose call signature the checker did not resolve is a later slice")
	}
	// A tag whose signature the boxed-signature pass rewrote takes the boxed
	// parameters, the same overlay the declaration emitted, so the substitutions bridge
	// against what the Go function actually declares.
	if fn, ok := r.calleeFuncNode(n); ok {
		sig = r.boxedSig(fn, sig)
	}
	// The strings object is a value.Value, so the slot it lands in has to be one. A tag
	// declared in JavaScript gets that for nothing, since an unannotated parameter is
	// any; a tag annotated TemplateStringsArray asks for a read-only string array with a
	// raw property, which is a shape rather than a box and has no slot the object fits.
	if len(sig.Params) == 0 {
		return nil, r.notLowerable(n, "a tag that takes its template strings through a rest parameter is a later slice")
	}
	if !r.isDynamicType(sig.Params[0].Type) {
		return nil, r.notLowerable(n, "a tag whose template-strings parameter is not typed any is a later slice")
	}
	// A tag with an omittable fixed parameter needs the call site to fill the slot from
	// the declaration's default, which is read off the parameter nodes by position. The
	// leading strings object shifts those positions by one, so rather than reconstruct
	// the mapping the form is named. A rest parameter is not omittable in that sense:
	// it gathers whatever substitutions are left, however many there are.
	if sig.MinArgs < len(sig.Params) {
		return nil, r.notLowerable(n, "a tag with an optional or defaulted parameter is a later slice")
	}
	return r.buildCall(ident(name), []ast.Expr{strsObj}, subs, sig.Params[1:], sig.RestParam, nil, false, false, false)
}

// boxedSubs builds the argument list of a dynamic tag call: the strings object first,
// then each substitution boxed, which is what a callee reading its arguments off a
// value slice expects whatever static shape the substitution had.
func (r *Renderer) boxedSubs(strsObj ast.Expr, subs []frontend.Node) ([]ast.Expr, error) {
	args := make([]ast.Expr, 0, len(subs)+1)
	args = append(args, strsObj)
	for _, s := range subs {
		boxed, err := r.boxOperand(s)
		if err != nil {
			return nil, err
		}
		args = append(args, boxed)
	}
	return args, nil
}

// isStringRawTag reports whether a tag names the built-in String.raw, the ambient
// global's own static rather than a user binding that happens to be spelled that way.
func (r *Renderer) isStringRawTag(tag frontend.Node) bool {
	if tag.Kind() != frontend.NodePropertyAccessExpression {
		return false
	}
	kids := r.prog.Children(tag)
	if len(kids) != 2 || r.prog.Text(kids[1]) != "raw" {
		return false
	}
	return r.prog.Text(kids[0]) == "String" && r.isAmbientGlobal(kids[0])
}

// templateStringsArg reads the template's parts and returns the substitution nodes
// alongside the expression naming the site's template strings object. A template with
// no substitutions is one cooked part and no subs, the shape String.raw`\n` takes.
func (r *Renderer) templateStringsArg(site, tmpl frontend.Node) ([]frontend.Node, ast.Expr, error) {
	var cooked [][]uint16
	var raw []string
	var subs []frontend.Node

	addPart := func(tok frontend.Node) error {
		text := r.prog.Text(tok)
		// A tagged template is the one place the language allows an escape the cooked
		// side cannot decode: the cooked part is undefined there and only raw is
		// readable. Naming that shape is honest; emitting a cooked part that decoded to
		// nothing would hand the tag a string the source never wrote.
		units, ok := templateCooked(text)
		if !ok {
			return r.notLowerable(site, "a tagged template with an escape the cooked strings cannot decode is a later slice")
		}
		rawText, ok := templateRaw(text)
		if !ok {
			return r.notLowerable(site, "a tagged template part with unexpected delimiters is a later slice")
		}
		cooked = append(cooked, units)
		raw = append(raw, rawText)
		return nil
	}

	switch tmpl.Kind() {
	case frontend.NodeNoSubstitutionTemplateLiteral:
		if err := addPart(tmpl); err != nil {
			return nil, nil, err
		}
	case frontend.NodeTemplateExpression:
		kids := r.prog.Children(tmpl)
		if len(kids) < 2 {
			return nil, nil, r.notLowerable(site, "a tagged template did not expose a head and at least one span")
		}
		if err := addPart(kids[0]); err != nil {
			return nil, nil, err
		}
		for _, span := range kids[1:] {
			parts := r.prog.Children(span)
			if len(parts) != 2 {
				return nil, nil, r.notLowerable(site, "a tagged template span did not expose an expression and a literal")
			}
			subs = append(subs, parts[0])
			if err := addPart(parts[1]); err != nil {
				return nil, nil, err
			}
		}
	default:
		return nil, nil, r.notLowerable(site, "a tagged template wrapping something that is not a template literal is a later slice")
	}
	return subs, r.internTemplateObject(site, cooked, raw), nil
}

// internTemplateObject reserves one package-level var for this call site's template
// strings object and returns the name to read it by. Reserving per site rather than
// per content is what the language asks for: its template registry is keyed on the
// parse node, so one site hands out one object however often it runs and two sites
// spelling the same text hand out two.
func (r *Renderer) internTemplateObject(site frontend.Node, cooked [][]uint16, raw []string) ast.Expr {
	if name, ok := r.tmplObjs[site]; ok {
		return ident(name)
	}
	r.requireImport(valuePkg)
	name := "tmplStrings" + itoa(len(r.tmplObjOrder)+1)
	cookedElts := make([]ast.Expr, len(cooked))
	for i, units := range cooked {
		cookedElts[i] = &ast.CallExpr{Fun: sel("value", "StringValue"), Args: []ast.Expr{r.bstrLit(units)}}
	}
	rawElts := make([]ast.Expr, len(raw))
	for i, text := range raw {
		rawElts[i] = &ast.CallExpr{Fun: sel("value", "StringValue"), Args: []ast.Expr{
			&ast.CallExpr{Fun: sel("value", "FromGoString"), Args: []ast.Expr{
				&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(text)},
			}},
		}}
	}
	r.tmplObjs[site] = name
	r.tmplObjOrder = append(r.tmplObjOrder, templateObjectDecl{
		name:   name,
		cooked: cookedElts,
		raw:    rawElts,
	})
	return ident(name)
}

// templateObjectDecl is one call site's reserved template strings object: the var
// name and the two element lists it is built from.
type templateObjectDecl struct {
	name   string
	cooked []ast.Expr
	raw    []ast.Expr
}

// templateObjectDecls emits one package-level var per tagged template site, in
// first-use order so the output is deterministic:
//
//	var tmplStrings1 = value.TemplateObject(
//		[]value.Value{value.StringValue(value.FromGoString("a"))},
//		[]value.Value{value.StringValue(value.FromGoString("a"))},
//	)
//
// Each is built once at init and frozen there, which is how the site keeps the one
// identity the language gives it across every evaluation of the template.
func (r *Renderer) templateObjectDecls() []ast.Decl {
	if len(r.tmplObjOrder) == 0 {
		return nil
	}
	valueSlice := func(elts []ast.Expr) ast.Expr {
		return &ast.CompositeLit{
			Type: &ast.ArrayType{Elt: sel("value", "Value")},
			Elts: elts,
		}
	}
	out := make([]ast.Decl, 0, len(r.tmplObjOrder))
	for _, d := range r.tmplObjOrder {
		out = append(out, &ast.GenDecl{
			Tok: token.VAR,
			Specs: []ast.Spec{&ast.ValueSpec{
				Names: []*ast.Ident{ident(d.name)},
				Values: []ast.Expr{&ast.CallExpr{
					Fun:  sel("value", "TemplateObject"),
					Args: []ast.Expr{valueSlice(d.cooked), valueSlice(d.raw)},
				}},
			}},
		})
	}
	return out
}
