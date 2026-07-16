package lower

import (
	"go/ast"
	"go/token"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers the Map and Set iterators (09 group 5): a for...of over a Set,
// over map.keys()/map.values()/set.values()/set.keys(), and over the [key, value]
// pairs a Map or the [value, value] pairs a Set hands its entries()/default iterator.
// The runtime keeps its entries in insertion-ordered slices, and Keys, Values, and
// Members hand back a snapshot of that order, so a for...of ranges the snapshot the
// idiomatic Go way rather than build and drive an iterator object. A pair binds its
// two names directly off the range, the way the array entries loop does, since a
// heterogeneous [K, V] tuple does not lower; the single-binding pair forms, which
// would have to materialize that tuple, hand back. The live view an iterator has of
// a mutation made during the loop is a later slice, so the snapshot is taken once at
// loop entry.

// collCall builds a receiver.Method() call, the one shape every Map and Set iterator
// loop reaches for to read an insertion-ordered snapshot slice off the runtime.
func collCall(recv ast.Expr, method string) *ast.CallExpr {
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(method)}}
}

// mapSetIterForOfCall reports whether a for...of iterable is m.keys(), m.values(),
// or m.entries() over a Map or Set receiver, the form for...of consumes without ever
// building the iterator object. It returns the receiver node, the method, and whether
// the receiver is a "map" or a "set" so the loop ranges the runtime's insertion-
// ordered backing directly. The call must take no argument; a receiver that is not a
// Map or Set is not this shape and takes the general iterable path.
func (r *Renderer) mapSetIterForOfCall(iterable frontend.Node) (recv frontend.Node, method, kind string, ok bool) {
	if iterable.Kind() != frontend.NodeCallExpression {
		return nil, "", "", false
	}
	kids := r.prog.Children(iterable)
	if len(kids) != 1 || kids[0].Kind() != frontend.NodePropertyAccessExpression {
		return nil, "", "", false
	}
	parts := r.prog.Children(kids[0])
	if len(parts) != 2 {
		return nil, "", "", false
	}
	switch r.prog.Text(parts[1]) {
	case "values", "keys", "entries":
	default:
		return nil, "", "", false
	}
	switch {
	case r.isMap(parts[0]):
		return parts[0], r.prog.Text(parts[1]), "map", true
	case r.isSet(parts[0]):
		return parts[0], r.prog.Text(parts[1]), "set", true
	}
	return nil, "", "", false
}

// collIterAccessor recognizes a Map or Set keys()/values() call used as an iterable in
// a value position (a spread, a destructuring source) and reports the receiver node,
// the Go accessor whose typed snapshot slice holds the yielded members (Keys, Values,
// or Members), and the member type. A Set's keys() and values() both yield its
// members, so both map to Members; a Map's keys() and values() map to Keys and Values.
// entries() yields a [key, value] tuple, which does not lower, so it reports ok=false,
// as does a receiver whose key, value, or member type the checker does not expose.
func (r *Renderer) collIterAccessor(operand frontend.Node) (recv frontend.Node, accessor string, elem frontend.Type, ok bool) {
	rn, method, kind, isCall := r.mapSetIterForOfCall(operand)
	if !isCall || method == "entries" {
		return nil, "", frontend.Type{}, false
	}
	switch kind {
	case "set":
		e, eok := r.setElem(r.prog.TypeAt(rn))
		if !eok {
			return nil, "", frontend.Type{}, false
		}
		return rn, "Members", e, true
	case "map":
		k, v, mok := r.mapKeyVal(r.prog.TypeAt(rn))
		if !mok {
			return nil, "", frontend.Type{}, false
		}
		if method == "keys" {
			return rn, "Keys", k, true
		}
		return rn, "Values", v, true
	}
	return nil, "", frontend.Type{}, false
}

