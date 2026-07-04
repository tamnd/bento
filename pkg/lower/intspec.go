package lower

import (
	"go/ast"
	"go/token"
	"math"
	"strconv"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
	"github.com/tamnd/bento/pkg/value"
)

// This file specializes a number-typed local to a Go int32 when the checker's
// facts prove every value it ever holds is a 32-bit integer. A JavaScript number
// is a float64, so bento lowers a number local to a Go float64 and every bitwise
// operator on it round trips through value.ToInt32/ToUint32 and, for the bit-exact
// Math methods, through the non-inlined value.Imul and value.Clz32. That is the
// honest lowering for a value that could be any double, but a loop counter or a
// bit-mixing accumulator never leaves the int32 range, and paying the coercion on
// every operation is pure overhead. When int32LocalsOf can prove a local stays in
// range, varDeclStmt gives it a Go int32 type and its writes lower through int32Of,
// which keeps the integer in the machine register and emits native Go bit operators
// with no float conversion. The read side is unchanged: lowerExpr wraps an
// int32-specialized local in float64(name) so every existing consumer, from
// arithmetic to console.log to a Math call, sees the same float64 it did before.
// The specialization is invisible except in speed.
//
// The two ends agree by construction. ECMAScript defines the bitwise operators and
// Math.imul as ToInt32 of an integer result, and native Go two's-complement
// arithmetic on int32 wraps modulo 2^32, which is exactly ToInt32 for a value that
// is already an integer. A native + or - of two int32 values agrees with the
// float64-then-ToInt32 form because the true sum has magnitude below 2^33, far
// under 2^53, so no double rounding occurs and the wrap is the coercion. A native *
// does not have that guarantee, since a product can exceed 2^53, so only Math.imul,
// whose specification is the low 32 bits of the product, takes the native multiply;
// a plain * falls back to the float path. The analysis is conservative: a local is
// specialized only when every write is provably int32 and every update keeps it in
// range, so a value that could reach a fraction or a magnitude past 2^31 stays a
// float64 and keeps the coercing lowering.

