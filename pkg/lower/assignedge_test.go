package lower

import (
	"strings"
	"testing"
)

// Item 1: a computed member assignment used as a value.

func TestElementAssignValueLowers(t *testing.T) {
	const src = `let o: any = {}; let r = (o["k"] = 5); console.log(r);`
	out := renderProgramTolerant(t, src)
	if !strings.Contains(out, ".SetKey(") {
		t.Fatalf("want a SetKey store, got:\n%s", out)
	}
}

func TestElementAssignValueRuns(t *testing.T) {
	const src = `let o: any = {}; let r = (o["k"] = 5); console.log(r); console.log(o["k"]);`
	if got, want := runProgramGoTolerant(t, src), "5\n5\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestElementAssignValueDynamicRuns(t *testing.T) {
	// The whole expression is dynamic, so the store's box is the result directly.
	const src = `let o: any = {}; let r: any = (o["k"] = "v"); console.log(r);`
	if got, want := runProgramGoTolerant(t, src), "v\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// Item 2: a compound assignment to a computed member target.

func TestElementCompoundLowers(t *testing.T) {
	const src = `let o: any = {a: 2}; o["a"] += 3; console.log(o["a"]);`
	out := renderProgramTolerant(t, src)
	if !strings.Contains(out, "value.Add(") || !strings.Contains(out, ".Get(") {
		t.Fatalf("want a load through Get and a value.Add, got:\n%s", out)
	}
}

func TestElementCompoundRuns(t *testing.T) {
	cases := map[string]string{
		`let o: any = {a: 2}; o["a"] += 3; console.log(o["a"]);`:     "5\n",
		`let a: any = [1, 2, 3]; a[1] *= 10; console.log(a[1]);`:     "20\n",
		`let o: any = {s: "a"}; o["s"] += "b"; console.log(o["s"]);`: "ab\n",
		`let o: any = {a: 12}; o["a"] %= 5; console.log(o["a"]);`:    "2\n",
		`let o: any = {a: 6}; o["a"] &= 3; console.log(o["a"]);`:     "2\n",
		`let o: any = {a: 1}; o["a"] <<= 3; console.log(o["a"]);`:    "8\n",
	}
	for src, want := range cases {
		if got := runProgramGoTolerant(t, src); got != want {
			t.Errorf("%s: got %q want %q", src, got, want)
		}
	}
}

func TestElementCompoundSideEffectHandsBack(t *testing.T) {
	const src = `let o: any = {a: 1}; function g(): any { return o; } g()["a"] += 1;`
	reason := renderProgramTolerantHandBack(t, src)
	if !strings.Contains(reason, "side-effecting receiver or key") {
		t.Fatalf("reason = %q, want a side-effecting-receiver handback", reason)
	}
}

// Item 3: an assignment value flowing through a larger expression already lowers.

func TestChainedAssignValueRuns(t *testing.T) {
	const src = `let a = 0, b = 0; a = (b = 5); console.log(a, b);`
	if got, want := runProgramGoTolerant(t, src), "5 5\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestAssignValueInCallArgRuns(t *testing.T) {
	const src = `let a = 0; function f(x: number): number { return x; } console.log(f(a = 1)); console.log(a);`
	if got, want := runProgramGoTolerant(t, src), "1\n1\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// Item 4: a compound assignment whose result is used.

func TestCompoundValueRuns(t *testing.T) {
	const src = `let x = 4; let r = (x += 1); console.log(r, x);`
	if got, want := runProgramGoTolerant(t, src), "5 5\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestCompoundValueOnDynamicHandsBack(t *testing.T) {
	const src = `let x: any = 1; let r = (x += 1); console.log(r);`
	reason := renderProgramTolerantHandBack(t, src)
	if !strings.Contains(reason, "dynamic or narrowed-storage local") {
		t.Fatalf("reason = %q, want a dynamic-local handback", reason)
	}
}
