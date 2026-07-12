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
		switch {
		case kind == "set":
			// A Set's keys() and values() both yield its members.
			if method == "entries" {
				return nil, true, &NotYetLowerable{Reason: "a for...of over a Set's entries() with a single binding needs a tuple, a later slice"}
			}
			return r.rangeCollSingle(recv, bindNode, name, bodyNode, "Members")
		case method == "keys":
			return r.rangeCollSingle(recv, bindNode, name, bodyNode, "Keys")
		case method == "values":
			return r.rangeCollSingle(recv, bindNode, name, bodyNode, "Values")
		default:
			return nil, true, &NotYetLowerable{Reason: "a for...of over a Map's entries() with a single binding needs a tuple, a later slice"}
		}
	}
	if r.isSet(iterable) {
		return r.rangeCollSingle(iterable, bindNode, name, bodyNode, "Members")
	}
	if r.isMap(iterable) {
		return nil, true, &NotYetLowerable{Reason: "a for...of over a Map with a single binding yields a [key, value] pair, which needs a tuple, a later slice"}
	}
	return nil, false, nil
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
