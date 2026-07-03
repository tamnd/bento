package partition

import "github.com/tamnd/bento/pkg/frontend"

// Tier is the cost tier of one value crossing a compiled-to-interpreted boundary
// (06_compile_vs_interpret.md section 10.1), cheapest first. The spec fixes only
// the ordering Tier0 < Tier1 < Tier2 < Tier3 < Tier4; the magnitudes are measured
// per backend and fed in.
type Tier int

const (
	// Tier0 is no crossing: a compiled callee taking monomorphic values, a plain
	// Go call. This is the target for all hot code.
	Tier0 Tier = iota
	// Tier1 is a shared boxed value crossing: the value is already a canonical
	// Value, so crossing is passing a pointer through the engine SPI with no
	// conversion.
	Tier1
	// Tier2 is a primitive box or unbox: a number, boolean, or small primitive
	// converts to or from a Value, allocation-light with the tagged representation.
	Tier2
	// Tier3 is a structural marshal: a struct, array, or object shape is wrapped so
	// the other side sees a correctly shaped object with preserved identity, paid
	// once per object and near-free on reuse.
	Tier3
	// Tier4 is a deopt, the most expensive, priced by the deopt cost of section 8.5
	// and kept off hot paths by construction.
	Tier4
)

func (t Tier) String() string {
	switch t {
	case Tier0:
		return "Tier0 native"
	case Tier1:
		return "Tier1 boxed"
	case Tier2:
		return "Tier2 primitive"
	case Tier3:
		return "Tier3 structural"
	case Tier4:
		return "Tier4 deopt"
	default:
		return "Tier?"
	}
}

// TierWeights assigns a relative per-crossing weight to each tier plus the base
// invocation cost. Only the ordering is load-bearing (section 10.3): the defaults
// honor Tier0 < Tier1 < Tier2 < Tier3 < Tier4 and stand in until the benchmark
// harness feeds measured per-backend costs, which replace them without touching the
// classification.
type TierWeights struct {
	// Base is the invocation cost every non-native crossing pays on top of its
	// argument and result tiers.
	Base float64
	// Tier holds the weight of each tier, indexed by Tier.
	Tier [5]float64
}

// DefaultTierWeights returns the placeholder weights used until measured costs
// replace them. A native call costs only the base; each further tier costs
// strictly more than the one below it.
func DefaultTierWeights() TierWeights {
	return TierWeights{Base: 1, Tier: [5]float64{0, 1, 2, 5, 20}}
}

// Crossing describes the boundary cost of one call site: whether it is a native
// Tier 0 call or a real crossing, and if a crossing, the tier of each argument and
// of the result flowing back.
type Crossing struct {
	// Native is true when the callee is compiled, so the call is a plain Go call
	// with no boxing and the argument and result tiers do not apply.
	Native bool
	// Args holds the crossing tier of each argument, in argument order.
	Args []Tier
	// Result is the crossing tier of the value returned across the boundary.
	Result Tier
}

// Cost prices a crossing under these weights: a native call costs only the base,
// and a real crossing costs the base plus every argument tier plus the result tier,
// which is the per-call estimate section 10.2 uses for the layout and loop-hoisting
// decisions.
func (w TierWeights) Cost(c Crossing) float64 {
	if c.Native {
		return w.Base
	}
	total := w.Base + w.Tier[c.Result]
	for _, t := range c.Args {
		total += w.Tier[t]
	}
	return total
}

// SiteCrossing is one call site's crossing, labeled with the calling unit and the
// name of the callee, so the cost of a program's boundaries can be read per site.
type SiteCrossing struct {
	Caller   Unit
	Callee   string
	Crossing Crossing
}

