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

func TestCompoundValueOnDynamicRuns(t *testing.T) {
	// The target local is stored as a box, so the read-modify-write runs in a closure
	// that returns the box; a dynamic-context use keeps it.
	const src = `let x: any = 1; let r: any = (x += 1); console.log(r); console.log(x);`
	if got, want := runProgramGoTolerant(t, src), "2\n2\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestCompoundValueOnDynamicCoercesToStatic(t *testing.T) {
	// A static-primitive context coerces the returned box down through ToNumber.
	const src = `let x: any = 1; let n: number = (x += 4); console.log(n);`
	out := renderProgramTolerant(t, src)
	if !strings.Contains(out, "value.ToNumber(") {
		t.Fatalf("want a ToNumber coercion of the closure result, got:\n%s", out)
	}
	if got, want := runProgramGoTolerant(t, src), "5\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// Item 5: a logical assignment on a member target.

func TestMemberLogicalAssignRuns(t *testing.T) {
	cases := map[string]string{
		`let o: any = {}; o.k ??= 7; console.log(o.k); o.k ??= 9; console.log(o.k);`: "7\n7\n",
		`let o: any = {a: 0}; o.a ||= 5; console.log(o.a);`:                          "5\n",
		`let o: any = {a: 3}; o.a &&= 8; console.log(o.a);`:                          "8\n",
	}
	for src, want := range cases {
		if got := runProgramGoTolerant(t, src); got != want {
			t.Errorf("%s: got %q want %q", src, got, want)
		}
	}
}

func TestMemberLogicalAssignShortCircuits(t *testing.T) {
	// ??= must not store when the property is already present, so the side-effecting
	// right-hand side never runs.
	const src = `let o: any = {k: 1}; let ran = 0; function v(): number { ran = 1; return 2; } o.k ??= v(); console.log(o.k, ran);`
	if got, want := runProgramGoTolerant(t, src), "1 0\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestMemberLogicalStaticHandsBack(t *testing.T) {
	const src = `interface P { a: number } function f(p: P): void { p.a ||= 5; }`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "object descriptor model") {
		t.Fatalf("reason = %q, want an object-descriptor-model handback", reason)
	}
}

func TestMemberLogicalSideEffectHandsBack(t *testing.T) {
	const src = `let o: any = {a: 0}; function g(): any { return o; } g().a ||= 5;`
	reason := renderProgramTolerantHandBack(t, src)
	if !strings.Contains(reason, "side-effecting receiver") {
		t.Fatalf("reason = %q, want a side-effecting-receiver handback", reason)
	}
}
