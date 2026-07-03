package partition

import "testing"

// TestCostModelPromotesHighConfidence pins that when the guard nearly always hits,
// the guarded compiled path beats interpreting and the unit is promoted. Both the
// declared-but-widened default and a certain hit clear the section 7.5 inequality
// with a single guard.
func TestCostModelPromotesHighConfidence(t *testing.T) {
	m := DefaultCostModel()
	if !m.Promote(0.9, 1) {
		t.Errorf("Promote(0.9, 1) = false, want true (a near-certain guard beats interpreting)")
	}
	if !m.Promote(1.0, 1) {
		t.Errorf("Promote(1.0, 1) = false, want true (a certain guard is pure win)")
	}
}

// TestCostModelDeclinesLowConfidence pins the teeth of the cost model: when the
// guard misses often, the deopt tail plus the interpreted run it falls back to
// costs more than interpreting outright, so the unit is not promoted.
func TestCostModelDeclinesLowConfidence(t *testing.T) {
	m := DefaultCostModel()
	if m.Promote(0.2, 1) {
		t.Errorf("Promote(0.2, 1) = true, want false (a guard that misses 4 in 5 loses to interpreting)")
	}
}

// TestCostModelGuardCap pins that a unit needing more guards than the cap is left
// interpreted even when each guard would hit: a body that needs many independent
// speculations to compile is not really typed and is too fragile to be worth it.
// At the cap it still promotes.
func TestCostModelGuardCap(t *testing.T) {
	m := DefaultCostModel()
	if !m.Promote(0.9, m.MaxGuards) {
		t.Errorf("Promote(0.9, %d) = false, want true (at the cap is still allowed)", m.MaxGuards)
	}
	if m.Promote(0.9, m.MaxGuards+1) {
		t.Errorf("Promote(0.9, %d) = true, want false (past the cap is too fragile)", m.MaxGuards+1)
	}
}

// TestCostModelZeroGuards pins that a unit with nothing to guard is never a
// speculation: there is no assumption to compile under, so it is not promoted.
func TestCostModelZeroGuards(t *testing.T) {
	m := DefaultCostModel()
	if m.Promote(0.9, 0) {
		t.Errorf("Promote(0.9, 0) = true, want false (no guard means no speculation)")
	}
}
