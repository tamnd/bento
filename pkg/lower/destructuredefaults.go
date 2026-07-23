package lower

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// A default on a destructuring pattern element fills the target when the source
// slot is undefined and only then, and JavaScript evaluates the default at most
// once and lazily, so a default that calls a function or reads another binding
// runs solely on the undefined path. This file lowers that fill for the array and
// object declaration forms; the assignment and parameter forms reuse the same
// shape from their own paths.

// arrayDefaultElem describes one element of an array binding pattern once its
// shape is classified: a plain name binds the slot directly, a defaulted name
// fills from its default when the slot is undefined. nameNode carries the
// binding's type and defNode the default expression, present only when hasDefault.
type arrayDefaultElem struct {
	name       string
	nameNode   frontend.Node
	hasDefault bool
	defNode    frontend.Node
	nested     frontend.Node
}

// classifyArrayElem reads one array binding pattern element into an
// arrayDefaultElem. A single identifier child is a plain name; a single array or
// object pattern child is a nested pattern, `[[a, b]]` or `[{x}]`, whose inner
// pattern binds against the slot the outer element selects; an identifier followed
// by an expression is a defaulted name, `[a = d]`. A hole or a rest is a later
// slice, so it hands back rather than mislowering.
func (r *Renderer) classifyArrayElem(el frontend.Node) (arrayDefaultElem, error) {
	ec := r.prog.Children(el)
	switch {
	case len(ec) == 1 && ec[0].Kind() == frontend.NodeIdentifier:
		return arrayDefaultElem{nameNode: ec[0]}, nil
	case len(ec) == 1 && r.patternNode(ec[0]):
		return arrayDefaultElem{nested: ec[0]}, nil
	case len(ec) == 2 && ec[0].Kind() == frontend.NodeIdentifier:
		if r.defaultNeedsNamedEvaluation(ec[1]) {
			return arrayDefaultElem{}, namedEvaluationHandBack
		}
		return arrayDefaultElem{nameNode: ec[0], hasDefault: true, defNode: ec[1]}, nil
	default:
		return arrayDefaultElem{}, &NotYetLowerable{Reason: "an array destructuring hole or rest is a later slice"}
	}
}

// namedEvaluationHandBack is the reason a destructuring element hands back when its
// default is an anonymous function or arrow. JavaScript's NamedEvaluation gives such a
// default the binding's own name, so `[fn = function () {}]` makes fn.name === "fn".
// bento models a function as a bare Go func with no name slot, so it cannot carry that
// inferred name; a later read of it would fold to undefined, a wrong answer. The
// element hands back rather than bind a value whose reflective name is wrong.
var namedEvaluationHandBack = &NotYetLowerable{Reason: "a destructuring default that is an anonymous function or arrow takes the binding's name by NamedEvaluation, a reflective name the static function model does not host"}

// defaultNeedsNamedEvaluation reports whether a destructuring element default is an
// anonymous function or arrow, the NamedEvaluation case above. A named function
// expression keeps its own name and is not renamed, so it is not this case; only an
// arrow, which has no name, and a function expression with no name qualify.
func (r *Renderer) defaultNeedsNamedEvaluation(defNode frontend.Node) bool {
	// A cover default parenthesizes its function, `cover = (function () {})`, and the
	// parentheses are transparent to NamedEvaluation, so the wrapper is peeled to reach
	// the function it covers before the kind is judged.
	for defNode.Kind() == frontend.NodeParenthesizedExpression {
		kids := r.prog.Children(defNode)
		if len(kids) != 1 {
			return false
		}
		defNode = kids[0]
	}
	switch defNode.Kind() {
	case frontend.NodeArrowFunction:
		return true
	case frontend.NodeFunctionExpression:
		_, named := r.funcExprNameNode(defNode)
		return !named
	default:
		return false
	}
}