// int32LocalsOf analyzes a body and returns the set of local names that can be
// specialized to Go int32. The body is the sequence of statements of a function or
// of the module's top-level, and the walk descends through nested blocks so a
// counter declared in an inner loop is seen. A name is eligible when it is a plain
// number local (not dynamic, string, or boolean), is declared exactly once, is
// used at least once as an integer (an operand of a bitwise operator, ~, or a
// Math.imul/Math.clz32 argument), has no compound assignment, and every value ever
// written to it is inherently int32. A name that also takes ++ or -- must be a
// bounded for-counter (an int32-literal start and an int32-literal relational bound)
// so the increment provably cannot leave the range; a name with no ++ or -- needs
// only the write test. A name declared more than once is disqualified outright,
// since two declarations may disagree on type and the flat name set cannot tell
// their scopes apart.
func (r *Renderer) int32LocalsOf(body []frontend.Node) map[string]bool {
	f := &int32Facts{
		numberLocal: map[string]bool{},
		declCount:   map[string]int{},
		writes:      map[string][]frontend.Node{},
		compound:    map[string]bool{},
		incDec:      map[string]bool{},
		counterOK:   map[string]bool{},
		intUse:      map[string]bool{},
	}
	for _, n := range body {
		r.collectInt32Facts(n, f)
	}

	out := map[string]bool{}
	for name, isNum := range f.numberLocal {
		if !isNum {
			continue
		}
		if f.declCount[name] != 1 || f.compound[name] || !f.intUse[name] {
			continue
		}
		if f.incDec[name] && !f.counterOK[name] {
			continue
		}
		allInt32 := true
		for _, w := range f.writes[name] {
			if !r.inherentlyInt32(w) {
				allInt32 = false
				break
			}
		}
		if !allInt32 {
			continue
		}
		out[name] = true
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// int32Facts accumulates, per local name, the observations int32LocalsOf needs to
// decide eligibility. Every map is keyed on the Go local name.
type int32Facts struct {
	numberLocal map[string]bool            // declared as a plain number local
	declCount   map[string]int             // number of declarations of the name
	writes      map[string][]frontend.Node // initializers and assignment right-hand sides
	compound    map[string]bool            // has a compound assignment (+=, &=, ...)
	incDec      map[string]bool            // has a ++ or --
	counterOK   map[string]bool            // is a bounded int32 for-counter
	intUse      map[string]bool            // is used at least once as an integer
}

// collectInt32Facts walks one node, records the facts it carries about the locals
// it touches, and recurses into its children. It is a single pass: the declaration,
// assignment, and update shapes contribute the write and update facts, the bitwise
// and Math shapes contribute the integer-use facts, and a for-statement adds the
// counter test on top of the generic recursion into its parts.
func (r *Renderer) collectInt32Facts(n frontend.Node, f *int32Facts) {
	switch n.Kind() {
	case frontend.NodeVariableDeclaration:
		r.collectDeclFacts(n, f)
	case frontend.NodeBinaryExpression:
		r.collectBinaryFacts(n, f)
	case frontend.NodePrefixUnaryExpression, frontend.NodePostfixUnaryExpression:
		r.collectUnaryFacts(n, f)
	case frontend.NodeCallExpression:
		r.collectCallFacts(n, f)
	case frontend.NodeForStatement:
		if name, ok := r.forCounter(n); ok {
			f.counterOK[name] = true
		}
	case frontend.NodeElementAccessExpression:
		r.collectIndexFacts(n, f)
	}
	for _, c := range r.prog.Children(n) {
		r.collectInt32Facts(c, f)
	}
}

// collectDeclFacts records a variable declaration: the name is a number local when
// the checker types it a plain number, the declaration is counted, and the
// initializer, if any, is recorded as a write.
func (r *Renderer) collectDeclFacts(d frontend.Node, f *int32Facts) {
	kids := r.prog.Children(d)
	if len(kids) == 0 || kids[0].Kind() != frontend.NodeIdentifier {
		return
	}
	name, ok := localName(r.prog.Text(kids[0]))
	if !ok {
		return
	}
	f.declCount[name]++
	if r.isPlainNumber(kids[0]) {
		f.numberLocal[name] = true
	} else {
		// A name that is ever declared as something other than a plain number is not
		// specializable, and marking it false keeps a later plain-number declaration of
		// the same name from resurrecting it.
		f.numberLocal[name] = f.numberLocal[name] && false
	}
	if len(kids) == 2 || len(kids) == 3 {
		f.writes[name] = append(f.writes[name], kids[len(kids)-1])
	}
}

// collectBinaryFacts records an assignment's right-hand side as a write, a compound
// assignment as disqualifying, and marks an identifier operand of a bitwise
// operator as an integer use.
func (r *Renderer) collectBinaryFacts(bin frontend.Node, f *int32Facts) {
	parts := r.prog.Children(bin)
	if len(parts) != 3 {
		return
	}
	opText := r.prog.Text(parts[1])
	if parts[0].Kind() == frontend.NodeIdentifier {
		if name, ok := localName(r.prog.Text(parts[0])); ok {
			if opText == "=" {
				f.writes[name] = append(f.writes[name], parts[2])
			} else if _, isCompound := compoundBaseOp(opText); isCompound {
				f.compound[name] = true
			}
		}
	}
	if _, _, _, ok := bitwiseOp(opText); ok {
		r.markIntUse(parts[0], f)
		r.markIntUse(parts[2], f)
	}
	// A remainder by a nonzero integer literal, x % 97, reads x as an integer: the
	// funcgen modulo fast path lowers it to a native Go % when x is int32-producing,
	// so counting it as an integer use lets a loop counter that only ever feeds a
	// modulo (the replace workload's i % 97 and i % 13) specialize and skip math.Mod.
	// Only the left operand is the integer use; the literal divisor is not a local.
	if opText == "%" && r.isInt32Literal(parts[2]) && !r.isZeroLiteral(parts[2]) {
		r.markIntUse(parts[0], f)
	}
}

// collectIndexFacts marks the locals that drive an element-access index as integer
// uses. An index expression a[i] reads i as an integer, so a bounded for-counter
// that only ever indexes an array or typed array still qualifies for int32
// specialization and lowers to a native slice index. The walk descends through the
// arithmetic an index commonly takes, a[i - 1] or a[i + 1], so the counter inside
// is marked even when the index is not a bare identifier. Marking a name here only
// makes it eligible; the write and counter tests still decide specialization, so a
// float that happens to sit in an index is never wrongly narrowed.
func (r *Renderer) collectIndexFacts(n frontend.Node, f *int32Facts) {
	kids := r.prog.Children(n)
	if len(kids) != 2 {
		return
	}
	r.markIndexIdents(kids[1], f)
}

// markIndexIdents marks every identifier an index expression reads as an integer
// use, descending through parentheses and the additive, multiplicative, bitwise,
// and remainder operators an index is built from. A literal carries no name and a
// shape it does not recognize simply contributes nothing.
func (r *Renderer) markIndexIdents(n frontend.Node, f *int32Facts) {
	switch n.Kind() {
	case frontend.NodeParenthesizedExpression:
		kids := r.prog.Children(n)
		if len(kids) == 1 {
			r.markIndexIdents(kids[0], f)
		}
	case frontend.NodeIdentifier:
		r.markIntUse(n, f)
	case frontend.NodeBinaryExpression:
		parts := r.prog.Children(n)
		if len(parts) == 3 {
			r.markIndexIdents(parts[0], f)
			r.markIndexIdents(parts[2], f)
		}
	}
}

// collectUnaryFacts records a ++ or -- on an identifier and marks the operand of a
// bitwise not as an integer use.
func (r *Renderer) collectUnaryFacts(u frontend.Node, f *int32Facts) {
	kids := r.prog.Children(u)
	if len(kids) != 1 {
		return
	}
	operand := kids[0]
	op := unaryOpText(r.prog.Text(u), r.prog.Text(operand))
	switch op {
	case "++", "--":
		if operand.Kind() == frontend.NodeIdentifier {
			if name, ok := localName(r.prog.Text(operand)); ok {
				f.incDec[name] = true
			}
		}
	case "~":
		r.markIntUse(operand, f)
	}
}

// collectCallFacts marks the identifier arguments of Math.imul and Math.clz32 as
// integer uses, the two Math methods whose arguments are read as 32-bit integers.
func (r *Renderer) collectCallFacts(call frontend.Node, f *int32Facts) {
	method, args, ok := r.mathMethodCall(call)
	if !ok {
		return
	}
	if method == "imul" || method == "clz32" {
		for _, a := range args {
			r.markIntUse(a, f)
		}
	}
}

// markIntUse marks an identifier, seen through any parentheses, as used at least
// once as an integer. A non-identifier operand carries no name to mark.
func (r *Renderer) markIntUse(n frontend.Node, f *int32Facts) {
	if name, ok := r.identName(n); ok {
		f.intUse[name] = true
	}
}

// forCounter recognizes a for-statement whose loop variable is a bounded int32
// counter and returns its name. The initializer must be a single declaration of an
// int32-literal start, the condition must be a relational compare of that name
// against an int32-literal bound, and the update must be name++ or name--. Those
// three together keep the counter inside the int32 range for the life of the loop,
// so specializing it to int32 cannot overflow. Any other shape returns false and
// leaves the counter a float64.
func (r *Renderer) forCounter(n frontend.Node) (string, bool) {
	kids := r.prog.Children(n)
	if len(kids) != 4 {
		return "", false
	}
	var decls []frontend.Node
	collectVarDecls(r.prog, kids[0], &decls)
	if len(decls) != 1 {
		return "", false
	}
	dkids := r.prog.Children(decls[0])
	if len(dkids) < 2 || dkids[0].Kind() != frontend.NodeIdentifier {
		return "", false
	}
	name, ok := localName(r.prog.Text(dkids[0]))
	if !ok {
		return "", false
	}
	if !r.isInt32ConstOrLiteral(dkids[len(dkids)-1]) {
		return "", false
	}

	cond := kids[1]
	if cond.Kind() != frontend.NodeBinaryExpression {
		return "", false
	}
	cparts := r.prog.Children(cond)
	if len(cparts) != 3 {
		return "", false
	}
	if _, isRel := relationalToken(r.prog.Text(cparts[1])); !isRel {
		return "", false
	}
	condName, ok := r.identName(cparts[0])
	if !ok || condName != name || !r.isInt32ConstOrLiteral(cparts[2]) {
		return "", false
	}

	incr := kids[2]
	if incr.Kind() != frontend.NodePrefixUnaryExpression && incr.Kind() != frontend.NodePostfixUnaryExpression {
		return "", false
	}
	ikids := r.prog.Children(incr)
	if len(ikids) != 1 {
		return "", false
	}
	iname, ok := r.identName(ikids[0])
	if !ok || iname != name {
		return "", false
	}
	switch unaryOpText(r.prog.Text(incr), r.prog.Text(ikids[0])) {
	case "++", "--":
		return name, true
	}
	return "", false
}

// inherentlyInt32 reports whether an expression's value is provably a 32-bit signed
// integer regardless of what its operands hold. A bitwise operator (other than the
// unsigned right shift, whose result can exceed the signed range), a bitwise not, a
// Math.imul or Math.clz32 call, and an integer literal in the int32 range all
// qualify. Everything else, including a bare +, does not, because a sum can leave
// the range unless a surrounding coercion pulls it back. int32LocalsOf uses this on
// every write to a candidate local: a local is specialized only when it is written
// nothing but inherently-int32 values, which is what makes the Go int32 variable a
// faithful mirror of the JavaScript number at every assignment.
func (r *Renderer) inherentlyInt32(n frontend.Node) bool {
	switch n.Kind() {
	case frontend.NodeParenthesizedExpression:
		kids := r.prog.Children(n)
		return len(kids) == 1 && r.inherentlyInt32(kids[0])
	case frontend.NodeNumericLiteral:
		return r.isInt32Literal(n)
	case frontend.NodeBinaryExpression:
		parts := r.prog.Children(n)
		if len(parts) != 3 {
			return false
		}
		op := r.prog.Text(parts[1])
		if op == ">>>" {
			return false
		}
		_, _, _, ok := bitwiseOp(op)
		return ok
	case frontend.NodePrefixUnaryExpression:
		kids := r.prog.Children(n)
		if len(kids) != 1 {
			return false
		}
		return unaryOpText(r.prog.Text(n), r.prog.Text(kids[0])) == "~"
	case frontend.NodeCallExpression:
		method, _, ok := r.mathMethodCall(n)
		return ok && (method == "imul" || method == "clz32")
	default:
		return false
	}
}

// int32Producing reports whether int32Of will lower this node to a native Go int32
// expression rather than the value.ToInt32 fallback. It drives two decisions: a +
// or - lowers to a native Go operator only when both sides are int32-producing, and
// uint32Of reinterprets rather than coerces only when its operand is. It mirrors the
// structure int32Of walks, so the two never disagree.
func (r *Renderer) int32Producing(n frontend.Node) bool {
	switch n.Kind() {
	case frontend.NodeParenthesizedExpression:
		kids := r.prog.Children(n)
		return len(kids) == 1 && r.int32Producing(kids[0])
	case frontend.NodeIdentifier:
		name, ok := localName(r.prog.Text(n))
		return ok && r.int32Locals[name]
	case frontend.NodeNumericLiteral:
		_, ok := foldInt32Literal(r.prog.Text(n))
		return ok
	case frontend.NodePrefixUnaryExpression:
		kids := r.prog.Children(n)
		return len(kids) == 1 && unaryOpText(r.prog.Text(n), r.prog.Text(kids[0])) == "~"
	case frontend.NodeBinaryExpression:
		parts := r.prog.Children(n)
		if len(parts) != 3 {
			return false
		}
		op := r.prog.Text(parts[1])
		switch op {
		case "&", "|", "^", "<<", ">>":
			return true
		case "+", "-":
			return r.int32Producing(parts[0]) && r.int32Producing(parts[2])
		default:
			return false
		}
	case frontend.NodeCallExpression:
		method, _, ok := r.mathMethodCall(n)
		return ok && (method == "imul" || method == "clz32")
	case frontend.NodeElementAccessExpression:
		// A proven-in-range read of an integer typed array is a native slice index
		// widened to int32, so it stays in the integer domain and composes with the +,
		// -, and bitwise producers around it.
		_, _, ok := r.provenTypedRead(n)
		return ok
	default:
		return false
	}
}

// int32Of lowers an expression in the int32 domain: it returns a Go expression of
// type int32 that equals the ECMAScript ToInt32 of the node's value. It is used for
// the initializer and the assignment right-hand side of an int32-specialized local,
// where the surrounding assignment to an int32 variable is the ToInt32 the value
// would otherwise get from a | 0 or a Math.imul. The native cases (an int32 local
// read, an integer literal folded through ToInt32, the bitwise operators, ~, a
// native + or - of two int32 producers, and Math.imul and Math.clz32) stay in
// registers. Anything else falls back to value.ToInt32 of the ordinary float64
// lowering, which is always correct, so an unrecognized shape loses the speedup but
// never the answer.
func (r *Renderer) int32Of(n frontend.Node) (ast.Expr, error) {
	switch n.Kind() {
	case frontend.NodeParenthesizedExpression:
		kids := r.prog.Children(n)
		if len(kids) != 1 {
			return r.int32Fallback(n)
		}
		inner, err := r.int32Of(kids[0])
		if err != nil {
			return nil, err
		}
		return &ast.ParenExpr{X: inner}, nil

	case frontend.NodeIdentifier:
		name, ok := localName(r.prog.Text(n))
		if ok && r.int32Locals[name] {
			return ident(name), nil
		}
		return r.int32Fallback(n)

	case frontend.NodeNumericLiteral:
		if v, ok := foldInt32Literal(r.prog.Text(n)); ok {
			return int32Lit(v), nil
		}
		return r.int32Fallback(n)

	case frontend.NodePrefixUnaryExpression:
		kids := r.prog.Children(n)
		if len(kids) == 1 && unaryOpText(r.prog.Text(n), r.prog.Text(kids[0])) == "~" {
			x, err := r.int32Of(kids[0])
			if err != nil {
				return nil, err
			}
			return &ast.UnaryExpr{Op: token.XOR, X: x}, nil
		}
		return r.int32Fallback(n)

	case frontend.NodeBinaryExpression:
		return r.int32Binary(n)

	case frontend.NodeCallExpression:
		return r.int32Call(n)

	case frontend.NodeElementAccessExpression:
		// A proven-in-range integer typed-array read lowers to int32(recv.Data()[idx]):
		// the stored element widened to int32 is the ToInt32 of the Number the read
		// would hand out, so it stands in for the checked read with none of its bounds
		// branch or truncation. An access that is not proven in range falls back to the
		// ToInt32 of the ordinary At read, which is always correct.
		if info, idxNode, ok := r.provenTypedRead(n); ok {
			recvNode := r.prog.Children(n)[0]
			return r.typedSliceRead(recvNode, idxNode, "int32", info)
		}
		return r.int32Fallback(n)

	default:
		return r.int32Fallback(n)
	}
}

// int32Binary lowers a binary expression in the int32 domain. The bitwise operators
// map to the Go token straight, a shift masks its count to five bits the way the
// bitwise lowering does, x | 0 folds to x since it is the identity on an int32, and
// a + or - stays native only when both operands are int32-producing. Every other
// operator, and a + or - with a non-integer side, falls back.
func (r *Renderer) int32Binary(n frontend.Node) (ast.Expr, error) {
	parts := r.prog.Children(n)
	if len(parts) != 3 {
		return r.int32Fallback(n)
	}
	op := r.prog.Text(parts[1])
	switch op {
	case "|":
		// x | 0 is the identity on a 32-bit integer, the idiom that coerces a number to
		// int32. When one side is the literal 0 and the other is already int32, the OR
		// is dropped so the specialized form matches the hand-written native shape.
		if r.isZeroLiteral(parts[2]) && r.int32Producing(parts[0]) {
			return r.int32Of(parts[0])
		}
		if r.isZeroLiteral(parts[0]) && r.int32Producing(parts[2]) {
			return r.int32Of(parts[2])
		}
		return r.int32BitOp(token.OR, parts[0], parts[2])
	case "&":
		return r.int32BitOp(token.AND, parts[0], parts[2])
	case "^":
		return r.int32BitOp(token.XOR, parts[0], parts[2])
	case "<<":
		return r.int32Shift(token.SHL, parts[0], parts[2])
	case ">>":
		return r.int32Shift(token.SHR, parts[0], parts[2])
	case "+", "-":
		if r.int32Producing(parts[0]) && r.int32Producing(parts[2]) {
			l, err := r.int32Of(parts[0])
			if err != nil {
				return nil, err
			}
			rr, err := r.int32Of(parts[2])
			if err != nil {
				return nil, err
			}
			tok := token.ADD
			if op == "-" {
				tok = token.SUB
			}
			return &ast.BinaryExpr{X: l, Op: tok, Y: rr}, nil
		}
		return r.int32Fallback(n)
	default:
		return r.int32Fallback(n)
	}
}

// int32BitOp lowers a non-shift bitwise operator to a Go binary expression on the
// int32 lowering of each operand.
func (r *Renderer) int32BitOp(tok token.Token, left, right frontend.Node) (ast.Expr, error) {
	l, err := r.int32Of(left)
	if err != nil {
		return nil, err
	}
	rr, err := r.int32Of(right)
	if err != nil {
		return nil, err
	}
	return &ast.BinaryExpr{X: l, Op: tok, Y: rr}, nil
}

// int32Shift lowers << or >> to a Go shift whose count is the operand coerced to
// uint32 and masked to five bits, the ECMAScript rule that a shift amount is taken
// modulo 32. The left operand is an int32, so Go's >> is arithmetic, which matches
// the signed right shift; the unsigned >>> is not routed here, since its result can
// exceed the signed range.
func (r *Renderer) int32Shift(tok token.Token, left, count frontend.Node) (ast.Expr, error) {
	l, err := r.int32Of(left)
	if err != nil {
		return nil, err
	}
	amount, err := r.uint32Of(count)
	if err != nil {
		return nil, err
	}
	masked := &ast.ParenExpr{X: &ast.BinaryExpr{X: amount, Op: token.AND, Y: &ast.BasicLit{Kind: token.INT, Value: "31"}}}
	return &ast.BinaryExpr{X: l, Op: tok, Y: masked}, nil
}

// int32Call lowers Math.imul and Math.clz32 in the int32 domain. imul is a native
// int32 multiply, the one multiply that keeps only the low 32 bits, so it needs no
// coercion around the product. clz32 is value.Clz32U, the integer core of clz32
// that skips the ToUint32-and-widen its float64 form pays, fed the uint32 of its
// argument. Any other call falls back.
func (r *Renderer) int32Call(n frontend.Node) (ast.Expr, error) {
	method, args, ok := r.mathMethodCall(n)
	if !ok {
		return r.int32Fallback(n)
	}
	switch {
	case method == "imul" && len(args) == 2:
		l, err := r.int32Of(args[0])
		if err != nil {
			return nil, err
		}
		rr, err := r.int32Of(args[1])
		if err != nil {
			return nil, err
		}
		return &ast.BinaryExpr{X: l, Op: token.MUL, Y: rr}, nil
	case method == "clz32" && len(args) == 1:
		u, err := r.uint32Of(args[0])
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "Clz32U"), Args: []ast.Expr{u}}, nil
	default:
		return r.int32Fallback(n)
	}
}

