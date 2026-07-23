package lower

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// A destructuring pattern whose elements carry no static type reads its source as a
// boxed value.Value rather than a Go struct or slice. This is the JS-as-TS shape the
// checker flags with "Binding element 'X' implicitly has an 'any' type" (7031): the
// pattern has no annotation, so every element resolves to any, and there is no fixed
// shape to intern to a field or an index. The typed binder cannot serve it, since it
// would read `.Field` off a struct the argument does not carry, so an untyped pattern
// binds each element through the same dynamic protocol a dynamic member or index read
// takes: an object property through Get. Each bound name is a value.Value the body
// then reads on the dynamic path.

// dynamicParamSlot reports whether a parameter is a destructured pattern with no
// static shape, the JS-as-TS parameter the checker flags 7031 (or the same pattern
// annotated any). Such a parameter arrives as one boxed value.Value slot rather than
// a Go struct, and its bound names read out of it through the dynamic protocol. The
// name is synthesized (__0, __1, and so on), which only a destructured parameter
// carries, so a plainly named any parameter is left alone; the type is any, or an
// object every one of whose properties is dynamic, which is the shape a fully untyped
// pattern infers. A pattern with a static leaf keeps the typed binder, which reads
// that leaf through the field or index its shape declares.
func (r *Renderer) dynamicParamSlot(p frontend.Param) bool {
	if !isSynthParamName(p.Name) {
		return false
	}
	return r.allLeavesDynamic(p.Type, 0)
}

// allLeavesDynamic reports whether every leaf a destructuring pattern would read from a
// type is dynamic (any or unknown), the shape a fully untyped pattern infers. A bare any
// is a dynamic leaf; a tuple or array pattern's positions are its numeric-keyed
// properties, so each must be dynamic in turn, its own methods being object-typed and
// read past; an object pattern's leaves are its named properties. A type with any static
// leaf is not fully dynamic, so a pattern over it keeps the typed binder, which reads
// that leaf through the field or index its shape declares. The recursion follows nested
// patterns, whose position holds a further pattern rather than a bare any, and a depth
// bound stops a self-referential type from looping.
func (r *Renderer) allLeavesDynamic(t frontend.Type, depth int) bool {
	if depth > 64 {
		return false
	}
	if t.Flags&(frontend.TypeAny|frontend.TypeUnknown) != 0 {
		return true
	}
	if t.Flags&frontend.TypeObject == 0 {
		return false
	}
	props := r.prog.Properties(t)
	if len(props) == 0 {
		return false
	}
	numeric := false
	for _, pr := range props {
		if !isNumericName(pr.Name) {
			continue
		}
		numeric = true
		if !r.allLeavesDynamic(pr.Type, depth+1) {
			return false
		}
	}
	if numeric {
		return true
	}
	for _, pr := range props {
		if !r.allLeavesDynamic(pr.Type, depth+1) {
			return false
		}
	}
	return true
}