// objectDefaultElem describes one element of an object binding pattern once its
// shape is classified: a plain shorthand name binds the property of the same name,
// a defaulted shorthand name fills from its default when the property is undefined,
// and a rename ({a: b}) feeds a source property into a differently named target.
// nameNode is the source property, read for the field and the optional flag; bindNode
// is the local the value binds to. They are the same node for a shorthand and differ
// only for a rename.
type objectDefaultElem struct {
	nameNode   frontend.Node
	bindNode   frontend.Node
	hasDefault bool
	defNode    frontend.Node
	keyName    string
}

// elemSourceProp is the source property an object binding element reads: the resolved
// constant string of a computed const key when the element carries one, else the text of
// its name node. A computed key [k] whose k is a const of a literal string type folds to
// that string here, so `const { [k]: v } = o` reads the same o.a field `const { a: v } = o`
// does, while a non-computed element keeps reading through its name node unchanged.
func (r *Renderer) elemSourceProp(info objectDefaultElem) string {
	if info.keyName != "" {
		return info.keyName
	}
	return strings.TrimSpace(r.prog.Text(info.nameNode))
}

// classifyObjectElem reads one object binding pattern element into an
// objectDefaultElem. A single identifier is a plain shorthand, `{x}`; two
// identifiers under a `:` separator are a rename, `{x: a}`, whose source property is
// the first and whose target is the second; an identifier followed by an expression
// under an `=` separator is a shorthand default, `{x = d}`; a source property, a
// target, and a default under `:` then `=` are a renamed default, `{x: a = d}`. A
// computed key, a rest, or a nested pattern is a later slice, so it hands back. The
// separator between the name and the second child tells a default (`=`) from a rename
// (`:`), which the child kinds alone cannot when the default is itself an identifier.
func (r *Renderer) classifyObjectElem(el frontend.Node) (objectDefaultElem, error) {
	// A rest property gathers the own enumerable properties the pattern did not name
	// into a new object, which needs the object model to enumerate a value's own keys,
	// a phase 7 capability. It hands back explicitly rather than emit a partial gather.
	if strings.HasPrefix(strings.TrimSpace(r.prog.Text(el)), "...") {
		return objectDefaultElem{}, &NotYetLowerable{Reason: "an object destructuring rest property gathers the remaining own properties into an object, which needs the object model of phase 7"}
	}
	ec := r.prog.Children(el)
	// A computed key ({[k]: v}) whose key the checker proved a constant string folds to
	// that string, so the element reads the fixed-shape source's field by the folded name
	// the same way a named property does: `const { [k]: v } = o` with `const k = "a"` reads
	// o.a. The resolved name is carried on keyName so the static readers select the field
	// by it rather than by the key expression's text. Only a plain `[k]: v` shape folds, an
	// identifier target and no default; a computed key with a default or a non-identifier
	// target has more than two children, so objectComputedElem declines it. A key with no
	// constant string type (a wide string, a side-effecting expression, a symbol, a number)
	// has no static field to select and hands back, which keeps a run-time key over a static
	// struct a later slice, since the dynamic-source path serves it through GetElem instead.
	if key, target, ok := r.objectComputedElem(el); ok {
		name, ok := r.pureConstStringKey(key)
		if !ok {
			return objectDefaultElem{}, &NotYetLowerable{Reason: "an object destructuring computed key whose key is not a constant string reads the source by a key computed at run time, which needs the dynamic object model of phase 7"}
		}
		return objectDefaultElem{nameNode: key, bindNode: target, keyName: name}, nil
	}
	if len(ec) > 0 && strings.HasPrefix(strings.TrimSpace(r.prog.Text(ec[0])), "[") {
		return objectDefaultElem{}, &NotYetLowerable{Reason: "an object destructuring computed key with a default or a non-identifier target is a later slice"}
	}
	switch {
	case len(ec) == 1 && ec[0].Kind() == frontend.NodeIdentifier:
		return objectDefaultElem{nameNode: ec[0], bindNode: ec[0]}, nil
	case len(ec) == 2 && ec[0].Kind() == frontend.NodeIdentifier && ec[1].Kind() == frontend.NodeIdentifier && strings.Contains(r.childGap(el, ec[0], ec[1]), ":"):
		return objectDefaultElem{nameNode: ec[0], bindNode: ec[1]}, nil
	case len(ec) == 2 && ec[0].Kind() == frontend.NodeIdentifier && !strings.Contains(r.childGap(el, ec[0], ec[1]), ":"):
		if r.defaultNeedsNamedEvaluation(ec[1]) {
			return objectDefaultElem{}, namedEvaluationHandBack
		}
		return objectDefaultElem{nameNode: ec[0], bindNode: ec[0], hasDefault: true, defNode: ec[1]}, nil
	case len(ec) == 3 && ec[0].Kind() == frontend.NodeIdentifier && ec[1].Kind() == frontend.NodeIdentifier && strings.Contains(r.childGap(el, ec[0], ec[1]), ":") && strings.Contains(r.childGap(el, ec[1], ec[2]), "="):
		if r.defaultNeedsNamedEvaluation(ec[2]) {
			return objectDefaultElem{}, namedEvaluationHandBack
		}
		return objectDefaultElem{nameNode: ec[0], bindNode: ec[1], hasDefault: true, defNode: ec[2]}, nil
	default:
		return objectDefaultElem{}, &NotYetLowerable{Reason: "an object destructuring nested pattern is a later slice"}
	}
}