// uint32Of lowers an expression to a Go uint32 that equals its ECMAScript ToUint32.
// An int32-producing operand is reinterpreted with a Go conversion, which is the
// two's-complement bit pattern ToUint32 defines, so no float round trip is needed.
// Anything else goes through value.ToUint32 of the ordinary float64 lowering.
func (r *Renderer) uint32Of(n frontend.Node) (ast.Expr, error) {
	if r.int32Producing(n) {
		x, err := r.int32Of(n)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: ident("uint32"), Args: []ast.Expr{x}}, nil
	}
	e, err := r.lowerExpr(n)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "ToUint32"), Args: []ast.Expr{e}}, nil
}

// int32Fallback lowers a node the int32 domain does not recognize by taking its
// ordinary float64 lowering and coercing it with value.ToInt32. It is always
// correct, so it is the safe floor under every int32Of case.
func (r *Renderer) int32Fallback(n frontend.Node) (ast.Expr, error) {
	e, err := r.lowerExpr(n)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "ToInt32"), Args: []ast.Expr{e}}, nil
}

// mathMethodCall recognizes a call of the form Math.method(args...) and returns the
// method name and the argument nodes. The callee must be a property access whose
// object is the ambient Math global, so a method named the same on a user object
// does not match.
func (r *Renderer) mathMethodCall(n frontend.Node) (string, []frontend.Node, bool) {
	if n.Kind() != frontend.NodeCallExpression {
		return "", nil, false
	}
	kids := r.prog.Children(n)
	if len(kids) == 0 || kids[0].Kind() != frontend.NodePropertyAccessExpression {
		return "", nil, false
	}
	access := r.prog.Children(kids[0])
	if len(access) != 2 {
		return "", nil, false
	}
	if !r.isGlobalRef(access[0], "Math") {
		return "", nil, false
	}
	return r.prog.Text(access[1]), kids[1:], true
}

