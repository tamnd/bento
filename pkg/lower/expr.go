package lower

import (
	"go/ast"
	"go/token"
	"math"
	"strconv"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file is the expression dispatch and the operator lowerings: unary,
// binary, conditional, bitwise, and the operator token tables. Calls, member
// access, literals, and templates each live in their own file; lowerExpr routes
// to them.

// lowerExpr lowers one expression node to a Go expression. It covers the leaves
// and operators a numeric-typed body is built from: identifiers, numeric
// literals, parentheses, and binary arithmetic on numbers.
func (r *Renderer) lowerExpr(n frontend.Node) (ast.Expr, error) {
	switch n.Kind() {
	case frontend.NodeIdentifier:
		// A bare reference to a go: import binding used as a value is a constant read
		// into the Go package, marshaled by the constant's Go type. It is checked by the
		// binding's own text, the key the import recorded, before the local-name path,
		// which would otherwise treat the binding as an undeclared local.
		if expr, handled, err := r.goConstRef(r.prog.Text(n)); err != nil {
			return nil, err
		} else if handled {
			return expr, nil
		}
		// A bare reference to a top-level function used as a value (passed as a
		// callback, stored in a variable) is the function itself, so it lowers to the
		// exported Go name its declaration takes, the same name a direct call uses. It
		// is checked before the local-name path, which would otherwise emit the source
		// name and miss the exported declaration.
		if sym, ok := r.prog.SymbolAt(n); ok && sym.Flags&frontend.SymbolFunction != 0 {
			// A function whose lowered arity exceeds its minimal call (it has a
			// defaulted, optional, or rest parameter) fills the omitted slot at the call
			// site, so it has no single Go func value that fits a slot expecting the
			// minimal arity. Used as a value rather than called, it hands back until a
			// defaulting wrapper is modeled.
			if r.funcOmittable(sym) {
				return nil, &NotYetLowerable{Reason: "a function with an omittable parameter used as a value needs a defaulting wrapper, a later slice"}
			}
			if goName, ok := exportedField(sym.Name); ok {
				return ident(goName), nil
			}
		}
		// NaN and Infinity are ambient number globals, not user bindings, so they
		// lower to the doubles they name: math.NaN() and math.Inf(1). A source
		// binding that shadows either name fails isGlobalRef and stays a local.
		if r.isGlobalRef(n, "NaN") {
			r.requireImport("math")
			return &ast.CallExpr{Fun: sel("math", "NaN")}, nil
		}
		if r.isGlobalRef(n, "Infinity") {
			r.requireImport("math")
			return &ast.CallExpr{Fun: sel("math", "Inf"), Args: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: "1"}}}, nil
		}
		// undefined is the ambient global whose only value is the absent one, so it
		// lowers to the value.Undefined singleton. The type check keeps a user binding
		// that shadows the name, which would not be typed exactly undefined, a local.
		if r.isUndefinedLiteral(n) {
			r.requireImport(valuePkg)
			return sel("value", "Undefined"), nil
		}
		name, ok := localName(r.prog.Text(n))
		if !ok {
			return nil, &NotYetLowerable{Reason: "identifier is not a Go identifier"}
		}
		// A catch binding read outside of its .message or .name property (which
		// propertyAccess handles before it reaches here) hands back, since the runtime
		// models a caught error as a value.Error rather than a general boxed value, so
		// passing it on or reassigning it has no lowering yet.
		if r.errorLocals[name] {
			return nil, &NotYetLowerable{Reason: "a caught error used other than reading .message or .name is a later slice"}
		}
		// A local specialized to Go int32 is read back as float64(name), so every
		// consumer of the read (arithmetic, a Math call, console.log) sees the same
		// float64 a number local always presented. The int32 type is visible only on
		// the declaration and on the writes lowered through int32Of; the read side is
		// transparent, which is what lets the specialization stay local to this file.
		if r.int32Locals[name] {
			return &ast.CallExpr{Fun: ident("float64"), Args: []ast.Expr{ident(name)}}, nil
		}
		// An int64-specialized local reads back the same way: float64(name) is
		// exact, since the analysis bounded the value inside the safe-integer
		// range, so the read side stays transparent for this tier too.
		if r.int64Locals[name] {
			return &ast.CallExpr{Fun: ident("float64"), Args: []ast.Expr{ident(name)}}, nil
		}
		// A local declared as an optional (value.Opt[T]) that the checker narrowed to
		// T at this use, past a presence guard like `x !== undefined`, reads the stored
		// value out with .Get(). The narrowing shows as the type at this node no longer
		// carrying undefined; a read where the type is still the optional keeps the bare
		// Opt value, which is what the presence test and an Opt-to-Opt assignment want.
		if r.optLocals[name] && !r.isOptional(n) {
			return &ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(name), Sel: ident("Get")}}, nil
		}
		// A boxed dynamic local the checker narrowed to a single primitive at this
		// use, past a typeof guard, reads through the matching accessor so the
		// static expression it flows into (a concat, a compare, a Math call) sees
		// the unboxed Go value. A read where the type is still any or unknown
		// keeps the bare box, which is what the runtime helpers take.
		if r.dynLocals[name] {
			if acc, ok := dynAccessor(r.primitiveFlags(n)); ok {
				return &ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(name), Sel: ident(acc)}}, nil
			}
		}
		// A local or parameter of a tagged-sum union type that the checker narrowed
		// to one arm at this use reads that arm's field off the struct, the same
		// unwrap the optional does with .Get() but for a discriminated member: past
		// a typeof or discriminant guard the reference is a plain number or string,
		// so it lowers to name.num or name.str. A reference where the type is still
		// the whole union keeps the bare struct for an assignment or a pass-through.
		if read, ok := r.narrowedUnionRead(name, n); ok {
			return read, nil
		}
		return ident(name), nil

	case frontend.NodeParenthesizedExpression:
		kids := r.prog.Children(n)
		if len(kids) != 1 {
			return nil, &NotYetLowerable{Reason: "parenthesized expression did not wrap a single expression"}
		}
		inner, err := r.lowerExpr(kids[0])
		if err != nil {
			return nil, err
		}
		return &ast.ParenExpr{X: inner}, nil

	case frontend.NodeNumericLiteral:
		return r.numericLiteral(n)

	case frontend.NodeBigIntLiteral:
		return r.bigIntLiteral(n)

	case frontend.NodeStringLiteral:
		return r.stringLiteral(n)

	case frontend.NodeNoSubstitutionTemplateLiteral:
		return r.noSubTemplate(n)

	case frontend.NodeTemplateExpression:
		return r.templateExpression(n)

	case frontend.NodePropertyAccessExpression:
		return r.propertyAccess(n)

	case frontend.NodeThisKeyword:
		// Inside a lowered constructor or method, this is the receiver.
		if r.thisName == "" {
			return nil, &NotYetLowerable{Reason: "this outside a lowered class body is a later slice"}
		}
		return ident(r.thisName), nil

	case frontend.NodeSuperKeyword:
		// Inside a lowered derived-class body, super is the embedded base value,
		// so super.m() and super.x reach the base member through the embedded
		// field selector; with no overrides that static dispatch is exactly what
		// the source means. A bare super() call never reaches here: the
		// constructor lowering folds it into the base assignment.
		if r.thisName == "" || r.curClass == nil || r.curClass.base == nil {
			return nil, &NotYetLowerable{Reason: "super outside a lowered derived class body is a later slice"}
		}
		return &ast.SelectorExpr{X: ident(r.thisName), Sel: ident(r.curClass.base.goName)}, nil

	case frontend.NodeTrueKeyword:
		return ident("true"), nil

	case frontend.NodeFalseKeyword:
		return ident("false"), nil

	case frontend.NodeNullKeyword:
		// The null literal has no Go value of its own; its only representation is the
		// boxed value.Null singleton, so it appears where a dynamic slot holds it. A
		// typed null inside a union keeps its own representation and never reaches the
		// bare literal here, since the union compares route the null before lowering.
		r.requireImport(valuePkg)
		return sel("value", "Null"), nil

	case frontend.NodeBinaryExpression:
		return r.binaryExpr(n)

	case frontend.NodeCallExpression:
		return r.callExpr(n)

	case frontend.NodePrefixUnaryExpression:
		return r.prefixUnary(n)

	case frontend.NodePostfixUnaryExpression:
		return r.postfixUnary(n)

	case frontend.NodeConditionalExpression:
		return r.conditionalExpr(n)

	case frontend.NodeArrayLiteralExpression:
		return r.arrayLiteral(n)

	case frontend.NodeElementAccessExpression:
		return r.elementAccess(n)

	case frontend.NodeObjectLiteralExpression:
		return r.objectLiteral(n)

	case frontend.NodeArrowFunction:
		return r.arrowFunc(n)

	case frontend.NodeFunctionExpression:
		return r.functionExpr(n)

	case frontend.NodeNewExpression:
		return r.newExpr(n)

	case frontend.NodeAsExpression:
		// `inner as T` leads with the value, so the inner expression is the first
		// child and the type node follows it.
		return r.castExpr(n, 0)

	case frontend.NodeTypeAssertion:
		// `<T>inner` leads with the type, so the inner expression is the second
		// child.
		return r.castExpr(n, 1)

	case frontend.NodeUnknown:
		// typeof and a handful of other prefix operators the shim does not give a
		// distinct kind surface as the catch-all node, told apart by the keyword their
		// source text leads with. Only typeof lowers today; the rest hand back.
		if r.isTypeofExpr(n) {
			return r.typeofExpr(n)
		}
		return nil, &NotYetLowerable{Reason: "expression kind " + kindName(n.Kind()) + " is a later slice"}

	default:
		return nil, &NotYetLowerable{Reason: "expression kind " + kindName(n.Kind()) + " is a later slice"}
	}
}