// forOfMapSetSingle lowers a for...of with a single loop binding whose iterable is a
// Set, or a Map or Set's keys()/values() call, to a range over the runtime's
// insertion-ordered snapshot. A Set and set.values()/set.keys() range Members, a
// Map's keys() ranges Keys, and its values() ranges Values, each binding the one value
// the kind yields. It returns handled false when the iterable is neither a Map nor a
// Set so the caller keeps looking. The pair-yielding forms, a Map used directly, and
// any entries() with a single binding, would have to materialize a [key, value] tuple
// the value model does not lower, so they hand back.
func (r *Renderer) forOfMapSetSingle(iterable, bindNode frontend.Node, name string, bodyNode frontend.Node) (ast.Stmt, bool, error) {
	if recv, method, kind, ok := r.mapSetIterForOfCall(iterable); ok {
		if hb := r.forOfCollMutationHandback(recv, kind, bodyNode); hb != nil {
			return nil, true, hb
		}
		switch {
		case kind == "set":
			// A Set's keys() and values() both yield its members. entries() yields a
			// [member, member] pair, which now binds one name to the materialized tuple.
			if method == "entries" {
				return r.rangeCollPairSingle(recv, bindNode, name, bodyNode, "set")
			}
			return r.rangeCollSingle(recv, bindNode, name, bodyNode, "Members")
		case method == "keys":
			return r.rangeCollSingle(recv, bindNode, name, bodyNode, "Keys")
		case method == "values":
			return r.rangeCollSingle(recv, bindNode, name, bodyNode, "Values")
		default:
			return r.rangeCollPairSingle(recv, bindNode, name, bodyNode, "map")
		}
	}
	if r.isSet(iterable) {
		if hb := r.forOfCollMutationHandback(iterable, "set", bodyNode); hb != nil {
			return nil, true, hb
		}
		return r.rangeCollSingle(iterable, bindNode, name, bodyNode, "Members")
	}
	if r.isMap(iterable) {
		if hb := r.forOfCollMutationHandback(iterable, "map", bodyNode); hb != nil {
			return nil, true, hb
		}
		return r.rangeCollPairSingle(iterable, bindNode, name, bodyNode, "map")
	}
	return nil, false, nil
}

// forOfCollMutationHandback returns a handback when the loop body mutates the very
// Map or Set the for...of iterates. The lowering ranges an insertion-ordered snapshot
// taken once at loop entry, which is faithful for a body that only reads the
// collection, but a body that adds, deletes, or clears it would, in JavaScript, see
// the iterator's live view of that change: a deleted entry not yet reached is skipped,
// and an entry added ahead of the cursor is visited. The snapshot cannot show that, so
// rather than emit a loop that silently diverges, a body that mutates the iterated
// collection hands back. The live iterator that observes the mutation is a later slice.
func (r *Renderer) forOfCollMutationHandback(collNode frontend.Node, kind string, bodyNode frontend.Node) *NotYetLowerable {
	if !r.bodyMutatesColl(bodyNode, collNode) {
		return nil
	}
	return &NotYetLowerable{Reason: "a for...of that mutates the " + kind + " it iterates needs the iterator's live view of the change, a later slice"}
}

// bodyMutatesColl reports whether n calls a mutating method, add, set, delete, or
// clear, on the collection collNode iterates. When collNode is a plain identifier it
// matches a call on that same identifier, so a loop that builds a second collection
// while reading the first still lowers. When collNode is any other expression there is
// no identifier to match against, so it takes the conservative reading: any mutating
// call on a Map or Set in the body counts, which may hand back a loop that mutates an
// unrelated collection but never lets a real mutation through.
func (r *Renderer) bodyMutatesColl(n, collNode frontend.Node) bool {
	if n.Kind() == frontend.NodeCallExpression {
		kids := r.prog.Children(n)
		if len(kids) >= 1 && kids[0].Kind() == frontend.NodePropertyAccessExpression {
			parts := r.prog.Children(kids[0])
			if len(parts) == 2 {
				switch r.prog.Text(parts[1]) {
				case "add", "set", "delete", "clear":
					recv := parts[0]
					if collNode.Kind() == frontend.NodeIdentifier {
						if recv.Kind() == frontend.NodeIdentifier && r.prog.Text(recv) == r.prog.Text(collNode) {
							return true
						}
					} else if r.isMap(recv) || r.isSet(recv) {
						return true
					}
				}
			}
		}
	}
	for _, k := range r.prog.Children(n) {
		if r.bodyMutatesColl(k, collNode) {
			return true
		}
	}
	return false
}

