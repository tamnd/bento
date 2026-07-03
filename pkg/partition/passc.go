package partition

// defaultPHit is the guard-hit probability Pass C assumes for a soft-blocked unit
// when it has no sharper evidence. It is the declared-but-widened confidence of
// 06_compile_vs_interpret.md section 7.5: a value the checker widened to any at a
// boundary is, in practice, almost always the one concrete shape the code was
// written for, so the entry guard almost always hits. Call-site agreement raises
// this to a certainty and profile feedback can lower it; both are a later slice,
// and until they land Pass C decides on this one default. It is deliberately
// short of 1.0 so the cost model still charges the deopt tail.
const defaultPHit = 0.9

// PassC is the third and final partitioner pass: it promotes the narrow, guardable
// soft blockers left by Pass B into speculations and seals every verdict
// (06_compile_vs_interpret.md sections 4.2 and 7). A unit that did not compile
// cleanly, but whose every blocker is soft and at least one is a spot a runtime
// guard can stand in for, becomes Speculative exactly when the section 7.5 cost
// model says the guarded compiled path beats interpreting outright. Everything
// else keeps the verdict it arrived with: cleanly compiled units stay Compiled,
// hard-blocked units stay Interpreted, and soft blockers the cost model or the
// guardability rule turns down stay Interpreted too.
func (pt *Partitioner) PassC(pb []Result) []Result {
	return seal(pb, DefaultCostModel())
}

// seal is PassC parameterized by the cost model, so the promotion boundary can be
// exercised with a model whose costs are known rather than only the default. It
// copies the input so the caller's slice is untouched and never revisits a unit
// that is already Compiled or already Speculative; only an Interpreted unit is a
// promotion candidate.
func seal(pb []Result, m CostModel) []Result {
	out := make([]Result, len(pb))
	copy(out, pb)
	for i := range out {
		if out[i].Verdict != Interpreted {
			continue
		}
		if !out[i].SpeculationCandidate() {
			continue
		}
		guards := guardCount(out[i].Reasons)
		if guards == 0 {
			continue
		}
		if m.Promote(defaultPHit, guards) {
			out[i].Verdict = Speculative
		}
	}
	return out
}

// guardCount is how many guards a speculation of this unit would carry: one for
// each guardable soft blocker it hit. A unit with no guardable blocker has nothing
// to speculate on and is not promoted; a unit with more guardable blockers than the
// cost model's cap is too fragile to promote. The reasons are already deduplicated
// by kind, so this counts distinct guardable causes rather than repeated sightings
// of one.
func guardCount(reasons []Reason) int {
	n := 0
	for _, r := range reasons {
		if r.Kind.Guardable() {
			n++
		}
	}
	return n
}
