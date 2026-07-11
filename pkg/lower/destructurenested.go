package lower

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// A destructuring pattern can nest inside another pattern in any element position,
// so `const [[a, b], [c, d]] = m` binds four names off a two-level array shape and
// `const { p: { x } } = o` reads a property of a property. Go has no destructuring,
// so a nested pattern lowers by minting a temporary for the slot the outer pattern
// selects, then binding the inner pattern against that held value; the recursion
// composes the same read-into-a-temp step to any depth. This file holds the
// recursive core the declaration and parameter paths route a nested element through.

// patternNode reports whether n is a nested binding pattern, an array pattern
// (`[...]`) or an object pattern (`{...}`) appearing as an element of an outer
// pattern. The frontend wraps such an element in an opaque node whose text opens
// with the pattern's bracket, so the shape is read off the leading token.
func (r *Renderer) patternNode(n frontend.Node) bool {
	if n.Kind() != frontend.NodeUnknown {
		return false
	}
	t := strings.TrimSpace(r.prog.Text(n))
	return strings.HasPrefix(t, "[") || strings.HasPrefix(t, "{")
}

// bindSubPattern binds a nested pattern against a receiver expression that already
// holds the value the outer pattern selected. It dispatches on the pattern's shape,
// an array or an object, and recurses so nested patterns compose to any depth. The
// leaves bind with tok, so a declaration or a parameter binds fresh names with a
// `:=` while an assignment target stores into existing names with a `=`.
func (r *Renderer) bindSubPattern(pat frontend.Node, recv ast.Expr, patType frontend.Type, tok token.Token) ([]ast.Stmt, error) {
	txt := strings.TrimSpace(r.prog.Text(pat))
	switch {
	case strings.HasPrefix(txt, "["):
		return r.bindSubArray(pat, recv, patType, tok)
	case strings.HasPrefix(txt, "{"):
		return r.bindSubObject(pat, recv, patType, tok)
	}
	return nil, &NotYetLowerable{Reason: "a nested destructuring element that is neither an array nor an object pattern is a later slice"}
}

// objectNestedElem reports whether an object binding pattern element renames a
// source property into a nested pattern, `{ p: { x } }` or `{ p: [a] }`, and returns
// the source property node and the nested pattern. Such an element has the source
// property and the pattern as its two children under a `:`, the shape of a rename
// whose target is a pattern rather than a plain identifier.
func (r *Renderer) objectNestedElem(el frontend.Node) (source, sub frontend.Node, ok bool) {
	ec := r.prog.Children(el)
	if len(ec) == 2 && ec[0].Kind() == frontend.NodeIdentifier && r.patternNode(ec[1]) && strings.Contains(r.childGap(el, ec[0], ec[1]), ":") {
		return ec[0], ec[1], true
	}
	return nil, nil, false
}

