package lower

import (
	"go/ast"
	"go/token"
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
		// A local declared as an optional (value.Opt[T]) that the checker narrowed to
		// T at this use, past a presence guard like `x !== undefined`, reads the stored
		// value out with .Get(). The narrowing shows as the type at this node no longer
		// carrying undefined; a read where the type is still the optional keeps the bare
		// Opt value, which is what the presence test and an Opt-to-Opt assignment want.
		if r.optLocals[name] && !r.isOptional(n) {
			return &ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(name), Sel: ident("Get")}}, nil
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

	case frontend.NodeBinaryExpression:
		return r.binaryExpr(n)

	case frontend.NodeCallExpression:
		return r.callExpr(n)

	case frontend.NodePrefixUnaryExpression:
		return r.prefixUnary(n)

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

	case frontend.NodeNewExpression:
		return r.newExpr(n)

	default:
		return nil, &NotYetLowerable{Reason: "expression kind " + kindName(n.Kind()) + " is a later slice"}
	}
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
		if !r.isNumber(operand) {
			return nil, &NotYetLowerable{Reason: "unary minus on a non-number is a later slice"}
		}
		x, err := r.lowerExpr(operand)
		if err != nil {
			return nil, err
		}
		return &ast.UnaryExpr{Op: token.SUB, X: x}, nil
	case "+":
		if !r.isNumber(operand) {
			return nil, &NotYetLowerable{Reason: "unary plus on a non-number is a later slice"}
		}
		return r.lowerExpr(operand)
	case "!":
		if !r.isBool(operand) {
			return nil, &NotYetLowerable{Reason: "logical not on a non-boolean needs truthiness, a later slice"}
		}
		x, err := r.lowerExpr(operand)
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
		// binary bitwise operators use, not a Go ^ on the float64.
		if !r.isNumber(operand) {
			return nil, &NotYetLowerable{Reason: "bitwise not on a non-number is a later slice"}
		}
		x, err := r.lowerExpr(operand)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		conv := &ast.CallExpr{Fun: sel("value", "ToInt32"), Args: []ast.Expr{x}}
		return &ast.CallExpr{Fun: ident("float64"), Args: []ast.Expr{&ast.UnaryExpr{Op: token.XOR, X: conv}}}, nil
	default:
		return nil, &NotYetLowerable{Reason: "prefix operator " + op + " is a later slice"}
	}
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
		return nil, &NotYetLowerable{Reason: "assignment used as a value is a later slice"}
	}
	return r.combineBinary(opText, left, right)
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

	// Nullish coalescing is a presence test on the left, not a truthiness test,
	// so it routes here before the operator table rather than desugaring to || .
	// It lowers only the optional shape with a pure fallback and hands back the
	// rest (section on null and undefined).
	if opText == "??" {
		return r.nullishCoalesce(left, right)
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

	// === and !== on two strings compare by UTF-16 code unit, which is what
	// JavaScript string equality does and what value.BStr.Equal implements. A Go
	// == on the struct would compare backing fields instead and call two strings
	// unequal when one is UTF-8 backed and the other UTF-16 backed but they hold
	// the same code units. Handled before the operator table so the string path
	// emits the method call, negated for !==.
	if (opText == "===" || opText == "!==") && r.isString(left) && r.isString(right) {
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

	// Exponentiation on numbers is the other arithmetic operator that is not a Go
	// binary token: JavaScript a ** b is the same operation as Math.pow(a, b),
	// which Go spells math.Pow, so it emits a call here before the operator table
	// the way % does. ** is right-associative, so a ** b ** c parses as
	// a ** (b ** c); each operand already arrives as its own lowered subtree, so
	// the nesting is carried by the AST and produces math.Pow(a, math.Pow(b, c))
	// without any special handling here.
	if opText == "**" && r.isNumber(left) && r.isNumber(right) {
		l, err := r.lowerExpr(left)
		if err != nil {
			return nil, err
		}
		rr, err := r.lowerExpr(right)
		if err != nil {
			return nil, err
		}
		r.requireImport("math")
		return &ast.CallExpr{Fun: sel("math", "Pow"), Args: []ast.Expr{l, rr}}, nil
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
	return &ast.BinaryExpr{X: l, Op: goOp, Y: rr}, nil
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
	return &ast.CallExpr{Fun: ident("float64"), Args: []ast.Expr{inner}}, nil
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
	default:
		return nil, &NotYetLowerable{Reason: "string concatenation with a non-primitive operand is a later slice"}
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
		return token.ILLEGAL, &NotYetLowerable{Reason: "binary operator on mixed or non-primitive operands is a later slice"}
	}
}

// numericBinaryOp maps a TypeScript operator on number operands to its Go token.
// The arithmetic operators whose float64 semantics match JavaScript's number
// semantics are here, along with the relational and strict-equality operators,
// which compare two float64 the same way in both languages (=== on numbers is Go
// ==, !== is !=). Not here because they are not a Go binary token: %, which is
// fmod and lowers to a math.Mod call in binaryExpr. Left out on purpose: the
// bitwise operators, which coerce to int32 first, and loose == and !=, whose
// coercion has no direct Go spelling. Each is a later slice.
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
	default:
		return token.ILLEGAL, false
	}
}

// booleanBinaryOp maps a TypeScript operator on boolean operands to its Go
// token. The short-circuit && and || carry over directly: Go evaluates the left
// operand first and skips the right on the same condition JavaScript does, and
// with both operands typed boolean the result is boolean in both languages, so
// there is no truthiness gap to bridge. Strict === / !== on two booleans are Go
// == / !=. Left out on purpose: loose == and !=, whose coercion has no direct Go
// spelling, a later slice.
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
