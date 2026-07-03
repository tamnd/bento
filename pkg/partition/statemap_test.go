package partition

import "testing"

// realStateMaps compiles a snippet, runs all three passes, and builds the deopt
// state maps for the sealed result.
func realStateMaps(t *testing.T, src string) []StateMap {
	t.Helper()
	pt := New(loadReal(t, src, false))
	return pt.StateMaps(pt.PassC(pt.PassB(pt.PassA())))
}

// stateMapNamed returns the state map for the unit with the given name.
func stateMapNamed(maps []StateMap, name string) (StateMap, bool) {
	for _, m := range maps {
		if m.Unit.Name == name {
			return m, true
		}
	}
	return StateMap{}, false
}

// slotNamed returns a guard's live slot with the given binding name.
func slotNamed(g Guard, name string) (LiveSlot, bool) {
	for _, s := range g.Live {
		if s.Name == name {
			return s, true
		}
	}
	return LiveSlot{}, false
}

// TestStateMapEntryGuardForSpeculatedUnit pins that a speculated unit gets one
// entry-guard state map: a resume position at the top of the unit, an empty handler
// stack, and its parameter live with a box plan. fee is speculated on its any
// parameter, so its guard carries raw as a slot that is already boxed.
func TestStateMapEntryGuardForSpeculatedUnit(t *testing.T) {
	maps := realStateMaps(t, "export function fee(raw: any): number { return raw.amount * 0.029 + 30; }\n")

	m, ok := stateMapNamed(maps, "fee")
	if !ok {
		t.Fatalf("no state map for fee in %v", maps)
	}
	if len(m.Guards) != 1 {
		t.Fatalf("fee has %d guards, want 1 entry guard", len(m.Guards))
	}
	g := m.Guards[0]
	if g.ResumePos != m.Unit.Root.Pos() {
		t.Errorf("entry guard resume position = %d, want the unit's own position %d", g.ResumePos, m.Unit.Root.Pos())
	}
	if g.HandlerDepth != 0 {
		t.Errorf("entry guard handler depth = %d, want 0 (no try wraps the entry)", g.HandlerDepth)
	}
	raw, ok := slotNamed(g, "raw")
	if !ok {
		t.Fatalf("raw is not a live slot at the entry guard, live = %v", g.Live)
	}
	if raw.Box != BoxAlready {
		t.Errorf("raw box plan = %v, want already boxed (an any parameter is held as a Value)", raw.Box)
	}
}

// TestStateMapBoxPlansByType pins that each live scalar gets the box plan its
// declared type dictates, so the deopt routine knows how to turn every compiled
// slot back into a Value. mix is speculated on its any parameter and carries a
// number, a string, and a boolean parameter, each with its own plan.
func TestStateMapBoxPlansByType(t *testing.T) {
	src := "export function mix(raw: any, n: number, s: string, flag: boolean): number { return raw.x; }\n"
	maps := realStateMaps(t, src)

	m, ok := stateMapNamed(maps, "mix")
	if !ok {
		t.Fatalf("no state map for mix in %v", maps)
	}
	g := m.Guards[0]
	for name, want := range map[string]BoxKind{"n": BoxNumber, "s": BoxString, "flag": BoxBoolean, "raw": BoxAlready} {
		slot, ok := slotNamed(g, name)
		if !ok {
			t.Errorf("%s is not a live slot, live = %v", name, g.Live)
			continue
		}
		if slot.Box != want {
			t.Errorf("%s box plan = %v, want %v", name, slot.Box, want)
		}
	}
}

// TestStateMapObjectParamBoxesByShape pins that an object parameter is boxed by its
// static shape rather than treated as already boxed. audit is speculated on its any
// parameter and its object parameter rec carries the object box plan.
func TestStateMapObjectParamBoxesByShape(t *testing.T) {
	src := "export function audit(raw: any, rec: { total: number }): number { return raw.x + rec.total; }\n"
	maps := realStateMaps(t, src)

	m, ok := stateMapNamed(maps, "audit")
	if !ok {
		t.Fatalf("no state map for audit in %v", maps)
	}
	rec, ok := slotNamed(m.Guards[0], "rec")
	if !ok {
		t.Fatalf("rec is not a live slot, live = %v", m.Guards[0].Live)
	}
	if rec.Box != BoxObject {
		t.Errorf("rec box plan = %v, want object (an object slot boxes by its shape)", rec.Box)
	}
}

// TestStateMapOnlyForSpeculated pins that a state map is emitted only for a
// speculated unit: a cleanly compiled unit needs no deopt path, and an interpreted
// unit never runs compiled to deopt from. clean stays Compiled and bad stays
// Interpreted, so neither has a state map.
func TestStateMapOnlyForSpeculated(t *testing.T) {
	src := "export function clean(n: number): number { return n; }\n" +
		"export function bad(): number { eval(\"1\"); return 0; }\n"
	maps := realStateMaps(t, src)

	if _, ok := stateMapNamed(maps, "clean"); ok {
		t.Errorf("clean has a state map, but a compiled unit never deopts")
	}
	if _, ok := stateMapNamed(maps, "bad"); ok {
		t.Errorf("bad has a state map, but an interpreted unit never runs compiled")
	}
}

// TestStateMapCallbackParamDoesNotLeakInnerParams pins that a callback-typed
// parameter contributes only itself as a live slot, not the parameters of its
// function type. onEach is speculated on its any parameter, and its callback
// parameter cb is one slot with no phantom inner parameter alongside it.
func TestStateMapCallbackParamDoesNotLeakInnerParams(t *testing.T) {
	src := "export function onEach(raw: any, cb: (n: number) => void): void { cb(raw); }\n"
	maps := realStateMaps(t, src)

	m, ok := stateMapNamed(maps, "onEach")
	if !ok {
		t.Fatalf("no state map for onEach in %v", maps)
	}
	if _, ok := slotNamed(m.Guards[0], "n"); ok {
		t.Errorf("the callback's inner parameter n leaked into the live set, live = %v", m.Guards[0].Live)
	}
	if _, ok := slotNamed(m.Guards[0], "cb"); !ok {
		t.Errorf("the callback parameter cb is missing from the live set, live = %v", m.Guards[0].Live)
	}
}
