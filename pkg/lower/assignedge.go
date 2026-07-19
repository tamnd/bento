package lower

import (
	"go/ast"
	"go/token"
	"strconv"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers the assignment edge cases M4 left handed back: a computed
// member target on a dynamic receiver, both the plain store and the compound
// read-modify-write, and a compound assignment read for its value. The plain
// element store and the identifier assignment-as-value already lower elsewhere;
// what remains is the computed target and the compound result, which need the
// runtime element store and the boxed dynamic arithmetic the value model gives.

// elementStoreMethod picks the runtime store method for a bracket write o[k] = v
// on a dynamic receiver by the key's own type, the write mirror of the dynamic
// element read: a number index writes through SetIndex, another dynamic value
// through SetElem, and a string key through SetKey, so o[3] = x lands in an array
// element and o["k"] = x in a named property by the same rule the read resolves
// them. A key that is none of these is its own later slice.
func (r *Renderer) elementStoreMethod(idxNode frontend.Node) (string, error) {
	switch {
	case r.isNumber(idxNode):
		return "SetIndex", nil
	case r.isDynamic(idxNode):
		return "SetElem", nil
	case r.isString(idxNode):
		return "SetKey", nil
	case r.isSymbolKey(idxNode):
		return "SetElem", nil
	default:
		return "", &NotYetLowerable{Reason: "a dynamic element write with a non-number, non-string index is a later slice"}
	}
}

// assignValueElement lowers a computed-member assignment read for its value,
// (o[k] = v) in an expression position, on a dynamic receiver. The runtime store
// returns the boxed assigned value, so when the whole expression is itself
// dynamic the bare store call both writes and yields it, evaluating the receiver,
// key, and value once in source order. When the checker types the expression a
// static primitive, that being the assigned value's type, the box is read back to
// its static form through the same coercion a dynamic read into that slot takes,
// still a single evaluation. A non-dynamic receiver has no runtime element slot
// this path writes and hands back, the wall the static array element write hits.
func (r *Renderer) assignValueElement(n, left, right frontend.Node) (ast.Expr, error) {
	parts := r.prog.Children(left)
	if len(parts) != 2 {
		return nil, &NotYetLowerable{Reason: "element assignment value target did not expose a receiver and an index"}
	}
	recvNode, idxNode := parts[0], parts[1]
	if !r.isDynamic(recvNode) {
		return nil, &NotYetLowerable{Reason: "assignment value into a statically typed element needs the object descriptor model, a later slice"}
	}
	method, err := r.elementStoreMethod(idxNode)
	if err != nil {
		return nil, err
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	idx, err := r.lowerExpr(idxNode)
	if err != nil {
		return nil, err
	}
	val, err := r.boxOperand(right)
	if err != nil {
		return nil, err
	}
	store := &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(method)}, Args: []ast.Expr{idx, val}}
	if r.isDynamic(n) {
		return store, nil
	}
	return r.coerceDynamicToStatic(store, n)
}

// elementLoadMethod picks the runtime load method for a bracket read o[k] on a
// dynamic receiver, the load half a compound member write needs, by the same key
// type split elementStoreMethod uses: a number reads through GetIndex, a dynamic
// value through GetElem, and a string through Get, so the loaded slot is the one
// the matching store writes back.
func (r *Renderer) elementLoadMethod(idxNode frontend.Node) (string, error) {
	switch {
	case r.isNumber(idxNode):
		return "GetIndex", nil
	case r.isDynamic(idxNode):
		return "GetElem", nil
	case r.isString(idxNode):
		return "Get", nil
	case r.isSymbolKey(idxNode):
		return "GetElem", nil
	default:
		return "", &NotYetLowerable{Reason: "a dynamic element read with a non-number, non-string index is a later slice"}
	}
}

