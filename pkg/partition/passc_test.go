package partition

import "testing"

// partitionRealC runs all three passes over a real compile and returns the sealed
// results, the final verdicts the partitioner commits to.
func partitionRealC(t *testing.T, src string) []Result {
	t.Helper()
	pt := New(loadReal(t, src, false))
	return pt.PassC(pt.PassB(pt.PassA()))
}

// TestPassCPromotesGuardableAnyParam pins the section 7.6 worked case: a function
// blocked only by an any parameter is a guardable soft blocker, so Pass C compiles
// it under an entry guard and seals it Speculative rather than leaving it on the
// engine. fee reads its any parameter, the one soft blocker, and is promoted.
func TestPassCPromotesGuardableAnyParam(t *testing.T) {
	results := partitionRealC(t, "export function fee(raw: any): number { return raw.amount * 0.029 + 30; }\n")

	fee, ok := resultNamed(results, "fee")
	if !ok {
		t.Fatalf("no unit named fee in %v", results)
	}
	if fee.Verdict != Speculative {
		t.Errorf("fee verdict = %v reasons = %v, want Speculative (an any parameter is a guardable soft blocker)", fee.Verdict, fee.Reasons)
	}
}

// TestPassCLeavesHardBlockerInterpreted pins that Pass C never rescues a hard
// blocker: a unit that evals cannot be compiled under any guard, so it stays
// Interpreted through the seal. sink evals and is left on the engine.
func TestPassCLeavesHardBlockerInterpreted(t *testing.T) {
	results := partitionRealC(t, "export function sink(): number { eval(\"1\"); return 0; }\n")

	sink, ok := resultNamed(results, "sink")
	if !ok {
		t.Fatalf("no unit named sink in %v", results)
	}
	if sink.Verdict != Interpreted {
		t.Errorf("sink verdict = %v reasons = %v, want Interpreted (a hard blocker is never speculated)", sink.Verdict, sink.Reasons)
	}
}

// TestPassCLeavesUnlowerableTypeInterpreted pins the guardability line: a soft
// blocker that no guard can stand in for is not a speculation. A bare type
// parameter needs monomorphization, not a runtime shape check, so identity is left
// Interpreted until the lowering set reaches it rather than wrapped in a guard that
// could never miss.
func TestPassCLeavesUnlowerableTypeInterpreted(t *testing.T) {
	results := partitionRealC(t, "export function identity<T>(x: T): T { return x; }\n")

	identity, ok := resultNamed(results, "identity")
	if !ok {
		t.Fatalf("no unit named identity in %v", results)
	}
	if identity.Verdict != Interpreted {
		t.Errorf("identity verdict = %v reasons = %v, want Interpreted (an unlowerable type is not guardable)", identity.Verdict, identity.Reasons)
	}
}

// TestPassCKeepsCompiledSealed pins that a cleanly compiled unit passes through the
// seal untouched. priceOf is fully typed and stays Compiled after all three passes.
func TestPassCKeepsCompiledSealed(t *testing.T) {
	results := partitionRealC(t, "export function priceOf(rec: { a: number; b: number }): number { return rec.a + rec.b; }\n")

	priceOf, ok := resultNamed(results, "priceOf")
	if !ok {
		t.Fatalf("no unit named priceOf in %v", results)
	}
	if priceOf.Verdict != Compiled {
		t.Errorf("priceOf verdict = %v reasons = %v, want Compiled (a clean unit is sealed as it stands)", priceOf.Verdict, priceOf.Reasons)
	}
}

// TestPassCControlInversionStaysSpeculative pins that a unit Pass B already made
// Speculative for a control inversion stays Speculative through the seal, and is
// not disturbed by Pass C. tick is inverted through an any callback and remains
// Speculative.
func TestPassCControlInversionStaysSpeculative(t *testing.T) {
	src := "function tick(n: number): void {}\n" +
		"export function schedule(cb: any): void { cb(); }\n" +
		"export function main(): void { schedule(tick); }\n"
	results := partitionRealC(t, src)

	tick, ok := resultNamed(results, "tick")
	if !ok {
		t.Fatalf("no unit named tick in %v", results)
	}
	if tick.Verdict != Speculative {
		t.Errorf("tick verdict = %v reasons = %v, want Speculative (a Pass B inversion survives the seal)", tick.Verdict, tick.Reasons)
	}
}
