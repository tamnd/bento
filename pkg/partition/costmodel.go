package partition

// CostModel prices Pass C's promotion decision: whether compiling a soft-blocked
// unit under a guarded assumption beats just interpreting it
// (06_compile_vs_interpret.md section 7.5). Speculation is not free, a guard that
// fails often costs the guard, the deopt, and the interpreted run anyway, so a
// unit is promoted only when the speculated expected cost per call beats the
// interpreted cost.
//
// The costs are relative, not absolute nanoseconds. Section 10.3 fixes only their
// ordering, Compiled < Guard < Deopt < Interp in effect, and leaves the real
// magnitudes to be measured per engine backend by the benchmark harness and fed
// in. The defaults here honor that ordering and are the placeholder until measured
// costs replace them; the promotion logic reads them the same either way.
type CostModel struct {
	// Compiled is the per-call cost of the compiled body.
	Compiled float64
	// Interp is the per-call cost of interpreting the unit.
	Interp float64
	// Guard is the cost of one entry or shape guard that passes.
	Guard float64
	// Deopt is the one-time cost of a deopt transition when a guard misses.
	Deopt float64
	// MaxGuards caps how many guards a single unit may carry. Past the cap the
	// unit is left interpreted, because a unit that needs many independent
	// speculations to compile is not really typed and is not worth the fragility.
	MaxGuards int
}

// DefaultCostModel returns the placeholder cost model used until the benchmark
// harness feeds measured per-backend costs. Compiled code is far cheaper than
// interpretation, a passing guard is tiny, and a deopt is dear, which is the
// ordering the spec fixes.
func DefaultCostModel() CostModel {
	return CostModel{Compiled: 1, Interp: 10, Guard: 0.2, Deopt: 5, MaxGuards: 4}
}

// Promote reports whether a unit with the given per-call guard-hit probability and
// guard count should be speculated. It is the section 7.5 inequality:
//
//	guards*Guard + pHit*Compiled + (1-pHit)*(Deopt + Interp)  <  Interp
//
// The speculated expected cost, paying every guard, running compiled when the
// guards hit and deopting to the interpreter when they miss, must beat interpreting
// outright. Because Compiled is far below Interp and Guard is tiny, this holds
// easily when pHit is high and fails when pHit is low, which is exactly the
// behavior the spec wants. A unit with no guardable spot has nothing to speculate,
// and a unit past the guard cap is too fragile, so both decline.
func (m CostModel) Promote(pHit float64, guards int) bool {
	if guards <= 0 || guards > m.MaxGuards {
		return false
	}
	expected := float64(guards)*m.Guard + pHit*m.Compiled + (1-pHit)*(m.Deopt+m.Interp)
	return expected < m.Interp
}