// castExpr lowers a type-cast expression, `inner as T` or `<T>inner`. A cast
// carries no runtime effect in JavaScript, so it erases to its inner value and
// the only work left is bridging that value from the inner expression's type to
// the cast's own type. That bridge is exactly the coercion a binding applies
// across the dynamic boundary, so an unknown value cast to a number unboxes
// through the same path an assignment would take, and a same-typed cast passes
// through untouched. The inner expression sits at a fixed child index that
// differs between the two forms, given by innerIdx.
func (r *Renderer) castExpr(n frontend.Node, innerIdx int) (ast.Expr, error) {
	kids := r.prog.Children(n)
	if innerIdx >= len(kids) {
		return nil, &NotYetLowerable{Reason: "cast expression did not expose an inner expression"}
	}
	inner := kids[innerIdx]
	expr, err := r.lowerExpr(inner)
	if err != nil {
		return nil, err
	}
	return r.coerceToTarget(expr, inner, n)
}

// conditionalExpr lowers a ternary cond ? whenTrue : whenFalse. Go has no
// conditional operator, so it lowers to an immediately-invoked function that
// returns one branch or the other. The function form, rather than a helper
// taking both values, is what preserves JavaScript's laziness: only the taken
// branch's expression runs, so a side effect or a call in the untaken branch
// does not fire. The condition reuses lowerCondition, so it must be a boolean
// (a truthy number or object condition hands back until truthiness lands), and
// the result type comes from the checker's type for the whole expression, which
// is the common supertype of the two branches.
func (r *Renderer) conditionalExpr(n frontend.Node) (ast.Expr, error) {
	kids := r.prog.Children(n)
	if len(kids) != 5 || r.prog.Text(kids[1]) != "?" || r.prog.Text(kids[3]) != ":" {
		return nil, &NotYetLowerable{Reason: "conditional expression did not expose condition, true, and false branches"}
	}
	// A condition the checker proved always truthy or always falsy collapses to the
	// branch that runs, so cond ? a : b over a non-null object becomes a. Only the
	// taken branch is lowered, which is JavaScript's laziness kept exactly: the other
	// branch never runs, so its side effect never fires and it need not even lower.
	// The collapse is taken only for a side-effect-free condition, so dropping the
	// condition itself loses nothing; a condition with a side effect keeps its test.
	if val, known := r.staticTruthy(kids[0]); known && r.repeatableOperand(kids[0]) {
		if val {
			return r.lowerExpr(kids[2])
		}
		return r.lowerExpr(kids[4])
	}
	cond, err := r.lowerCondition(kids[0])
	if err != nil {
		return nil, err
	}
	whenTrue, err := r.lowerExpr(kids[2])
	if err != nil {
		return nil, err
	}
	whenFalse, err := r.lowerExpr(kids[4])
	if err != nil {
		return nil, err
	}
	// The IIFE's return type is the branches' widened primitive, not the checker's
	// type for the whole expression: TypeScript types a ternary as the union of
	// its branch types, so two string literals give a "a" | "b" literal union and
	// a chained numeric ternary gives a numeric-literal union, neither of which is
	// the value.BStr or float64 the branch expressions actually produce. Both
	// branches must widen to the same primitive; a ternary that mixes types (or
	// whose branches are objects) needs the tagged union and hands back.
	retType, kind, ok := r.condBranchType(kids[2])
	_, otherKind, otherOK := r.condBranchType(kids[4])
	if !ok || !otherOK || kind != otherKind {
		return nil, &NotYetLowerable{Reason: "conditional whose branches are not both the same primitive type needs a union, a later slice"}
	}
	lit := &ast.FuncLit{
		Type: &ast.FuncType{
			Params:  &ast.FieldList{},
			Results: &ast.FieldList{List: []*ast.Field{{Type: retType}}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.IfStmt{
				Cond: cond,
				Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{whenTrue}}}},
			},
			&ast.ReturnStmt{Results: []ast.Expr{whenFalse}},
		}},
	}
	return &ast.CallExpr{Fun: lit}, nil
}

// condBranchType reports the Go type a ternary branch lowers to and a name for
// its primitive kind, seeing through parentheses and a nested ternary so a
// chained a ? x : b ? y : z types by its leaf primitive rather than by the
// checker's literal union. It returns ok == false for a branch that is not a
// number, string, or boolean, so a ternary over objects or mixed types hands
// back rather than guessing a return type.
func (r *Renderer) condBranchType(n frontend.Node) (ast.Expr, string, bool) {
	switch {
	case r.isNumber(n):
		return ident("float64"), "number", true
	case r.isBool(n):
		return ident("bool"), "bool", true
	case r.isString(n):
		r.requireImport(valuePkg)
		return sel("value", "BStr"), "string", true
	case r.isTypeofExpr(n):
		// typeof always lowers to a BStr, folded to its tag constant or read at
		// runtime, even when the checker leaves the node itself untyped.
		r.requireImport(valuePkg)
		return sel("value", "BStr"), "string", true
	}
	switch n.Kind() {
	case frontend.NodeParenthesizedExpression:
		if kids := r.prog.Children(n); len(kids) == 1 {
			return r.condBranchType(kids[0])
		}
	case frontend.NodeConditionalExpression:
		if kids := r.prog.Children(n); len(kids) == 5 {
			return r.condBranchType(kids[2])
		}
	}
	return nil, "", false
}