// bindSubObject binds an object pattern nested inside an outer pattern. It reads each
// property off the receiver through the struct-field selector, the same read a
// top-level object element takes, and binds a further-nested element by minting a
// temporary for the selected property and recursing. A default or a rest inside the
// nesting composes the fill and gather rules through a level and is a later item, so
// it hands back for now.
func (r *Renderer) bindSubObject(pat frontend.Node, recv ast.Expr, patType frontend.Type, tok token.Token) ([]ast.Stmt, error) {
	if patType.Flags&frontend.TypeObject == 0 {
		return nil, &NotYetLowerable{Reason: "a nested object pattern over a non-object type is a later slice"}
	}
	if _, err := r.decls.internStruct(r, patType); err != nil {
		return nil, err
	}
	elems := r.prog.Children(pat)
	if len(elems) == 0 {
		return nil, &NotYetLowerable{Reason: "an empty object destructuring pattern binds nothing"}
	}
	propType := map[string]frontend.Type{}
	optionalField := map[string]bool{}
	for _, pr := range r.prog.Properties(patType) {
		propType[pr.Name] = pr.Type
		optionalField[pr.Name] = pr.Optional
	}
	var out []ast.Stmt
	for _, el := range elems {
		if source, sub, ok := r.objectNestedElem(el); ok {
			prop := r.prog.Text(source)
			srcName, ok := localName(prop)
			if !ok {
				return nil, &NotYetLowerable{Reason: "destructured name is not a Go identifier"}
			}
			field, ok := exportedField(srcName)
			if !ok {
				return nil, &NotYetLowerable{Reason: "destructured property is not a Go field name"}
			}
			pt, known := propType[prop]
			if !known {
				return nil, &NotYetLowerable{Reason: "a nested object pattern over an unknown property is a later slice"}
			}
			tmp := r.freshTemp()
			out = append(out, &ast.AssignStmt{Lhs: []ast.Expr{ident(tmp)}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.SelectorExpr{X: recv, Sel: ident(field)}}})
			inner, err := r.bindSubPattern(sub, ident(tmp), pt, tok)
			if err != nil {
				return nil, err
			}
			out = append(out, inner...)
			continue
		}
		info, err := r.classifyObjectElem(el)
		if err != nil {
			return nil, err
		}
		prop := r.prog.Text(info.nameNode)
		srcName, ok := localName(prop)
		if !ok {
			return nil, &NotYetLowerable{Reason: "destructured name is not a Go identifier"}
		}
		field, ok := exportedField(srcName)
		if !ok {
			return nil, &NotYetLowerable{Reason: "destructured property is not a Go field name"}
		}
		name, ok := localName(r.prog.Text(info.bindNode))
		if !ok {
			return nil, &NotYetLowerable{Reason: "destructured target is not a Go identifier"}
		}
		read := &ast.SelectorExpr{X: recv, Sel: ident(field)}
		// A default over a required field can never fire, since the property is always
		// present, so it binds the read directly and the default is dead. A default over
		// an optional field fills only when the property is undefined, which needs the
		// source to omit it; an omitting nested object literal is inferred into a struct
		// whose omitted field is a plain value rather than the annotated Opt, so the Opt
		// fill would read a field the source value does not carry. That nested-object
		// literal coercion is a phase 7 capability, so a live optional default composed
		// through the nesting hands back rather than emit an Opt read the source cannot
		// answer.
		if info.hasDefault && optionalField[prop] {
			return nil, &NotYetLowerable{Reason: "an optional-field default composed through a nested object pattern needs the nested-object literal coercion of phase 7"}
		}
		out = append(out, &ast.AssignStmt{Lhs: []ast.Expr{ident(name)}, Tok: tok, Rhs: []ast.Expr{read}})
	}
	return out, nil
}

// assignPatternNode reports whether n is a nested assignment pattern. An assignment
// target parses as an expression, so a nested array pattern is an array literal and a
// nested object pattern is an object literal, unlike a declaration pattern which the
// frontend wraps in an opaque bracketed node.
func (r *Renderer) assignPatternNode(n frontend.Node) bool {
	return n.Kind() == frontend.NodeArrayLiteralExpression || n.Kind() == frontend.NodeObjectLiteralExpression
}

// objectAssignNestedElem reports whether an object assignment property renames a
// source property into a nested pattern, `({ p: { x } } = o)` or `({ p: [a] } = o)`,
// and returns the source property node and the nested pattern. It is the assignment
// sibling of objectNestedElem, differing only in that the nested pattern is an object
// or array literal rather than an opaque bracketed node.
func (r *Renderer) objectAssignNestedElem(el frontend.Node) (source, sub frontend.Node, ok bool) {
	ec := r.prog.Children(el)
	if len(ec) == 2 && ec[0].Kind() == frontend.NodeIdentifier && r.assignPatternNode(ec[1]) && strings.Contains(r.childGap(el, ec[0], ec[1]), ":") {
		return ec[0], ec[1], true
	}
	return nil, nil, false
}