// rangeCollSingle ranges the snapshot the named method returns, binding each value to
// the loop variable, the loop a Set or a Map's keys()/values() collapses to. A binding
// the body never reads drops to a bare range, the same unused-binding rule the array
// loop applies, so the Go loop compiles rather than leave an unused local.
func (r *Renderer) rangeCollSingle(recvNode, bindNode frontend.Node, name string, bodyNode frontend.Node, method string) (ast.Stmt, bool, error) {
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, true, err
	}
	body, err := r.loopBody(bodyNode)
	if err != nil {
		return nil, true, err
	}
	rng := &ast.RangeStmt{X: collCall(recv, method), Body: body}
	if r.bodyUsesName(bodyNode, r.prog.Text(bindNode)) {
		rng.Key = ident("_")
		rng.Value = ident(name)
		rng.Tok = token.DEFINE
	}
	return rng, true, nil
}

// rangeCollPairSingle lowers a for...of with a single binding whose iterable yields a
// pair, `for (const e of map)`, its entries() spelling, and `for (const e of
// set.entries())`, binding the one name to the materialized [key, value] tuple each
// turn. It is the single-binding counterpart of forOfMapSetDestructure, which binds a
// two-name pattern straight off the snapshots; here the body reads the pair through the
// bound name (e[0], e[1]), so the tuple has to exist as a value, which it now does. A
// Map ranges its Keys and Values snapshots in parallel by index and builds Tuple{E0:
// key, E1: values[i]}; a Set's entries pair is the member twice, so it builds Tuple{E0:
// member, E1: member}. A binding the body never reads drops to a bare range with no
// tuple built, the same unused-binding rule the other collection loops apply. The
// binding's type is the [K, V] tuple; if the checker did not type it as a two-element
// tuple this hands back rather than guess a shape.
func (r *Renderer) rangeCollPairSingle(recvNode, bindNode frontend.Node, name string, bodyNode frontend.Node, kind string) (ast.Stmt, bool, error) {
	used := r.bodyUsesName(bodyNode, r.prog.Text(bindNode))
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, true, err
	}
	body, err := r.loopBody(bodyNode)
	if err != nil {
		return nil, true, err
	}
	// An unused binding drives the loop off one snapshot with no tuple, since there is
	// nothing to bind. Keys drives a Map, Members a Set, each an insertion-ordered walk
	// of the right length.
	if !used {
		drive := "Members"
		if kind == "map" {
			drive = "Keys"
		}
		return &ast.RangeStmt{X: collCall(recv, drive), Body: body}, true, nil
	}
	elems, ok := r.prog.TupleElements(r.prog.TypeAt(bindNode))
	if !ok || len(elems) != 2 {
		return nil, true, &NotYetLowerable{Reason: "a for...of over a " + kind + "'s pair with a single binding needs a two-element tuple, a later slice"}
	}
	tname, err := r.decls.internTuple(r, r.prog.TypeAt(bindNode), elems)
	if err != nil {
		return nil, true, err
	}
	tuple := func(e0, e1 ast.Expr) ast.Expr {
		return &ast.CompositeLit{Type: ident(tname), Elts: []ast.Expr{
			&ast.KeyValueExpr{Key: ident("E0"), Value: e0},
			&ast.KeyValueExpr{Key: ident("E1"), Value: e1},
		}}
	}
	if kind == "set" {
		// The pair is the member twice, so both fields read the ranged member.
		mem := r.freshTemp()
		bind := &ast.AssignStmt{Lhs: []ast.Expr{ident(name)}, Tok: token.DEFINE, Rhs: []ast.Expr{tuple(ident(mem), ident(mem))}}
		body.List = append([]ast.Stmt{bind}, body.List...)
		return &ast.RangeStmt{Key: ident("_"), Value: ident(mem), Tok: token.DEFINE, X: collCall(recv, "Members"), Body: body}, true, nil
	}
	// A Map ranges its Keys and Values snapshots in parallel, indexing Values by the
	// range index so each turn's tuple pairs the key with its own value.
	m := r.freshTemp()
	ks := r.freshTemp()
	vs := r.freshTemp()
	idx := r.freshTemp()
	key := r.freshTemp()
	decls := []ast.Stmt{
		&ast.AssignStmt{Lhs: []ast.Expr{ident(m)}, Tok: token.DEFINE, Rhs: []ast.Expr{recv}},
		&ast.AssignStmt{Lhs: []ast.Expr{ident(ks)}, Tok: token.DEFINE, Rhs: []ast.Expr{collCall(ident(m), "Keys")}},
		&ast.AssignStmt{Lhs: []ast.Expr{ident(vs)}, Tok: token.DEFINE, Rhs: []ast.Expr{collCall(ident(m), "Values")}},
	}
	bind := &ast.AssignStmt{
		Lhs: []ast.Expr{ident(name)},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{tuple(ident(key), &ast.IndexExpr{X: ident(vs), Index: ident(idx)})},
	}
	body.List = append([]ast.Stmt{bind}, body.List...)
	rng := &ast.RangeStmt{Key: ident(idx), Value: ident(key), Tok: token.DEFINE, X: ident(ks), Body: body}
	return &ast.BlockStmt{List: append(decls, rng)}, true, nil
}