// memberAssignTarget lowers a destructuring assignment target that is a property
// access on a fixed-shape object, o.a, to the Go selector that names its field, so a
// destructured element or property can store into it. It hands back for a member the
// static struct model cannot assign into as a Go lvalue: a dynamic receiver, an array
// or typed-array element, a map, a set, a class instance whose field write the class
// path owns, or a property the shape never declared. Each of those needs a runtime
// store rather than a Go field selector.
func (r *Renderer) memberAssignTarget(tgt frontend.Node) (ast.Expr, error) {
	tParts := r.prog.Children(tgt)
	if len(tParts) != 2 {
		return nil, &NotYetLowerable{Reason: "a destructuring assignment into an unusual member target is a later slice"}
	}
	obj := tParts[0]
	if r.isDynamic(obj) {
		return nil, &NotYetLowerable{Reason: "a destructuring assignment into a property of a dynamic receiver needs the runtime store, a later slice"}
	}
	objType := r.prog.TypeAt(obj)
	if objType.Flags&frontend.TypeObject == 0 {
		return nil, &NotYetLowerable{Reason: "a destructuring assignment into a property of a non-object receiver is a later slice"}
	}
	if _, isArray := r.prog.ElementType(objType); isArray {
		return nil, &NotYetLowerable{Reason: "a destructuring assignment into an array-element member target is a later slice"}
	}
	if r.isTypedArray(obj) || r.isMap(obj) || r.isSet(obj) {
		return nil, &NotYetLowerable{Reason: "a destructuring assignment into a typed-array, map, or set member target is a later slice"}
	}
	if _, ok := r.classReceiver(obj); ok {
		return nil, &NotYetLowerable{Reason: "a destructuring assignment into a class-instance field is a later slice"}
	}
	if _, present := r.shapeProp(objType, r.prog.Text(tParts[1])); !present {
		return nil, &NotYetLowerable{Reason: "a destructuring assignment into a property the fixed-shape object never declared needs the object's runtime shape, a later slice"}
	}
	return r.lowerExpr(tgt)
}

// arrayAssignElem describes one target of an array destructuring assignment,
// `[a, b = d] = rhs`. Unlike the declaration pattern, whose element wraps its
// binding, an assignment target is the identifier itself, or an `a = d` assignment
// expression when it carries a default.
type arrayAssignElem struct {
	nameNode   frontend.Node
	hasDefault bool
	defNode    frontend.Node
	memberNode frontend.Node
}