// prefixUnary lowers a prefix unary expression. The operator is not a child
// node, so it is read as the part of the node's text before the operand.
// Negation on a number and logical not on a boolean map to their Go unary
// operators, and a unary plus on a number is the no-op it is in both languages,
// so the operand passes through. The increment and decrement operators mutate
// their operand and are a later slice.
func (r *Renderer) prefixUnary(n frontend.Node) (ast.Expr, error) {
	kids := r.prog.Children(n)
	if len(kids) != 1 {
		return nil, &NotYetLowerable{Reason: "prefix expression did not expose a single operand"}
	}
	operand := kids[0]
	op := strings.TrimSpace(strings.TrimSuffix(r.prog.Text(n), r.prog.Text(operand)))
	switch op {
	case "-":
		// A bigint negation is a *big.Int method, not a Go unary minus, so -x lowers
		// to new(big.Int).Neg(x), a fresh value the way every bigint operator returns
		// one, leaving the operand untouched.
		if r.isBigInt(operand) {
			x, err := r.lowerExpr(operand)
			if err != nil {
				return nil, err
			}
			r.requireImport("math/big")
			fresh := &ast.CallExpr{Fun: ident("new"), Args: []ast.Expr{sel("big", "Int")}}
			return &ast.CallExpr{Fun: &ast.SelectorExpr{X: fresh, Sel: ident("Neg")}, Args: []ast.Expr{x}}, nil
		}
		// -0 is the negative-zero double, but a Go constant has no signed zero:
		// -0 and -0.0 both fold to +0, so the literal form goes through
		// math.Copysign, the Go spelling of the value. Negating a variable is
		// float64 negation and keeps the sign on its own.
		if operand.Kind() == frontend.NodeNumericLiteral {
			if v, err := strconv.ParseFloat(strings.TrimSpace(r.prog.Text(operand)), 64); err == nil && v == 0 {
				r.requireImport("math")
				return &ast.CallExpr{
					Fun: sel("math", "Copysign"),
					Args: []ast.Expr{
						&ast.BasicLit{Kind: token.INT, Value: "0"},
						&ast.UnaryExpr{Op: token.SUB, X: &ast.BasicLit{Kind: token.INT, Value: "1"}},
					},
				}, nil
			}
		}
		// Every other operand coerces to its float64 the way ToNumber does, and the
		// minus applies to the result: -x on a number is Go negation, on a string
		// value.StringToNumber then negation, on a dynamic value -value.ToNumber(x).
		// A non-primitive hands back through the shared coercion.
		num, err := r.unaryOperandToNumber(operand)
		if err != nil {
			return nil, err
		}
		return &ast.UnaryExpr{Op: token.SUB, X: num}, nil
	case "+":
		// Unary plus is ToNumber and nothing else: a number passes through, a string
		// or boolean coerces through its numeric conversion, and a dynamic value runs
		// value.ToNumber. A bigint reaching the dynamic path throws the same TypeError
		// the language raises, and a non-primitive hands back through the coercion.
		return r.unaryOperandToNumber(operand)
	case "!":
		// ! negates the operand's truthiness, so a non-boolean rides the same
		// ToBoolean lowerCondition uses and the not wraps the resulting bool: !s is
		// !(s.Length() > 0), the empty-string test negated.
		x, err := r.lowerTruthy(operand)
		if err != nil {
			return nil, err
		}
		return &ast.UnaryExpr{Op: token.NOT, X: x}, nil
	case "~":
		// A bigint ~ is infinite two's complement, -(x+1) with no 32-bit window, which
		// is exactly big.Int's Not, so it lowers to new(big.Int).Not(x), a fresh value
		// the way every bigint operator returns one.
		if r.isBigInt(operand) {
			x, err := r.lowerExpr(operand)
			if err != nil {
				return nil, err
			}
			r.requireImport("math/big")
			fresh := &ast.CallExpr{Fun: ident("new"), Args: []ast.Expr{sel("big", "Int")}}
			return &ast.CallExpr{Fun: &ast.SelectorExpr{X: fresh, Sel: ident("Not")}, Args: []ast.Expr{x}}, nil
		}
		// Bitwise NOT is the unary member of the bitwise family: it coerces its
		// operand to a 32-bit integer, complements it, and returns the result as a
		// number, so it lowers to float64(^value.ToInt32(x)), the same coercion the
		// binary bitwise operators use, not a Go ^ on the float64. A string or boolean
		// coerces through its numeric conversion, a dynamic operand through ToNumber,
		// the float64 that ToInt32 then narrows; a non-primitive hands back.
		x, err := r.unaryOperandToNumber(operand)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		conv := &ast.CallExpr{Fun: sel("value", "ToInt32"), Args: []ast.Expr{x}}
		return &ast.CallExpr{Fun: ident("float64"), Args: []ast.Expr{&ast.UnaryExpr{Op: token.XOR, X: conv}}}, nil
	case "++", "--":
		return r.incDecValue(operand, op, true)
	default:
		return nil, &NotYetLowerable{Reason: "prefix operator " + op + " is a later slice"}
	}
}

// postfixUnary lowers a value-position postfix ++ or -- (n++ or n-- used where a
// value is read, as in const r = n++ or a[i++]). A statement-position postfix
// never reaches here: lowerUpdate takes n++; on its own line and emits a Go
// IncDecStmt, so this path is only the form that must also yield the operand's
// value.
func (r *Renderer) postfixUnary(n frontend.Node) (ast.Expr, error) {
	kids := r.prog.Children(n)
	if len(kids) != 1 {
		return nil, &NotYetLowerable{Reason: "postfix expression did not expose a single operand"}
	}
	operand := kids[0]
	op := strings.TrimSpace(strings.TrimPrefix(r.prog.Text(n), r.prog.Text(operand)))
	if op != "++" && op != "--" {
		return nil, &NotYetLowerable{Reason: "postfix operator " + op + " is a later slice"}
	}
	return r.incDecValue(operand, op, false)
}

// incDecValue lowers a value-position ++ or -- on a number local, the increment
// form that yields a value rather than standing alone as a statement. Go has no
// expression that both mutates a variable and evaluates to a number, so the update
// rides an immediately-called closure that captures the local: a prefix ++n
// increments and returns the new value, and a postfix n++ saves the old value,
// increments, and returns the saved one, which is the read timing JavaScript
// specifies. The target has to be a plain float64 local: a refined-integer local
// would make the float64 return type mismatch its int, and a non-identifier or
// dynamic target has no such closure yet, so both hand back to a later slice.
func (r *Renderer) incDecValue(operand frontend.Node, op string, prefix bool) (ast.Expr, error) {
	if operand.Kind() != frontend.NodeIdentifier {
		return nil, &NotYetLowerable{Reason: "value-position " + op + " on a non-identifier target is a later slice"}
	}
	if !r.isNumber(operand) || r.isDynamic(operand) {
		return nil, &NotYetLowerable{Reason: "value-position " + op + " on a non-number target is a later slice"}
	}
	name, ok := localName(r.prog.Text(operand))
	if !ok {
		return nil, &NotYetLowerable{Reason: "value-position " + op + " target is not a Go identifier"}
	}
	if r.int32Locals[name] || r.int64Locals[name] {
		return nil, &NotYetLowerable{Reason: "value-position " + op + " on a refined-integer local is a later slice"}
	}
	tok := token.INC
	if op == "--" {
		tok = token.DEC
	}
	inc := &ast.IncDecStmt{X: ident(name), Tok: tok}
	var body []ast.Stmt
	if prefix {
		body = []ast.Stmt{inc, &ast.ReturnStmt{Results: []ast.Expr{ident(name)}}}
	} else {
		// prev holds the value the postfix form reads before the update. It shadows
		// any outer name harmlessly, since the closure only reads it to return it.
		body = []ast.Stmt{
			&ast.AssignStmt{Lhs: []ast.Expr{ident("prev")}, Tok: token.DEFINE, Rhs: []ast.Expr{ident(name)}},
			inc,
			&ast.ReturnStmt{Results: []ast.Expr{ident("prev")}},
		}
	}
	lit := &ast.FuncLit{
		Type: &ast.FuncType{
			Params:  &ast.FieldList{},
			Results: &ast.FieldList{List: []*ast.Field{{Type: ident("float64")}}},
		},
		Body: &ast.BlockStmt{List: body},
	}
	return &ast.CallExpr{Fun: lit}, nil
}

// binaryExpr lowers a binary expression on two operands of the same primitive
// type. On two numbers the arithmetic operators map directly on float64 and the
// relational and equality operators map to Go comparisons that yield bool. On
// two booleans the short-circuit && and || map to Go's &&/||, which evaluate in
// the same order and short-circuit the same way, and === / !== map to Go
// ==/!=. The operand types are guarded because + on strings is a different-typed
// concatenation and === on objects is reference identity, each its own later
// slice. An assignment (the "=" operator) is a statement form and is handled
// there, so as a value it hands back. The children are left, operator, right,
// the shape the frontend exposes.
func (r *Renderer) binaryExpr(n frontend.Node) (ast.Expr, error) {
	kids := r.prog.Children(n)
	if len(kids) != 3 {
		return nil, &NotYetLowerable{Reason: "binary expression did not expose left, operator, right"}
	}
	left, op, right := kids[0], kids[1], kids[2]
	opText := r.prog.Text(op)
	if opText == "=" {
		return r.assignValue(left, right)
	}
	return r.combineBinary(opText, left, right)
}

// assignValue lowers an assignment read for its value (const r = (x = 5), the
// (i = i + 1) a while condition steps with, or console.log(p.x = 7)), the form
// whose result is the value assigned. Go's assignment is a statement and yields
// nothing, so the write rides an immediately-called closure that performs the
// store and returns the assigned value, which is what JavaScript's assignment
// expression evaluates to. A local target names its own Go slot; a property
// target reuses the statement path's field gate, so a plain class or object field
// lowers to a selector lvalue and a setter, dynamic receiver, or element access
// hands back. A chained a = b = 5 falls out for free: the inner assignment is the
// right operand of the outer, so it lowers through this same path.
func (r *Renderer) assignValue(left, right frontend.Node) (ast.Expr, error) {
	switch left.Kind() {
	case frontend.NodeIdentifier:
		return r.assignValueLocal(left, right)
	case frontend.NodePropertyAccessExpression:
		return r.assignValueProperty(left, right)
	default:
		return nil, &NotYetLowerable{Reason: "assignment value into a target that is neither a local nor a property is a later slice"}
	}
}