// spreadCollEntries lowers a spread of a Map used directly, or of any Map or Set
// entries() call, in an array literal to a slice of the interned [key, value] tuple
// the append then splices. A Map's default iterator and its entries() both yield the
// [key, value] pairs, a Set's entries() yields the member twice, so each spliced
// element is the materialized tuple the target array's element type names. It reports
// ok=false when the operand is neither shape, so the caller keeps looking; ok=true with
// a hand-back when the target element type is not a two-element tuple or its fields do
// not lower to the same Go types as the pair, so no wrong slice is spliced.
func (r *Renderer) spreadCollEntries(operand frontend.Node, tupleT frontend.Type, hasTupleT bool) (ast.Expr, bool, error) {
	var recvNode frontend.Node
	kind := ""
	if recv, method, k, ok := r.mapSetIterForOfCall(operand); ok && method == "entries" {
		recvNode, kind = recv, k
	} else if r.isMap(operand) {
		recvNode, kind = operand, "map"
	} else {
		return nil, false, nil
	}
	if !hasTupleT {
		return nil, true, &NotYetLowerable{Reason: "spread of a " + kind + "'s entries needs the target's tuple element type, a later slice"}
	}
	elems, ok := r.prog.TupleElements(tupleT)
	if !ok || len(elems) != 2 {
		return nil, true, &NotYetLowerable{Reason: "spread of a " + kind + "'s entries into other than a two-element tuple array is a later slice"}
	}
	// The tuple's two field types must lower to the same Go types as the pair the
	// runtime yields, a Map's key and value or a Set's member twice, or the composite
	// literal that packs each entry would not compile, so a mismatch hands back.
	e0Go, err := r.typeExpr(elems[0].Type)
	if err != nil {
		return nil, true, err
	}
	e1Go, err := r.typeExpr(elems[1].Type)
	if err != nil {
		return nil, true, err
	}
	var fst, snd frontend.Type
	if kind == "set" {
		m, ok := r.setElem(r.prog.TypeAt(recvNode))
		if !ok {
			return nil, true, &NotYetLowerable{Reason: "spread of a set's entries whose member type is unreadable is a later slice"}
		}
		fst, snd = m, m
	} else {
		k, v, ok := r.mapKeyVal(r.prog.TypeAt(recvNode))
		if !ok {
			return nil, true, &NotYetLowerable{Reason: "spread of a map's entries whose key or value type is unreadable is a later slice"}
		}
		fst, snd = k, v
	}
	fstGo, err := r.typeExpr(fst)
	if err != nil {
		return nil, true, err
	}
	sndGo, err := r.typeExpr(snd)
	if err != nil {
		return nil, true, err
	}
	if sameFst, err := sameGoType(e0Go, fstGo); err != nil {
		return nil, true, err
	} else if sameSnd, err := sameGoType(e1Go, sndGo); err != nil {
		return nil, true, err
	} else if !sameFst || !sameSnd {
		return nil, true, &NotYetLowerable{Reason: "spread of a " + kind + "'s entries into a tuple with a different field type is a later slice"}
	}
	tname, err := r.decls.internTuple(r, tupleT, elems)
	if err != nil {
		return nil, true, err
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, true, err
	}
	return r.collEntriesTupleSlice(recv, tname, kind), true, nil
}

