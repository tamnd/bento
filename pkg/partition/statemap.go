package partition

import "github.com/tamnd/bento/pkg/frontend"

// BoxKind says how one live value in a compiled frame becomes a canonical engine
// Value when a guard misses and the activation deopts to the interpreter
// (06_compile_vs_interpret.md section 8.2, 10_value_model.md). A monomorphic scalar
// is boxed from its Go representation; an object is boxed by its static shape; a
// value the frame already holds boxed needs no conversion.
type BoxKind int

const (
	// BoxNumber boxes a monomorphic float64 slot into a number Value.
	BoxNumber BoxKind = iota
	// BoxString boxes a monomorphic string slot into a string Value.
	BoxString
	// BoxBoolean boxes a monomorphic bool slot into a boolean Value.
	BoxBoolean
	// BoxBigInt boxes a monomorphic big-integer slot into a bigint Value.
	BoxBigInt
	// BoxObject boxes a monomorphic Go struct slot into an object Value using the
	// value's static shape, which lowering records alongside the slot.
	BoxObject
	// BoxAlready marks a slot the compiled frame already holds as a Value, an any
	// or unknown binding or a value escape analysis boxed up front, which the deopt
	// routine copies across without conversion.
	BoxAlready
)

func (b BoxKind) String() string {
	switch b {
	case BoxNumber:
		return "number"
	case BoxString:
		return "string"
	case BoxBoolean:
		return "boolean"
	case BoxBigInt:
		return "bigint"
	case BoxObject:
		return "object"
	case BoxAlready:
		return "already boxed"
	default:
		return "unknown"
	}
}

// LiveSlot is one live binding at a guard: the binding it names and how the deopt
// routine boxes it into a Value. The physical location of the value in the compiled
// frame is assigned by lowering; at the partition layer a slot names the binding
// and fixes its box plan, which is the part that depends only on the checker's
// types and not on code generation.
type LiveSlot struct {
	Name   string
	Symbol frontend.Symbol
	Box    BoxKind
}

// Guard is one guard point in a speculated unit and the state map that lets its
// miss deopt correctly (section 8.2): the source position interpretation resumes
// at, the depth of the active try-handler stack there so catch and finally fire
// exactly as the language requires (section 8.4), and every live binding with its
// box plan.
type Guard struct {
	ResumePos    frontend.Pos
	HandlerDepth int
	Live         []LiveSlot
}

// StateMap is the deopt table for one speculated unit: its guards and, per guard,
// the data to rebuild an interpreter activation from the compiled frame (section
// 8.2). It is generated once at build time and never runs unless a guard misses.
type StateMap struct {
	Unit   Unit
	Guards []Guard
}

// StateMaps builds the deopt state map for every unit the seal left Speculative.
// Each speculated function carries an entry guard that checks its parameter shapes
// on the way in; a miss deopts by boxing the live parameters and resuming
// interpretation from the top of the unit, which reproduces exactly what pure
// interpretation would have done (section 8.1). The live set at entry is the unit's
// parameters, and the handler stack there is empty, so the entry guard's state map
// is the parameters with their box plans and a resume at the unit's own position.
//
// The live set is over-approximated to the full parameter list rather than only the
// parameters a later use makes live, which is sound for deopt: boxing a value the
// interpreted continuation never reads wastes a little work but never changes the
// result. Guards inside the body, whose live sets need real liveness and whose
// resume positions are mid-body, are a later slice; the entry guard is the one every
// speculation carries.
func (pt *Partitioner) StateMaps(sealed []Result) []StateMap {
	var maps []StateMap
	for _, r := range sealed {
		if r.Verdict != Speculative || r.Unit.Kind != UnitFunction {
			continue
		}
		guard := Guard{
			ResumePos:    r.Unit.Root.Pos(),
			HandlerDepth: 0,
			Live:         pt.entryParams(r.Unit.Root),
		}
		maps = append(maps, StateMap{Unit: r.Unit, Guards: []Guard{guard}})
	}
	return maps
}

// entryParams collects the unit's own parameters as live slots, each with the box
// plan its declared type dictates. It walks the function subtree the way the other
// Pass B and Pass C walks do, stopping at a nested function boundary so a nested
// arrow's parameters are not mistaken for this unit's.
func (pt *Partitioner) entryParams(root frontend.Node) []LiveSlot {
	var slots []LiveSlot
	pt.collectParams(root, &slots)
	return slots
}

func (pt *Partitioner) collectParams(node frontend.Node, slots *[]LiveSlot) {
	for _, child := range pt.prog.Children(node) {
		if child.Kind() == frontend.NodeParameter {
			if slot, ok := pt.paramSlot(child); ok {
				*slots = append(*slots, slot)
			}
			// Do not descend into the parameter's own subtree: a callback-typed
			// parameter carries a function-type annotation whose inner parameters
			// belong to that type, not to this unit.
			continue
		}
		if functionLike(child.Kind()) {
			continue
		}
		pt.collectParams(child, slots)
	}
}

// paramSlot turns a parameter node into a live slot: its binding symbol, its name,
// and the box plan for its declared type. A parameter with no plain-identifier name
// (a destructuring pattern) names no single binding to box and is skipped for now.
func (pt *Partitioner) paramSlot(param frontend.Node) (LiveSlot, bool) {
	for _, child := range pt.prog.Children(param) {
		if child.Kind() != frontend.NodeIdentifier {
			continue
		}
		sym, ok := pt.valueBindingSymbol(child)
		if !ok {
			return LiveSlot{}, false
		}
		return LiveSlot{Name: sym.Name, Symbol: sym, Box: boxKindFor(pt.prog.TypeAt(child))}, true
	}
	return LiveSlot{}, false
}

// boxKindFor maps a checker type to the deopt box plan for a value of that type. A
// monomorphic scalar boxes from its Go representation, an object boxes by its
// static shape, and anything the compiled frame already holds as a Value, an any or
// unknown binding above all, is copied across already boxed.
func boxKindFor(t frontend.Type) BoxKind {
	switch {
	case t.Flags&frontend.TypeNumber != 0:
		return BoxNumber
	case t.Flags&frontend.TypeString != 0:
		return BoxString
	case t.Flags&frontend.TypeBoolean != 0:
		return BoxBoolean
	case t.Flags&frontend.TypeBigInt != 0:
		return BoxBigInt
	case t.Flags&frontend.TypeObject != 0:
		return BoxObject
	default:
		return BoxAlready
	}
}