// assignValueCompound lowers a compound assignment read for its value, (x += 1)
// in an expression position, whose result is the updated value the statement path
// discards. It reuses the statement lowering to build the read-modify-write, then
// wraps it in a closure that runs the store and returns the target, so the whole
// expression evaluates to the value JavaScript's compound assignment yields. A
// refined-integer local returns an int a float64 context would mismatch, so it
// hands back, and a member target hands back inside lowerAssign, each a later
// slice. A dynamic or narrowed-storage local keeps its value in a box, so the
// closure returns that box as a value.Value and the whole read coerces down to a
// static primitive when the expression's context is one.
func (r *Renderer) assignValueCompound(n, left frontend.Node) (ast.Expr, error) {
	if left.Kind() == frontend.NodePropertyAccessExpression || left.Kind() == frontend.NodeElementAccessExpression {
		return r.assignValueMemberCompound(n, left)
	}
	if left.Kind() != frontend.NodeIdentifier {
		return nil, &NotYetLowerable{Reason: "compound assignment value on a non-identifier target is a later slice"}
	}
	name, ok := localName(r.prog.Text(left))
	if !ok {
		return nil, &NotYetLowerable{Reason: "compound assignment value target is not a Go identifier"}
	}
	if r.int32Locals[name] || r.int64Locals[name] {
		return nil, &NotYetLowerable{Reason: "compound assignment value on a refined-integer local is a later slice"}
	}
	stmt, err := r.lowerAssign(n)
	if err != nil {
		return nil, err
	}
	// A local whose Go storage is a box (a dynamic local, or one the compiler boxed
	// to hold a wide type) holds a value.Value, not the narrowed static type the
	// checker gives the identifier here. The closure returns that box, and when the
	// whole compound expression sits in a static-primitive context the boxed read
	// coerces down through the ToNumber family, the same coercion a boxed member read
	// takes; a dynamic context keeps the box.
	if r.isDynamic(left) || r.localStorageDynamic(left) {
		r.requireImport(valuePkg)
		body := []ast.Stmt{stmt, &ast.ReturnStmt{Results: []ast.Expr{ident(name)}}}
		closure := r.valueClosure(sel("value", "Value"), body)
		if r.isDynamic(n) {
			return closure, nil
		}
		return r.coerceDynamicToStatic(closure, n)
	}
	retType, err := r.typeExpr(r.prog.TypeAt(left))
	if err != nil {
		return nil, err
	}
	body := []ast.Stmt{stmt, &ast.ReturnStmt{Results: []ast.Expr{ident(name)}}}
	return r.valueClosure(retType, body), nil
}

// assignValueMemberCompound lowers a compound assignment read for its value on a
// member or element target, (o.k += v) or (o[k] += v) in an expression position.
// It reuses the statement lowering to build the read-modify-write store, then
// re-reads the target to yield the updated value the language's compound
// assignment evaluates to. bento objects carry no getters on these paths, so the
// re-read returns exactly what the store wrote. The store already evaluates the
// receiver (and, for an element, the key) once; the re-read evaluates it again, so
// a side-effecting receiver or key hands back to keep it evaluated once, the same
// guard the dynamic member and element statement stores use. A static array
// element compound has no statement lowering yet, so lowerUpdate hands it back and
// this propagates it. A dynamic target returns a value.Value box that coerces down
// when the whole expression sits in a static-primitive context.
func (r *Renderer) assignValueMemberCompound(n, left frontend.Node) (ast.Expr, error) {
	kids := r.prog.Children(left)
	if len(kids) < 1 {
		return nil, &NotYetLowerable{Reason: "compound assignment value target did not expose a receiver"}
	}
	if !r.repeatableOperand(kids[0]) {
		return nil, &NotYetLowerable{Reason: "compound assignment value on a member with a side-effecting receiver is a later slice"}
	}
	if left.Kind() == frontend.NodeElementAccessExpression {
		if len(kids) != 2 {
			return nil, &NotYetLowerable{Reason: "compound assignment value target did not expose a receiver and a key"}
		}
		if !r.repeatableOperand(kids[1]) {
			return nil, &NotYetLowerable{Reason: "compound assignment value on an element with a side-effecting key is a later slice"}
		}
	}
	stmt, err := r.lowerUpdate(n)
	if err != nil {
		return nil, err
	}
	read, err := r.lowerExpr(left)
	if err != nil {
		return nil, err
	}
	if r.isDynamic(left) {
		r.requireImport(valuePkg)
		body := []ast.Stmt{stmt, &ast.ReturnStmt{Results: []ast.Expr{read}}}
		closure := r.valueClosure(sel("value", "Value"), body)
		if r.isDynamic(n) {
			return closure, nil
		}
		return r.coerceDynamicToStatic(closure, n)
	}
	retType, err := r.typeExpr(r.prog.TypeAt(left))
	if err != nil {
		return nil, err
	}
	body := []ast.Stmt{stmt, &ast.ReturnStmt{Results: []ast.Expr{read}}}
	return r.valueClosure(retType, body), nil
}

