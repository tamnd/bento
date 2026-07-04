package lower

import (
	"go/ast"
	"go/token"
	"math/bits"
	"strconv"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
	"github.com/tamnd/bento/pkg/value"
)

// This file proves that an integer typed-array index stays inside the array, so
// the access can drop the runtime bounds branch and the Number round trip and read
// or write the backing slice directly. The ordinary lowering routes a[i] through
// the At and SetAt methods, which fold a NaN, truncate the Number index, bounds
// check, and, on a store, run the element kind's coerce function through a pointer.
// A loop counter that walks a fixed-length array never leaves the array's bounds
// and never holds a NaN, so all of that is dead work. When the counter's range is
// known to sit inside the array's fixed length, the access lowers to bData[i] on
// the array's own slice (16 §6.3, plan 05 §M5): the read is a plain slice index and
// an integer store is a Go conversion to the element width, which wraps exactly as
// the store coercion does. The proof is what makes the bare slice index sound: the
// out-of-range read-zero and drop-write the checked methods give never happen, so
// replacing them with a slice index that would instead panic out of range changes
// nothing observable. The slice still carries Go's own bounds check, so a wrong
// proof is memory safe, never a corruption; the proof only has to be right for the
// program to keep its meaning.

// ivl is an inclusive integer interval [lo, hi] a value is known to stay within. It
// is used for a for-counter's range and for an index expression built from the
// counter and integer literals; an index proven to sit inside [0, length-1] takes
// the native slice path.
type ivl struct {
	lo, hi int
}

// typedArrInfo records what the native path needs to know about a fixed-length
// integer typed array: how many elements it holds and the Go element type a store
// converts to. Only the wrapping integer kinds appear here (Int8/Int16/Uint16/
// Int32/Uint32); the clamped and float kinds keep the checked store because their
// coercion is not a plain width conversion, and the byte buffer has its own slice
// type.
type typedArrInfo struct {
	length int
	elemGo string
}

// intTypedArrayLen maps a typed-array constructor name to the Go element type of
// its wrapping-integer store, and ok=false for a kind whose store is not a plain
// width conversion. Uint8ClampedArray clamps rather than wraps, the float kinds
// round, and Uint8Array is a separate byte buffer, so none take the native store.
func intTypedArrayLen(name string) (string, bool) {
	switch name {
	case "Int8Array":
		return "int8", true
	case "Int16Array":
		return "int16", true
	case "Uint16Array":
		return "uint16", true
	case "Int32Array":
		return "int32", true
	case "Uint32Array":
		return "uint32", true
	default:
		return "", false
	}
}