// collEntriesTupleSlice builds the immediately-called function literal that collects a
// Map or Set's entries into a fresh []Tname the spread's append splices. A Map ranges
// its Keys and Values snapshots in parallel by index so each entry pairs a key with its
// own value; a Set ranges its Members and pairs each with itself. The slice is
// preallocated to the snapshot length, the same walk a for...of over the entries takes.
func (r *Renderer) collEntriesTupleSlice(recv ast.Expr, tname, kind string) ast.Expr {
	out := r.freshTemp()
	sliceT := &ast.ArrayType{Elt: ident(tname)}
	tuple := func(e0, e1 ast.Expr) ast.Expr {
		return &ast.CompositeLit{Type: ident(tname), Elts: []ast.Expr{
			&ast.KeyValueExpr{Key: ident("E0"), Value: e0},
			&ast.KeyValueExpr{Key: ident("E1"), Value: e1},
		}}
	}
	var body []ast.Stmt
	if kind == "set" {
		ms := r.freshTemp()
		idx := r.freshTemp()
		mem := r.freshTemp()
		assign := &ast.AssignStmt{
			Lhs: []ast.Expr{&ast.IndexExpr{X: ident(out), Index: ident(idx)}},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{tuple(ident(mem), ident(mem))},
		}
		body = []ast.Stmt{
			&ast.AssignStmt{Lhs: []ast.Expr{ident(ms)}, Tok: token.DEFINE, Rhs: []ast.Expr{collCall(recv, "Members")}},
			&ast.AssignStmt{Lhs: []ast.Expr{ident(out)}, Tok: token.DEFINE, Rhs: []ast.Expr{makeSlice(sliceT, ident(ms))}},
			&ast.RangeStmt{Key: ident(idx), Value: ident(mem), Tok: token.DEFINE, X: ident(ms), Body: &ast.BlockStmt{List: []ast.Stmt{assign}}},
			&ast.ReturnStmt{Results: []ast.Expr{ident(out)}},
		}
	} else {
		m := r.freshTemp()
		ks := r.freshTemp()
		vs := r.freshTemp()
		idx := r.freshTemp()
		key := r.freshTemp()
		assign := &ast.AssignStmt{
			Lhs: []ast.Expr{&ast.IndexExpr{X: ident(out), Index: ident(idx)}},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{tuple(ident(key), &ast.IndexExpr{X: ident(vs), Index: ident(idx)})},
		}
		body = []ast.Stmt{
			&ast.AssignStmt{Lhs: []ast.Expr{ident(m)}, Tok: token.DEFINE, Rhs: []ast.Expr{recv}},
			&ast.AssignStmt{Lhs: []ast.Expr{ident(ks)}, Tok: token.DEFINE, Rhs: []ast.Expr{collCall(ident(m), "Keys")}},
			&ast.AssignStmt{Lhs: []ast.Expr{ident(vs)}, Tok: token.DEFINE, Rhs: []ast.Expr{collCall(ident(m), "Values")}},
			&ast.AssignStmt{Lhs: []ast.Expr{ident(out)}, Tok: token.DEFINE, Rhs: []ast.Expr{makeSlice(sliceT, ident(ks))}},
			&ast.RangeStmt{Key: ident(idx), Value: ident(key), Tok: token.DEFINE, X: ident(ks), Body: &ast.BlockStmt{List: []ast.Stmt{assign}}},
			&ast.ReturnStmt{Results: []ast.Expr{ident(out)}},
		}
	}
	fn := &ast.FuncLit{
		Type: &ast.FuncType{Params: &ast.FieldList{}, Results: &ast.FieldList{List: []*ast.Field{{Type: sliceT}}}},
		Body: &ast.BlockStmt{List: body},
	}
	return &ast.CallExpr{Fun: fn}
}

