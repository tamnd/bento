package lower

import (
	"strings"
	"testing"
)

// TestArrayAtOnAnyElemUnwrapsOpt pins that Array.prototype.at on an any[] lowers
// through value.OptValue. On an any-element array the checker collapses the
// declared T | undefined back to plain any, so downstream the result must be a
// value.Value; the runtime AtOpt still returns Opt[value.Value], so the call must
// be unwrapped or the emitted Go passes an Opt where a Value is wanted and does not
// compile (the test/built-ins/Array/prototype/at gobuild fail). A typed-element
// array keeps the real optional and must NOT take the unwrap.
func TestArrayAtOnAnyElemUnwrapsOpt(t *testing.T) {
	const src = `function sink(v: any): void { console.log(v); }
const a: any[] = [1, 2];
sink(a.at(-2));`
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.OptValue(") || !strings.Contains(out, "AtOpt(") {
		t.Fatalf("at on any[] did not unwrap the optional through value.OptValue:\n%s", out)
	}
}

// TestArrayPopFindShiftOnAnyElemUnwrapOpt covers the other Opt-returning methods
// (pop, shift, find, findLast) on an any[]: each must unwrap through value.OptValue
// so its Opt[value.Value] result satisfies the value.Value the collapsed any type
// demands downstream.
func TestArrayPopFindShiftOnAnyElemUnwrapOpt(t *testing.T) {
	const src = `function sink(v: any): void { console.log(v); }
const a: any[] = [1, 2];
sink(a.pop());
sink(a.shift());
sink(a.find(x => true));
sink(a.findLast(x => true));`
	out := renderProgram(t, src)
	for _, m := range []string{"Pop()", "Shift()", "Find(", "FindLast("} {
		if !strings.Contains(out, "value.OptValue("+strings.TrimSuffix(m, "(")) &&
			!strings.Contains(out, "value.OptValue(a."+m) {
			t.Fatalf("method %s on any[] not unwrapped through value.OptValue:\n%s", m, out)
		}
	}
}

// TestArrayAtOnTypedElemKeepsOpt pins that a typed-element array does NOT take the
// any-only unwrap: at on a number[] keeps the Opt result the normal number|undefined
// coercion boxes, so value.OptValue must not appear around AtOpt here.
func TestArrayAtOnTypedElemKeepsOpt(t *testing.T) {
	const src = `const a: number[] = [1, 2];
const x: number | undefined = a.at(-2);
if (x !== undefined) console.log(x);`
	out := renderProgram(t, src)
	if strings.Contains(out, "value.OptValue(a.AtOpt") {
		t.Fatalf("at on number[] wrongly took the any-only OptValue unwrap:\n%s", out)
	}
}

// TestArrayAtOnAnyElemRuns runs the any[] at/find shapes end to end and checks the
// values match Node: at(-2) is the first element, an out-of-range at is undefined,
// and a find with no match is undefined.
func TestArrayAtOnAnyElemRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const a: any[] = [10, 20];
const r1: any = a.at(-2);
const r2: any = a.at(5);
const r3: any = a.find((x: any) => x === 99);
console.log(String(r1));
console.log(String(r2));
console.log(String(r3));`
	if got, want := runProgramGo(t, src), "10\nundefined\nundefined\n"; got != want {
		t.Fatalf("any[] at/find printed %q, want %q", got, want)
	}
}