// assignValueLocal lowers an assignment-as-value whose target is a local: the
// closure assigns the coerced right-hand side to the local and returns it, typed
// by the local's own Go slot. A refined-integer local returns an int the float64
// slot type would mismatch, and a dynamic-storage local (a var with no
// initializer, narrowed on read) has a boxed slot the narrowed type would not
// name, so both hand back to a later slice; a plain number, string, or boolean
// local passes through.
func (r *Renderer) assignValueLocal(left, right frontend.Node) (ast.Expr, error) {
	name, ok := localName(r.prog.Text(left))
	if !ok {
		return nil, &NotYetLowerable{Reason: "assignment value target is not a Go identifier"}
	}
	if r.int32Locals[name] || r.int64Locals[name] {
		return nil, &NotYetLowerable{Reason: "assignment value on a refined-integer local is a later slice"}
	}
	if r.isDynamic(left) || r.localStorageDynamic(left) {
		return nil, &NotYetLowerable{Reason: "assignment value on a dynamic or narrowed-storage local is a later slice"}
	}
	retType, err := r.typeExpr(r.prog.TypeAt(left))
	if err != nil {
		return nil, err
	}
	rhs, err := r.lowerExpr(right)
	if err != nil {
		return nil, err
	}
	rhs, err = r.coerceToTarget(rhs, right, left)
	if err != nil {
		return nil, err
	}
	body := []ast.Stmt{
		&ast.AssignStmt{Lhs: []ast.Expr{ident(name)}, Tok: token.ASSIGN, Rhs: []ast.Expr{rhs}},
		&ast.ReturnStmt{Results: []ast.Expr{ident(name)}},
	}
	return r.valueClosure(retType, body), nil
}

// assignValueProperty lowers an assignment-as-value whose target is a property.
// It reuses the same field gate the statement store uses: a class field or a
// static-shape object field resolves to a Go selector lvalue, so the closure
// binds the coerced right-hand side to a temp, writes it through the lvalue, and
// returns the temp. Evaluating the right-hand side into the temp first keeps the
// stored value and the returned value the same object. A setter, a dynamic
// receiver, an array element, or a typed-array/map/set member uses method access
// rather than a plain lvalue and hands back to a later slice.
func (r *Renderer) assignValueProperty(left, right frontend.Node) (ast.Expr, error) {
	_, f, isField, err := r.classFieldOfTarget(left)
	if err != nil {
		return nil, err
	}
	coerceTo := f.ident
	if !isField {
		tParts := r.prog.Children(left)
		if len(tParts) != 2 {
			return nil, &NotYetLowerable{Reason: "assignment value property target is malformed"}
		}
		obj := tParts[0]
		objType := r.prog.TypeAt(obj)
		if r.isDynamic(obj) || objType.Flags&frontend.TypeObject == 0 {
			return nil, &NotYetLowerable{Reason: "assignment value into a dynamic-receiver or non-object property is a later slice"}
		}
		if _, isArray := r.prog.ElementType(objType); isArray {
			return nil, &NotYetLowerable{Reason: "assignment value into an array element is a later slice"}
		}
		if r.isTypedArray(obj) || r.isMap(obj) || r.isSet(obj) {
			return nil, &NotYetLowerable{Reason: "assignment value into a typed-array, map, or set member is a later slice"}
		}
		coerceTo = left
	}
	lhs, err := r.lowerExpr(left)
	if err != nil {
		return nil, err
	}
	rhs, err := r.lowerExpr(right)
	if err != nil {
		return nil, err
	}
	rhs, err = r.coerceToTarget(rhs, right, coerceTo)
	if err != nil {
		return nil, err
	}
	retType, err := r.typeExpr(r.prog.TypeAt(left))
	if err != nil {
		return nil, err
	}
	// The temp is declared with the field's Go type rather than inferred with :=, so
	// a bare numeric constant right-hand side does not settle to Go's default int and
	// mismatch a float64 field.
	tmp := r.freshTemp()
	body := []ast.Stmt{
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{
			Names:  []*ast.Ident{ident(tmp)},
			Type:   retType,
			Values: []ast.Expr{rhs},
		}}}},
		&ast.AssignStmt{Lhs: []ast.Expr{lhs}, Tok: token.ASSIGN, Rhs: []ast.Expr{ident(tmp)}},
		&ast.ReturnStmt{Results: []ast.Expr{ident(tmp)}},
	}
	return r.valueClosure(retType, body), nil
}

// valueClosure wraps a body that ends in a return into an immediately-called
// function literal returning the given type, the shape a value-position statement
// (an assignment or an increment) rides so it can both run and yield a value.
func (r *Renderer) valueClosure(retType ast.Expr, body []ast.Stmt) ast.Expr {
	lit := &ast.FuncLit{
		Type: &ast.FuncType{
			Params:  &ast.FieldList{},
			Results: &ast.FieldList{List: []*ast.Field{{Type: retType}}},
		},
		Body: &ast.BlockStmt{List: body},
	}
	return &ast.CallExpr{Fun: lit}
}