// makeSlice builds make([]T, len(src)), preallocating a slice to a snapshot's length.
func makeSlice(sliceT ast.Expr, src ast.Expr) ast.Expr {
	return &ast.CallExpr{Fun: ident("make"), Args: []ast.Expr{
		sliceT,
		&ast.CallExpr{Fun: ident("len"), Args: []ast.Expr{src}},
	}}
}

// forOfMapSetDestructure lowers `for (const [k, v] of map)`, its entries() spelling,
// and `for (const [a, b] of set.entries())` to a range over the runtime's snapshot
// whose two values bind the pattern's two names directly, sidestepping the [K, V]
// tuple the value model does not lower. It returns handled false when the iterable is
// not a Map, nor a Map/Set entries() call, so the caller keeps looking. A Set used
// directly yields single members, not pairs, so it is not this shape; keys() and
// values() likewise yield a single value, so destructuring one hands back.
func (r *Renderer) forOfMapSetDestructure(iterable, pattern, bodyNode frontend.Node) (ast.Stmt, bool, error) {
	var recvNode frontend.Node
	kind := ""
	if recv, method, k, ok := r.mapSetIterForOfCall(iterable); ok {
		if method != "entries" {
			return nil, true, &NotYetLowerable{Reason: "a destructuring for...of over a " + k + "'s " + method + "() yields a single value, a later slice"}
		}
		recvNode, kind = recv, k
	} else if r.isMap(iterable) {
		recvNode, kind = iterable, "map"
	} else {
		return nil, false, nil
	}
	if hb := r.forOfCollMutationHandback(recvNode, kind, bodyNode); hb != nil {
		return nil, true, hb
	}
	names, used, ok := r.twoNamePattern(pattern, bodyNode)
	if !ok {
		return nil, true, &NotYetLowerable{Reason: "a destructuring for...of over a " + kind + "'s entries with other than a flat two-name pattern is a later slice"}
	}
	if kind == "map" {
		stmt, err := r.forOfMapEntriesDestructure(recvNode, names, used, bodyNode)
		return stmt, true, err
	}
	stmt, err := r.forOfSetEntriesDestructure(recvNode, names, used, bodyNode)
	return stmt, true, err
}

// twoNamePattern reads a flat two-name array pattern, [a, b], into the two Go
// identifiers it binds and whether the body reads each, the shape a Map or Set entry
// pair binds against. A hole, a default, a rest, a nested pattern, or a name that is
// not a Go identifier is not this shape and reports false, so the caller hands back.
func (r *Renderer) twoNamePattern(pattern, bodyNode frontend.Node) (names [2]string, used [2]bool, ok bool) {
	elems := r.prog.Children(pattern)
	if len(elems) != 2 {
		return names, used, false
	}
	for i, el := range elems {
		ec := r.prog.Children(el)
		if len(ec) != 1 || ec[0].Kind() != frontend.NodeIdentifier {
			return names, used, false
		}
		nm, ok := localName(r.prog.Text(ec[0]))
		if !ok {
			return names, used, false
		}
		names[i] = nm
		used[i] = r.bodyUsesName(bodyNode, r.prog.Text(ec[0]))
	}
	return names, used, true
}