// classifyArrayAssignElem reads one array assignment target into an arrayAssignElem.
// A bare identifier is a plain target; an `a = d` binary expression is a defaulted
// target; a property access `o.a` is a member target the element stores into. A hole,
// a rest, a nested pattern, or an element-access target is a later slice.
func (r *Renderer) classifyArrayAssignElem(tgt frontend.Node) (arrayAssignElem, error) {
	if tgt.Kind() == frontend.NodeIdentifier {
		return arrayAssignElem{nameNode: tgt}, nil
	}
	if tgt.Kind() == frontend.NodePropertyAccessExpression {
		return arrayAssignElem{memberNode: tgt}, nil
	}
	c := r.prog.Children(tgt)
	if tgt.Kind() == frontend.NodeBinaryExpression && len(c) == 3 && r.prog.Text(c[1]) == "=" && c[0].Kind() == frontend.NodeIdentifier {
		return arrayAssignElem{nameNode: c[0], hasDefault: true, defNode: c[2]}, nil
	}
	return arrayAssignElem{}, &NotYetLowerable{Reason: "an array assignment hole, rest, nested pattern, or element-access target is a later slice"}
}

// objectAssignElem describes one target of an object destructuring assignment,
// `({x, y = d} = o)`. A plain shorthand property has a single identifier child; a
// rename ({x: a}) has the source property and the target identifier under a `:`; a
// defaulted shorthand has the identifier, an `=` separator, and the default. nameNode
// is the source property, read for the field and the optional flag; bindNode is the
// existing target the value stores into, the same node for a shorthand and different
// only for a rename.
type objectAssignElem struct {
	nameNode   frontend.Node
	bindNode   frontend.Node
	hasDefault bool
	defNode    frontend.Node
	bindMember frontend.Node
	rest       bool
	restStruct string
	restType   frontend.Type
}

// classifyObjectAssignElem reads one object assignment property into an
// objectAssignElem. A single identifier is a plain shorthand; two identifiers under a
// `:` separator are a rename, `{x: a}`; a source property under `:` over an `a = d`
// assignment is a renamed default, `{x: a = d}`; an identifier, an `=`, and an
// expression is a shorthand default. A computed key, a rest, or a nested pattern is a
// later slice.
func (r *Renderer) classifyObjectAssignElem(prop frontend.Node) (objectAssignElem, error) {
	if strings.HasPrefix(strings.TrimSpace(r.prog.Text(prop)), "...") {
		return objectAssignElem{}, &NotYetLowerable{Reason: "an object assignment rest property gathers the remaining own properties into an object, which needs the object model of phase 7"}
	}
	pc := r.prog.Children(prop)
	// A computed key reads the source by a run-time key, which needs the dynamic object
	// model of phase 7 the same way the declaration form does.
	if len(pc) > 0 && strings.HasPrefix(strings.TrimSpace(r.prog.Text(pc[0])), "[") {
		return objectAssignElem{}, &NotYetLowerable{Reason: "an object assignment computed key reads the source by a key computed at run time, which needs the dynamic object model of phase 7"}
	}
	switch {
	case len(pc) == 1 && pc[0].Kind() == frontend.NodeIdentifier:
		return objectAssignElem{nameNode: pc[0], bindNode: pc[0]}, nil
	case len(pc) == 2 && pc[0].Kind() == frontend.NodeIdentifier && pc[1].Kind() == frontend.NodeIdentifier && strings.Contains(r.childGap(prop, pc[0], pc[1]), ":"):
		return objectAssignElem{nameNode: pc[0], bindNode: pc[1]}, nil
	case len(pc) == 2 && pc[0].Kind() == frontend.NodeIdentifier && pc[1].Kind() == frontend.NodePropertyAccessExpression && strings.Contains(r.childGap(prop, pc[0], pc[1]), ":"):
		// A source property renamed onto a member target, {a: o.a}: the value stores
		// into the property access rather than a fresh local.
		return objectAssignElem{nameNode: pc[0], bindMember: pc[1]}, nil
	case len(pc) == 2 && pc[0].Kind() == frontend.NodeIdentifier && pc[1].Kind() == frontend.NodeBinaryExpression && strings.Contains(r.childGap(prop, pc[0], pc[1]), ":"):
		// A rename carrying a default, {x: a = d}: the target and default parse as an
		// `a = d` assignment under the source property's colon.
		bc := r.prog.Children(pc[1])
		if len(bc) == 3 && bc[0].Kind() == frontend.NodeIdentifier && r.prog.Text(bc[1]) == "=" {
			return objectAssignElem{nameNode: pc[0], bindNode: bc[0], hasDefault: true, defNode: bc[2]}, nil
		}
		return objectAssignElem{}, &NotYetLowerable{Reason: "an object assignment nested pattern is a later slice"}
	case len(pc) == 3 && pc[0].Kind() == frontend.NodeIdentifier && r.prog.Text(pc[1]) == "=":
		return objectAssignElem{nameNode: pc[0], bindNode: pc[0], hasDefault: true, defNode: pc[2]}, nil
	default:
		return objectAssignElem{}, &NotYetLowerable{Reason: "an object assignment nested pattern is a later slice"}
	}
}

