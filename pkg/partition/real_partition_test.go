package partition

import (
	"testing"

	"github.com/tamnd/bento/pkg/frontend"
)

// realFS is an in-memory FileSystem for compiling a snippet through the real
// checker, the partition package's own copy since frontend's helper is
// unexported.
type realFS struct{ files map[string]string }

func (m realFS) ReadFile(path string) (string, bool) { s, ok := m.files[path]; return s, ok }
func (m realFS) FileExists(path string) bool         { _, ok := m.files[path]; return ok }

func (m realFS) DirectoryExists(path string) bool {
	prefix := path
	if prefix != "" && prefix[len(prefix)-1] != '/' {
		prefix += "/"
	}
	for name := range m.files {
		if len(name) > len(prefix) && name[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

func partitionReal(t *testing.T, src string) []Result {
	t.Helper()
	prog := loadReal(t, src, false)
	return New(prog).PassA()
}

// partitionRealAllowDiag is partitionReal for sources that deliberately contain a
// construct the checker flags, such as a with statement, which is a strict-mode
// error yet still parses to the AST node the partitioner classifies. The verdict,
// not the diagnostic, is the thing under test.
func partitionRealAllowDiag(t *testing.T, src string) []Result {
	t.Helper()
	prog := loadReal(t, src, true)
	return New(prog).PassA()
}

func loadReal(t *testing.T, src string, allowDiag bool) *frontend.Program {
	t.Helper()
	prog, err := frontend.Load(frontend.LoadOptions{
		Dir:   "/",
		Roots: []string{"/m.ts"},
		FS:    realFS{files: map[string]string{"/m.ts": src}},
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !allowDiag {
		for _, d := range prog.Diagnostics() {
			if d.Category == frontend.CategoryError {
				t.Fatalf("unexpected type error: %s", d.Message)
			}
		}
	}
	return prog
}

// loadTwo compiles a multi-file program through the real checker, for the cases
// where a call crosses a module boundary and the callee is declared in a
// different file than the caller. roots names the entry files in load order; the
// checker pulls in any file they import from the same FS.
func loadTwo(t *testing.T, files map[string]string, roots ...string) *frontend.Program {
	t.Helper()
	prog, err := frontend.Load(frontend.LoadOptions{
		Dir:   "/",
		Roots: roots,
		FS:    realFS{files: files},
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, d := range prog.Diagnostics() {
		if d.Category == frontend.CategoryError {
			t.Fatalf("unexpected type error: %s", d.Message)
		}
	}
	return prog
}

func resultNamed(results []Result, name string) (Result, bool) {
	for _, r := range results {
		if r.Unit.Name == name {
			return r, true
		}
	}
	return Result{}, false
}

// TestRealFullyTypedFunctionCompiles drives the partitioner over a real compile:
// a function with concrete parameter and return types and a plain arithmetic
// body classifies as Compiled.
func TestRealFullyTypedFunctionCompiles(t *testing.T) {
	results := partitionReal(t, "export function taxed(cents: number, rate: number): number { return cents * rate; }\n")
	r, ok := resultNamed(results, "taxed")
	if !ok {
		t.Fatalf("no unit named taxed in %v", results)
	}
	if r.Verdict != Compiled {
		t.Fatalf("taxed verdict = %v, reasons = %v, want Compiled", r.Verdict, r.Reasons)
	}
}

// TestRealEvalIsInterpreted proves a hard blocker survives a real compile: a
// function that calls eval cannot be compiled and is not a speculation
// candidate.
func TestRealEvalIsInterpreted(t *testing.T) {
	results := partitionReal(t, "export function runs(): number { return eval(\"1\"); }\n")
	r, ok := resultNamed(results, "runs")
	if !ok {
		t.Fatalf("no unit named runs in %v", results)
	}
	if r.Verdict != Interpreted {
		t.Fatalf("runs verdict = %v, want Interpreted", r.Verdict)
	}
	if r.SpeculationCandidate() {
		t.Error("eval is a hard blocker; the unit must not be a speculation candidate")
	}
}

// TestRealObjectAndUnionParamsAreLowerable proves the lowerable predicate recurses
// through object properties and union members over a real compile: a function
// over a fixed-shape object and a string-or-number union is Compiled.
func TestRealObjectAndUnionParamsAreLowerable(t *testing.T) {
	results := partitionReal(t, "export function plot(p: { x: number; y: number }, label: string | number): number { return p.x; }\n")
	r, ok := resultNamed(results, "plot")
	if !ok {
		t.Fatalf("no unit named plot in %v", results)
	}
	if r.Verdict != Compiled {
		t.Errorf("plot verdict = %v, reasons = %v, want Compiled", r.Verdict, r.Reasons)
	}
}

// TestRealWithIsHardInterpreted pins section 6.1 against the checker: a with
// statement is a hard blocker, so the unit is Interpreted and not a speculation
// candidate. with is a strict-mode error the checker also flags, but it still
// parses to the node the partitioner reads.
func TestRealWithIsHardInterpreted(t *testing.T) {
	results := partitionRealAllowDiag(t, "function loose(o: object): number { with (o) { return 1; } }\n")
	r, ok := resultNamed(results, "loose")
	if !ok {
		t.Fatalf("no unit named loose in %v", results)
	}
	if r.Verdict != Interpreted {
		t.Fatalf("loose verdict = %v, want Interpreted", r.Verdict)
	}
	if r.SpeculationCandidate() {
		t.Error("with is a hard blocker; the unit must not be a speculation candidate")
	}
	if len(r.Reasons) != 1 || r.Reasons[0].Kind != ReasonWith {
		t.Errorf("reasons = %v, want a single ReasonWith", r.Reasons)
	}
	if r.Reasons[0].Severity() != Hard {
		t.Error("ReasonWith severity is not Hard")
	}
}

// TestRealNewFunctionIsHardInterpreted pins new Function(...) as a hard blocker
// resolved by the callee's bound global name.
func TestRealNewFunctionIsHardInterpreted(t *testing.T) {
	results := partitionReal(t, "export function builds(): void { new Function(\"return 1\"); }\n")
	r, ok := resultNamed(results, "builds")
	if !ok {
		t.Fatalf("no unit named builds in %v", results)
	}
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

// TestRealAnyParamIsSoftSpeculationCandidate pins section 6.2 against the checker:
// a declared any parameter is a soft blocker, so the unit is Interpreted for now
// but is a Pass C speculation candidate.
func TestRealAnyParamIsSoftSpeculationCandidate(t *testing.T) {
	results := partitionReal(t, "export function fee(raw: any): number { return 1; }\n")
	r, ok := resultNamed(results, "fee")
	if !ok {
		t.Fatalf("no unit named fee in %v", results)
	}
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

// TestRealTypeParameterIsUnlowerable pins that a bare type parameter is not
// lowerable yet, so a generic signature is a soft blocker until monomorphization
// lands.
func TestRealTypeParameterIsUnlowerable(t *testing.T) {
	results := partitionReal(t, "export function identity<T>(x: T): T { return x; }\n")
	r, ok := resultNamed(results, "identity")
	if !ok {
		t.Fatalf("no unit named identity in %v", results)
	}
	if !r.SpeculationCandidate() {
		t.Fatalf("identity is not a speculation candidate, verdict = %v reasons = %v", r.Verdict, r.Reasons)
	}
	if len(r.Reasons) != 1 || r.Reasons[0].Kind != ReasonUnlowerableType {
		t.Errorf("reasons = %v, want a single ReasonUnlowerableType", r.Reasons)
	}
}

// TestRealNestedFunctionDoesNotContaminateOuter pins the unit boundary: a with
// statement inside a nested function blocks the nested unit only, while the outer
// unit stays Compiled. Both, plus the module body, are enumerated as units.
func TestRealNestedFunctionDoesNotContaminateOuter(t *testing.T) {
	results := partitionRealAllowDiag(t, "export function outer(): number { function inner(): number { with (globalThis) { return 1; } } return 2; }\n")
	out, ok := resultNamed(results, "outer")
	if !ok {
		t.Fatalf("no unit named outer in %v", results)
	}
	if out.Verdict != Compiled {
		t.Errorf("outer verdict = %v reasons = %v, want Compiled (nested with must not leak)", out.Verdict, out.Reasons)
	}
	in, ok := resultNamed(results, "inner")
	if !ok {
		t.Fatalf("no unit named inner in %v", results)
	}
	if in.Verdict != Interpreted || len(in.Reasons) == 0 || in.Reasons[0].Kind != ReasonWith {
		t.Errorf("inner verdict = %v reasons = %v, want Interpreted with ReasonWith", in.Verdict, in.Reasons)
	}
}

// TestRealModuleRootIsAUnit pins that a source-file body is enumerated as a module
// unit, named by its path, alongside the functions it contains.
func TestRealModuleRootIsAUnit(t *testing.T) {
	results := partitionReal(t, "export function helper(): number { return 1; }\n")
	mod, ok := resultNamed(results, "/m.ts")
	if !ok {
		t.Fatalf("no module unit named /m.ts in %v", results)
	}
	if mod.Unit.Kind != UnitModule {
		t.Errorf("module unit kind = %v, want UnitModule", mod.Unit.Kind)
	}
	if mod.Verdict != Compiled {
		t.Errorf("module body verdict = %v, want Compiled", mod.Verdict)
	}
}