// isNumericName reports whether a property name is an all-digit index position, the
// key an array pattern interns for each of its slots. It tells an array pattern's
// positional properties from an object pattern's named ones.
func isNumericName(name string) bool {
	if name == "" {
		return false
	}
	for _, c := range name {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// isSynthParamName reports whether a parameter name is the synthesized __N the
// frontend gives a destructured parameter, which has no source identifier of its own.
// It is the tell that a parameter binds a pattern rather than a plain name.
func isSynthParamName(name string) bool {
	if !strings.HasPrefix(name, "__") || len(name) == 2 {
		return false
	}
	for _, c := range name[2:] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// pushDynBound sets the dynamic-bound-locals set from a signature's destructured
// parameters and returns the restore that clears it, so a caller writes
// `defer r.pushDynBound(paramNodes, sig)()` next to where it lowers a body. The set
// is built before the body lowers, since a read of a rest binding sits in the body
// that lowers ahead of the entry bindings paramDestructureBindings appends, so the
// read needs the set already in place to route the boxed way.
func (r *Renderer) pushDynBound(paramNodes []frontend.Node, sig frontend.Signature) func() {
	prev := r.dynBoundLocals
	r.dynBoundLocals = r.dynBoundLocalsOf(paramNodes, sig)
	return func() { r.dynBoundLocals = prev }
}

// dynBoundLocalsOf collects the names an untyped destructuring parameter binds to a
// boxed value.Value the checker did not type any, the object rest bindings whose
// property reads must dispatch dynamically rather than fold to a fixed-shape miss.
// Only a dynamic-slot parameter contributes, since a typed pattern binds its rest to
// a real struct the shape path reads. An empty result is nil so a body with no such
// binding pays nothing.
func (r *Renderer) dynBoundLocalsOf(paramNodes []frontend.Node, sig frontend.Signature) map[string]bool {
	out := map[string]bool{}
	for i, pn := range paramNodes {
		if i >= len(sig.Params) {
			break
		}
		pkids := r.prog.Children(pn)
		if len(pkids) == 0 || pkids[0].Kind() == frontend.NodeIdentifier {
			continue
		}
		if !r.dynamicParamSlot(sig.Params[i]) {
			continue
		}
		r.collectDynRestNames(pkids[0], out)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// collectDynRestNames records the rest binding names a dynamic pattern gathers,
// recursing into a nested pattern so a rest one level down is seen too. Only a rest is
// recorded: a shorthand or renamed leaf is typed any already, so isDynamic routes its
// read the boxed way without help, and marking it would risk shadowing a same-named
// static local later in the body. An array rest is bound as a boxed value.Value tail, so
// its name is recorded too, and nested array positions are followed for a deeper rest.
// collectAssignedNames records every non-blank identifier a dynamic pattern's binding
// statements assign, so each bound name is marked dynamic before the body that reads it
// lowers. The names come straight off the emitted binds, which is exact where the pattern
// gives a leaf a concrete checker type (a catch-destructured name inferred a number) whose
// Go slot is nonetheless a boxed value.Value: reading the assign targets, not the checker
// type, is what keeps the two in step. A per-iteration temporary the binder introduced is
// harmless to include, since no user read names it.
func (r *Renderer) collectAssignedNames(stmts []ast.Stmt, out map[string]bool) {
	for _, s := range stmts {
		a, ok := s.(*ast.AssignStmt)
		if !ok {
			continue
		}
		for _, lhs := range a.Lhs {
			if id, ok := lhs.(*ast.Ident); ok && id.Name != "_" {
				out[id.Name] = true
			}
		}
	}
}

func (r *Renderer) collectDynRestNames(pat frontend.Node, out map[string]bool) {
	txt := strings.TrimSpace(r.prog.Text(pat))
	if strings.HasPrefix(txt, "{") {
		for _, el := range r.prog.Children(pat) {
			if node, ok := r.objectRestElem(el); ok {
				if name, ok := localName(r.prog.Text(node)); ok {
					out[name] = true
				}
				continue
			}
			if _, sub, ok := r.objectNestedElem(el); ok {
				r.collectDynRestNames(sub, out)
			}
		}
		return
	}
	if strings.HasPrefix(txt, "[") {
		elems := r.prog.Children(pat)
		fixed, restNode, hasRest, err := r.splitArrayRest(elems)
		if err != nil {
			return
		}
		if hasRest {
			if name, ok := localName(r.prog.Text(restNode)); ok {
				out[name] = true
			}
		}
		for _, el := range fixed {
			info, err := r.classifyArrayElem(el)
			if err != nil {
				continue
			}
			if info.nested != nil {
				r.collectDynRestNames(info.nested, out)
			}
		}
	}
}

// bindDynamicPattern binds a pattern against a dynamic receiver, reading each element
// through the boxed value's dynamic protocol and binding it as a value.Value. An object
// pattern reads its properties through Get and an array pattern its positions through
// GetIndex, and each carries the rename, default, computed key, rest, and nested forms
// the typed path carries, so an untyped pattern binds every shape a typed one does. tok
// is DEFINE for a declaration or parameter binding.
func (r *Renderer) bindDynamicPattern(pat frontend.Node, recv ast.Expr, tok token.Token) ([]ast.Stmt, error) {
	txt := strings.TrimSpace(r.prog.Text(pat))
	if strings.HasPrefix(txt, "{") {
		return r.bindDynamicObject(pat, recv, tok)
	}
	if strings.HasPrefix(txt, "[") {
		return r.bindDynamicArray(pat, recv, tok)
	}
	return nil, &NotYetLowerable{Reason: "an untyped destructuring pattern that is neither an object nor an array is a later slice"}
}

// throwIfNullish emits `if recv.IsNullish() { value.Throw(value.NewTypeError(...)) }`,
// the guard destructuring a nested sub-pattern against a nullish element needs.
// JavaScript reaches for the element's iterator when the sub-pattern is an array and
// coerces it to an object when the sub-pattern is an object, and both operations throw
// a TypeError on null or undefined before any name binds. The index-based dynamic read
// would instead yield undefined and silently bind past the error, so a nested element
// guards its held value first, matching node, which throws at that point. The message
// is not observed by the parity tests, which check the error's type alone.
func (r *Renderer) throwIfNullish(recv ast.Expr, msg string) ast.Stmt {
	r.usesThrow = true
	r.requireImport(valuePkg)
	typeErr := &ast.CallExpr{
		Fun: sel("value", "NewTypeError"),
		Args: []ast.Expr{&ast.CallExpr{
			Fun:  sel("value", "FromGoString"),
			Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(msg)}},
		}},
	}
	return &ast.IfStmt{
		Cond: &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("IsNullish")}},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.ExprStmt{X: &ast.CallExpr{Fun: sel("value", "Throw"), Args: []ast.Expr{typeErr}}},
		}},
	}
}