// combineBinary lowers a JavaScript binary operator applied to two operand
// nodes to the Go expression with the same meaning. It is the shared core of
// binaryExpr and of a compound assignment (x += y desugars to x = x + y), so
// the string, remainder, and bitwise special cases apply the same way whether
// the operator was written on its own or fused to an assignment.
func (r *Renderer) combineBinary(opText string, left, right frontend.Node) (ast.Expr, error) {
	// + where either operand is dynamic (typed any or unknown) cannot pick a Go
	// operator at compile time, because the operand's runtime kind decides whether
	// the result is a numeric sum or a string concatenation. It lowers to value.Add,
	// the boxed + that runs ToPrimitive on both sides and then adds or concatenates
	// the way the language does. Each operand is boxed to a value.Value first: a
	// dynamic operand already is one, and a static primitive is lifted through its
	// constructor. This routes before the static operator paths below, which read
	// isString and isNumber that a dynamic operand answers no to. Only + is dynamic
	// here; another operator on a dynamic operand falls through and hands back.
	// instanceof on a caught error narrows the binding so a catch can read its
	// .message or .name; it routes first because a catch binding is typed unknown,
	// which the operand tests below would send down the dynamic path. Only a caught
	// error against a built-in error constructor is covered here, and any other
	// instanceof hands back until class-instance narrowing lands.
	if opText == "instanceof" {
		expr, handled, err := r.errorInstanceof(left, right)
		if err != nil {
			return nil, err
		}
		if handled {
			return expr, nil
		}
		return nil, &NotYetLowerable{Reason: "instanceof outside a caught built-in error is a later slice"}
	}

	// "prop" in s on a discriminated object union narrows to the arms that carry the
	// property, so it lowers to a tag disjunction, s.tag == A || s.tag == B, rather
	// than probing a runtime property map (tagunion.go). A test over a non-union or a
	// property that every arm or no arm carries does not narrow and hands back, since
	// a general in on an arbitrary object is its own later slice.
	if opText == "in" {
		expr, handled, err := r.inUnionCompare(left, right)
		if err != nil {
			return nil, err
		}
		if handled {
			return expr, nil
		}
		return nil, &NotYetLowerable{Reason: "the in operator outside a discriminated-union narrowing is a later slice"}
	}

	// Nullish coalescing is a presence test on the left, not a truthiness test,
	// so it routes here before the operator table rather than desugaring to || .
	// It lowers only the optional shape with a pure fallback and hands back the
	// rest (section on null and undefined).
	if opText == "??" {
		return r.nullishCoalesce(left, right)
	}

	// && and || return an operand, not a boolean, so over two numbers or two strings
	// they lower to a value-returning if rather than a Go operator (the boolean
	// section on keeping && and || value-returning). valueLogical reports handled for
	// that shape and leaves the two-boolean case for the operator table below, where
	// Go's own && and || carry the boolean result with the same short-circuit.
	if opText == "&&" || opText == "||" {
		expr, handled, err := r.valueLogical(opText, left, right)
		if err != nil {
			return nil, err
		}
		if handled {
			return expr, nil
		}
	}

	// An equality between a caught error and the null or undefined literal folds to
	// a constant: the runtime holds a caught value as a non-nil *value.Error, so it
	// is never null or undefined, and throwing null or undefined hands back, so no
	// caught binding is ever one of them. It routes before the dynamic path, which
	// would box the binding and hand back, since the binding has no general value
	// form. Only a null or undefined literal on the other side is folded; a compare
	// against another value stays a real comparison and is not this case.
	if expr, handled := r.caughtErrorNullCompare(opText, left, right); handled {
		return expr, nil
	}

	if r.combineIsDynamic(opText, left, right) {
		l, err := r.boxOperand(left)
		if err != nil {
			return nil, err
		}
		rr, err := r.boxOperand(right)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "Add"), Args: []ast.Expr{l, rr}}, nil
	}

	// The other operators a dynamic operand reaches: strict and loose equality
	// through the runtime's StrictEquals and LooseEquals (or the IsUndefined/IsNull
	// presence forms against a literal), the relationals through the runtime's
	// Abstract Relational Comparison, and the multiplicative operators through
	// ToNumber on each side.
	if expr, handled, err := r.dynamicBinary(opText, left, right); err != nil {
		return nil, err
	} else if handled {
		return expr, nil
	}

	// + where either operand is a string is concatenation of a UTF-16 string, not
	// a Go string +, which would be UTF-8, and not a Go operator at all since bstr
	// is a struct. JavaScript + is string concatenation as soon as one operand is a
	// string: the other operand is coerced to a string with the same ToString the
	// value model runs, so a number becomes value.NumberToString and a boolean
	// value.BoolToString before both are joined with value.Concat, which picks the
	// wider backing form and copies once (section 5). It is handled before the
	// operator table so the string path emits a call rather than reaching the
	// number/bool dispatch, and a string operand against a non-primitive (an object
	// or array, whose ToString is a later slice) hands back through stringifyOperand.
	if opText == "+" && (r.isString(left) || r.isString(right)) {
		// Flatten a left-leaning chain of string + into the operands of one ConcatN,
		// so "a" + x + "b" + y builds in a single strings.Builder pass and one
		// allocation rather than folding pairwise through Concat, which allocates a
		// fresh string at every join. The operands keep source order, so each one's
		// ToString still runs left to right exactly as the pairwise fold ran it. A
		// two-operand concatenation stays a plain Concat, which is already its one
		// copy, so only a chain of three or more takes the join.
		operands := r.stringPlusOperands(left, right)
		pieces := make([]ast.Expr, len(operands))
		for i, op := range operands {
			p, err := r.stringifyOperand(op)
			if err != nil {
				return nil, err
			}
			pieces[i] = p
		}
		r.requireImport(valuePkg)
		if len(pieces) == 2 {
			return &ast.CallExpr{Fun: sel("value", "Concat"), Args: pieces}, nil
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: pieces[0], Sel: ident("ConcatN")}, Args: pieces[1:]}, nil
	}

	// typeof x === "string" on a tagged-sum union lowers to a discriminant compare,
	// x.tag == UnionStr, rather than building the "string" tag and matching it: the
	// union carries a tag that already distinguishes the arms, so the narrowing is
	// one integer compare (tagunion.go). It routes before the string-equality path
	// below because the right side is a string literal that path would otherwise
	// try to compare against a built "string" value. A typeof against a non-arm tag,
	// or over an operand that is not a union local, returns not-handled and falls
	// through to typeof's own folding and the value compare.
	if expr, handled, err := r.typeofUnionCompare(opText, left, right); err != nil {
		return nil, err
	} else if handled {
		return expr, nil
	}

	// s.kind === "circle" on a discriminated object union lowers to a tag compare,
	// s.tag == ShapeOrCircleCircle, rather than reading a kind field and matching a
	// string: the tag already distinguishes the arms, so the narrowing is one integer
	// compare (tagunion.go). It routes before the string-equality path because the
	// discriminant read is a string property the value compare would otherwise lower
	// against a field the sum struct does not carry. A read that is not the union's
	// discriminant, or a literal naming no arm, falls through.
	if expr, handled, err := r.discriminantUnionCompare(opText, left, right); err != nil {
		return nil, err
	} else if handled {
		return expr, nil
	}

	// === and !== on two strings compare by UTF-16 code unit, which is what
	// JavaScript string equality does and what value.BStr.Equal implements. A Go
	// == on the struct would compare backing fields instead and call two strings
	// unequal when one is UTF-8 backed and the other UTF-16 backed but they hold
	// the same code units. Handled before the operator table so the string path
	// emits the method call, negated for !==. A typeof expression counts as a
	// string operand here even when the checker leaves its node untyped: typeof
	// always lowers to a BStr, folded to its tag constant or read at runtime
	// through TypeOf, so typeof x !== "object" over a dynamic x lands on the
	// same Equal call.
	if (opText == "===" || opText == "!==") &&
		(r.isString(left) || r.isTypeofExpr(left)) &&
		(r.isString(right) || r.isTypeofExpr(right)) {
		l, err := r.lowerExpr(left)
		if err != nil {
			return nil, err
		}
		rr, err := r.lowerExpr(right)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		eq := &ast.CallExpr{Fun: &ast.SelectorExpr{X: l, Sel: ident("Equal")}, Args: []ast.Expr{rr}}
		if opText == "!==" {
			return &ast.UnaryExpr{Op: token.NOT, X: eq}, nil
		}
		return eq, nil
	}

	// The relational operators on two strings order them by UTF-16 code unit, the
	// Abstract Relational Comparison, which value.BStr.Compare implements as a
	// three-way result. A Go relational operator on the struct would not compile,
	// and comparing the UTF-8 views with Go's < would misorder any string past the
	// BMP. So each operator lowers to a comparison of Compare against zero: a < b is
	// a.Compare(b) < 0, and so on. Handled before the operator table so the string
	// path emits the method call.
	if relOp, ok := relationalToken(opText); ok && r.isString(left) && r.isString(right) {
		l, err := r.lowerExpr(left)
		if err != nil {
			return nil, err
		}
		rr, err := r.lowerExpr(right)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		cmp := &ast.CallExpr{Fun: &ast.SelectorExpr{X: l, Sel: ident("Compare")}, Args: []ast.Expr{rr}}
		return &ast.BinaryExpr{X: cmp, Op: relOp, Y: &ast.BasicLit{Kind: token.INT, Value: "0"}}, nil
	}

	// === and !== against undefined, where the other operand is an optional
	// (T | undefined, lowered to value.Opt[T]), is a presence test, not a value
	// compare. undefined has no Go value to put on the right of a ==, and the
	// optional carries its presence in a flag, so the comparison lowers to the
	// optional's IsUndefined: x === undefined is x.IsUndefined() and x !== undefined
	// is its negation. Only the undefined operand is skipped; the optional operand
	// is lowered normally to receive the method. A === or !== between two optionals,
	// or an optional against a defined value, is a value compare and not this case,
	// so both operands being checked keeps this to the undefined-literal shape.
	if opText == "===" || opText == "!==" {
		if optNode, ok := r.optionalUndefinedCompare(left, right); ok {
			opt, err := r.lowerExpr(optNode)
			if err != nil {
				return nil, err
			}
			check := &ast.CallExpr{Fun: &ast.SelectorExpr{X: opt, Sel: ident("IsUndefined")}}
			if opText == "!==" {
				return &ast.UnaryExpr{Op: token.NOT, X: check}, nil
			}
			return check, nil
		}
	}

	// A bigint operator is a *big.Int method, never a Go binary operator, because
	// the value is a pointer to an arbitrary-precision integer. The arithmetic
	// operators allocate a fresh big.Int for the result so a shared operand is never
	// mutated (section 4), the relational operators compare through Cmp, and === / !==
	// are a Cmp against zero. Handled before the operator table, whose Go operator
	// would not compile on a *big.Int, and typed on both operands because TypeScript
	// forbids mixing a bigint with any other type in an operator.
	if r.isBigInt(left) && r.isBigInt(right) {
		return r.bigIntBinary(opText, left, right)
	}

	// Remainder on numbers is the one arithmetic operator that is not a Go binary
	// operator: JavaScript % is fmod (a floating remainder that keeps the sign of
	// the dividend), which Go spells math.Mod, not the integer-only % token. It is
	// handled here, before the operator table, so the number path can emit a call.
	if opText == "%" && r.isNumber(left) && r.isNumber(right) {
		// When both sides are integers in the 32-bit range and the divisor is a
		// nonzero integer literal, the remainder is a native Go int32 modulo, not a
		// float one. JavaScript % keeps the sign of the dividend, and so does Go's %
		// on signed integers, so for int32 operands the two agree bit for bit, and
		// the result is always smaller in magnitude than the divisor, so it stays in
		// int32 and the float64 cast is exact. The divisor literal must be nonzero:
		// Go's % panics on a zero divisor where JavaScript yields NaN, so the guard
		// keeps the native form to the cases where a panic cannot happen. A counter
		// folded through i % 26 in a hot loop pays a register op here instead of the
		// math.Mod call the general path emits.
		if r.int32Producing(left) && r.isInt32Literal(right) && !r.isZeroLiteral(right) {
			l, err := r.int32Of(left)
			if err != nil {
				return nil, err
			}
			rr, err := r.int32Of(right)
			if err != nil {
				return nil, err
			}
			mod := &ast.BinaryExpr{X: &ast.ParenExpr{X: l}, Op: token.REM, Y: rr}
			return &ast.CallExpr{Fun: ident("float64"), Args: []ast.Expr{mod}}, nil
		}
		l, err := r.lowerExpr(left)
		if err != nil {
			return nil, err
		}
		rr, err := r.lowerExpr(right)
		if err != nil {
			return nil, err
		}
		r.requireImport("math")
		return &ast.CallExpr{Fun: sel("math", "Mod"), Args: []ast.Expr{l, rr}}, nil
	}

	// Exponentiation on numbers is not a Go operator either: a ** b is defined as
	// Math.pow(a, b) with the same result for every input, so it lowers to value.Pow,
	// the same helper Math.pow lowers to, which keeps the operator and the method
	// identical rather than growing a second spelling that could drift. value.Pow
	// rather than math.Pow because JavaScript returns NaN at a unit base with an
	// infinite or NaN exponent where Go's math.Pow keeps one. Handled before the
	// operator table for the same reason remainder is, so the number path emits the
	// call. A bigint ** routed through its own path earlier, so both operands here
	// are numbers.
	if opText == "**" && r.isNumber(left) && r.isNumber(right) {
		l, err := r.lowerExpr(left)
		if err != nil {
			return nil, err
		}
		rr, err := r.lowerExpr(right)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "Pow"), Args: []ast.Expr{l, rr}}, nil
	}

	// The bitwise operators on numbers do not work on float64: JavaScript coerces
	// each operand to a 32-bit integer, operates, and turns the result back into a
	// number. So they cannot be a plain Go operator on the float64 values; they
	// wrap the operands in value.ToInt32/ToUint32 and the result in a float64 cast.
	// Handled here, before the operator table, so the number path can emit that
	// form rather than a bare Go bitwise operator that would reject a float.
	if goOp, shift, unsignedLeft, ok := bitwiseOp(opText); ok && r.isNumber(left) && r.isNumber(right) {
		return r.bitwiseExpr(goOp, shift, unsignedLeft, left, right)
	}

	goOp, err := r.binaryOp(opText, left, right)
	if err != nil {
		return nil, err
	}

	l, err := r.lowerExpr(left)
	if err != nil {
		return nil, err
	}
	rr, err := r.lowerExpr(right)
	if err != nil {
		return nil, err
	}
	// Arithmetic on two constant number operands folds to a float64 literal, because
	// JavaScript number arithmetic is float64 and Go's is not for integer-spelled
	// constants: 5 + 3 under := infers int, and 7 / 2 folds to the integer 3 rather
	// than 3.5. Folding the node to its float64 value gives := the float64 type it
	// means and the division its real quotient, and it also carries the non-finite
	// cases a Go constant expression rejects: a sum or product past the range is an
	// infinity, and a divide by a constant zero is +Inf, -Inf, or NaN by the sign of
	// the numerator, each emitted as math.Inf or math.NaN so it reads as that number
	// rather than a build error. 5 + 3, 18 / 2 / 9, 1e308 * 2, and 1 / (0 + 0) in the
	// test262 number tests take this shape. The fold fires only when both operands are
	// constant; a runtime operand keeps the live BinaryExpr, which already evaluates in
	// float64 because the number locals are.
	if f, ok := floatConstArith(l, rr, goOp); ok {
		if math.IsInf(f, 0) || math.IsNaN(f) {
			r.requireImport("math")
			return nonFiniteCall(f), nil
		}
		return floatConstLit(f), nil
	}
	return &ast.BinaryExpr{X: l, Op: goOp, Y: rr}, nil
}

