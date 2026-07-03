package partition

import "github.com/tamnd/bento/pkg/frontend"

// nodeKey identifies a node by its source span, which is unique within one
// program: a node covers exactly one half-open range in exactly one file. It
// lets the call graph map a callee's declaration node back to the unit rooted
// there without a node-identity method on the frontend Node, which the adapter
// deliberately does not expose.
type nodeKey struct {
	path string
	pos  frontend.Pos
	end  frontend.Pos
}

func keyOf(n frontend.Node) nodeKey {
	return nodeKey{path: n.File().Path, pos: n.Pos(), end: n.End()}
}

// CallGraph is the direct-call relation over a program's units: for each unit,
// which other units it calls, and whether it calls anything outside this
// program. It is the graph Pass B propagates its contamination and boxing
// lattices over (06_compile_vs_interpret.md section 4.2), the graph the boundary
// cost model prices crossings on (section 10), and the graph whole-program
// partitioning walks in build mode (section 11).
//
// Building it is resolution, not inference. Every call's callee is resolved
// through the checker's own symbols to the declaration the checker already
// bound it to, so the partitioner adds no type knowledge; it only reads which
// unit a call lands in. A callee that resolves to no unit in this program (a
// node: builtin, a go: import, an unresolved dynamic target) is an external
// edge, which the cost model reads as a boundary crossing rather than a native
// call.
type CallGraph struct {
	// Callees[i] holds the indexes of the units unit i calls directly,
	// deduplicated and in first-seen order. A unit that calls itself records its
	// own index, so direct recursion is visible rather than silently dropped.
	Callees [][]int
	// External[i] is true when unit i calls at least one callee that does not
	// resolve to a unit in this program.
	External []bool
}

// CallGraph builds the direct-call graph over the partitioner's units. The unit
// enumeration is the same one Pass A classifies, so a call-graph index and a
// PassA result index name the same unit.
func (pt *Partitioner) CallGraph() CallGraph {
	units := pt.Units()
	index := make(map[nodeKey]int, len(units))
	for i, u := range units {
		index[keyOf(u.Root)] = i
	}
	cg := CallGraph{
		Callees:  make([][]int, len(units)),
		External: make([]bool, len(units)),
	}
	for i, u := range units {
		seen := map[int]bool{}
		pt.collectCalls(u.Root, u.Root, index, &cg, i, seen)
	}
	return cg
}

// collectCalls walks unit i's own body and records the callee of every call and
// new expression it contains. It stops at a nested function boundary, since a
// call inside a nested function belongs to that nested unit, not this one, the
// same boundary Pass A's walkBody honors. The root is passed so the walk can
// descend into the unit's own function node while still refusing to descend into
// any other function-like node it meets.
func (pt *Partitioner) collectCalls(root, node frontend.Node, index map[nodeKey]int, cg *CallGraph, i int, seen map[int]bool) {
	for _, child := range pt.prog.Children(node) {
		switch child.Kind() {
		case frontend.NodeCallExpression, frontend.NodeNewExpression:
			pt.resolveCallee(child, index, cg, i, seen)
		}
		if functionLike(child.Kind()) {
			continue // nested function: its calls belong to its own unit
		}
		pt.collectCalls(root, child, index, cg, i, seen)
	}
}

// resolveCallee resolves one call or new expression's callee to a unit edge. The
// callee is the first child of the call node. When it is a bound reference whose
// declaration is a unit in this program, the call is an in-program edge; an
// import alias is followed to the original declaration first, so a call to an
// imported function still lands on the unit that defines it. Anything else, an
// unbound callee, a property access, or a declaration outside this program, is
// an external edge.
func (pt *Partitioner) resolveCallee(call frontend.Node, index map[nodeKey]int, cg *CallGraph, i int, seen map[int]bool) {
	kids := pt.prog.Children(call)
	if len(kids) == 0 {
		return
	}
	sym, ok := pt.prog.SymbolAt(kids[0])
	if !ok {
		cg.External[i] = true
		return
	}
	sym = pt.prog.Aliased(sym)
	matched := false
	for _, decl := range pt.prog.Declarations(sym) {
		j, ok := index[keyOf(decl)]
		if !ok {
			continue
		}
		matched = true
		if !seen[j] {
			seen[j] = true
			cg.Callees[i] = append(cg.Callees[i], j)
		}
	}
	if !matched {
		cg.External[i] = true
	}
}