// blankDynBinding appends `_ = name` when the dynamic pattern member bound as name is
// a fresh declaration the body never reads. bindDynamicPattern reaches this binder only
// from a destructured parameter or a destructured catch clause, both of which introduce
// their names with := and would leave an unread member declared and not used in Go. The
// blank is withheld unless the bind uses DEFINE, so a destructuring assignment, which
// stores into existing names, is never touched, and bindingUnused only ever withholds
// the blank, so a member read elsewhere keeps its use.
func (r *Renderer) blankDynBinding(out []ast.Stmt, bindNode frontend.Node, name string, tok token.Token) []ast.Stmt {
	if tok != token.DEFINE {
		return out
	}
	return r.blankUnusedParamBinding(out, bindNode, name)
}

// bindDynamicArray binds an array pattern against a dynamic receiver. Each fixed
// position reads through GetIndex and binds its name as a value.Value, the same indexed
// read a dynamic element access lowers to, so an untyped array pattern reads its slots
// the way a typed one reads them through AtI. A defaulted element fills from its default
// when the position is undefined, and a nested element binds its inner pattern against
// the held position. A trailing rest gathers the source's tail past the fixed slots into
// a boxed value.Value array through IndexRest, and the rest name is marked dynamic so its
// body reads dispatch the boxed way rather than through a typed array's own methods. A
// hole is a later slice on this path and hands back.
func (r *Renderer) bindDynamicArray(pat frontend.Node, recv ast.Expr, tok token.Token) ([]ast.Stmt, error) {
	elems := r.prog.Children(pat)
	if len(elems) == 0 {
		return nil, &NotYetLowerable{Reason: "an empty array destructuring pattern binds nothing"}
	}
	fixed, restNode, hasRest, err := r.splitArrayRest(elems)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	var out []ast.Stmt
	for i, el := range fixed {
		info, err := r.classifyArrayElem(el)
		if err != nil {
			return nil, err
		}
		read := dynIndex(recv, i)
		if info.nested != nil {
			tmp := r.freshTemp()
			out = append(out, define(tmp, read))
			out = append(out, r.throwIfNullish(ident(tmp), "Cannot destructure a null or undefined array element"))
			inner, err := r.bindDynamicPattern(info.nested, ident(tmp), tok)
			if err != nil {
				return nil, err
			}
			out = append(out, inner...)
			continue
		}
		name, ok := localName(r.prog.Text(info.nameNode))
		if !ok {
			return nil, &NotYetLowerable{Reason: "an untyped destructuring target is not a Go identifier"}
		}
		if info.hasDefault {
			fill, err := r.dynDefaultFill(name, read, info.defNode, tok)
			if err != nil {
				return nil, err
			}
			out = append(out, fill...)
			out = r.blankDynBinding(out, info.nameNode, name, tok)
			continue
		}
		out = append(out, &ast.AssignStmt{Lhs: []ast.Expr{ident(name)}, Tok: tok, Rhs: []ast.Expr{read}})
		out = r.blankDynBinding(out, info.nameNode, name, tok)
	}
	// A trailing rest gathers the source's tail past the fixed slots into a boxed
	// value.Value array through IndexRest. The rest name is marked dynamic in
	// collectDynRestNames before the body lowers, so a body read of it dispatches the
	// boxed way rather than through a typed array's own methods a value.Value lacks.
	if hasRest {
		name, ok := localName(r.prog.Text(restNode))
		if !ok {
			return nil, &NotYetLowerable{Reason: "an untyped destructuring rest target is not a Go identifier"}
		}
		out = append(out, &ast.AssignStmt{Lhs: []ast.Expr{ident(name)}, Tok: tok, Rhs: []ast.Expr{dynIndexRest(recv, len(fixed))}})
		out = r.blankDynBinding(out, restNode, name, tok)
	}
	return out, nil
}

