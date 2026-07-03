package lower

import (
	"github.com/tamnd/bento/pkg/frontend"
)

// This file decides which bigint locals own their *big.Int, the aliasing analysis
// doc 05 section 4 asks for. A JavaScript bigint is immutable, so the honest
// lowering of acc = acc * i allocates a fresh big.Int for every result:
// acc = new(big.Int).Mul(acc, i). But a hand-written Go accumulator is
// acc.Mul(acc, i), storing the product into the receiver's own backing array with
// no allocation, and that form is sound exactly when nothing else can hold acc's
// pointer: the assignment kills the old value, so mutating it in place is
// unobservable. bigOwnedLocalsOf proves that per local, conservatively. A local
// that ever shares its pointer (let b = a, a bare return, an argument to a user
// function, a wide-literal initializer that names a shared package var, a nested
// function that captures it) is left unowned and keeps the always-fresh lowering,
// which is correct for every aliasing pattern at the cost of the allocation. The
// analysis errs the same way everywhere: an expression shape it does not recognize
// marks the bigint locals under it escaped, so a wrong answer costs an allocation,
// never a corrupted value.

// bigOwnedLocalsOf analyzes a body and returns the set of bigint local names
// (keyed on the Go local name, like int32Locals) whose *big.Int is provably
// unshared, so a self-referential update may mutate it in place. A name qualifies
// when it is declared exactly once, its initializer allocates fresh (a literal
// within int64, an operator result, a conversion), every plain reassignment also
// writes a fresh value, and no use of the name lets the pointer escape: the
// allowed uses are operand positions of bigint operators and comparisons, the
// argument of a console call or a String/Number/Boolean conversion (each reads
// the value and returns something new), and the target side of its own
// assignments. Everything else, a bare copy into another binding, a
// user-function argument, a return, a ternary arm, a container literal, a nested
// function that mentions it, disqualifies the name.
func (r *Renderer) bigOwnedLocalsOf(body []frontend.Node) map[string]bool {
	f := &bigOwnFacts{
		declCount: map[string]int{},
		fresh:     map[string]bool{},
		escaped:   map[string]bool{},
	}
	for _, n := range body {
		r.collectBigOwnFacts(n, f)
	}

	out := map[string]bool{}
	for name, count := range f.declCount {
		if count == 1 && f.fresh[name] && !f.escaped[name] {
			out[name] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// bigOwnFacts accumulates, per bigint local name, the observations the ownership
// decision needs. Every map is keyed on the Go local name.
type bigOwnFacts struct {
	declCount map[string]int  // number of declarations of the name
	fresh     map[string]bool // every write so far allocated a fresh big.Int
	escaped   map[string]bool // the pointer may be shared somewhere
}

// collectBigOwnFacts walks one node in read position, records what it says about
// bigint locals, and recurses. Read position covers the statements and the
// expression contexts that only read a bigint: a condition, an operator operand,
// a template piece. The contexts that store a value route through
// collectBigOwnValueUse instead, and the container shapes that retain their
// elements (an array or object literal, a new expression) escape everything under
// them, since the container holds the pointer for as long as it lives.
func (r *Renderer) collectBigOwnFacts(n frontend.Node, f *bigOwnFacts) {
	switch n.Kind() {
	case frontend.NodeVariableDeclaration:
		r.collectBigOwnDecl(n, f)
	case frontend.NodeBinaryExpression:
		r.collectBigOwnBinary(n, f)
	case frontend.NodeCallExpression:
		r.collectBigOwnCall(n, f)
	case frontend.NodeFunctionDeclaration, frontend.NodeFunctionExpression, frontend.NodeArrowFunction:
		// A nested function sees the enclosing locals by reference, so any bigint
		// local it mentions may be read after the enclosing body mutates it, at a
		// moment the analysis here cannot order. Everything it names escapes.
		r.markBigIdentsEscaped(n, f)
	case frontend.NodeArrayLiteralExpression, frontend.NodeObjectLiteralExpression, frontend.NodeNewExpression:
		// A container holds its elements' pointers for as long as it lives, past any
		// point this walk can see, so every bigint local stored in one escapes.
		r.markBigIdentsEscaped(n, f)
	case frontend.NodeReturnStatement:
		// A bare `return a` hands the pointer to the caller; an operator result is
		// fresh and returns nothing of a's backing. The returned expression is a
		// stored value either way, so it walks as a value use.
		for _, c := range r.prog.Children(n) {
			r.collectBigOwnValueUse(c, f)
		}
	default:
		for _, c := range r.prog.Children(n) {
			r.collectBigOwnFacts(c, f)
		}
	}
}

// collectBigOwnDecl records a declaration: the count, whether the initializer is
// fresh, and, through the value-use walk of the initializer, that a bare
// identifier initializer (let b = a) put the source's pointer in a second
// binding. A declaration of a non-bigint name still walks its initializer as a
// value use, since an array or object initializer can retain a bigint element.
func (r *Renderer) collectBigOwnDecl(n frontend.Node, f *bigOwnFacts) {
	kids := r.prog.Children(n)
	if len(kids) == 0 {
		return
	}
	nameNode := kids[0]
	init := kids[len(kids)-1]
	hasInit := len(kids) >= 2
	if nameNode.Kind() != frontend.NodeIdentifier || !r.isBigInt(nameNode) {
		if hasInit {
			r.collectBigOwnValueUse(init, f)
		}
		return
	}
	name, ok := localName(r.prog.Text(nameNode))
	if !ok {
		return
	}
	f.declCount[name]++
	if !hasInit {
		f.fresh[name] = false
		return
	}
	if _, seen := f.fresh[name]; !seen {
		f.fresh[name] = true
	}
	if !r.bigExprIsFresh(init) {
		f.fresh[name] = false
	}
	r.collectBigOwnValueUse(init, f)
}

// collectBigOwnBinary records what an assignment does to its target and walks the
// rest. A compound assignment on a bigint local writes an operator result, which
// is fresh either way it is emitted, and only reads its right-hand side; a plain
// assignment is fresh only when its right-hand side is, and stores that side, so
// it walks as a value use. An assignment to any other target (a property, an
// element) stores its right-hand side somewhere this walk cannot see, so that
// side is a value use too. A non-assignment binary is an operator whose operands
// are read, not retained.
func (r *Renderer) collectBigOwnBinary(n frontend.Node, f *bigOwnFacts) {
	parts := r.prog.Children(n)
	if len(parts) != 3 {
		return
	}
	op := r.prog.Text(parts[1])
	_, compound := compoundBaseOp(op)
	if op != "=" && !compound {
		for _, c := range parts {
			r.collectBigOwnFacts(c, f)
		}
		return
	}
	target := parts[0]
	if target.Kind() == frontend.NodeIdentifier && r.isBigInt(target) {
		if name, ok := localName(r.prog.Text(target)); ok {
			if op == "=" {
				if !r.bigExprIsFresh(parts[2]) {
					f.fresh[name] = false
				}
				r.collectBigOwnValueUse(parts[2], f)
			} else {
				r.collectBigOwnFacts(parts[2], f)
			}
			return
		}
	}
	r.collectBigOwnFacts(target, f)
	r.collectBigOwnValueUse(parts[2], f)
}

// collectBigOwnCall walks a call. The conversion and console callees read a
// bigint argument and build something new from it, so their arguments stay owned;
// any other callee may retain or return the pointer, so its arguments walk as
// value uses: a bare bigint identifier argument escapes, while an
// operator-expression argument passes a fresh value and only its own operands
// are walked.
func (r *Renderer) collectBigOwnCall(n frontend.Node, f *bigOwnFacts) {
	kids := r.prog.Children(n)
	if len(kids) == 0 {
		return
	}
	callee := kids[0]
	if r.bigReadOnlyCallee(callee) {
		for _, a := range kids[1:] {
			r.collectBigOwnFacts(a, f)
		}
		return
	}
	for _, a := range kids[1:] {
		r.collectBigOwnValueUse(a, f)
	}
	r.collectBigOwnFacts(callee, f)
}

// collectBigOwnValueUse walks an expression whose value is stored, passed, or
// returned. A bare bigint identifier in that position shares its pointer, so it
// escapes; BigInt(x) is transparent because on a bigint argument it is the
// identity and hands back the same pointer; an operator expression produces a
// fresh value, so only its operands are walked; a ternary stores whichever arm it
// took, so both arms are value uses. Any shape this walk does not recognize
// escapes every bigint local under it, so an unmodeled store costs the
// optimization rather than a corrupted value.
func (r *Renderer) collectBigOwnValueUse(n frontend.Node, f *bigOwnFacts) {
	switch n.Kind() {
	case frontend.NodeIdentifier:
		r.markBigEscaped(n, f)
	case frontend.NodeParenthesizedExpression:
		for _, c := range r.prog.Children(n) {
			r.collectBigOwnValueUse(c, f)
		}
	case frontend.NodeConditionalExpression:
		kids := r.prog.Children(n)
		if len(kids) != 3 {
			r.markBigIdentsEscaped(n, f)
			return
		}
		r.collectBigOwnFacts(kids[0], f)
		r.collectBigOwnValueUse(kids[1], f)
		r.collectBigOwnValueUse(kids[2], f)
	case frontend.NodeCallExpression:
		kids := r.prog.Children(n)
		if len(kids) == 2 && kids[0].Kind() == frontend.NodeIdentifier &&
			r.prog.Text(kids[0]) == "BigInt" && r.isAmbientGlobal(kids[0]) && r.isBigInt(kids[1]) {
			r.collectBigOwnValueUse(kids[1], f)
			return
		}
		r.collectBigOwnFacts(n, f)
	case frontend.NodeBinaryExpression:
		parts := r.prog.Children(n)
		if len(parts) == 3 {
			op := r.prog.Text(parts[1])
			_, compound := compoundBaseOp(op)
			if op == "=" || compound {
				// An assignment used as a value hands the same pointer to two bindings
				// at once, a shape the lowering rejects anyway; everything under it
				// escapes.
				r.markBigIdentsEscaped(n, f)
				return
			}
		}
		r.collectBigOwnFacts(n, f)
	case frontend.NodePrefixUnaryExpression, frontend.NodePostfixUnaryExpression,
		frontend.NodeTemplateExpression, frontend.NodeNoSubstitutionTemplateLiteral,
		frontend.NodeBigIntLiteral, frontend.NodeNumericLiteral, frontend.NodeStringLiteral,
		frontend.NodeTrueKeyword, frontend.NodeFalseKeyword, frontend.NodeNullKeyword:
		r.collectBigOwnFacts(n, f)
	default:
		r.markBigIdentsEscaped(n, f)
	}
}

// bigReadOnlyCallee reports whether a callee reads a bigint argument without
// retaining its pointer: a console method, or a String/Number/Boolean conversion,
// each of which lowers to a helper that builds a new value from the digits. The
// BigInt conversion is deliberately not here: on a bigint argument it is the
// identity, so its aliasing depends on where its result goes, which
// collectBigOwnValueUse tracks.
func (r *Renderer) bigReadOnlyCallee(callee frontend.Node) bool {
	switch callee.Kind() {
	case frontend.NodePropertyAccessExpression:
		kids := r.prog.Children(callee)
		return len(kids) == 2 && kids[0].Kind() == frontend.NodeIdentifier &&
			r.prog.Text(kids[0]) == "console" && r.isAmbientGlobal(kids[0])
	case frontend.NodeIdentifier:
		switch r.prog.Text(callee) {
		case "String", "Number", "Boolean":
			return r.isAmbientGlobal(callee)
		}
	}
	return false
}

// bigExprIsFresh reports whether an expression's value is a big.Int no other
// binding can hold: a literal small enough to lower to big.NewInt (a wide literal
// lowers to a shared package var, so it is not fresh), an operator result, or a
// conversion from a number, string, or boolean. A bare identifier, a call into
// user code, and anything unrecognized are not fresh.
func (r *Renderer) bigExprIsFresh(n frontend.Node) bool {
	switch n.Kind() {
	case frontend.NodeBigIntLiteral:
		v, ok := bigIntLiteralValue(r.prog.Text(n))
		return ok && v.IsInt64()
	case frontend.NodeParenthesizedExpression:
		kids := r.prog.Children(n)
		return len(kids) == 1 && r.bigExprIsFresh(kids[0])
	case frontend.NodeBinaryExpression:
		parts := r.prog.Children(n)
		if len(parts) != 3 || !r.isBigInt(n) {
			return false
		}
		op := r.prog.Text(parts[1])
		_, compound := compoundBaseOp(op)
		return op != "=" && !compound
	case frontend.NodePrefixUnaryExpression:
		return r.isBigInt(n)
	case frontend.NodeCallExpression:
		kids := r.prog.Children(n)
		if len(kids) != 2 || kids[0].Kind() != frontend.NodeIdentifier {
			return false
		}
		// BigInt(x) allocates for a number, string, or boolean argument and is the
		// identity on a bigint one, so freshness follows the argument.
		if r.prog.Text(kids[0]) == "BigInt" && r.isAmbientGlobal(kids[0]) {
			return !r.isBigInt(kids[1])
		}
		return false
	default:
		return false
	}
}

// markBigEscaped marks one identifier's name escaped when it is a bigint local.
func (r *Renderer) markBigEscaped(n frontend.Node, f *bigOwnFacts) {
	if !r.isBigInt(n) {
		return
	}
	if name, ok := localName(r.prog.Text(n)); ok {
		f.escaped[name] = true
	}
}

// markBigIdentsEscaped walks a subtree and marks every bigint identifier in it
// escaped, the blanket rule for a nested function body, a container literal, and
// any value-position shape the analysis does not model.
func (r *Renderer) markBigIdentsEscaped(n frontend.Node, f *bigOwnFacts) {
	if n.Kind() == frontend.NodeIdentifier {
		r.markBigEscaped(n, f)
		return
	}
	for _, c := range r.prog.Children(n) {
		r.markBigIdentsEscaped(c, f)
	}
}