// memberLogicalAssign lowers a logical assignment on a member target, o.k ??= v
// and o.k ||= v and o.k &&= v, on a dynamic receiver. Like the identifier form it
// short-circuits: the receiver's property is loaded, the operator's trigger is
// tested, and the boxed right-hand side is stored only when the trigger fires, so
// a right-hand side with a side effect runs only when it should. The receiver is
// read on both the guard load and the store, so a side-effecting receiver hands
// back to keep it evaluated once; a repeatable one reads the same object on both.
// A statically typed receiver has no runtime slot to load and store and hands back
// for the object descriptor model, the wall the dotted member delete hits.
func (r *Renderer) memberLogicalAssign(target frontend.Node, op string, value frontend.Node) (ast.Stmt, bool, error) {
	kids := r.prog.Children(target)
	if len(kids) != 2 {
		return nil, true, &NotYetLowerable{Reason: "logical assignment target did not expose an object and a property name"}
	}
	obj, nameNode := kids[0], kids[1]
	if !r.isDynamic(obj) {
		return nil, true, &NotYetLowerable{Reason: "logical assignment to a statically typed property needs the object descriptor model, a later slice"}
	}
	if !r.repeatableOperand(obj) {
		return nil, true, &NotYetLowerable{Reason: "logical assignment to a member with a side-effecting receiver is a later slice"}
	}
	r.requireImport(valuePkg)
	key := func() ast.Expr {
		return &ast.CallExpr{Fun: sel("value", "FromGoString"), Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(r.prog.Text(nameNode))}}}
	}
	recvLoad, err := r.lowerExpr(obj)
	if err != nil {
		return nil, true, err
	}
	load := &ast.CallExpr{Fun: &ast.SelectorExpr{X: recvLoad, Sel: ident("Get")}, Args: []ast.Expr{key()}}
	var cond ast.Expr
	switch op {
	case "??=":
		cond = &ast.CallExpr{Fun: &ast.SelectorExpr{X: load, Sel: ident("IsNullish")}}
	case "||=":
		cond = &ast.UnaryExpr{Op: token.NOT, X: &ast.CallExpr{Fun: sel("value", "ToBoolean"), Args: []ast.Expr{load}}}
	case "&&=":
		cond = &ast.CallExpr{Fun: sel("value", "ToBoolean"), Args: []ast.Expr{load}}
	default:
		return nil, false, nil
	}
	val, err := r.boxOperand(value)
	if err != nil {
		return nil, true, err
	}
	recvStore, err := r.lowerExpr(obj)
	if err != nil {
		return nil, true, err
	}
	store := &ast.ExprStmt{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: recvStore, Sel: ident("Set")}, Args: []ast.Expr{key(), val}}}
	return &ast.IfStmt{Cond: cond, Body: &ast.BlockStmt{List: []ast.Stmt{store}}}, true, nil
}

// dynamicCompoundResult builds the boxed value.Value a compound member write
// stores, given the loaded old value (already a box) and the right-hand side node.
// A + fuses through value.Add, which concatenates when a string is present and
// adds otherwise, the same operation the dynamic + lowers to. Every other
// arithmetic and bitwise operator coerces both sides to a number and boxes the
// float64 result back with value.Number, reusing the same ToNumber-and-native-op
// construction the dynamic binary path uses, so o[k] -= v and o[k] <<= v run the
// arithmetic the language does before storing the box.
func (r *Renderer) dynamicCompoundResult(baseOp string, old ast.Expr, right frontend.Node) (ast.Expr, error) {
	if baseOp == "+" {
		val, err := r.boxOperand(right)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "Add"), Args: []ast.Expr{old, val}}, nil
	}
	r.requireImport(valuePkg)
	ln := &ast.CallExpr{Fun: sel("value", "ToNumber"), Args: []ast.Expr{old}}
	rn, err := r.operandToNumber(right)
	if err != nil {
		return nil, err
	}
	var num ast.Expr
	switch baseOp {
	case "-", "*", "/":
		ops := map[string]token.Token{"-": token.SUB, "*": token.MUL, "/": token.QUO}
		num = &ast.BinaryExpr{X: ln, Op: ops[baseOp], Y: rn}
	case "%":
		r.requireImport("math")
		num = &ast.CallExpr{Fun: sel("math", "Mod"), Args: []ast.Expr{ln, rn}}
	case "**":
		num = &ast.CallExpr{Fun: sel("value", "Pow"), Args: []ast.Expr{ln, rn}}
	default:
		goOp, shift, unsignedLeft, ok := bitwiseOp(baseOp)
		if !ok {
			return nil, &NotYetLowerable{Reason: "compound assignment operator " + baseOp + "= on a dynamic member is a later slice"}
		}
		num = r.bitwiseFromFloat(goOp, shift, unsignedLeft, ln, rn)
	}
	return &ast.CallExpr{Fun: sel("value", "Number"), Args: []ast.Expr{num}}, nil
}