// identName returns the local name of an identifier seen through any parentheses,
// the shape a bitwise operand or a for-counter often takes.
func (r *Renderer) identName(n frontend.Node) (string, bool) {
	if n.Kind() == frontend.NodeParenthesizedExpression {
		kids := r.prog.Children(n)
		if len(kids) != 1 {
			return "", false
		}
		return r.identName(kids[0])
	}
	if n.Kind() != frontend.NodeIdentifier {
		return "", false
	}
	return localName(r.prog.Text(n))
}

// isPlainNumber reports whether a node's type is a plain number, the type that
// lowers to a Go float64 and is a candidate for int32 specialization. A dynamic,
// string, or boolean type is not, so a union or an any keeps its own lowering.
func (r *Renderer) isPlainNumber(n frontend.Node) bool {
	return r.isNumber(n) && !r.isDynamic(n) && !r.isString(n) && !r.isBool(n)
}

// isInt32Literal reports whether a node is a numeric literal whose value is an
// integer in the signed 32-bit range, the shape that can start or bound a counter
// or be written to an int32 local without leaving the range.
func (r *Renderer) isInt32Literal(n frontend.Node) bool {
	if n.Kind() == frontend.NodeParenthesizedExpression {
		kids := r.prog.Children(n)
		return len(kids) == 1 && r.isInt32Literal(kids[0])
	}
	if n.Kind() != frontend.NodeNumericLiteral {
		return false
	}
	v, ok := parseIntegerLiteral(r.prog.Text(n))
	return ok && v >= math.MinInt32 && v <= math.MaxInt32
}