// floatConstLit renders a finite float64 as a Go float literal, the folded form of a
// constant number arithmetic. An integer-valued result is written in full with a
// trailing .0 so 8 reads as 8.0 and infers float64 rather than int, and a result with
// a fraction or one large enough that the full form would be unwieldy uses the shortest
// round-tripping decimal, which already carries a point or exponent.
func floatConstLit(f float64) ast.Expr {
	var s string
	if f == math.Trunc(f) && math.Abs(f) < 1e21 {
		s = strconv.FormatFloat(f, 'f', -1, 64) + ".0"
	} else {
		s = strconv.FormatFloat(f, 'g', -1, 64)
	}
	return &ast.BasicLit{Kind: token.FLOAT, Value: s}
}

// nonFiniteCall builds the math call that names a non-finite float64: math.Inf(1)
// for a positive infinity, math.Inf(-1) for a negative one, and math.NaN otherwise.
func nonFiniteCall(f float64) ast.Expr {
	switch {
	case math.IsInf(f, 1):
		return &ast.CallExpr{Fun: sel("math", "Inf"), Args: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: "1"}}}
	case math.IsInf(f, -1):
		return &ast.CallExpr{Fun: sel("math", "Inf"), Args: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: "-1"}}}
	default:
		return &ast.CallExpr{Fun: sel("math", "NaN")}
	}
}

// floatConstArith evaluates l op rr in the float64 domain when both operands are
// constant floats and op is one of the four arithmetic operators, and reports the
// float64 result. This is the arithmetic JavaScript performs on number literals, so
// the result carries the value the language yields: 5 + 3 is 8, 7 / 2 is 3.5, a sum or
// product past the range is an infinity, and 1 / 0 is +Inf, -1 / 0 is -Inf, 0 / 0 is
// NaN. It fires only when both operands are themselves constant floats; anything with
// a runtime operand already evaluates at runtime under IEEE rules in float64.
func floatConstArith(l, rr ast.Expr, op token.Token) (float64, bool) {
	lf, ok := astConstFloat(l)
	if !ok {
		return 0, false
	}
	rf, ok := astConstFloat(rr)
	if !ok {
		return 0, false
	}
	switch op {
	case token.ADD:
		return lf + rf, true
	case token.SUB:
		return lf - rf, true
	case token.MUL:
		return lf * rf, true
	case token.QUO:
		return lf / rf, true
	}
	return 0, false
}

// astConstFloat evaluates a lowered Go expression to its float64 value when it is a
// constant one, so floatConstNonFinite can fold a binary node before it is emitted.
// It walks the literal, parenthesized, signed, and arithmetic shapes a lowered float
// expression is built from, plus the value package's named numeric constants, and
// returns not-ok for anything else, which keeps a runtime operand from being mistaken
// for a constant. The arithmetic is float64 step by step to match the value Go would
// fold and the value JavaScript would compute; division is included here so a divisor
// like -1 / MaxValue + 1 / MaxValue that cancels to zero is seen before the outer
// divide trips the compiler.
func astConstFloat(e ast.Expr) (float64, bool) {
	switch t := e.(type) {
	case *ast.BasicLit:
		if t.Kind != token.INT && t.Kind != token.FLOAT {
			return 0, false
		}
		f, err := strconv.ParseFloat(t.Value, 64)
		if err != nil {
			return 0, false
		}
		return f, true
	case *ast.ParenExpr:
		return astConstFloat(t.X)
	case *ast.UnaryExpr:
		x, ok := astConstFloat(t.X)
		if !ok {
			return 0, false
		}
		switch t.Op {
		case token.SUB:
			return -x, true
		case token.ADD:
			return x, true
		}
		return 0, false
	case *ast.BinaryExpr:
		x, ok := astConstFloat(t.X)
		if !ok {
			return 0, false
		}
		y, ok := astConstFloat(t.Y)
		if !ok {
			return 0, false
		}
		switch t.Op {
		case token.ADD:
			return x + y, true
		case token.SUB:
			return x - y, true
		case token.MUL:
			return x * y, true
		case token.QUO:
			return x / y, true
		case token.REM:
			// A modulus on two int32 constants lowers to a Go % inside a float64 cast
			// (the remainder path above), so a constant zero divisor downstream, 1 / (1 % 1),
			// reaches the fold as a constant. JavaScript % is math.Mod, which keeps the sign
			// of the dividend and yields NaN on a zero divisor, so evaluating it here lets the
			// outer divide fold to the infinity the language gives rather than a Go constant
			// division-by-zero the compiler rejects.
			return math.Mod(x, y), true
		}
		return 0, false
	case *ast.CallExpr:
		// The int32 remainder path wraps its Go % in a float64 conversion, so a constant
		// modulus reaches the fold as float64((a)%b). Unwrapping the conversion lets the
		// remainder inside it evaluate; any other one-argument float64(...) over a constant
		// folds through unchanged, and a conversion over a runtime value stays not-ok.
		if fn, ok := t.Fun.(*ast.Ident); ok && fn.Name == "float64" && len(t.Args) == 1 {
			return astConstFloat(t.Args[0])
		}
		return 0, false
	case *ast.SelectorExpr:
		if pkg, ok := t.X.(*ast.Ident); ok && pkg.Name == "value" {
			if f, ok := valueNumberConst[t.Sel.Name]; ok {
				return f, true
			}
		}
	}
	return 0, false
}