// Crossings classifies every call and new expression in the program into its
// crossing tier, using the sealed verdicts to decide which callees are native. A
// call into a compiled unit is Tier 0; a call into a speculative or interpreted
// unit, or out of the program entirely, crosses the boundary and each argument and
// the result is priced by its value tier. The sealed slice is the one Units
// produced, so its index and the unit enumeration agree, which lets a callee's
// verdict be read directly.
func (pt *Partitioner) Crossings(sealed []Result) []SiteCrossing {
	units := pt.Units()
	index := make(map[nodeKey]int, len(units))
	for i, u := range units {
		index[keyOf(u.Root)] = i
	}
	var sites []SiteCrossing
	for i, u := range units {
		pt.collectCrossings(u.Root, units[i], index, sealed, &sites)
	}
	return sites
}

func (pt *Partitioner) collectCrossings(node frontend.Node, caller Unit, index map[nodeKey]int, sealed []Result, sites *[]SiteCrossing) {
	for _, child := range pt.prog.Children(node) {
		switch child.Kind() {
		case frontend.NodeCallExpression, frontend.NodeNewExpression:
			*sites = append(*sites, pt.crossingAt(child, caller, index, sealed))
		}
		if functionLike(child.Kind()) {
			continue
		}
		pt.collectCrossings(child, caller, index, sealed, sites)
	}
}

// crossingAt classifies one call site. A callee that resolves to a compiled unit in
// this program is a native call; anything else, a speculative or interpreted unit
// or an external target, is a crossing whose argument and result tiers are read off
// the checker's types.
func (pt *Partitioner) crossingAt(call frontend.Node, caller Unit, index map[nodeKey]int, sealed []Result) SiteCrossing {
	kids := pt.prog.Children(call)
	callee := "(unknown)"
	if len(kids) > 0 {
		callee = pt.prog.Text(kids[0])
	}

	if j, ok := pt.calleeUnit(call, index); ok {
		callee = sealed[j].Unit.Name
		if sealed[j].Verdict == Compiled {
			return SiteCrossing{Caller: caller, Callee: callee, Crossing: Crossing{Native: true}}
		}
	}

	var args []Tier
	for _, arg := range kids[1:] {
		args = append(args, tierForCrossingValue(pt.prog.TypeAt(arg)))
	}
	return SiteCrossing{
		Caller:   caller,
		Callee:   callee,
		Crossing: Crossing{Args: args, Result: tierForCrossingValue(pt.prog.TypeAt(call))},
	}
}

// calleeUnit resolves a call's callee to the index of the unit it lands on, or
// reports that it resolves to no unit in this program, by the same symbol path the
// call graph uses.
func (pt *Partitioner) calleeUnit(call frontend.Node, index map[nodeKey]int) (int, bool) {
	kids := pt.prog.Children(call)
	if len(kids) == 0 {
		return 0, false
	}
	sym, ok := pt.prog.SymbolAt(kids[0])
	if !ok {
		return 0, false
	}
	sym = pt.prog.Aliased(sym)
	for _, decl := range pt.prog.Declarations(sym) {
		if j, ok := index[keyOf(decl)]; ok {
			return j, true
		}
	}
	return 0, false
}

// tierForCrossingValue maps the type of a value to the tier it crosses at. A value
// already held as a canonical Value, an any or unknown binding, crosses at Tier 1
// with no conversion. A primitive boxes or unboxes at Tier 2. An object shape is
// structurally marshaled at Tier 3. A value that carries nothing across, void or
// undefined or null as a result, is Tier 0.
func tierForCrossingValue(t frontend.Type) Tier {
	switch {
	case t.Flags&(frontend.TypeVoid|frontend.TypeUndefined|frontend.TypeNull) != 0:
		return Tier0
	case t.Flags&(frontend.TypeAny|frontend.TypeUnknown) != 0:
		return Tier1
	case t.Flags&frontend.TypeObject != 0:
		return Tier3
	case t.Flags&(frontend.TypeNumber|frontend.TypeString|frontend.TypeBoolean|frontend.TypeBigInt) != 0:
		return Tier2
	default:
		return Tier1
	}
}
