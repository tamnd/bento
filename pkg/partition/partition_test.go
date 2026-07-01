package partition

import (
	"testing"

	"github.com/tamnd/bento/pkg/frontend"
	"github.com/tamnd/bento/pkg/frontend/adapter"
)

// wrap builds a frontend.Program over a fake program rooted at the given nodes.
func wrap(roots ...*adapter.FakeNode) *frontend.Program {
	f := adapter.NewFake()
	a, handle := f.Program(roots...)
	return frontend.Wrap(a, handle)
}

// ident builds an identifier node bound to a symbol of the given name, so Pass A
// can resolve a callee the way the checker would.
func ident(name string) *adapter.FakeNode {
	return &adapter.FakeNode{
		Kind:   adapter.NodeIdentifier,
		Symbol: &adapter.FakeSymbol{NameText: name},
	}
}

// resultByName finds the Pass A result for a unit with the given name.
func resultByName(t *testing.T, results []Result, name string) Result {
	t.Helper()
	for _, r := range results {
		if r.Unit.Name == name {
			return r
		}
	}
	t.Fatalf("no unit named %q in %d results", name, len(results))
	return Result{}
}

// TestFullyTypedFunctionCompiles pins the base rule of section 5.1: a fully
// typed arithmetic function over lowerable types, with no dynamic feature, is
// Compiled with no reasons.
func TestFullyTypedFunctionCompiles(t *testing.T) {
	f := adapter.NewFake()
	num := f.Prim(adapter.TypeNumber)
	taxed := f.Func("taxed", f.Sig([]adapter.ParamInfo{f.Param("cents", num), f.Param("rate", num)}, num),
		f.Node(adapter.NodeReturnStatement, nil, f.Node(adapter.NodeBinaryExpression, num)),
	)

	prog := wrap(taxed)
	results := New(prog).PassA()

	r := resultByName(t, results, "taxed")
	if r.Verdict != Compiled {
		t.Fatalf("taxed verdict = %v, reasons = %v, want Compiled", r.Verdict, r.Reasons)
	}
	if len(r.Reasons) != 0 {
		t.Errorf("taxed reasons = %v, want none", r.Reasons)
	}
}

// TestObjectAndUnionParamsAreLowerable proves the lowerable predicate recurses
// through object properties and union members, so a function over a fixed-shape
// object and a union of primitives compiles.
func TestObjectAndUnionParamsAreLowerable(t *testing.T) {
	f := adapter.NewFake()
	num := f.Prim(adapter.TypeNumber)
	str := f.Prim(adapter.TypeString)
	point := f.Object(f.Prop("x", num), f.Prop("y", num))
	either := f.Union(str, num)

	fn := f.Func("plot", f.Sig([]adapter.ParamInfo{f.Param("p", point), f.Param("label", either)}, num))
	results := New(wrap(fn)).PassA()

	r := resultByName(t, results, "plot")
	if r.Verdict != Compiled {
		t.Errorf("plot verdict = %v, reasons = %v, want Compiled", r.Verdict, r.Reasons)
	}
}

// TestWithStatementIsHardInterpreted pins section 6.1: a with statement is a
// hard blocker, so the unit is Interpreted and is not a speculation candidate.
func TestWithStatementIsHardInterpreted(t *testing.T) {
	f := adapter.NewFake()
	num := f.Prim(adapter.TypeNumber)
	fn := f.Func("loose", f.Sig(nil, num),
		f.Node(adapter.NodeWithStatement, nil),
	)
	results := New(wrap(fn)).PassA()

	r := resultByName(t, results, "loose")
	if r.Verdict != Interpreted {
		t.Fatalf("loose verdict = %v, want Interpreted", r.Verdict)
	}
	if r.SpeculationCandidate() {
		t.Error("with unit reported as a speculation candidate, want not")
	}
	if len(r.Reasons) != 1 || r.Reasons[0].Kind != ReasonWith {
		t.Errorf("reasons = %v, want a single ReasonWith", r.Reasons)
	}
	if r.Reasons[0].Severity() != Hard {
		t.Error("ReasonWith severity is not Hard")
	}
}

// TestEvalIsHardInterpreted pins that a call to eval is a hard blocker resolved
// by the callee's bound name.
func TestEvalIsHardInterpreted(t *testing.T) {
	f := adapter.NewFake()
	num := f.Prim(adapter.TypeNumber)
	call := f.Node(adapter.NodeCallExpression, num, ident("eval"))
	fn := f.Func("runs", f.Sig(nil, num), call)
	results := New(wrap(fn)).PassA()

	r := resultByName(t, results, "runs")
	if r.Verdict != Interpreted || r.SpeculationCandidate() {
		t.Fatalf("runs verdict = %v candidate = %v, want Interpreted non-candidate", r.Verdict, r.SpeculationCandidate())
	}
	if len(r.Reasons) != 1 || r.Reasons[0].Kind != ReasonEval {
		t.Errorf("reasons = %v, want a single ReasonEval", r.Reasons)
	}
}