// valueNumberConst maps the value package's finite numeric constants to their float64
// value. Number.MAX_VALUE and its siblings lower to a reference like value.NumberMaxValue
// rather than an inline literal, so astConstFloat resolves the reference here to fold
// expressions built from them; Number.MAX_VALUE + Number.MAX_VALUE overflows to an
// infinity the same way 1e308 + 1e308 does, and this is what lets the fold see it. The
// values match pkg/value/constants.go exactly.
var valueNumberConst = map[string]float64{
	"MathE":                2.718281828459045,
	"MathLN10":             2.302585092994046,
	"MathLN2":              0.6931471805599453,
	"MathLOG10E":           0.4342944819032518,
	"MathLOG2E":            1.4426950408889634,
	"MathPI":               3.141592653589793,
	"MathSQRT12":           0.7071067811865476,
	"MathSQRT2":            1.4142135623730951,
	"NumberEpsilon":        2.220446049250313e-16,
	"NumberMaxSafeInteger": 9007199254740991,
	"NumberMinSafeInteger": -9007199254740991,
	"NumberMaxValue":       1.7976931348623157e308,
	"NumberMinValue":       5e-324,
}

// relationalToken maps a TypeScript relational operator to the Go comparison
// token that has the same meaning against a three-way compare result: a < b is
// Compare(a, b) < 0, and the other three follow the same shape. Only the four
// ordering operators are here; equality is not relational and lowers separately.
func relationalToken(op string) (token.Token, bool) {
	switch op {
	case "<":
		return token.LSS, true
	case "<=":
		return token.LEQ, true
	case ">":
		return token.GTR, true
	case ">=":
		return token.GEQ, true
	default:
		return token.ILLEGAL, false
	}
}

// bitwiseOp maps a TypeScript bitwise operator to the Go token that computes it
// on the coerced integers, and reports how the operands are coerced. shift is
// true for the three shift operators, whose right operand is a shift count masked
// to five bits rather than a second full operand. unsignedLeft is true only for
// the unsigned right shift >>>, whose left operand is coerced with ToUint32 so Go
// does a logical shift and the result is non-negative; every other operator
// coerces the left operand with ToInt32. Arithmetic-versus-logical right shift is
// carried entirely by the operand's signedness, since Go's >> is arithmetic on a
// signed type and logical on an unsigned one, exactly matching >> and >>>.
func bitwiseOp(op string) (goOp token.Token, shift, unsignedLeft, ok bool) {
	switch op {
	case "&":
		return token.AND, false, false, true
	case "|":
		return token.OR, false, false, true
	case "^":
		return token.XOR, false, false, true
	case "<<":
		return token.SHL, true, false, true
	case ">>":
		return token.SHR, true, false, true
	case ">>>":
		return token.SHR, true, true, true
	default:
		return token.ILLEGAL, false, false, false
	}
}

// bitwiseExpr lowers a bitwise expression on two numbers. The operands are
// coerced with value.ToInt32 (or ToUint32 for the left operand of >>>), the Go
// bitwise operator runs on the integers, and the result is cast back to float64
// because a JavaScript bitwise result is a number. For a shift, the right operand
// is a count masked to the low five bits (value.ToUint32(r) & 31), the ECMAScript
// rule that a shift by 32 is a shift by 0.
// wrapInt32AsFloat lowers a node in the int32 domain and casts the result back to
// float64, the shape a bitwise expression takes when its result flows into number
// context. It is the body of the x | 0 identity fold, where the operand is already
// int32 and only the surrounding float64 is needed.
func (r *Renderer) wrapInt32AsFloat(n frontend.Node) (ast.Expr, error) {
	x, err := r.int32Of(n)
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: ident("float64"), Args: []ast.Expr{x}}, nil
}

func (r *Renderer) bitwiseExpr(goOp token.Token, shift, unsignedLeft bool, left, right frontend.Node) (ast.Expr, error) {
	// When both operands already lower to a native int32 (a specialized counter, a
	// bitwise subexpression, a folded literal), the coercions are redundant: their
	// Go int32 value is already the ToInt32 the operator wants. A non-shift bitwise
	// op then stays in registers as float64(l & r) with no value.ToInt32 round trip.
	// The unsigned-left >>> is excluded because its result can exceed the signed
	// range, so it keeps the float64 path that widens through a uint32.
	if !shift && !unsignedLeft && r.int32Producing(left) && r.int32Producing(right) {
		// x | 0 and 0 | x are the coerce-to-int32 idiom, the identity on a value that
		// is already int32, so the OR drops and the operand feeds the float64 cast
		// directly, the same fold int32Binary makes in the int32 domain. This keeps a
		// hot accumulator's trailing | 0 down to float64(acc) with no bitwise op.
		if goOp == token.OR {
			if r.isZeroLiteral(right) {
				return r.wrapInt32AsFloat(left)
			}
			if r.isZeroLiteral(left) {
				return r.wrapInt32AsFloat(right)
			}
		}
		l, err := r.int32Of(left)
		if err != nil {
			return nil, err
		}
		rr, err := r.int32Of(right)
		if err != nil {
			return nil, err
		}
		inner := &ast.BinaryExpr{X: &ast.ParenExpr{X: l}, Op: goOp, Y: rr}
		return &ast.CallExpr{Fun: ident("float64"), Args: []ast.Expr{inner}}, nil
	}
	l, err := r.lowerExpr(left)
	if err != nil {
		return nil, err
	}
	rr, err := r.lowerExpr(right)
	if err != nil {
		return nil, err
	}
	return r.bitwiseFromFloat(goOp, shift, unsignedLeft, l, rr), nil
}

// bitwiseFromFloat builds a bitwise result from two operands already lowered to
// float64: each is coerced with value.ToInt32 (or ToUint32 for the left operand of
// >>>), the Go bitwise operator runs on the integers, and the result casts back to
// float64 because a JavaScript bitwise result is a number. A shift masks its count
// to the low five bits, the ECMAScript rule that a shift by 32 is a shift by 0. This
// is the shared tail of the static-number bitwise path and the dynamic-operand one:
// the first reaches it with a number expression, the second with a ToNumber-coerced
// dynamic value, so both spell the same ToInt32-based form.
func (r *Renderer) bitwiseFromFloat(goOp token.Token, shift, unsignedLeft bool, l, rr ast.Expr) ast.Expr {
	r.requireImport(valuePkg)
	leftConv := "ToInt32"
	if unsignedLeft {
		leftConv = "ToUint32"
	}
	lx := &ast.CallExpr{Fun: sel("value", leftConv), Args: []ast.Expr{l}}
	var inner ast.Expr
	if shift {
		count := &ast.ParenExpr{X: &ast.BinaryExpr{
			X:  &ast.CallExpr{Fun: sel("value", "ToUint32"), Args: []ast.Expr{rr}},
			Op: token.AND,
			Y:  &ast.BasicLit{Kind: token.INT, Value: "31"},
		}}
		inner = &ast.BinaryExpr{X: lx, Op: goOp, Y: count}
	} else {
		rx := &ast.CallExpr{Fun: sel("value", "ToInt32"), Args: []ast.Expr{rr}}
		inner = &ast.BinaryExpr{X: lx, Op: goOp, Y: rx}
	}
	return &ast.CallExpr{Fun: ident("float64"), Args: []ast.Expr{inner}}
}

// stringPlusOperands flattens a left-leaning chain of string + into its operands
// in source order. JavaScript parses a + b + c as ((a + b) + c), so the left spine
// of a string concatenation is a run of + nodes the checker still types as string;
// descending that spine and taking each right operand, with the deepest left as the
// head, yields the flat operand list one ConcatN joins in a single pass. A left
// node that is not a string + stops the descent and becomes one head operand, so a
// parenthesized subexpression or a non-string + folded into the chain stays whole.
func (r *Renderer) stringPlusOperands(left, right frontend.Node) []frontend.Node {
	var out []frontend.Node
	var descend func(n frontend.Node)
	descend = func(n frontend.Node) {
		if n.Kind() == frontend.NodeBinaryExpression && r.isString(n) {
			if kids := r.prog.Children(n); len(kids) == 3 && r.prog.Text(kids[1]) == "+" {
				descend(kids[0])
				out = append(out, kids[2])
				return
			}
		}
		out = append(out, n)
	}
	descend(left)
	out = append(out, right)
	return out
}

