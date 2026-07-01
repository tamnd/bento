// Package partition is bento's partitioner: it reads a fully checked TypeScript
// program through the frontend and decides, for every function and module,
// whether it runs as ahead-of-time compiled Go, as interpreted engine code, or
// as speculatively compiled code guarded at runtime. It implements
// 06_compile_vs_interpret.md.
//
// The algorithm is three passes over the unit set. Pass A classifies each unit
// locally, from its own syntax and its own types, in parallel across units. Pass
// B propagates contamination and boxing to a fixpoint over the call and
// data-flow graphs. Pass C promotes narrow, guardable soft blockers to
// speculations and seals every verdict. This file wires the passes; Pass A lands
// here first (passa.go), with B and C following in later slices.
//
// The partitioner never re-derives types. It trusts the checker's types
// verbatim, querying them only through pkg/frontend, so it holds no
// typescript-go type and speaks bento's own value vocabulary throughout.
package partition

import (
	"runtime"
	"sync"

	"github.com/tamnd/bento/pkg/frontend"
)

// Partitioner classifies the units of one checked program.
type Partitioner struct {
	prog *frontend.Program
}

// New builds a partitioner over a checked program.
func New(prog *frontend.Program) *Partitioner {
	return &Partitioner{prog: prog}
}

// Result is a unit's preliminary classification after Pass A: a verdict plus the
// reasons behind it. A Compiled verdict carries no reasons. An Interpreted
// verdict carries at least one reason; when every reason is Soft the unit is a
// Pass C speculation candidate, reported by SpeculationCandidate.
type Result struct {
	Unit    Unit
	Verdict Verdict
	Reasons []Reason
}

// SpeculationCandidate reports whether Pass C should consider promoting this
// unit to Speculative. That is exactly the case when the unit did not compile
// cleanly but every blocker it hit is soft; a single hard blocker rules it out
// permanently.
func (r Result) SpeculationCandidate() bool {
	if len(r.Reasons) == 0 {
		return false
	}
	for _, reason := range r.Reasons {
		if reason.Severity() == Hard {
			return false
		}
	}
	return true
}

// verdictFor turns a unit's Pass A reasons into a preliminary verdict. No reason
// means Compiled-eligible. Any reason means the unit does not compile cleanly
// yet, so Pass A leaves it Interpreted; Pass C later lifts the soft-only ones to
// Speculative.
func verdictFor(reasons []Reason) Verdict {
	if len(reasons) == 0 {
		return Compiled
	}
	return Interpreted
}

// PassA runs local classification over every unit and returns one Result per
// unit. It is parallel across units, which is sound because classifying a unit
// only reads the program and the frontend interners are concurrency-safe. The
// results preserve the order of Units so a caller can rely on a stable ordering.
func (pt *Partitioner) PassA() []Result {
	units := pt.Units()
	results := make([]Result, len(units))

	workers := max(min(runtime.GOMAXPROCS(0), len(units)), 1)

	var wg sync.WaitGroup
	indexes := make(chan int)
	for range workers {
		wg.Go(func() {
			for i := range indexes {
				reasons := pt.classifyLocal(units[i])
				results[i] = Result{
					Unit:    units[i],
					Verdict: verdictFor(reasons),
					Reasons: reasons,
				}
			}
		})
	}
	for i := range units {
		indexes <- i
	}
	close(indexes)
	wg.Wait()

	return results
}
