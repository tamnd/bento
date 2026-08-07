package lower

import (
	"go/ast"
	"go/token"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers the write half of a tuple element access, t[i] = v and its
// compound and increment spellings. The read half selects the positional field the
// literal index names (tuples.go), and the write selects the same field on the left
// of a Go assignment.
//
// The whole subtlety is that a tuple is a struct held by value while the array it
// stands for is an object held by reference. Two names for one array see each
// other's writes; two names for one struct do not, because the second name holds a
// copy. Reads alone cannot tell the two apart, which is why the read half needed no
// such question, and a write is exactly what makes the difference observable. So a
// write is lowered only when the binding it goes through is the array's only holder,
// proved below rather than assumed.

// tupleElementAssign lowers a tuple element write t[i] = v, and its compound form
// t[i] <op>= v, to the Go field assignment t.E<i> = v. It reports ok=false when the
// statement is not an element write whose receiver the checker types a tuple, so
// lowerUpdate falls through to the array, byte-buffer, and dynamic paths that own the
// other receivers.
//
// The index must be a literal inside the arity, the same bound the read half needs:
// the field E<i> is chosen at compile time, and an index the compiler cannot read is
// an index it cannot turn into a field name. A rest or optional position hands back
// for the reason internTuple does not emit their fields at all.
func (r *Renderer) tupleElementAssign(bin frontend.Node) (ast.Stmt, bool, error) {
	parts := r.prog.Children(bin)
	if len(parts) != 3 {
		return nil, false, nil
	}
	opText := r.prog.Text(parts[1])
	baseOp, compound := compoundBaseOp(opText)
	if opText != "=" && !compound {
		return nil, false, nil
	}
	target := parts[0]
	lhs, ok, err := r.tupleFieldTarget(target)
	if !ok || err != nil {
		return nil, ok, err
	}
	if compound {
		// A compound write's value is the element read combined with the right-hand
		// side, which combineBinary builds from the target node itself, so the read
		// side lowers to the same field selector the write side assigns to and a +=
		// on a string position concatenates exactly as the binary operator would.
		val, err := r.combineBinary(nil, baseOp, target, parts[2])
		if err != nil {
			return nil, false, err
		}
		return &ast.AssignStmt{Lhs: []ast.Expr{lhs}, Tok: token.ASSIGN, Rhs: []ast.Expr{val}}, true, nil
	}
	val, err := r.lowerExpr(parts[2])
	if err != nil {
		return nil, false, err
	}
	// The value coerces to the position's type the same way a local assignment
	// coerces to its target, so a widening or a boxing the field needs is applied
	// rather than dropped.
	val, err = r.coerceToTarget(val, parts[2], target)
	if err != nil {
		return nil, false, err
	}
	return &ast.AssignStmt{Lhs: []ast.Expr{lhs}, Tok: token.ASSIGN, Rhs: []ast.Expr{val}}, true, nil
}

// tupleElementIncDec lowers t[i]++ and t[i]-- on a number position to the step the
// compound write already lowers, since a statement discards the result and the prefix
// and postfix forms are then the same store. It reports ok=false when the target is
// not a tuple element, leaving the array, typed-array, and dynamic receivers to
// lowerIncDec's own paths.
func (r *Renderer) tupleElementIncDec(target frontend.Node, tok token.Token) (ast.Stmt, bool, error) {
	lhs, ok, err := r.tupleFieldTarget(target)
	if !ok || err != nil {
		return nil, ok, err
	}
	if !r.isNumber(target) {
		return nil, true, &NotYetLowerable{Reason: "an increment of a tuple position that is not a number is a later slice"}
	}
	op := token.INC
	if tok == token.DEC {
		op = token.DEC
	}
	return &ast.IncDecStmt{X: lhs, Tok: op}, true, nil
}

// tupleFieldTarget resolves an element access whose receiver is a tuple to the Go
// field selector a write lands on. It reports ok=false when the node is not such an
// access, and hands back with a named reason for the tuple shapes whose write this
// slice does not lower.
func (r *Renderer) tupleFieldTarget(target frontend.Node) (ast.Expr, bool, error) {
	if target.Kind() != frontend.NodeElementAccessExpression {
		return nil, false, nil
	}
	kids := r.prog.Children(target)
	if len(kids) != 2 {
		return nil, false, nil
	}
	recvNode, idxNode := kids[0], kids[1]
	elems, ok := r.prog.TupleElements(r.prog.TypeAt(recvNode))
	if !ok {
		return nil, false, nil
	}
	idx, ok := r.tupleLiteralIndex(idxNode, len(elems))
	if !ok {
		return nil, true, &NotYetLowerable{Reason: "a tuple element write whose index is not a literal inside the arity is a later slice"}
	}
	if elems[idx].Rest {
		return nil, true, &NotYetLowerable{Reason: "a write to a tuple's rest position is a later slice"}
	}
	if elems[idx].Optional {
		return nil, true, &NotYetLowerable{Reason: "a write to a tuple's optional position is a later slice"}
	}
	if err := r.tupleWriteThrough(recvNode); err != nil {
		return nil, true, err
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, false, err
	}
	return &ast.SelectorExpr{X: recv, Sel: ident("E" + itoa(idx))}, true, nil
}

// tupleWriteThrough answers whether a write through this receiver reaches the array
// the program means, and hands back naming what is in the way when it does not.
//
// A tuple lowers to a struct held by value, so the Go slot a write lands on is the
// only array anyone can see through that slot. That matches JavaScript exactly when
// the binding is the array's sole holder, and diverges the moment a second name
// holds it: the source shares one object, the Go program holds two structs, and a
// write through one is invisible to the other. So the receiver must be a plain name
// whose every use in the program keeps the array where it is.
//
// Three uses keep it there. Indexing it reads or writes a position and hands out no
// array. Reading its length folds to a constant. Spreading it into a call splices
// the positional field reads rather than pass the struct. Every other use, passing
// the name as an argument, binding it to a second name, returning it, storing it in
// a literal, or assigning a different array to it, either hands the array to
// something that copies it or replaces it with one somebody else may hold, and each
// is a later slice rather than a silent divergence.
func (r *Renderer) tupleWriteThrough(recvNode frontend.Node) error {
	if recvNode.Kind() != frontend.NodeIdentifier {
		return &NotYetLowerable{Reason: "a tuple element write through a receiver that is not a name is a later slice"}
	}
	sym, ok := r.prog.SymbolAt(recvNode)
	if !ok {
		return &NotYetLowerable{Reason: "a tuple element write through an unbound name is a later slice"}
	}
	sym = r.derefAlias(sym)
	escapes, seen := r.tupleEscapes[sym]
	if !seen {
		escapes = r.tupleSymbolEscapes(sym)
		r.tupleEscapes[sym] = escapes
	}
	if escapes {
		return &NotYetLowerable{Reason: "a tuple element write through a name the program holds or rebinds elsewhere is a later slice"}
	}
	// Not being handed out is half of it. The binding also has to have been handed a
	// fresh array to begin with, since a name initialized from one somebody else
	// already holds is a second holder however carefully it behaves afterwards.
	if !r.tupleBindingIsFresh(sym) {
		return &NotYetLowerable{Reason: "a tuple element write through a name that was not initialized with a fresh array is a later slice"}
	}
	return nil
}

// tupleBindingIsFresh reports whether sym was declared with an initializer that mints
// an array nothing else holds. An array literal does, by definition. A call does when
// every return in the callee's body answers an array literal, which is the shape of a
// function written to build one, and it is worth reading through because that is how
// the array a program writes to usually arrives.
//
// Anything else is refused, including a call the compiler cannot resolve to a body
// and a binding with no initializer at all. A function that answers a captured array
// hands out the array it captured, and a write through the copy Go took would be
// invisible to the name the source shares it with.
func (r *Renderer) tupleBindingIsFresh(sym frontend.Symbol) bool {
	decls := r.prog.Declarations(sym)
	if len(decls) != 1 || decls[0].Kind() != frontend.NodeVariableDeclaration {
		return false
	}
	kids := r.prog.Children(decls[0])
	if len(kids) == 0 {
		return false
	}
	return r.mintsFreshTuple(kids[len(kids)-1])
}

// mintsFreshTuple reports whether the expression builds an array no other name can
// already be holding.
func (r *Renderer) mintsFreshTuple(init frontend.Node) bool {
	switch init.Kind() {
	case frontend.NodeArrayLiteralExpression:
		return true
	case frontend.NodeCallExpression, frontend.NodeTaggedTemplateExpression:
		kids := r.prog.Children(init)
		if len(kids) == 0 {
			return false
		}
		sym, ok := r.prog.SymbolAt(kids[0])
		if !ok {
			return false
		}
		decls := r.prog.Declarations(r.derefAlias(sym))
		if len(decls) != 1 {
			return false
		}
		returns := 0
		fresh := true
		r.walkReturns(decls[0], func(n frontend.Node) {
			returns++
			kids := r.prog.Children(n)
			if len(kids) != 1 || kids[0].Kind() != frontend.NodeArrayLiteralExpression {
				fresh = false
			}
		})
		return returns > 0 && fresh
	}
	return false
}

// walkReturns visits every return statement in a subtree, skipping the bodies of
// nested functions, whose returns answer their own call rather than this one.
func (r *Renderer) walkReturns(n frontend.Node, visit func(frontend.Node)) {
	for _, c := range r.prog.Children(n) {
		switch c.Kind() {
		case frontend.NodeReturnStatement:
			visit(c)
		case frontend.NodeFunctionDeclaration, frontend.NodeFunctionExpression, frontend.NodeArrowFunction:
			continue
		}
		r.walkReturns(c, visit)
	}
}

// tupleSymbolEscapes walks the whole program and reports whether any use of sym hands
// the array to something that would hold a copy of it. The positions that do not are
// collected on the way down, since a node's role is a fact about its parent and the
// walk only ever descends: an element access marks its receiver, a .length read marks
// its receiver, a spread marks its operand, and a variable declaration marks the name
// it binds, which is the one occurrence that is the binding itself rather than a use
// of it.
//
// A parameter name is deliberately not marked. A tuple parameter holds a copy the
// caller cannot see, so a write through one is exactly the divergence this guards.
func (r *Renderer) tupleSymbolEscapes(sym frontend.Symbol) bool {
	if r.tupleHeld == nil {
		r.tupleHeld = map[frontend.Node]bool{}
		for _, f := range r.prog.SourceFiles() {
			r.markTupleHeld(f)
		}
	}
	escapes := false
	for _, f := range r.prog.SourceFiles() {
		r.walkIdents(f, func(n frontend.Node) {
			if escapes || r.tupleHeld[n] {
				return
			}
			s, ok := r.prog.SymbolAt(n)
			if !ok || r.derefAlias(s) != sym {
				return
			}
			escapes = true
		})
	}
	return escapes
}

// markTupleHeld records the nodes whose position keeps a tuple where it is, the set
// tupleSymbolEscapes tests each use of a name against.
func (r *Renderer) markTupleHeld(n frontend.Node) {
	kids := r.prog.Children(n)
	switch n.Kind() {
	case frontend.NodeElementAccessExpression:
		if len(kids) == 2 {
			r.tupleHeld[kids[0]] = true
		}
	case frontend.NodePropertyAccessExpression:
		if len(kids) == 2 && r.prog.Text(kids[1]) == "length" {
			r.tupleHeld[kids[0]] = true
		}
	case frontend.NodeSpreadElement:
		if len(kids) == 1 {
			r.tupleHeld[kids[0]] = true
		}
	case frontend.NodeVariableDeclaration:
		if len(kids) > 0 {
			r.tupleHeld[kids[0]] = true
		}
	}
	for _, c := range kids {
		r.markTupleHeld(c)
	}
}