// bindSubPatternAssign binds a nested assignment pattern against a receiver that holds
// the value the outer pattern selected, storing each leaf into its existing target. It
// is the assignment sibling of bindSubPattern: leaves assign with `=` rather than
// declaring with `:=`, and the pattern is an array or object literal rather than an
// opaque bracketed node.
func (r *Renderer) bindSubPatternAssign(pat frontend.Node, recv ast.Expr, patType frontend.Type) ([]ast.Stmt, error) {
	switch pat.Kind() {
	case frontend.NodeArrayLiteralExpression:
		return r.bindSubArrayAssign(pat, recv, patType)
	case frontend.NodeObjectLiteralExpression:
		return r.bindSubObjectAssign(pat, recv, patType)
	}
	return nil, &NotYetLowerable{Reason: "a nested assignment target that is neither an array nor an object pattern is a later slice"}
}

// bindSubArrayAssign binds an array assignment pattern nested inside an outer pattern,
// storing each leaf into its existing target through the AtI read, and recursing for a
// further-nested target by holding the slot in a temporary. A default fills into the
// target from AtOpt and a trailing rest gathers the tail, the same rules the top-level
// array assignment fill applies.
func (r *Renderer) bindSubArrayAssign(pat frontend.Node, recv ast.Expr, patType frontend.Type) ([]ast.Stmt, error) {
	elemT, ok := r.prog.ElementType(patType)
	if !ok {
		return nil, &NotYetLowerable{Reason: "a nested array assignment over a non-array or tuple type is a later slice"}
	}
	elemGo, err := r.typeExpr(elemT)
	if err != nil {
		return nil, err
	}
	targets := r.prog.Children(pat)
	if len(targets) == 0 {
		return nil, &NotYetLowerable{Reason: "an empty array assignment pattern binds nothing"}
	}
	fixedTargets, restNode, hasRest, err := r.splitArrayRest(targets)
	if err != nil {
		return nil, err
	}
	var out []ast.Stmt
	for i, tgt := range fixedTargets {
		if r.assignPatternNode(tgt) {
			tmp := r.freshTemp()
			read := &ast.CallExpr{
				Fun:  &ast.SelectorExpr{X: recv, Sel: ident("AtI")},
				Args: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(i)}},
			}
			out = append(out, &ast.AssignStmt{Lhs: []ast.Expr{ident(tmp)}, Tok: token.DEFINE, Rhs: []ast.Expr{read}})
			inner, err := r.bindSubPatternAssign(tgt, ident(tmp), elemT)
			if err != nil {
				return nil, err
			}
			out = append(out, inner...)
			continue
		}
		el, err := r.classifyArrayAssignElem(tgt)
		if err != nil {
			return nil, err
		}
		name, ok := localName(r.prog.Text(el.nameNode))
		if !ok {
			return nil, &NotYetLowerable{Reason: "array assignment target is not a Go identifier"}
		}
		tgtGo, err := r.typeExpr(r.prog.TypeAt(el.nameNode))
		if err != nil {
			return nil, err
		}
		if same, err := sameGoType(tgtGo, elemGo); err != nil {
			return nil, err
		} else if !same {
			if el.hasDefault {
				return nil, &NotYetLowerable{Reason: "a nested array assignment default over an optional-element source is a later slice"}
			}
			return nil, &NotYetLowerable{Reason: "array destructuring assignment where a target's type differs from the array element type is a later slice"}
		}
		if el.hasDefault {
			def, err := r.lowerExpr(el.defNode)
			if err != nil {
				return nil, err
			}
			def, err = r.coerceToType(def, el.defNode, r.prog.TypeAt(el.nameNode))
			if err != nil {
				return nil, err
			}
			out = append(out, r.defaultFillAssign(ident(name), arrayOptRead(recv, i), def))
			continue
		}
		read := &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: recv, Sel: ident("AtI")},
			Args: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(i)}},
		}
		out = append(out, &ast.AssignStmt{Lhs: []ast.Expr{ident(name)}, Tok: token.ASSIGN, Rhs: []ast.Expr{read}})
	}
	if hasRest {
		bind, err := r.arrayRestBinding(restNode, elemT, recv, len(fixedTargets), token.ASSIGN)
		if err != nil {
			return nil, err
		}
		out = append(out, bind)
	}
	return out, nil
}