// forOfMapEntriesDestructure ranges a Map's entries, binding the key and value names
// off the runtime's insertion-ordered snapshot. When the body reads both names, the
// receiver is evaluated once into a temporary and its Keys and Values snapshots are
// ranged in parallel by index, so the pair each turn reads stays a consistent entry.
// When it reads only one, the loop ranges that one snapshot alone; when it reads
// neither, it ranges the keys only to drive the iteration, matching the array loop's
// unused-binding handling so the Go loop compiles.
func (r *Renderer) forOfMapEntriesDestructure(recvNode frontend.Node, names [2]string, used [2]bool, bodyNode frontend.Node) (ast.Stmt, error) {
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	body, err := r.loopBody(bodyNode)
	if err != nil {
		return nil, err
	}
	switch {
	case used[0] && used[1]:
		m := r.freshTemp()
		ks := r.freshTemp()
		vs := r.freshTemp()
		idx := r.freshTemp()
		decls := []ast.Stmt{
			&ast.AssignStmt{Lhs: []ast.Expr{ident(m)}, Tok: token.DEFINE, Rhs: []ast.Expr{recv}},
			&ast.AssignStmt{Lhs: []ast.Expr{ident(ks)}, Tok: token.DEFINE, Rhs: []ast.Expr{collCall(ident(m), "Keys")}},
			&ast.AssignStmt{Lhs: []ast.Expr{ident(vs)}, Tok: token.DEFINE, Rhs: []ast.Expr{collCall(ident(m), "Values")}},
		}
		bindVal := &ast.AssignStmt{
			Lhs: []ast.Expr{ident(names[1])},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.IndexExpr{X: ident(vs), Index: ident(idx)}},
		}
		body.List = append([]ast.Stmt{bindVal}, body.List...)
		rng := &ast.RangeStmt{Key: ident(idx), Value: ident(names[0]), Tok: token.DEFINE, X: ident(ks), Body: body}
		return &ast.BlockStmt{List: append(decls, rng)}, nil
	case used[0]:
		return &ast.RangeStmt{Key: ident("_"), Value: ident(names[0]), Tok: token.DEFINE, X: collCall(recv, "Keys"), Body: body}, nil
	case used[1]:
		return &ast.RangeStmt{Key: ident("_"), Value: ident(names[1]), Tok: token.DEFINE, X: collCall(recv, "Values"), Body: body}, nil
	default:
		return &ast.RangeStmt{X: collCall(recv, "Keys"), Body: body}, nil
	}
}

// forOfSetEntriesDestructure ranges a Set's entries, whose pair is the same member
// twice (value, value), so both names bind that member. When the body reads both, the
// range binds the first and the second copies it; when it reads one, the range binds
// that name alone; when it reads neither, it ranges the members only to drive the
// iteration, the same unused-binding handling the other loops apply.
func (r *Renderer) forOfSetEntriesDestructure(recvNode frontend.Node, names [2]string, used [2]bool, bodyNode frontend.Node) (ast.Stmt, error) {
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	body, err := r.loopBody(bodyNode)
	if err != nil {
		return nil, err
	}
	call := collCall(recv, "Members")
	switch {
	case used[0] && used[1]:
		bindB := &ast.AssignStmt{Lhs: []ast.Expr{ident(names[1])}, Tok: token.DEFINE, Rhs: []ast.Expr{ident(names[0])}}
		body.List = append([]ast.Stmt{bindB}, body.List...)
		return &ast.RangeStmt{Key: ident("_"), Value: ident(names[0]), Tok: token.DEFINE, X: call, Body: body}, nil
	case used[0]:
		return &ast.RangeStmt{Key: ident("_"), Value: ident(names[0]), Tok: token.DEFINE, X: call, Body: body}, nil
	case used[1]:
		return &ast.RangeStmt{Key: ident("_"), Value: ident(names[1]), Tok: token.DEFINE, X: call, Body: body}, nil
	default:
		return &ast.RangeStmt{X: call, Body: body}, nil
	}
}
