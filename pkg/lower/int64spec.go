package lower

import (
	"go/ast"
	"go/token"
	"math"
	"math/big"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file specializes a number-typed local to a Go int64 when interval analysis
// proves every value it ever holds is an integer inside the safe-integer range
// (|v| <= 2^53 - 1) but wider than 32 bits, so a big-sum accumulator or a wide
// integer constant lives in a machine register instead of a float64. It is the
// second integer tier after intspec.go: the int32 tier takes the locals whose
// writes are inherently 32-bit (bitwise results, imul, bounded counters), and this
// tier takes the ones whose writes are plain + - * arithmetic over proven-small
// integers, the shape the int32 tier must refuse because a sum or product can
// leave its range.
//
// The soundness argument is different from int32's and worth stating. JavaScript
// computes the arithmetic in float64, and float64 represents every integer of
// magnitude at most 2^53 exactly, so a + - * whose true result stays inside the
// safe range is computed exactly by the double and exactly by a Go int64, and the
// two agree bit for bit. The analysis therefore proves an interval for every
// operation in the value's lifetime, not just the final write: each + - * node's
// result interval must sit inside the safe range, which makes the double exact at
// that node, and an int64 cannot overflow there either, since the interval bounds
// it far below 2^63. An accumulator write like sum = sum + d inside bounded loops
// is proven by drift: the deltas' interval times the loops' total trip count,
// added to the initializer's interval, must stay inside the safe range, which
// bounds every partial sum along the way.
//
// Unlike the int32 tier there is no always-correct runtime fallback: ToInt32 is a
// real ECMAScript coercion the fallback can lean on, but nothing in the language
// coerces a double to "the int64 it already is". So where int32Of falls back,
// int64Of must never be reached with an unproven shape; the analysis rejects the
// whole local instead, and the local stays a float64. The failure mode is a missed
// optimization, never a wrong answer.
//
// The read side follows intspec.go exactly: lowerExpr wraps an int64-specialized
// local in float64(name), which is exact for a value inside the safe range, so
// every consumer sees the float64 a number local always presented and the
// specialization is visible only on the declaration and the writes.

// safeIntMax is Number.MAX_SAFE_INTEGER, 2^53 - 1, the largest integer magnitude a
// float64 represents exactly along with all of its predecessors. Every interval
// this analysis accepts sits inside [-safeIntMax, safeIntMax].
const safeIntMax = int64(1)<<53 - 1

// ivl64 is an inclusive int64 interval [lo, hi] a value is proven to stay within.
// It is the wide sibling of ivl, which the counter-range proof keeps in int.
type ivl64 struct {
	lo, hi int64
}

// inSafeRange reports whether the whole interval sits inside the safe-integer
// range, the bound that makes float64 arithmetic exact over it.
func (v ivl64) inSafeRange() bool {
	return v.lo >= -safeIntMax && v.hi <= safeIntMax
}

// inInt32Range reports whether the whole interval sits inside the signed 32-bit
// range. A local whose envelope fits there is left alone: the int64 tier exists
// for values wider than 32 bits, and the narrow ones belong to the int32 tier or
// to the plain float64 lowering.
func (v ivl64) inInt32Range() bool {
	return v.lo >= math.MinInt32 && v.hi <= math.MaxInt32
}

// hull64 is the smallest interval containing both arguments.
func hull64(a, b ivl64) ivl64 {
	if b.lo < a.lo {
		a.lo = b.lo
	}
	if b.hi > a.hi {
		a.hi = b.hi
	}
	return a
}

// int64Write records one write to a candidate local: the right-hand side (or
// declaration initializer) and the product of the trip counts of the bounded
// loops enclosing the write site. trips is nil when any enclosing construct can
// repeat an unknown number of times (a while, a for the range proof does not
// recognize, a nested function), which disqualifies an accumulator write there.
type int64Write struct {
	rhs   frontend.Node
	trips *big.Int
}

// int64LocalsOf analyzes a body and returns the set of local names to specialize
// to Go int64. A name is eligible when it is a plain number local declared exactly
// once, has no compound assignment and no ++/--, is not already int32-specialized,
// is never the target of a destructuring write, and its value envelope, the
// interval covering the initializer, every plain write, and the drift of every
// accumulator write over its bounded loops, is proven to sit inside the
// safe-integer range while reaching past the int32 range. The envelope test is
// what admits sum = sum + i * i over a bounded counter and rejects the same write
// under a while loop or with a bound that could push the sum past 2^53.
func (r *Renderer) int64LocalsOf(body []frontend.Node) map[string]bool {
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
	cands := map[string]bool{}
	for name, isNum := range f.numberLocal {
		if !isNum || f.declCount[name] != 1 || f.compound[name] || f.incDec[name] || r.int32Locals[name] {
			continue
		}
		cands[name] = true
	}
	if len(cands) == 0 {
		return nil
	}

	writes := map[string][]int64Write{}
	badTarget := map[string]bool{}
	var walk func(n frontend.Node, trips *big.Int)
	walk = func(n frontend.Node, trips *big.Int) {
		childTrips := trips
		switch n.Kind() {
		case frontend.NodeVariableDeclaration:
			kids := r.prog.Children(n)
			if len(kids) >= 2 && kids[0].Kind() == frontend.NodeIdentifier {
				if name, ok := localName(r.prog.Text(kids[0])); ok && cands[name] {
					writes[name] = append(writes[name], int64Write{rhs: kids[len(kids)-1], trips: trips})
				}
			}
		case frontend.NodeBinaryExpression:
			parts := r.prog.Children(n)
			if len(parts) == 3 && r.prog.Text(parts[1]) == "=" {
				if parts[0].Kind() == frontend.NodeIdentifier {
					if name, ok := localName(r.prog.Text(parts[0])); ok && cands[name] {
						writes[name] = append(writes[name], int64Write{rhs: parts[2], trips: trips})
					}
				} else {
					// A destructuring target writes its names through a path this
					// specialization does not teach, so any candidate inside one keeps
					// its float64 type.
					for name := range cands {
						if r.mentionsName(parts[0], name) {
							badTarget[name] = true
						}
					}
				}
			}
		case frontend.NodeForStatement:
			if _, iv, ok := r.loopCounterRange(n); ok && trips != nil {
				count := big.NewInt(int64(iv.hi) - int64(iv.lo) + 1)
				childTrips = new(big.Int).Mul(trips, count)
			} else {
				childTrips = nil
			}
		case frontend.NodeForOfStatement, frontend.NodeForInStatement, frontend.NodeWhileStatement,
			frontend.NodeFunctionDeclaration, frontend.NodeFunctionExpression, frontend.NodeArrowFunction,
			frontend.NodeMethodDeclaration, frontend.NodeGetAccessor, frontend.NodeSetAccessor,
			frontend.NodeConstructor, frontend.NodeUnknown:
			// A loop with no proven trip count repeats a write an unknown number of
			// times, and a nested function runs whenever it is called; a do...while
			// and a labeled statement surface as NodeUnknown and land here too.
			// Writes below keep an unknown context, which rejects any accumulator
			// among them.
			childTrips = nil
		}
		for _, c := range r.prog.Children(n) {
			walk(c, childTrips)
		}
	}
	one := big.NewInt(1)
	for _, n := range body {
		walk(n, one)
	}

	out := map[string]bool{}
	for name := range cands {
		if badTarget[name] {
			continue
		}
		env, ok := r.int64Envelope(name, writes[name])
		if ok && env.inSafeRange() && !env.inInt32Range() {
			out[name] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// int64Envelope computes the interval a candidate's value stays within across its
// whole life, or ok=false when any write is unprovable. A write that does not
// mention the name contributes its own interval to the base hull. A write that
// does must be the accumulator shape name = name + d or name = name - d with the
// name only in that leading position and the site under bounded loops; it
// contributes a drift of at most trips times the delta interval, extended toward
// zero so a loop that runs fewer than its maximum iterations is covered too. The
// result is a bound on every value the local ever holds, including each partial
// sum, since a partial accumulation lies between the base and the full drift.
func (r *Renderer) int64Envelope(name string, ws []int64Write) (ivl64, bool) {
	if len(ws) == 0 {
		return ivl64{}, false
	}
	haveBase := false
	var base ivl64
	accLo := new(big.Int)
	accHi := new(big.Int)
	for _, w := range ws {
		rhs := r.unwrapParens(w.rhs)
		if !r.mentionsName(rhs, name) {
			iv, ok := r.int64IvlOf(rhs)
			if !ok {
				return ivl64{}, false
			}
			if haveBase {
				base = hull64(base, iv)
			} else {
				base, haveBase = iv, true
			}
			continue
		}
		if w.trips == nil || rhs.Kind() != frontend.NodeBinaryExpression {
			return ivl64{}, false
		}
		parts := r.prog.Children(rhs)
		if len(parts) != 3 {
			return ivl64{}, false
		}
		op := r.prog.Text(parts[1])
		if op != "+" && op != "-" {
			return ivl64{}, false
		}
		ln, ok := r.identName(parts[0])
		if !ok || ln != name || r.mentionsName(parts[2], name) {
			return ivl64{}, false
		}
		d, ok := r.int64IvlOf(parts[2])
		if !ok {
			return ivl64{}, false
		}
		lo, hi := d.lo, d.hi
		if op == "-" {
			lo, hi = -hi, -lo
		}
		if lo < 0 {
			accLo.Add(accLo, new(big.Int).Mul(w.trips, big.NewInt(lo)))
		}
		if hi > 0 {
			accHi.Add(accHi, new(big.Int).Mul(w.trips, big.NewInt(hi)))
		}
	}
	if !haveBase {
		// Every write is an accumulation, so nothing established a starting value;
		// a declaration with no provable initializer already failed above, and a
		// candidate always has its declaration in the write list.
		return ivl64{}, false
	}
	lo := new(big.Int).Add(big.NewInt(base.lo), accLo)
	hi := new(big.Int).Add(big.NewInt(base.hi), accHi)
	if !lo.IsInt64() || !hi.IsInt64() {
		return ivl64{}, false
	}
	return ivl64{lo: lo.Int64(), hi: hi.Int64()}, true
}

// int64IvlOf computes the interval an expression's value is proven to stay
// within, walking exactly the shapes int64Of lowers so the proof and the emitted
// code never disagree. A literal is its own value, a bounded counter its header
// range, a const its value, an int32-specialized local the int32 range, and an
// inherently-int32 expression (a bitwise result, ~, imul, clz32) the int32 range
// too. A + - * combines its operands' intervals and must land inside the safe
// range, which is what makes the float64 the language computes exact at that
// node. Any other shape has no proven range and reports false.
func (r *Renderer) int64IvlOf(n frontend.Node) (ivl64, bool) {
	switch n.Kind() {
	case frontend.NodeParenthesizedExpression:
		kids := r.prog.Children(n)
		if len(kids) != 1 {
			return ivl64{}, false
		}
		return r.int64IvlOf(kids[0])
	case frontend.NodeNumericLiteral:
		v, ok := parseIntegerLiteral(r.prog.Text(n))
		if !ok || v < -safeIntMax || v > safeIntMax {
			return ivl64{}, false
		}
		return ivl64{lo: v, hi: v}, true
	case frontend.NodeIdentifier:
		name, ok := localName(r.prog.Text(n))
		if !ok {
			return ivl64{}, false
		}
		if iv, ok := r.counterIvl[name]; ok {
			return ivl64{lo: int64(iv.lo), hi: int64(iv.hi)}, true
		}
		if v, ok := r.constInt[name]; ok {
			return ivl64{lo: int64(v), hi: int64(v)}, true
		}
		if r.int32Locals[name] {
			return ivl64{lo: math.MinInt32, hi: math.MaxInt32}, true
		}
		return ivl64{}, false
	case frontend.NodePrefixUnaryExpression:
		kids := r.prog.Children(n)
		if len(kids) != 1 {
			return ivl64{}, false
		}
		if unaryOpText(r.prog.Text(n), r.prog.Text(kids[0])) == "-" {
			iv, ok := r.int64IvlOf(kids[0])
			if !ok {
				return ivl64{}, false
			}
			return ivl64{lo: -iv.hi, hi: -iv.lo}, true
		}
		if r.inherentlyInt32(n) {
			return ivl64{lo: math.MinInt32, hi: math.MaxInt32}, true
		}
		return ivl64{}, false
	case frontend.NodeBinaryExpression:
		parts := r.prog.Children(n)
		if len(parts) != 3 {
			return ivl64{}, false
		}
		switch r.prog.Text(parts[1]) {
		case "+", "-", "*":
			a, ok := r.int64IvlOf(parts[0])
			if !ok {
				return ivl64{}, false
			}
			b, ok := r.int64IvlOf(parts[2])
			if !ok {
				return ivl64{}, false
			}
			switch r.prog.Text(parts[1]) {
			case "+":
				return safeIvlAdd(a, b)
			case "-":
				return safeIvlAdd(a, ivl64{lo: -b.hi, hi: -b.lo})
			default:
				return safeIvlMul(a, b)
			}
		}
		if r.inherentlyInt32(n) {
			return ivl64{lo: math.MinInt32, hi: math.MaxInt32}, true
		}
		return ivl64{}, false
	case frontend.NodeCallExpression:
		if r.inherentlyInt32(n) {
			return ivl64{lo: math.MinInt32, hi: math.MaxInt32}, true
		}
		return ivl64{}, false
	}
	return ivl64{}, false
}

// safeIvlAdd adds two intervals already inside the safe range and accepts the
// result only when it stays inside too. The operand bound keeps the int64 sums
// far from overflow: two magnitudes at most 2^53 sum to at most 2^54.
func safeIvlAdd(a, b ivl64) (ivl64, bool) {
	out := ivl64{lo: a.lo + b.lo, hi: a.hi + b.hi}
	if !out.inSafeRange() {
		return ivl64{}, false
	}
	return out, true
}

// safeIvlMul multiplies two intervals through big.Int corner products, since two
// safe-range operands can multiply past int64, and accepts the result only when
// it stays inside the safe range. Bounding the product there is also what keeps
// the runtime int64 multiply exact: every actual product lies between the
// corners, so none can overflow.
func safeIvlMul(a, b ivl64) (ivl64, bool) {
	corners := [4]*big.Int{
		new(big.Int).Mul(big.NewInt(a.lo), big.NewInt(b.lo)),
		new(big.Int).Mul(big.NewInt(a.lo), big.NewInt(b.hi)),
		new(big.Int).Mul(big.NewInt(a.hi), big.NewInt(b.lo)),
		new(big.Int).Mul(big.NewInt(a.hi), big.NewInt(b.hi)),
	}
	lo, hi := corners[0], corners[0]
	for _, c := range corners[1:] {
		if c.Cmp(lo) < 0 {
			lo = c
		}
		if c.Cmp(hi) > 0 {
			hi = c
		}
	}
	if !lo.IsInt64() || !hi.IsInt64() {
		return ivl64{}, false
	}
	out := ivl64{lo: lo.Int64(), hi: hi.Int64()}
	if !out.inSafeRange() {
		return ivl64{}, false
	}
	return out, true
}

// int64Of lowers an expression in the int64 domain: it returns a Go expression of
// type int64 that equals the number the expression evaluates to, which the
// analysis proved is an integer in the safe range. An int64 local reads as the
// bare name; a counter, const, or int32 local reads through a Go conversion,
// exact because the value is integral and in range; a literal folds to a Go
// integer literal; + - * are native int64 operators; and an inherently-int32
// shape routes through int32Of and widens. There is no coercing fallback here,
// deliberately: nothing in the language coerces a double to an int64, so a shape
// this walk does not recognize was never approved by int64IvlOf, and reaching one
// is a defensive hand-back rather than a silent wrong answer.
func (r *Renderer) int64Of(n frontend.Node) (ast.Expr, error) {
	switch n.Kind() {
	case frontend.NodeParenthesizedExpression:
		kids := r.prog.Children(n)
		if len(kids) != 1 {
			break
		}
		inner, err := r.int64Of(kids[0])
		if err != nil {
			return nil, err
		}
		return &ast.ParenExpr{X: inner}, nil

	case frontend.NodeNumericLiteral:
		v, ok := parseIntegerLiteral(r.prog.Text(n))
		if !ok {
			break
		}
		return goIntLit(v), nil

	case frontend.NodeIdentifier:
		name, ok := localName(r.prog.Text(n))
		if !ok {
			break
		}
		if r.int64Locals[name] {
			return ident(name), nil
		}
		if _, isCounter := r.counterIvl[name]; isCounter || r.int32Locals[name] {
			// The counter is a float64 (or an int32) variable holding an integer the
			// range proof bounded, so a Go conversion to int64 is exact.
			return &ast.CallExpr{Fun: ident("int64"), Args: []ast.Expr{ident(name)}}, nil
		}
		if _, isConst := r.constInt[name]; isConst {
			return &ast.CallExpr{Fun: ident("int64"), Args: []ast.Expr{ident(name)}}, nil
		}

	case frontend.NodePrefixUnaryExpression:
		kids := r.prog.Children(n)
		if len(kids) == 1 && unaryOpText(r.prog.Text(n), r.prog.Text(kids[0])) == "-" {
			x, err := r.int64Of(kids[0])
			if err != nil {
				return nil, err
			}
			return &ast.UnaryExpr{Op: token.SUB, X: x}, nil
		}
		if r.inherentlyInt32(n) {
			return r.int64FromInt32(n)
		}

	case frontend.NodeBinaryExpression:
		parts := r.prog.Children(n)
		if len(parts) != 3 {
			break
		}
		var tok token.Token
		switch r.prog.Text(parts[1]) {
		case "+":
			tok = token.ADD
		case "-":
			tok = token.SUB
		case "*":
			tok = token.MUL
		default:
			if r.inherentlyInt32(n) {
				return r.int64FromInt32(n)
			}
			tok = token.ILLEGAL
		}
		if tok != token.ILLEGAL {
			l, err := r.int64Of(parts[0])
			if err != nil {
				return nil, err
			}
			rr, err := r.int64Of(parts[2])
			if err != nil {
				return nil, err
			}
			return &ast.BinaryExpr{X: l, Op: tok, Y: rr}, nil
		}

	case frontend.NodeCallExpression:
		if r.inherentlyInt32(n) {
			return r.int64FromInt32(n)
		}
	}
	return nil, &NotYetLowerable{Reason: "expression outside the proven int64 domain"}
}

// int64FromInt32 lowers an inherently-int32 expression through the int32 domain
// and widens it to int64, exact since every int32 is a safe integer.
func (r *Renderer) int64FromInt32(n frontend.Node) (ast.Expr, error) {
	x, err := r.int32Of(n)
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: ident("int64"), Args: []ast.Expr{x}}, nil
}

// mentionsName reports whether an expression reads an identifier of the given
// name anywhere in its tree. It is deliberately coarse (a property named the same
// counts too), which only rejects more, never less.
func (r *Renderer) mentionsName(n frontend.Node, name string) bool {
	if n.Kind() == frontend.NodeIdentifier {
		nm, ok := localName(r.prog.Text(n))
		return ok && nm == name
	}
	for _, c := range r.prog.Children(n) {
		if r.mentionsName(c, name) {
			return true
		}
	}
	return false
}