// counterIvlOf returns, for each bounded increasing for-counter in the body, the
// interval it ranges over. A counter qualifies only when its start and bound are
// integer literals, its update is name++, and its condition is name < bound or
// name <= bound, so the values it takes are known exactly: [start, bound-1] for the
// strict compare and [start, bound] for the inclusive one. A counter that is
// mutated anywhere besides that single update, or that a second loop also declares,
// is dropped, since then the header no longer bounds its value. A decreasing loop
// is left out and its accesses stay on the checked path.
func (r *Renderer) counterIvlOf(body []frontend.Node) map[string]ivl {
	out := map[string]ivl{}
	var walk func(n frontend.Node)
	walk = func(n frontend.Node) {
		if n.Kind() == frontend.NodeForStatement {
			if name, iv, ok := r.loopCounterRange(n); ok {
				if _, dup := out[name]; dup {
					// A second loop over the same name cannot be told apart by the flat map, so
					// neither range is trusted.
					out[name] = ivl{lo: 1, hi: 0}
				} else {
					out[name] = iv
				}
			}
		}
		for _, c := range r.prog.Children(n) {
			walk(c)
		}
	}
	for _, n := range body {
		walk(n)
	}
	// A counter mutated outside its single header update no longer has the header's
	// range, so drop any whose body changes it. The header update counts as one
	// mutation; anything past that disqualifies.
	for name, iv := range out {
		if iv.lo > iv.hi || r.mutationCount(body, name) != 1 {
			delete(out, name)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// loopCounterRange reads a for-statement's header and returns the interval an
// increasing counter takes, or ok=false for any shape it does not recognize. It is
// stricter than forCounter: it accepts only name++ with a < or <= bound, since a
// decreasing loop and the other relational operators do not give a lower-bounded
// range from a literal start.
func (r *Renderer) loopCounterRange(n frontend.Node) (string, ivl, bool) {
	kids := r.prog.Children(n)
	if len(kids) != 4 {
		return "", ivl{}, false
	}
	var decls []frontend.Node
	collectVarDecls(r.prog, kids[0], &decls)
	if len(decls) != 1 {
		return "", ivl{}, false
	}
	dkids := r.prog.Children(decls[0])
	if len(dkids) < 2 || dkids[0].Kind() != frontend.NodeIdentifier {
		return "", ivl{}, false
	}
	name, ok := localName(r.prog.Text(dkids[0]))
	if !ok {
		return "", ivl{}, false
	}
	start, ok := r.intLiteralValue(dkids[len(dkids)-1])
	if !ok {
		return "", ivl{}, false
	}

	cond := kids[1]
	if cond.Kind() != frontend.NodeBinaryExpression {
		return "", ivl{}, false
	}
	cparts := r.prog.Children(cond)
	if len(cparts) != 3 {
		return "", ivl{}, false
	}
	condName, ok := r.identName(cparts[0])
	if !ok || condName != name {
		return "", ivl{}, false
	}
	bound, ok := r.intLiteralValue(cparts[2])
	if !ok {
		return "", ivl{}, false
	}
	var top int
	switch r.prog.Text(cparts[1]) {
	case "<":
		top = bound - 1
	case "<=":
		top = bound
	default:
		return "", ivl{}, false
	}

	incr := kids[2]
	if incr.Kind() != frontend.NodePrefixUnaryExpression && incr.Kind() != frontend.NodePostfixUnaryExpression {
		return "", ivl{}, false
	}
	ikids := r.prog.Children(incr)
	if len(ikids) != 1 {
		return "", ivl{}, false
	}
	iname, ok := r.identName(ikids[0])
	if !ok || iname != name {
		return "", ivl{}, false
	}
	if unaryOpText(r.prog.Text(incr), r.prog.Text(ikids[0])) != "++" {
		return "", ivl{}, false
	}
	if start > top {
		// An empty loop never runs its body, so there is no access to prove; report a
		// range that admits nothing so the counter is dropped rather than treated as
		// covering an impossible span.
		return "", ivl{}, false
	}
	return name, ivl{lo: start, hi: top}, true
}

// mutationCount returns how many times a name is the target of an assignment or an
// increment/decrement in the body. A pure for-counter has exactly one, its header
// update; a body that also writes the name has more, which is what tells a header
// counter from one the loop reassigns.
func (r *Renderer) mutationCount(body []frontend.Node, name string) int {
	count := 0
	var walk func(n frontend.Node)
	walk = func(n frontend.Node) {
		switch n.Kind() {
		case frontend.NodeBinaryExpression:
			parts := r.prog.Children(n)
			if len(parts) == 3 {
				op := r.prog.Text(parts[1])
				_, isCompound := compoundBaseOp(op)
				if op == "=" || isCompound {
					if nm, ok := r.identName(parts[0]); ok && nm == name {
						count++
					}
				}
			}
		case frontend.NodePrefixUnaryExpression, frontend.NodePostfixUnaryExpression:
			kids := r.prog.Children(n)
			if len(kids) == 1 {
				op := unaryOpText(r.prog.Text(n), r.prog.Text(kids[0]))
				if op == "++" || op == "--" {
					if nm, ok := r.identName(kids[0]); ok && nm == name {
						count++
					}
				}
			}
		}
		for _, c := range r.prog.Children(n) {
			walk(c)
		}
	}
	for _, n := range body {
		walk(n)
	}
	return count
}

// fixedTypedArraysOf returns the integer typed-array locals whose length is a
// compile-time constant and that are never reassigned, so their backing slice is
// safe to index directly. A local qualifies when it is declared exactly once with a
// new Int32Array(N) or new Int32Array([...]) whose length N is an integer literal
// or a literal element list, its type is a wrapping-integer kind, and no assignment
// ever rebinds the name. A typed array never grows its backing, so the construction
// length is the length for the array's whole life.
func (r *Renderer) fixedTypedArraysOf(body []frontend.Node) map[string]typedArrInfo {
	out := map[string]typedArrInfo{}
	declCount := map[string]int{}
	var walk func(n frontend.Node)
	walk = func(n frontend.Node) {
		switch n.Kind() {
		case frontend.NodeVariableDeclaration:
			r.recordFixedTypedArray(n, out, declCount)
		case frontend.NodeBinaryExpression:
			parts := r.prog.Children(n)
			if len(parts) == 3 && r.prog.Text(parts[1]) == "=" {
				if nm, ok := r.identName(parts[0]); ok {
					// A reassignment can point the name at a different array, so its length is no
					// longer the construction length; drop it.
					delete(out, nm)
					declCount[nm] = 2
				}
			}
		}
		for _, c := range r.prog.Children(n) {
			walk(c)
		}
	}
	for _, n := range body {
		walk(n)
	}
	for name, c := range declCount {
		if c != 1 {
			delete(out, name)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// recordFixedTypedArray records a single declaration whose initializer constructs a
// fixed-length integer typed array. It reads the element type from the declared
// binding and the length from the constructor argument, and counts the declaration
// so a name declared twice is dropped by the caller.
func (r *Renderer) recordFixedTypedArray(d frontend.Node, out map[string]typedArrInfo, declCount map[string]int) {
	kids := r.prog.Children(d)
	if len(kids) == 0 || kids[0].Kind() != frontend.NodeIdentifier {
		return
	}
	name, ok := localName(r.prog.Text(kids[0]))
	if !ok {
		return
	}
	declCount[name]++
	if len(kids) != 2 && len(kids) != 3 {
		return
	}
	arrName, ok := r.typedArrayName(r.prog.TypeAt(kids[0]))
	if !ok {
		return
	}
	elemGo, ok := intTypedArrayLen(arrName)
	if !ok {
		return
	}
	length, ok := r.newTypedArrayLen(kids[len(kids)-1])
	if !ok {
		return
	}
	out[name] = typedArrInfo{length: length, elemGo: elemGo}
}

// newTypedArrayLen reads the element count a typed-array constructor produces when
// it is a compile-time constant: new X(N) with an integer literal N, or new X([...])
// with a literal element list. A length from a variable or an expression is not
// constant, so it reports false and the array keeps the checked access.
func (r *Renderer) newTypedArrayLen(init frontend.Node) (int, bool) {
	if init.Kind() != frontend.NodeNewExpression {
		return 0, false
	}
	kids := r.prog.Children(init)
	if len(kids) != 2 {
		return 0, false
	}
	arg := kids[1]
	if arg.Kind() == frontend.NodeArrayLiteralExpression {
		elems := r.prog.Children(arg)
		for _, e := range elems {
			if e.Kind() == frontend.NodeSpreadElement {
				return 0, false
			}
		}
		return len(elems), true
	}
	if v, ok := r.intLiteralValue(arg); ok && v >= 0 {
		return v, true
	}
	return 0, false
}

// intLiteralValue reads a numeric literal that is an exact integer in the int32
// range and returns its value, the shape a counter start, a counter bound, and a
// constructor length are each written with. A parenthesized literal reads through,
// and a const local bound to such a literal resolves to its value, so an idiomatic
// const N = 4096 used as a length or a bound is recognized as the constant it is.
func (r *Renderer) intLiteralValue(n frontend.Node) (int, bool) {
	if name, ok := r.identName(n); ok {
		v, ok := r.constInt[name]
		return v, ok
	}
	if !r.isInt32Literal(n) {
		return 0, false
	}
	if n.Kind() == frontend.NodeParenthesizedExpression {
		kids := r.prog.Children(n)
		if len(kids) != 1 {
			return 0, false
		}
		return r.intLiteralValue(kids[0])
	}
	v, ok := parseIntegerLiteral(r.prog.Text(n))
	if !ok {
		return 0, false
	}
	return int(v), true
}

// constIntsOf returns, for each const local in the body bound to an integer literal
// in the int32 range, the value it holds. The range analysis resolves such a name to
// its value where it expects an integer literal, so an idiomatic const N = 4096 used
// as a typed-array length or a loop bound is the constant 4096. A name declared more
// than once in the body is dropped, since a shadowing binding cannot be told from the
// first by name alone; a name the body ever assigns is dropped too, which a real const
// never is but a same-named let could be. The initializer is read as a plain literal
// here, not through the const resolution, so the map does not depend on the order the
// consts are visited.
func (r *Renderer) constIntsOf(body []frontend.Node) map[string]int {
	out := map[string]int{}
	declCount := map[string]int{}
	var walk func(n frontend.Node)
	walk = func(n frontend.Node) {
		if n.Kind() == frontend.NodeVariableStatement && r.isConstStatement(n) {
			var decls []frontend.Node
			collectVarDecls(r.prog, n, &decls)
			for _, d := range decls {
				dkids := r.prog.Children(d)
				if len(dkids) < 2 || dkids[0].Kind() != frontend.NodeIdentifier {
					continue
				}
				name, ok := localName(r.prog.Text(dkids[0]))
				if !ok {
					continue
				}
				declCount[name]++
				if v, ok := r.constIntLiteral(dkids[len(dkids)-1]); ok {
					out[name] = v
				}
			}
		}
		for _, c := range r.prog.Children(n) {
			walk(c)
		}
	}
	for _, n := range body {
		walk(n)
	}
	for name := range out {
		if declCount[name] != 1 || r.mutationCount(body, name) != 0 {
			delete(out, name)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// isConstStatement reports whether a variable statement is a const declaration, so
// its bindings cannot be reassigned. An export const is a const too. The kind is read
// from the statement text, since the const keyword is a leading token bento does not
// name as its own node.
func (r *Renderer) isConstStatement(n frontend.Node) bool {
	text := strings.TrimSpace(r.prog.Text(n))
	text = strings.TrimPrefix(text, "export ")
	return strings.HasPrefix(text, "const ")
}

// constIntLiteral reads a plain integer literal initializer for a const, without the
// const resolution intLiteralValue does, so building the const map does not depend on
// which const is visited first. A parenthesized literal reads through.
func (r *Renderer) constIntLiteral(n frontend.Node) (int, bool) {
	if n.Kind() == frontend.NodeParenthesizedExpression {
		kids := r.prog.Children(n)
		if len(kids) != 1 {
			return 0, false
		}
		return r.constIntLiteral(kids[0])
	}
	if !r.isInt32Literal(n) {
		return 0, false
	}
	v, ok := parseIntegerLiteral(r.prog.Text(n))
	if !ok {
		return 0, false
	}
	return int(v), true
}

// idxIvl computes the interval an index expression is known to stay within, built
// from for-counters and integer literals combined with + and -. A counter
// contributes its header range, a literal its own value, and a sum or difference
// the interval arithmetic of its parts. Any other shape has an unknown range and
// reports false, so the access stays on the checked path.
func (r *Renderer) idxIvl(n frontend.Node) (ivl, bool) {
	switch n.Kind() {
	case frontend.NodeParenthesizedExpression:
		kids := r.prog.Children(n)
		if len(kids) != 1 {
			return ivl{}, false
		}
		return r.idxIvl(kids[0])
	case frontend.NodeIdentifier:
		name, ok := localName(r.prog.Text(n))
		if !ok {
			return ivl{}, false
		}
		iv, ok := r.counterIvl[name]
		return iv, ok
	case frontend.NodeNumericLiteral:
		v, ok := r.intLiteralValue(n)
		if !ok {
			return ivl{}, false
		}
		return ivl{lo: v, hi: v}, true
	case frontend.NodeBinaryExpression:
		parts := r.prog.Children(n)
		if len(parts) != 3 {
			return ivl{}, false
		}
		a, ok := r.idxIvl(parts[0])
		if !ok {
			return ivl{}, false
		}
		b, ok := r.idxIvl(parts[2])
		if !ok {
			return ivl{}, false
		}
		switch r.prog.Text(parts[1]) {
		case "+":
			return ivl{lo: a.lo + b.lo, hi: a.hi + b.hi}, true
		case "-":
			return ivl{lo: a.lo - b.hi, hi: a.hi - b.lo}, true
		}
	}
	return ivl{}, false
}

// provenTypedRead reports whether an element access a[i] reads a fixed-length
// integer typed array at an index proven to sit inside it, the test both the
// native read and the native write share. The receiver must be a plain local in the
// fixed-array set, and the index interval must fall within [0, length-1]. It returns
// the array's info and the index node so the caller can emit the slice access.
func (r *Renderer) provenTypedRead(n frontend.Node) (typedArrInfo, frontend.Node, bool) {
	if n.Kind() != frontend.NodeElementAccessExpression {
		return typedArrInfo{}, nil, false
	}
	kids := r.prog.Children(n)
	if len(kids) != 2 {
		return typedArrInfo{}, nil, false
	}
	name, ok := r.identName(kids[0])
	if !ok {
		return typedArrInfo{}, nil, false
	}
	info, ok := r.fixedTArr[name]
	if !ok {
		return typedArrInfo{}, nil, false
	}
	iv, ok := r.idxIvl(kids[1])
	if !ok || iv.lo < 0 || iv.hi > info.length-1 {
		return typedArrInfo{}, nil, false
	}
	return info, kids[1], true
}

// typedSliceRead builds the native read of a proven-in-range integer typed-array
// access: recv.Data()[idx], the element widened to the requested Go type. A read in
// the int32 domain widens to int32 so it composes with int32Of, and a read in the
// Number domain widens to float64 so it drops into the ordinary numeric path
// unchanged. The index is the int32 lowering of the index node, which keeps a
// counter and its arithmetic in registers, and Go accepts an int32 slice index.
func (r *Renderer) typedSliceRead(recvNode, idxNode frontend.Node, wantGo string, info typedArrInfo) (ast.Expr, error) {
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	idx, err := r.int32Of(idxNode)
	if err != nil {
		return nil, err
	}
	elem := &ast.IndexExpr{
		X:     &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Data")}},
		Index: idx,
	}
	// An Int32Array element is already an int32, so a read in the int32 domain needs no
	// conversion around it; every other case widens the stored element to the domain's
	// Go type.
	if wantGo == info.elemGo {
		return elem, nil
	}
	return &ast.CallExpr{Fun: ident(wantGo), Args: []ast.Expr{elem}}, nil
}

// typedSliceStore builds the native store of a proven-in-range integer value into
// an integer typed array: recv.Data()[idx] = int8(v). The value is lowered in the
// int32 domain, and a conversion to the element type narrows it to the store width
// the same way the element kind's coercion does for an integer, so an Int8Array
// wraps modulo 256 into signed range and a Uint16Array modulo 65536 with no coerce
// call. The Int32Array element already matches the int32 value, so its store needs
// no conversion around the value.
func (r *Renderer) typedSliceStore(recvNode, idxNode, valNode frontend.Node, info typedArrInfo) (ast.Stmt, error) {
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	idx, err := r.int32Of(idxNode)
	if err != nil {
		return nil, err
	}
	val, err := r.int32Of(valNode)
	if err != nil {
		return nil, err
	}
	val = r.elementStoreValue(valNode, val, info.elemGo)
	lhs := &ast.IndexExpr{
		X:     &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Data")}},
		Index: idx,
	}
	return &ast.AssignStmt{Lhs: []ast.Expr{lhs}, Tok: token.ASSIGN, Rhs: []ast.Expr{val}}, nil
}

// elementStoreValue narrows an integer store value to the element's Go type. When
// the value is a compile-time constant it is folded to the element it wraps to and
// emitted as a plain literal, so a store like i8[0] = 200 becomes = -56 rather than
// int8(200), which Go rejects as a constant that overflows the element. When the
// value is not constant it takes a runtime conversion to the element type, int8(v),
// which wraps the same way and never trips the constant-overflow check. The
// Int32Array element already matches the int32 value, so it needs no wrap either
// way.
func (r *Renderer) elementStoreValue(valNode frontend.Node, val ast.Expr, elemGo string) ast.Expr {
	if v, ok := r.constFoldInt32(valNode); ok {
		return elementLit(v, elemGo)
	}
	if elemGo != "int32" {
		return &ast.CallExpr{Fun: ident(elemGo), Args: []ast.Expr{val}}
	}
	return val
}

// elementLit builds the Go literal for a folded int32 value narrowed to an element
// type, applying the same width conversion the store coercion does so the emitted
// constant is the element the array actually holds.
func elementLit(v int32, elemGo string) ast.Expr {
	switch elemGo {
	case "int8":
		return goIntLit(int64(int8(v)))
	case "int16":
		return goIntLit(int64(int16(v)))
	case "uint16":
		return goIntLit(int64(uint16(v)))
	case "uint32":
		return goIntLit(int64(uint32(v)))
	default:
		return goIntLit(int64(v))
	}
}

// goIntLit builds a Go integer literal, spelling a negative value with a unary minus
// since a Go integer token is unsigned. The constant is left untyped so it takes the
// element type from the slice it is assigned into.
func goIntLit(v int64) ast.Expr {
	if v < 0 {
		return &ast.UnaryExpr{Op: token.SUB, X: &ast.BasicLit{Kind: token.INT, Value: strconv.FormatInt(-v, 10)}}
	}
	return &ast.BasicLit{Kind: token.INT, Value: strconv.FormatInt(v, 10)}
}

// constFoldInt32 folds a fully constant integer expression to its ECMAScript ToInt32
// value, covering exactly the shapes int32Of produces natively: a literal, a bitwise
// not, the bitwise and shift operators, a native + or -, and Math.imul and
// Math.clz32 over constant arguments. Any leaf that is not a constant, an identifier
// or an element read, makes the whole expression non-constant, so it reports false
// and the store takes a runtime conversion instead. The + and - fold through float64
// to mirror the double arithmetic the language does before the surrounding store
// applies ToInt32; the bitwise and shift operators fold in int32 the way the runtime
// evaluates them.
func (r *Renderer) constFoldInt32(n frontend.Node) (int32, bool) {
	switch n.Kind() {
	case frontend.NodeParenthesizedExpression:
		kids := r.prog.Children(n)
		if len(kids) != 1 {
			return 0, false
		}
		return r.constFoldInt32(kids[0])
	case frontend.NodeNumericLiteral:
		return foldInt32Literal(r.prog.Text(n))
	case frontend.NodePrefixUnaryExpression:
		kids := r.prog.Children(n)
		if len(kids) == 1 && unaryOpText(r.prog.Text(n), r.prog.Text(kids[0])) == "~" {
			if v, ok := r.constFoldInt32(kids[0]); ok {
				return ^v, true
			}
		}
		return 0, false
	case frontend.NodeBinaryExpression:
		return r.constFoldBinary(n)
	case frontend.NodeCallExpression:
		method, args, ok := r.mathMethodCall(n)
		if !ok {
			return 0, false
		}
		switch {
		case method == "imul" && len(args) == 2:
			a, aok := r.constFoldInt32(args[0])
			b, bok := r.constFoldInt32(args[1])
			if aok && bok {
				return value.ToInt32(float64(int64(a) * int64(b))), true
			}
		case method == "clz32" && len(args) == 1:
			if a, aok := r.constFoldInt32(args[0]); aok {
				return int32(bits.LeadingZeros32(uint32(a))), true
			}
		}
		return 0, false
	}
	return 0, false
}

// constFoldBinary folds a constant binary expression in the int32 domain. The + and
// - fold through float64 so a sum past the int32 range wraps through ToInt32 exactly
// as the language would after the double add; the bitwise and shift operators fold
// in int32 with the shift count masked to five bits. The x | 0 identity folds to x.
// The unsigned >>> is not int32-producing and never reaches here.
func (r *Renderer) constFoldBinary(n frontend.Node) (int32, bool) {
	parts := r.prog.Children(n)
	if len(parts) != 3 {
		return 0, false
	}
	a, aok := r.constFoldInt32(parts[0])
	b, bok := r.constFoldInt32(parts[2])
	if !aok || !bok {
		return 0, false
	}
	switch r.prog.Text(parts[1]) {
	case "+":
		return value.ToInt32(float64(a) + float64(b)), true
	case "-":
		return value.ToInt32(float64(a) - float64(b)), true
	case "&":
		return a & b, true
	case "|":
		return a | b, true
	case "^":
		return a ^ b, true
	case "<<":
		return a << (uint32(b) & 31), true
	case ">>":
		return a >> (uint32(b) & 31), true
	}
	return 0, false
}