// isInt32ConstOrLiteral reports whether a node is an int32 literal or a const local
// bound to one, the shape that can start or bound a counter. Resolving a const N to
// its value lets an idiomatic for (let i = 0; i < N; i++) specialize its counter to a
// Go int32 the same way a written literal bound does. It reads the value through
// intLiteralValue, which returns a value only for a literal or a known const.
func (r *Renderer) isInt32ConstOrLiteral(n frontend.Node) bool {
	_, ok := r.intLiteralValue(n)
	return ok
}

// isZeroLiteral reports whether a node is the numeric literal 0, the right side of
// the x | 0 coercion idiom that int32Binary folds away.
func (r *Renderer) isZeroLiteral(n frontend.Node) bool {
	if n.Kind() == frontend.NodeParenthesizedExpression {
		kids := r.prog.Children(n)
		return len(kids) == 1 && r.isZeroLiteral(kids[0])
	}
	if n.Kind() != frontend.NodeNumericLiteral {
		return false
	}
	v, ok := parseIntegerLiteral(r.prog.Text(n))
	return ok && v == 0
}

// unaryOpText recovers a unary operator's text by removing the operand's text from
// the whole node's text, the same way lowerIncDec and prefixUnary read it. It works
// for the prefix forms (~x, -x) and the postfix forms (x++), since only the
// operator is left once the operand is stripped.
func unaryOpText(whole, operand string) string {
	return strings.TrimSpace(strings.ReplaceAll(whole, operand, ""))
}