// bindSubObjectAssign binds an object assignment pattern nested inside an outer
// pattern, storing each leaf into its existing target through the struct-field
// selector, and recursing for a further-nested target by holding the property in a
// temporary. An optional-field default composed through the nesting hands back the same
// way the declaration form does, since a live default needs the phase 7 nested-object
// literal coercion.
func (r *Renderer) bindSubObjectAssign(pat frontend.Node, recv ast.Expr, patType frontend.Type) ([]ast.Stmt, error) {
	if patType.Flags&frontend.TypeObject == 0 {
		return nil, &NotYetLowerable{Reason: "a nested object assignment over a non-object type is a later slice"}
	}
	if _, err := r.decls.internStruct(r, patType); err != nil {
		return nil, err
	}
	elems := r.prog.Children(pat)
	if len(elems) == 0 {
		return nil, &NotYetLowerable{Reason: "an empty object assignment pattern binds nothing"}
	}
	propType := map[string]frontend.Type{}
	optionalField := map[string]bool{}
	for _, pr := range r.prog.Properties(patType) {
		propType[pr.Name] = pr.Type
		optionalField[pr.Name] = pr.Optional
	}
	var out []ast.Stmt
	for _, el := range elems {
		if source, sub, ok := r.objectAssignNestedElem(el); ok {
			prop := r.prog.Text(source)
			srcName, nok := localName(prop)
			pt, known := propType[prop]
			if !nok || !known {
				return nil, &NotYetLowerable{Reason: "a nested object assignment over an unknown property is a later slice"}
			}
			field, fok := exportedField(srcName)
			if !fok {
				return nil, &NotYetLowerable{Reason: "object assignment property is not a Go field name"}
			}
			tmp := r.freshTemp()
			out = append(out, &ast.AssignStmt{Lhs: []ast.Expr{ident(tmp)}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.SelectorExpr{X: recv, Sel: ident(field)}}})
			inner, err := r.bindSubPatternAssign(sub, ident(tmp), pt)
			if err != nil {
				return nil, err
			}
			out = append(out, inner...)
			continue
		}
		info, err := r.classifyObjectAssignElem(el)
		if err != nil {
			return nil, err
		}
		prop := r.prog.Text(info.nameNode)
		srcName, ok := localName(prop)
		if !ok {
			return nil, &NotYetLowerable{Reason: "object assignment property is not a Go identifier"}
		}
		field, ok := exportedField(srcName)
		if !ok {
			return nil, &NotYetLowerable{Reason: "object assignment property is not a Go field name"}
		}
		name, ok := localName(r.prog.Text(info.bindNode))
		if !ok {
			return nil, &NotYetLowerable{Reason: "object assignment target is not a Go identifier"}
		}
		read := &ast.SelectorExpr{X: recv, Sel: ident(field)}
		if info.hasDefault && optionalField[prop] {
			return nil, &NotYetLowerable{Reason: "an optional-field default composed through a nested object assignment needs the nested-object literal coercion of phase 7"}
		}
		out = append(out, &ast.AssignStmt{Lhs: []ast.Expr{ident(name)}, Tok: token.ASSIGN, Rhs: []ast.Expr{read}})
	}
	return out, nil
}