// childGap returns the source text between two adjacent children of a pattern
// element, the operator that joins them: `=` for a default, `:` for a rename. It
// slices the parent element's own text between the first child's text and the
// second's, so it sees only the joining token and never the second expression's own
// text, which may itself contain a colon. Working within the element's text sidesteps
// the leading-trivia skew a raw file-absolute span read carries, where an
// identifier's end can reach past the joining token depending on the surrounding
// whitespace. It returns "" when either child's text is not found in order, so a
// caller reading it for a `:` treats an unreadable gap as not a rename.
func (r *Renderer) childGap(el, first, second frontend.Node) string {
	txt := r.prog.Text(el)
	ft, st := r.prog.Text(first), r.prog.Text(second)
	_, rest, ok := strings.Cut(txt, ft)
	if !ok {
		return ""
	}
	gap, _, ok := strings.Cut(rest, st)
	if !ok {
		return ""
	}
	return gap
}

// defaultFillStmts emits the lazy default fill for one binding: the target is
// declared with its own type, the source slot is read once through a bounds-aware
// AtOpt into a temporary, and the default rides the undefined branch while the
// present branch takes the read value. The default is lowered by the caller so it
// is only placed on the undefined path, evaluating at most once and only when the
// slot is missing, the order JavaScript's default fill takes.
func (r *Renderer) defaultFillStmts(name string, nameGo ast.Expr, read ast.Expr, def ast.Expr) []ast.Stmt {
	decl := &ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{
		Names: []*ast.Ident{ident(name)},
		Type:  nameGo,
	}}}}
	return []ast.Stmt{decl, r.defaultFillAssign(ident(name), read, def)}
}

// defaultFillAssign emits the lazy default fill for a target that is already
// declared, the assignment sibling of defaultFillStmts: it assigns rather than
// declares, so the destructuring assignment forms (`[a = d] = rhs`,
// `({x = d} = o)`) reuse the same undefined-then-default shape without minting a
// new local. The source slot is read once into a temporary, the default rides the
// undefined branch, and the present branch takes the read value.
func (r *Renderer) defaultFillAssign(target ast.Expr, read ast.Expr, def ast.Expr) *ast.IfStmt {
	opt := r.freshTemp()
	present := &ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(opt), Sel: ident("Get")}}
	return &ast.IfStmt{
		Init: &ast.AssignStmt{Lhs: []ast.Expr{ident(opt)}, Tok: token.DEFINE, Rhs: []ast.Expr{read}},
		Cond: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(opt), Sel: ident("IsUndefined")}},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{target}, Tok: token.ASSIGN, Rhs: []ast.Expr{def}}}},
		Else: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{target}, Tok: token.ASSIGN, Rhs: []ast.Expr{present}}}},
	}
}

