package partition

import "testing"

// partitionRealB runs Pass A then Pass B over a real compile and returns the
// post-B results, the verdicts that hold once contamination has propagated.
func partitionRealB(t *testing.T, src string) []Result {
	t.Helper()
	pt := New(loadReal(t, src, false))
	return pt.PassB(pt.PassA())
}

// hasReason reports whether a result carries a reason of the given kind.
func hasReason(r Result, kind ReasonKind) bool {
	for _, reason := range r.Reasons {
		if reason.Kind == kind {
			return true
		}
	}
	return false
}

// TestPassBContainmentOrdinaryCall pins the load-bearing guarantee of section
// 4.4: a compiled unit that calls an interpreted unit stays compiled. The call
// costs a boundary crossing, not contamination, so an ordinary call edge never
// demotes the caller. sink is interpreted for its eval; caller is fully typed and
// only calls sink, so it must remain Compiled after Pass B.
func TestPassBContainmentOrdinaryCall(t *testing.T) {
	src := "function sink(): number { eval(\"1\"); return 0; }\n" +
		"export function caller(x: number): number { return x + sink(); }\n"
	results := partitionRealB(t, src)

	sink, ok := resultNamed(results, "sink")
	if !ok {
		t.Fatalf("no unit named sink in %v", results)
	}
	if sink.Verdict != Interpreted {
		t.Fatalf("sink verdict = %v, want Interpreted (it evals)", sink.Verdict)
	}

	caller, ok := resultNamed(results, "caller")
	if !ok {
		t.Fatalf("no unit named caller in %v", results)
	}
	if caller.Verdict != Compiled {
		t.Errorf("caller verdict = %v reasons = %v, want Compiled (calling interpreted code does not contaminate)", caller.Verdict, caller.Reasons)
	}
}

// TestPassBControlInversionUntypedCallback pins the control-inversion edge: a
// compiled function handed to an any-typed callback position is promoted to
// Speculative, because interpreted or unknown code holding that reference may call
// it with wrong-typed arguments, so it needs entry guards. tick is compiled;
// schedule takes an any callback; main hands tick to schedule, which inverts
// control into tick.
func TestPassBControlInversionUntypedCallback(t *testing.T) {
	src := "function tick(n: number): void {}\n" +
		"export function schedule(cb: any): void { cb(); }\n" +
		"export function main(): void { schedule(tick); }\n"
	results := partitionRealB(t, src)

	tick, ok := resultNamed(results, "tick")
	if !ok {
		t.Fatalf("no unit named tick in %v", results)
	}
	if tick.Verdict != Speculative {
		t.Fatalf("tick verdict = %v reasons = %v, want Speculative (control inverted through an any callback)", tick.Verdict, tick.Reasons)
	}
	if !hasReason(tick, ReasonControlInversion) {
		t.Errorf("tick reasons = %v, want a ReasonControlInversion among them", tick.Reasons)
	}

	main, ok := resultNamed(results, "main")
	if !ok {
		t.Fatalf("no unit named main in %v", results)
	}
	if main.Verdict != Compiled {
		t.Errorf("main verdict = %v reasons = %v, want Compiled (handing a reference away does not demote the hander)", main.Verdict, main.Reasons)
	}
}

// TestPassBTypedCallbackDoesNotInvert pins the tight side of the rule: a compiled
// function handed to a concretely typed callback position is not inverted, because
// the type system models exactly how it is called, so no guard is needed. tick
// stays Compiled when passed to each, whose parameter is (n: number) => void.
func TestPassBTypedCallbackDoesNotInvert(t *testing.T) {
	src := "function tick(n: number): void {}\n" +
		"export function each(cb: (n: number) => void): void { cb(1); }\n" +
		"export function main(): void { each(tick); }\n"
	results := partitionRealB(t, src)

	tick, ok := resultNamed(results, "tick")
	if !ok {
		t.Fatalf("no unit named tick in %v", results)
	}
	if tick.Verdict != Compiled {
		t.Errorf("tick verdict = %v reasons = %v, want Compiled (a typed callback position does not invert control)", tick.Verdict, tick.Reasons)
	}
	if hasReason(tick, ReasonControlInversion) {
		t.Errorf("tick carries a control-inversion reason for a typed position, reasons = %v", tick.Reasons)
	}
}

// TestPassBOrdinaryCallIsNotInversion pins that calling a function, as opposed to
// passing it by reference, never inverts control: the callee sits in call
// position, not argument position, so an ordinary call through it leaves it
// Compiled even when the call flows through fully typed code.
func TestPassBOrdinaryCallIsNotInversion(t *testing.T) {
	src := "function helper(n: number): number { return n; }\n" +
		"export function main(x: number): number { return helper(x); }\n"
	results := partitionRealB(t, src)

	helper, ok := resultNamed(results, "helper")
	if !ok {
		t.Fatalf("no unit named helper in %v", results)
	}
	if helper.Verdict != Compiled {
		t.Errorf("helper verdict = %v reasons = %v, want Compiled (being called is not control inversion)", helper.Verdict, helper.Reasons)
	}
}

// TestPassBInterpretedUnitNotPromoted pins that control inversion only promotes a
// cleanly compiled unit. A unit Pass A left Interpreted for a hard blocker stays
// Interpreted even when its function is handed to an untyped callback position;
// entry guards cannot rescue code that cannot be compiled at all. bad evals, so it
// stays Interpreted despite being passed to an any callback.
func TestPassBInterpretedUnitNotPromoted(t *testing.T) {
	src := "function bad(): number { eval(\"1\"); return 0; }\n" +
		"export function sink(cb: any): void {}\n" +
		"export function run(): void { sink(bad); }\n"
	results := partitionRealB(t, src)

	bad, ok := resultNamed(results, "bad")
	if !ok {
		t.Fatalf("no unit named bad in %v", results)
	}
	if bad.Verdict != Interpreted {
		t.Errorf("bad verdict = %v reasons = %v, want Interpreted (a hard blocker is not rescued by guards)", bad.Verdict, bad.Reasons)
	}
	if hasReason(bad, ReasonControlInversion) {
		t.Errorf("bad was tagged with control inversion, but it is a hard blocker, reasons = %v", bad.Reasons)
	}
}