// defaultFillFor emits the lazy default fill for a nested leaf, picking the shape
// the leaf's binding token needs: a declaration or parameter leaf declares a fresh
// local and fills it, an assignment leaf fills into the existing target without a new
// declaration. It routes a default composed through a nesting to the same fill the
// top-level paths use.
func (r *Renderer) defaultFillFor(tok token.Token, name string, nameGo, read, def ast.Expr) []ast.Stmt {
	if tok == token.DEFINE {
		return r.defaultFillStmts(name, nameGo, read, def)
	}
	return []ast.Stmt{r.defaultFillAssign(ident(name), read, def)}
}

// bindSubArray binds an array pattern nested inside an outer pattern. It reads each
// fixed slot off the receiver by index, the same bounds-aware AtI read a top-level
// array element takes, and binds a further-nested element by minting a temporary for
// the slot and recursing. The receiver already holds the value, so no source
// temporary or iterator draining is needed here; that machinery stays with the
// top-level path. A default fills through the nesting from AtOpt and a trailing rest
// gathers the tail, the same fill and gather rules the top-level array path applies.
func (r *Renderer) bindSubArray(pat frontend.Node, recv ast.Expr, patType frontend.Type, tok token.Token) ([]ast.Stmt, error) {
	elemT, ok := r.prog.ElementType(patType)
	if !ok {
		return nil, &NotYetLowerable{Reason: "a nested array pattern over a non-array or tuple type is a later slice"}
	}
	elemGo, err := r.typeExpr(elemT)
	if err != nil {
		return nil, err
	}
	elems := r.prog.Children(pat)
	if len(elems) == 0 {
		return nil, &NotYetLowerable{Reason: "an empty array destructuring pattern binds nothing"}
	}
	fixedElems, restNode, hasRest, err := r.splitArrayRest(elems)
	if err != nil {
		return nil, err
	}
	var out []ast.Stmt
	for i, el := range fixedElems {
		info, err := r.classifyArrayElem(el)
		if err != nil {
			return nil, err
		}
		if info.nested != nil {
			tmp := r.freshTemp()
			read := &ast.CallExpr{
				Fun:  &ast.SelectorExpr{X: recv, Sel: ident("AtI")},
				Args: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(i)}},
			}
			out = append(out, &ast.AssignStmt{Lhs: []ast.Expr{ident(tmp)}, Tok: token.DEFINE, Rhs: []ast.Expr{read}})
			inner, err := r.bindSubPattern(info.nested, ident(tmp), elemT, tok)
			if err != nil {
				return nil, err
			}
			out = append(out, inner...)
			continue
		}
		name, ok := localName(r.prog.Text(info.nameNode))
		if !ok {
			return nil, &NotYetLowerable{Reason: "destructured name is not a Go identifier"}
		}
		nameGo, err := r.typeExpr(r.prog.TypeAt(info.nameNode))
		if err != nil {
			return nil, err
		}
		if same, err := sameGoType(nameGo, elemGo); err != nil {
			return nil, err
		} else if !same {
			if info.hasDefault {
				return nil, &NotYetLowerable{Reason: "a nested array default over an optional-element source is a later slice"}
			}
			return nil, &NotYetLowerable{Reason: "array destructuring where an element's type differs from the array element type is a later slice"}
		}
		if info.hasDefault {
			def, err := r.lowerExpr(info.defNode)
			if err != nil {
				return nil, err
			}
			def, err = r.coerceToType(def, info.defNode, r.prog.TypeAt(info.nameNode))
			if err != nil {
				return nil, err
			}
			out = append(out, r.defaultFillFor(tok, name, nameGo, arrayOptRead(recv, i), def)...)
			continue
		}
		read := &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: recv, Sel: ident("AtI")},
			Args: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(i)}},
		}
		out = append(out, &ast.AssignStmt{Lhs: []ast.Expr{ident(name)}, Tok: tok, Rhs: []ast.Expr{read}})
	}
	if hasRest {
		bind, err := r.arrayRestBinding(restNode, elemT, recv, len(fixedElems), tok)
		if err != nil {
			return nil, err
		}
		out = append(out, bind)
	}
	return out, nil
}
