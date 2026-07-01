package partition

import "github.com/tamnd/bento/pkg/frontend"

// UnitKind tells what scope a Unit stands for. A unit is the smallest thing the
// partitioner classifies (06_compile_vs_interpret.md section 3): a function,
// counting the top-level module body as a synthetic function, plus a few
// non-function scopes that behave like functions for classification.
type UnitKind int

const (
	// UnitFunction is an ordinary function-like scope: a function declaration or
	// expression, an arrow, a method, an accessor, or a constructor.
	UnitFunction UnitKind = iota
	// UnitModule is a source file's top-level body, treated as a synthetic
	// function.
	UnitModule
	// UnitStaticInit is a class static initializer block.
	UnitStaticInit
	// UnitFieldInit is a class field initializer.
	UnitFieldInit
	// UnitCatchArm is the arm of a try that carries a catch binding of dynamic
	// type.
	UnitCatchArm
)

func (k UnitKind) String() string {
	switch k {
	case UnitFunction:
		return "Function"
	case UnitModule:
		return "Module"
	case UnitStaticInit:
		return "StaticInit"
	case UnitFieldInit:
		return "FieldInit"
	case UnitCatchArm:
		return "CatchArm"
	default:
		return "Unknown"
	}
}

// Unit is one classifiable scope. Root is the node the unit is rooted at (the
// function node, or the source-file node for a module), and Name is a best-effort
// label for diagnostics.
type Unit struct {
	Kind UnitKind
	Root frontend.Node
	Name string
}

// functionLike reports whether a node kind introduces its own function scope, so
// unit enumeration stops descending into it (it becomes its own unit) and Pass A
// does not walk into a nested function's body when classifying the outer one.
func functionLike(k frontend.NodeKind) bool {
	switch k {
	case frontend.NodeFunctionDeclaration,
		frontend.NodeFunctionExpression,
		frontend.NodeArrowFunction,
		frontend.NodeMethodDeclaration,
		frontend.NodeGetAccessor,
		frontend.NodeSetAccessor,
		frontend.NodeConstructor:
		return true
	default:
		return false
	}
}

// Units enumerates every classifiable unit in the program. Each source file is a
// module unit, and every function-like node anywhere in the tree is its own
// unit. Static and field initializers and dynamic catch arms are recognized as
// their own kinds when the frontend surfaces them; the enumeration is written so
// adding them is a matter of extending the walk, not reshaping it.
func (pt *Partitioner) Units() []Unit {
	var units []Unit
	for _, root := range pt.prog.SourceFiles() {
		if functionLike(root.Kind()) {
			// A fake or a real program may hand back a function directly as a
			// root; treat it as a function unit and still walk it for nested
			// functions.
			units = append(units, Unit{Kind: UnitFunction, Root: root, Name: pt.nameOf(root)})
		} else {
			units = append(units, Unit{Kind: UnitModule, Root: root, Name: root.File().Path})
		}
		units = pt.collectNested(root, units)
	}
	return units
}

// collectNested walks below a root and adds every function-like node it finds as
// its own function unit, without descending past a function boundary more than
// once (each function's own children are visited when that function is the
// current node).
func (pt *Partitioner) collectNested(node frontend.Node, units []Unit) []Unit {
	for _, child := range pt.prog.Children(node) {
		if functionLike(child.Kind()) {
			units = append(units, Unit{Kind: UnitFunction, Root: child, Name: pt.nameOf(child)})
		}
		units = pt.collectNested(child, units)
	}
	return units
}

// nameOf returns the declared name of a function-like node when the checker bound
// one, falling back to an anonymous label.
func (pt *Partitioner) nameOf(node frontend.Node) string {
	if sym, ok := pt.prog.SymbolAt(node); ok && sym.Name != "" {
		return sym.Name
	}
	return "(anonymous)"
}