// bindDynamicObject binds an object pattern against a dynamic receiver. A shorthand or
// renamed property reads through Get by its source name and binds its target as a
// value.Value; a computed key reads through GetElem by the boxed key; a defaulted
// property fills from its default when the source is undefined; a nested property binds
// its inner pattern against the held source; and a trailing rest gathers the properties
// the pattern did not name through ObjectRest. Each read is emitted unconditionally, and a
// name the body never reads is blanked by blankDynBinding so the entry binding compiles.
func (r *Renderer) bindDynamicObject(pat frontend.Node, recv ast.Expr, tok token.Token) ([]ast.Stmt, error) {
	elems := r.prog.Children(pat)
	if len(elems) == 0 {
		return nil, &NotYetLowerable{Reason: "an empty object destructuring pattern binds nothing"}
	}
	r.requireImport(valuePkg)
	var out []ast.Stmt
	// A trailing rest gathers every own property the pattern did not name, so the names it
	// took are collected as they are bound and handed to ObjectRest as the keys to omit.
	var omit []ast.Expr
	computed := false
	for _, el := range elems {
		// A rest ({...rest}) gathers the remaining own properties into a new object. It is
		// syntactically last, so every named property is already in omit by the time it binds.
		if node, ok := r.objectRestElem(el); ok {
			if computed {
				return nil, &NotYetLowerable{Reason: "an object destructuring rest alongside a computed key cannot name the key to omit statically, a later slice"}
			}
			name, ok := localName(r.prog.Text(node))
			if !ok {
				return nil, &NotYetLowerable{Reason: "an untyped destructuring rest target is not a Go identifier"}
			}
			out = append(out, &ast.AssignStmt{Lhs: []ast.Expr{ident(name)}, Tok: tok, Rhs: []ast.Expr{dynObjectRest(recv, omit)}})
			out = r.blankDynBinding(out, node, name, tok)
			continue
		}
		// A nested property ({p: {x}}) reads the source into a temporary, then binds the inner
		// pattern against it, composing the dynamic read one level down.
		if source, sub, ok := r.objectNestedElem(el); ok {
			prop := strings.TrimSpace(r.prog.Text(source))
			tmp := r.freshTemp()
			out = append(out, define(tmp, dynGet(recv, prop)))
			out = append(out, r.throwIfNullish(ident(tmp), "Cannot destructure a null or undefined object property"))
			omit = append(omit, dynKey(prop))
			inner, err := r.bindDynamicPattern(sub, ident(tmp), tok)
			if err != nil {
				return nil, err
			}
			out = append(out, inner...)
			continue
		}
		// A computed key ({[k]: v}) reads the source by a key evaluated at run time, so the key
		// boxes to a value and reads through GetElem, the dynamic bracket read.
		if keyNode, valNode, ok := r.objectComputedElem(el); ok {
			key, err := r.boxOperand(keyNode)
			if err != nil {
				return nil, err
			}
			name, ok := localName(r.prog.Text(valNode))
			if !ok {
				return nil, &NotYetLowerable{Reason: "an untyped destructuring target is not a Go identifier"}
			}
			computed = true
			out = append(out, &ast.AssignStmt{Lhs: []ast.Expr{ident(name)}, Tok: tok, Rhs: []ast.Expr{dynGetElem(recv, key)}})
			out = r.blankDynBinding(out, valNode, name, tok)
			continue
		}
		info, err := r.classifyObjectElem(el)
		if err != nil {
			return nil, err
		}
		prop := strings.TrimSpace(r.prog.Text(info.nameNode))
		name, ok := localName(r.prog.Text(info.bindNode))
		if !ok {
			return nil, &NotYetLowerable{Reason: "an untyped destructuring target is not a Go identifier"}
		}
		omit = append(omit, dynKey(prop))
		read := dynGet(recv, prop)
		if info.hasDefault {
			fill, err := r.dynDefaultFill(name, read, info.defNode, tok)
			if err != nil {
				return nil, err
			}
			out = append(out, fill...)
			out = r.blankDynBinding(out, info.bindNode, name, tok)
			continue
		}
		out = append(out, &ast.AssignStmt{Lhs: []ast.Expr{ident(name)}, Tok: tok, Rhs: []ast.Expr{read}})
		out = r.blankDynBinding(out, info.bindNode, name, tok)
	}
	return out, nil
}