// stringifyOperand lowers an operand of a string concatenation to a value.BStr,
// coercing it the way JavaScript + does once the other operand is a string. A
// string operand is already a BStr and passes through; a number becomes
// value.NumberToString and a boolean value.BoolToString, the exact ToString the
// value model runs so a concatenated number reads the same bytes String(x) would
// and matches the engine. A non-primitive operand, whose ToString needs the
// object machinery, hands back for a later slice rather than emitting a coercion
// that does not exist yet.
func (r *Renderer) stringifyOperand(n frontend.Node) (ast.Expr, error) {
	// A caught error concatenated coerces through Error.prototype.toString, "Name:
	// message", the bento string the *value.Error produces directly. It routes before
	// the general lower below, which hands a caught error back because it has no value
	// form outside a .message, .name, or .constructor read.
	if r.isCaughtErrorRef(n) {
		r.requireImport(valuePkg)
		name, _ := localName(r.prog.Text(n))
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(name), Sel: ident("ToBStr")}}, nil
	}
	e, err := r.lowerExpr(n)
	if err != nil {
		return nil, err
	}
	switch {
	case r.isString(n):
		return e, nil
	case r.isNumber(n):
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "NumberToString"), Args: []ast.Expr{e}}, nil
	case r.isBool(n):
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "BoolToString"), Args: []ast.Expr{e}}, nil
	case r.isBigInt(n):
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "BigIntToString"), Args: []ast.Expr{e}}, nil
	case r.isDynamic(n):
		// A dynamic operand coerces at runtime through the value model's
		// ToString, which routes an object or array through ToPrimitive the
		// same way the + operator's own concatenation branch does.
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "ToString"), Args: []ast.Expr{e}}, nil
	default:
		// A non-primitive operand coerces through the same value.ToString protocol
		// the dynamic case uses: ToPrimitive on the object or array, then ToString on
		// the result, so { a: 1 } becomes "[object Object]" and [1, 2] becomes "1,2"
		// the way the engine joins an array. It must box into a dynamic value first,
		// which an object or array literal does through its live-value constructor; a
		// non-primitive whose only form is a Go struct or slice has no box yet and
		// hands back through boxOperand for a later slice.
		boxed, err := r.boxOperand(n)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "ToString"), Args: []ast.Expr{boxed}}, nil
	}
}

// binaryOp picks the Go operator for a TypeScript binary operator given its
// operand types. It dispatches on the operand types so a number path and a
// boolean path stay separate and each guards its own sound operator set; mixed
// or non-primitive operands hand back for a later slice.
func (r *Renderer) binaryOp(opText string, left, right frontend.Node) (token.Token, error) {
	switch {
	case r.isNumber(left) && r.isNumber(right):
		goOp, ok := numericBinaryOp(opText)
		if !ok {
			return token.ILLEGAL, &NotYetLowerable{Reason: "binary operator " + opText + " on numbers is a later slice"}
		}
		return goOp, nil
	case r.isBool(left) && r.isBool(right):
		goOp, ok := booleanBinaryOp(opText)
		if !ok {
			return token.ILLEGAL, &NotYetLowerable{Reason: "binary operator " + opText + " on booleans is a later slice"}
		}
		return goOp, nil
	default:
		if goOp, ok := r.referenceIdentityOp(opText, left, right); ok {
			return goOp, nil
		}
		return token.ILLEGAL, &NotYetLowerable{Reason: "binary operator on mixed or non-primitive operands is a later slice"}
	}
}

// referenceIdentityOp recognizes an equality between two operands of the same
// non-primitive reference type, an object, array, or class instance, and maps it
// to the matching Go pointer comparison. JavaScript === and !== on objects test
// reference identity, and == and != on two objects reduce to the same identity
// since neither operand is a primitive to coerce, so all four lower to Go's == or
// != on the two pointers, which compares the addresses the bindings hold, exactly
// the object identity the language means. Each such value lowers to a Go pointer
// and identity is preserved across its lifetime, so address equality is object
// equality. It fires only when both operands render to the same pointer type, so
// the Go comparison type-checks and stays a comparison of like references; the
// pointer requirement is what marks a by-reference value, and it excludes the Go
// func types that == does not admit. A mixed-type or object-to-primitive compare
// is not this case and keeps its hand-back.
func (r *Renderer) referenceIdentityOp(opText string, left, right frontend.Node) (token.Token, bool) {
	var goOp token.Token
	switch opText {
	case "===", "==":
		goOp = token.EQL
	case "!==", "!=":
		goOp = token.NEQ
	default:
		return token.ILLEGAL, false
	}
	lt := r.prog.TypeAt(left)
	rt := r.prog.TypeAt(right)
	if lt.Flags&frontend.TypeObject == 0 || rt.Flags&frontend.TypeObject == 0 {
		return token.ILLEGAL, false
	}
	ls, err := r.RenderType(lt)
	if err != nil {
		return token.ILLEGAL, false
	}
	rs, err := r.RenderType(rt)
	if err != nil {
		return token.ILLEGAL, false
	}
	if ls != rs || !strings.HasPrefix(ls, "*") {
		return token.ILLEGAL, false
	}
	return goOp, true
}

// numericBinaryOp maps a TypeScript operator on number operands to its Go token.
// The arithmetic operators whose float64 semantics match JavaScript's number
// semantics are here, along with the relational and equality operators, which
// compare two float64 the same way in both languages (=== on numbers is Go ==,
// !== is !=). Loose == and != join them: with both operands typed number no
// coercion runs, so == is exactly ===, and NaN and signed zero compare the same in
// Go as in JavaScript, so the mapping stays sound. Not here because they are not a
// Go binary token: %, which is fmod and lowers to a math.Mod call in binaryExpr.
// Left out on purpose: the bitwise operators, which coerce to int32 first, a later
// slice.
func numericBinaryOp(tsOp string) (token.Token, bool) {
	switch tsOp {
	case "+":
		return token.ADD, true
	case "-":
		return token.SUB, true
	case "*":
		return token.MUL, true
	case "/":
		return token.QUO, true
	case "<":
		return token.LSS, true
	case "<=":
		return token.LEQ, true
	case ">":
		return token.GTR, true
	case ">=":
		return token.GEQ, true
	case "===":
		return token.EQL, true
	case "!==":
		return token.NEQ, true
	case "==":
		return token.EQL, true
	case "!=":
		return token.NEQ, true
	default:
		return token.ILLEGAL, false
	}
}

// booleanBinaryOp maps a TypeScript operator on boolean operands to its Go
// token. The short-circuit && and || carry over directly: Go evaluates the left
// operand first and skips the right on the same condition JavaScript does, and
// with both operands typed boolean the result is boolean in both languages, so
// there is no truthiness gap to bridge. Strict === / !== on two booleans are Go
// == / !=, and loose == / != join them: with both operands typed boolean no
// coercion runs, so == is exactly === on the two bools.
func booleanBinaryOp(tsOp string) (token.Token, bool) {
	switch tsOp {
	case "&&":
		return token.LAND, true
	case "||":
		return token.LOR, true
	case "===":
		return token.EQL, true
	case "!==":
		return token.NEQ, true
	case "==":
		return token.EQL, true
	case "!=":
		return token.NEQ, true
	default:
		return token.ILLEGAL, false
	}
}

// kindName gives a short name for a node kind, for the hand-back reason a caller
// reads when a construct is not lowered yet. It names the kinds this slice
// touches and falls back to the numeric value for the rest, which is enough to
// tell one unhandled form from another in a diagnostic.
func kindName(k frontend.NodeKind) string {
	switch k {
	case frontend.NodeReturnStatement:
		return "return statement"
	case frontend.NodeExpressionStatement:
		return "expression statement"
	case frontend.NodeVariableStatement:
		return "variable statement"
	case frontend.NodeIfStatement:
		return "if statement"
	case frontend.NodeIdentifier:
		return "identifier"
	case frontend.NodeCallExpression:
		return "call expression"
	case frontend.NodePropertyAccessExpression:
		return "property access"
	case frontend.NodeStringLiteral:
		return "string literal"
	case frontend.NodeBinaryExpression:
		return "binary expression"
	default:
		return "kind#" + strconv.Itoa(int(k))
	}
}