// TestNewFunctionIsHardInterpreted pins new Function(...) as a hard blocker.
func TestNewFunctionIsHardInterpreted(t *testing.T) {
	f := adapter.NewFake()
	any := f.Any()
	neww := f.Node(adapter.NodeNewExpression, any, ident("Function"))
	fn := f.Func("builds", f.Sig(nil, f.Prim(adapter.TypeVoid)), neww)
	results := New(wrap(fn)).PassA()

	r := resultByName(t, results, "builds")
	if r.SpeculationCandidate() {
		t.Error("new Function unit reported as a speculation candidate, want not")
	}
	found := false
	for _, reason := range r.Reasons {
		if reason.Kind == ReasonNewFunction {
			found = true
		}
	}
	if !found {
		t.Errorf("reasons = %v, want ReasonNewFunction among them", r.Reasons)
	}
}

// TestAnyParamIsSoftSpeculationCandidate pins section 6.2: an any parameter used
// without narrowing is a soft blocker, so the unit is Interpreted for now but is
// a Pass C speculation candidate.
func TestAnyParamIsSoftSpeculationCandidate(t *testing.T) {
	f := adapter.NewFake()
	num := f.Prim(adapter.TypeNumber)
	fn := f.Func("fee", f.Sig([]adapter.ParamInfo{f.Param("raw", f.Any())}, num))
	results := New(wrap(fn)).PassA()

	r := resultByName(t, results, "fee")
	if r.Verdict != Interpreted {
		t.Fatalf("fee verdict = %v, want Interpreted (pre-speculation)", r.Verdict)
	}
	if !r.SpeculationCandidate() {
		t.Fatalf("fee is not a speculation candidate, reasons = %v", r.Reasons)
	}
	if len(r.Reasons) != 1 || r.Reasons[0].Kind != ReasonUntypedValue {
		t.Errorf("reasons = %v, want a single ReasonUntypedValue", r.Reasons)
	}
}

// TestTypeParameterIsUnlowerable pins that a bare type parameter is not lowerable
// yet, so a generic-over-T signature is a soft blocker until monomorphization
// lands.
func TestTypeParameterIsUnlowerable(t *testing.T) {
	f := adapter.NewFake()
	tp := f.Prim(adapter.TypeTypeParameter)
	fn := f.Func("identity", f.Sig([]adapter.ParamInfo{f.Param("x", tp)}, tp))
	results := New(wrap(fn)).PassA()

	r := resultByName(t, results, "identity")
	if !r.SpeculationCandidate() {
		t.Fatalf("identity is not a speculation candidate, verdict = %v reasons = %v", r.Verdict, r.Reasons)
	}
	if len(r.Reasons) != 1 || r.Reasons[0].Kind != ReasonUnlowerableType {
		t.Errorf("reasons = %v, want a single ReasonUnlowerableType", r.Reasons)
	}
}

// TestNestedFunctionDoesNotContaminateOuter pins the unit boundary: a with
// statement inside a nested function blocks the nested unit only. The outer unit
// stays Compiled, and both are enumerated as separate units.
func TestNestedFunctionDoesNotContaminateOuter(t *testing.T) {
	f := adapter.NewFake()
	num := f.Prim(adapter.TypeNumber)
	inner := f.Func("inner", f.Sig(nil, num), f.Node(adapter.NodeWithStatement, nil))
	outer := f.Func("outer", f.Sig(nil, num), inner)

	results := New(wrap(outer)).PassA()
	if len(results) != 2 {
		t.Fatalf("got %d units, want 2 (outer and inner)", len(results))
	}

	out := resultByName(t, results, "outer")
	if out.Verdict != Compiled {
		t.Errorf("outer verdict = %v reasons = %v, want Compiled (nested with must not leak)", out.Verdict, out.Reasons)
	}
	in := resultByName(t, results, "inner")
	if in.Verdict != Interpreted || in.Reasons[0].Kind != ReasonWith {
		t.Errorf("inner verdict = %v reasons = %v, want Interpreted with ReasonWith", in.Verdict, in.Reasons)
	}
}

// TestModuleRootIsAUnit pins that a source-file body is enumerated as a module
// unit when the root is not itself a function.
func TestModuleRootIsAUnit(t *testing.T) {
	f := adapter.NewFake()
	num := f.Prim(adapter.TypeNumber)
	fn := f.Func("helper", f.Sig(nil, num))
	moduleRoot := &adapter.FakeNode{Kind: adapter.NodeSourceFile, FilePath: "app.ts", Children: []*adapter.FakeNode{fn}}

	results := New(wrap(moduleRoot)).PassA()
	if len(results) != 2 {
		t.Fatalf("got %d units, want 2 (module body and helper)", len(results))
	}
	mod := resultByName(t, results, "app.ts")
	if mod.Unit.Kind != UnitModule {
		t.Errorf("module unit kind = %v, want UnitModule", mod.Unit.Kind)
	}
	if mod.Verdict != Compiled {
		t.Errorf("empty module body verdict = %v, want Compiled", mod.Verdict)
	}
}