// int32Lit builds a Go integer literal for a folded int32 constant, emitting a
// unary minus for a negative value since a Go integer literal token is unsigned.
// The constant is left untyped so it takes the int32 type from the operation it
// feeds.
func int32Lit(v int32) ast.Expr {
	if v < 0 {
		return &ast.UnaryExpr{Op: token.SUB, X: &ast.BasicLit{Kind: token.INT, Value: strconv.FormatInt(int64(-int64(v)), 10)}}
	}
	return &ast.BasicLit{Kind: token.INT, Value: strconv.FormatInt(int64(v), 10)}
}

// foldInt32Literal parses a numeric literal and folds it to its ECMAScript ToInt32
// value, so an out-of-range constant like 2654435761 becomes the int32 it coerces
// to (-1640531535) at compile time rather than at runtime. It handles the decimal,
// hex, octal, and binary integer forms and an integer-valued float, and reports
// false for a fractional or non-finite literal, which has no int32 lowering.
func foldInt32Literal(text string) (int32, bool) {
	t := strings.ReplaceAll(text, "_", "")
	if i, err := strconv.ParseInt(t, 0, 64); err == nil {
		return value.ToInt32(float64(i)), true
	}
	if u, err := strconv.ParseUint(t, 0, 64); err == nil {
		return value.ToInt32(float64(u)), true
	}
	if f, err := strconv.ParseFloat(t, 64); err == nil && !math.IsInf(f, 0) && !math.IsNaN(f) && f == math.Trunc(f) {
		return value.ToInt32(f), true
	}
	return 0, false
}

// parseIntegerLiteral parses a numeric literal that is an exact integer and returns
// its value, for the range tests that decide whether a literal is a safe int32
// start or bound. A fractional or non-finite literal reports false.
func parseIntegerLiteral(text string) (int64, bool) {
	t := strings.ReplaceAll(text, "_", "")
	if i, err := strconv.ParseInt(t, 0, 64); err == nil {
		return i, true
	}
	if f, err := strconv.ParseFloat(t, 64); err == nil && !math.IsInf(f, 0) && !math.IsNaN(f) && f == math.Trunc(f) {
		return int64(f), true
	}
	return 0, false
}
