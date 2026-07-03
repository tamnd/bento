package partition

import "testing"

// unitIndex returns the index of the unit named name in a partitioner's unit
// enumeration, which is the same index the call graph uses.
func unitIndex(t *testing.T, pt *Partitioner, name string) int {
	t.Helper()
	for i, u := range pt.Units() {
		if u.Name == name {
			return i
		}
	}
	t.Fatalf("no unit named %q", name)
	return -1
}

// calls reports whether the call graph records an edge from the unit named from
// to the unit named to.
func calls(t *testing.T, pt *Partitioner, cg CallGraph, from, to string) bool {
	t.Helper()
	fi, ti := unitIndex(t, pt, from), unitIndex(t, pt, to)
	for _, c := range cg.Callees[fi] {
		if c == ti {
			return true
		}
	}
	return false
}

// TestCallGraphInProgramEdge pins that a call to a function defined in the same
// program is recorded as an in-program edge, not an external one.
func TestCallGraphInProgramEdge(t *testing.T) {
	src := "export function callee(): number { return 1; }\n" +
		"export function caller(): number { return callee(); }\n"
	pt := New(loadReal(t, src, false))
	cg := pt.CallGraph()
	if !calls(t, pt, cg, "caller", "callee") {
		t.Errorf("caller->callee edge missing, callees = %v", cg.Callees)
	}
	if cg.External[unitIndex(t, pt, "caller")] {
		t.Error("caller marked external, but callee is in-program")
	}
}

// TestCallGraphExternalCallee pins that a call to a callee outside this program,
// a node: builtin like console.log reached through a property access, is an
// external edge and adds no in-program callee.
func TestCallGraphExternalCallee(t *testing.T) {
	src := "export function logs(x: number): void { console.log(x); }\n"
	pt := New(loadReal(t, src, false))
	cg := pt.CallGraph()
	i := unitIndex(t, pt, "logs")
	if !cg.External[i] {
		t.Error("logs not marked external, but console.log is outside this program")
	}
	if len(cg.Callees[i]) != 0 {
		t.Errorf("logs has in-program callees %v, want none", cg.Callees[i])
	}
}

// TestCallGraphDirectRecursion pins that a self-call records the unit's own
// index, so direct recursion is visible in the graph rather than dropped.
func TestCallGraphDirectRecursion(t *testing.T) {
	src := "export function fib(n: number): number { return n < 2 ? n : fib(n - 1) + fib(n - 2); }\n"
	pt := New(loadReal(t, src, false))
	cg := pt.CallGraph()
	if !calls(t, pt, cg, "fib", "fib") {
		t.Errorf("fib self-edge missing, callees = %v", cg.Callees)
	}
	i := unitIndex(t, pt, "fib")
	if got := len(cg.Callees[i]); got != 1 {
		t.Errorf("fib records %d callees, want 1 (deduplicated self-edge)", got)
	}
}

// TestCallGraphStopsAtNestedFunction pins the unit boundary: a call made inside
// a nested function belongs to the nested unit, not the outer one, so the outer
// unit does not record it.
func TestCallGraphStopsAtNestedFunction(t *testing.T) {
	src := "export function target(): number { return 1; }\n" +
		"export function outer(): number {\n" +
		"  function inner(): number { return target(); }\n" +
		"  return inner();\n" +
		"}\n"
	pt := New(loadReal(t, src, false))
	cg := pt.CallGraph()
	if calls(t, pt, cg, "outer", "target") {
		t.Error("outer->target edge present, but that call is inside inner")
	}
	if !calls(t, pt, cg, "inner", "target") {
		t.Error("inner->target edge missing")
	}
	if !calls(t, pt, cg, "outer", "inner") {
		t.Error("outer->inner edge missing")
	}
}

// TestCallGraphImportedCalleeFollowsAlias pins that a call to a function brought
// in through an import binding resolves to the unit that declares it, so the
// alias does not hide the in-program edge. Both files are units of the same
// program, so the edge is in-program, not external.
func TestCallGraphImportedCalleeFollowsAlias(t *testing.T) {
	prog := loadTwo(t,
		map[string]string{
			"/lib.ts":  "export function helper(): number { return 1; }\n",
			"/main.ts": "import { helper } from \"./lib\";\nexport function useHelper(): number { return helper(); }\n",
		},
		"/main.ts", "/lib.ts",
	)
	pt := New(prog)
	cg := pt.CallGraph()
	if !calls(t, pt, cg, "useHelper", "helper") {
		t.Errorf("useHelper->helper edge missing across the import, callees = %v", cg.Callees)
	}
}
