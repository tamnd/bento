package partition

import "testing"

// realCrossings compiles a snippet, seals it, and classifies every call site's
// boundary crossing.
func realCrossings(t *testing.T, src string) []SiteCrossing {
	t.Helper()
	pt := New(loadReal(t, src, false))
	return pt.Crossings(pt.PassC(pt.PassB(pt.PassA())))
}

// findCrossing returns the crossing for the call from caller to callee.
func findCrossing(sites []SiteCrossing, caller, callee string) (Crossing, bool) {
	for _, s := range sites {
		if s.Caller.Name == caller && s.Callee == callee {
			return s.Crossing, true
		}
	}
	return Crossing{}, false
}

// TestCrossingNativeCallIsTier0 pins that a call into a compiled unit is a native
// Tier 0 call with no crossing: outer calls inner, inner compiles cleanly, so the
// call is a plain Go call costing only the base.
func TestCrossingNativeCallIsTier0(t *testing.T) {
	src := "function inner(n: number): number { return n; }\n" +
		"export function outer(x: number): number { return inner(x); }\n"
	sites := realCrossings(t, src)

	c, ok := findCrossing(sites, "outer", "inner")
	if !ok {
		t.Fatalf("no crossing recorded for outer -> inner in %v", sites)
	}
	if !c.Native {
		t.Errorf("outer -> inner is not native, but inner is compiled; crossing = %+v", c)
	}
	if got := DefaultTierWeights().Cost(c); got != 1 {
		t.Errorf("native crossing cost = %v, want the base 1", got)
	}
}

// TestCrossingIntoInterpretedIsPriced pins that a call into an interpreted unit
// crosses the boundary, and each argument and the result is priced by its value
// tier. caller calls bad, which evals and is interpreted, passing a number and
// getting a number back, so the crossing is base plus two primitive tiers.
func TestCrossingIntoInterpretedIsPriced(t *testing.T) {
	src := "function bad(n: number): number { eval(\"1\"); return n; }\n" +
		"export function caller(x: number): number { return bad(x); }\n"
	sites := realCrossings(t, src)

	c, ok := findCrossing(sites, "caller", "bad")
	if !ok {
		t.Fatalf("no crossing recorded for caller -> bad in %v", sites)
	}
	if c.Native {
		t.Fatalf("caller -> bad is native, but bad is interpreted; crossing = %+v", c)
	}
	if len(c.Args) != 1 || c.Args[0] != Tier2 {
		t.Errorf("argument tiers = %v, want a single Tier2 primitive", c.Args)
	}
	if c.Result != Tier2 {
		t.Errorf("result tier = %v, want Tier2 primitive", c.Result)
	}
	w := DefaultTierWeights()
	if got, want := w.Cost(c), w.Base+w.Tier[Tier2]*2; got != want {
		t.Errorf("crossing cost = %v, want %v (base plus two primitive tiers)", got, want)
	}
}

// TestCrossingArgumentTiersByType pins that each crossing argument is classified by
// its own type: an object marshals structurally, a primitive boxes, an any value
// crosses already boxed, and a void result carries nothing back. feed hands all
// three to console.log, an external callee.
func TestCrossingArgumentTiersByType(t *testing.T) {
	src := "export function feed(rec: { total: number }, n: number, flag: any): void { console.log(rec, n, flag); }\n"
	sites := realCrossings(t, src)

	c, ok := findCrossing(sites, "feed", "console.log")
	if !ok {
		t.Fatalf("no crossing recorded for feed -> console.log in %v", sites)
	}
	want := []Tier{Tier3, Tier2, Tier1}
	if len(c.Args) != len(want) {
		t.Fatalf("argument tiers = %v, want %v", c.Args, want)
	}
	for i := range want {
		if c.Args[i] != want[i] {
			t.Errorf("argument %d tier = %v, want %v", i, c.Args[i], want[i])
		}
	}
	if c.Result != Tier0 {
		t.Errorf("result tier = %v, want Tier0 for a void result", c.Result)
	}
}

// TestTierWeightsOrdered pins the one invariant the spec fixes (section 10.3): the
// tiers are strictly increasing in cost, so a cheaper tier is always preferred.
func TestTierWeightsOrdered(t *testing.T) {
	w := DefaultTierWeights()
	for i := 1; i < len(w.Tier); i++ {
		if !(w.Tier[i] > w.Tier[i-1]) {
			t.Errorf("tier %d weight %v is not greater than tier %d weight %v", i, w.Tier[i], i-1, w.Tier[i-1])
		}
	}
}