// dynDefaultFill binds name to read, then fills it from the default when the read is
// undefined, the dynamic mirror of a defaulted element: `name := read` followed by
// `if name.IsUndefined() { name = <boxed default> }`. The default boxes through the same
// operand path a dynamic value takes, so it evaluates only on the undefined branch the
// way JavaScript evaluates a default lazily and at most once.
func (r *Renderer) dynDefaultFill(name string, read ast.Expr, defNode frontend.Node, tok token.Token) ([]ast.Stmt, error) {
	boxed, err := r.boxOperand(defNode)
	if err != nil {
		return nil, err
	}
	return []ast.Stmt{
		&ast.AssignStmt{Lhs: []ast.Expr{ident(name)}, Tok: tok, Rhs: []ast.Expr{read}},
		&ast.IfStmt{
			Cond: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(name), Sel: ident("IsUndefined")}},
			Body: &ast.BlockStmt{List: []ast.Stmt{
				&ast.AssignStmt{Lhs: []ast.Expr{ident(name)}, Tok: token.ASSIGN, Rhs: []ast.Expr{boxed}},
			}},
		},
	}, nil
}

// objectRestElem returns the identifier a trailing object-pattern rest element binds,
// `...rest`, and whether the element is a rest at all. A rest reads by its leading `...`
// token, and its bound name is the element's identifier child.
func (r *Renderer) objectRestElem(el frontend.Node) (frontend.Node, bool) {
	if !strings.HasPrefix(strings.TrimSpace(r.prog.Text(el)), "...") {
		return nil, false
	}
	for _, c := range r.prog.Children(el) {
		if c.Kind() == frontend.NodeIdentifier {
			return c, true
		}
	}
	return nil, false
}

// objectComputedElem reports whether an object binding pattern element reads a computed
// key into a target, `{[k]: v}`, and returns the key expression and the target. Such an
// element has the computed-key node and the target as its two children, the computed-key
// node opening with a `[`.
func (r *Renderer) objectComputedElem(el frontend.Node) (key, target frontend.Node, ok bool) {
	ec := r.prog.Children(el)
	if len(ec) != 2 || !strings.HasPrefix(strings.TrimSpace(r.prog.Text(ec[0])), "[") {
		return nil, nil, false
	}
	inner := r.prog.Children(ec[0])
	if len(inner) != 1 {
		return nil, nil, false
	}
	return inner[0], ec[1], true
}

// define builds `name := rhs`, the declaration form the dynamic binder uses for a
// temporary that holds a selected slot before an inner pattern reads it.
func define(name string, rhs ast.Expr) ast.Stmt {
	return &ast.AssignStmt{Lhs: []ast.Expr{ident(name)}, Tok: token.DEFINE, Rhs: []ast.Expr{rhs}}
}

// dynKey builds value.FromGoString("prop"), the boxed property key a dynamic read and
// ObjectRest's omit list both take.
func dynKey(prop string) ast.Expr {
	return &ast.CallExpr{Fun: sel("value", "FromGoString"), Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(prop)}}}
}

// dynGet reads a property off a dynamic receiver, recv.Get(value.FromGoString("prop")),
// the same boxed property read a dynamic member access lowers to.
func dynGet(recv ast.Expr, prop string) ast.Expr {
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Get")}, Args: []ast.Expr{dynKey(prop)}}
}

// dynGetElem reads a property off a dynamic receiver by a boxed key, recv.GetElem(key),
// the dynamic bracket read a computed-key element takes when the key is known only at run
// time.
func dynGetElem(recv, key ast.Expr) ast.Expr {
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("GetElem")}, Args: []ast.Expr{key}}
}

// dynIndex reads a position off a dynamic receiver, recv.GetIndex(i), the same boxed
// indexed read a dynamic element access lowers to. GetIndex takes a float64, and an
// untyped integer constant converts to it, so the position is emitted as an int literal.
func dynIndex(recv ast.Expr, i int) ast.Expr {
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("GetIndex")}, Args: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(i)}}}
}

// dynIndexRest gathers a dynamic receiver's tail from a start index, recv.IndexRest(from),
// the boxed array an untyped array pattern's trailing rest binds. IndexRest takes a
// float64, and an untyped integer constant converts to it, so the start is emitted as an
// int literal the way dynIndex emits a position.
func dynIndexRest(recv ast.Expr, from int) ast.Expr {
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("IndexRest")}, Args: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(from)}}}
}

// dynObjectRest gathers a dynamic receiver's remaining own properties, recv.ObjectRest(keys...),
// omitting the keys the pattern already named, the boxed object an object rest element binds.
func dynObjectRest(recv ast.Expr, omit []ast.Expr) ast.Expr {
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("ObjectRest")}, Args: omit}
}