// defaultFillBoxStmts is the bare-box sibling of defaultFillStmts: the source slot is a
// dynamic value.Value that carries its own undefined rather than a value.Opt, so the
// present branch takes the read value directly instead of peeling it with Get. It serves
// an optional narrowable-box field ({} or a string-index dictionary), whose optional
// collapses into the box rather than wrapping it in an Opt, so the read is a value.Value
// on which IsUndefined answers but Get is the two-argument property getter, not the
// no-argument Opt unwrap. The default rides the undefined branch and evaluates at most
// once, the same order defaultFillStmts takes.
func (r *Renderer) defaultFillBoxStmts(name string, nameGo, read, def ast.Expr) []ast.Stmt {
	decl := &ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{
		Names: []*ast.Ident{ident(name)},
		Type:  nameGo,
	}}}}
	tmp := r.freshTemp()
	fill := &ast.IfStmt{
		Init: &ast.AssignStmt{Lhs: []ast.Expr{ident(tmp)}, Tok: token.DEFINE, Rhs: []ast.Expr{read}},
		Cond: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(tmp), Sel: ident("IsUndefined")}},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ident(name)}, Tok: token.ASSIGN, Rhs: []ast.Expr{def}}}},
		Else: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ident(name)}, Tok: token.ASSIGN, Rhs: []ast.Expr{ident(tmp)}}}},
	}
	return []ast.Stmt{decl, fill}
}

// defaultFillUnionStmts emits the lazy default fill for a binding whose optional
// property is a multi-member tagged-sum union (number | string | undefined) rather
// than a value.Opt. Such a field carries the union struct directly, its undefined arm
// standing for the absent member, so the Opt protocol defaultFillStmts reads
// (IsUndefined, Get) does not exist on it. The fill switches the read's discriminant
// instead: the undefined arm takes the default, and each value arm rebuilds the
// binding's undefined-stripped union from that arm's own field. The binding type is
// the source union without its undefined arm, itself a tagged sum, so each value arm
// maps to the target union's matching arm constructor. The default is placed only on
// the undefined case, so it evaluates at most once and only when the slot is missing,
// the order JavaScript's default fill takes.
func (r *Renderer) defaultFillUnionStmts(name string, nameGo, read, def ast.Expr, src, tgt *unionInfo) []ast.Stmt {
	decl := &ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{
		Names: []*ast.Ident{ident(name)},
		Type:  nameGo,
	}}}}
	tmp := r.freshTemp()
	var cases []ast.Stmt
	for _, a := range src.arms {
		var rhs ast.Expr
		switch {
		case a.flag == frontend.TypeUndefined:
			rhs = def
		case a.tagOnly:
			// A tag-only sentinel that is not the undefined member (a null arm) has no
			// field to carry over, so the target's matching arm takes its no-argument
			// constructor.
			ta, ok := tgt.armForFlags(a.flag)
			if !ok {
				continue
			}
			rhs = &ast.CallExpr{Fun: ident(tgt.ctorName(ta))}
		default:
			ta, ok := tgt.armForFlags(a.flag)
			if !ok {
				continue
			}
			rhs = &ast.CallExpr{Fun: ident(tgt.ctorName(ta)), Args: []ast.Expr{&ast.SelectorExpr{X: ident(tmp), Sel: ident(a.field)}}}
		}
		cases = append(cases, &ast.CaseClause{
			List: []ast.Expr{ident(src.tagConst(a))},
			Body: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ident(name)}, Tok: token.ASSIGN, Rhs: []ast.Expr{rhs}}},
		})
	}
	sw := &ast.SwitchStmt{
		Init: &ast.AssignStmt{Lhs: []ast.Expr{ident(tmp)}, Tok: token.DEFINE, Rhs: []ast.Expr{read}},
		Tag:  &ast.SelectorExpr{X: ident(tmp), Sel: ident("tag")},
		Body: &ast.BlockStmt{List: cases},
	}
	return []ast.Stmt{decl, sw}
}

// arrayOptRead builds the bounds-aware read for a defaulted array element,
// recv.AtOpt(i), whose Opt is undefined exactly when the source has no element at
// that index. It is the read defaultFillStmts tests, the optional sibling of the
// plain AtI read a non-defaulted element takes.
func arrayOptRead(recv ast.Expr, index int) ast.Expr {
	return &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: recv, Sel: ident("AtOpt")},
		Args: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(index)}},
	}
}
