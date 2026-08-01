package lower

import (
	"strings"
	"testing"
)

// TestPromiseAllOverATupleLowers pins the capability. `await Promise.all([a(), b()])` has
// no contextual type to widen its literal, so the checker keeps one element type per
// position and the call produces a Promise of a tuple. value.All takes one element type
// for a whole slice, so that shape handed the build back; it routes to value.AllTuple now,
// which carries the positions in the caller's own closure.
func TestPromiseAllOverATupleLowers(t *testing.T) {
	cases := map[string]string{
		"mixed positions": "async function n(): Promise<number> { return 1; }\n" +
			"async function s(): Promise<string> { return \"a\"; }\n" +
			"async function main() { const [a, b] = await Promise.all([n(), s()]); console.log(a, b); }\nmain();",
		"uniform positions": "async function n(x: number): Promise<number> { return x; }\n" +
			"async function main() { const [a, b] = await Promise.all([n(1), n(2)]); console.log(a, b); }\nmain();",
		"single position": "async function n(): Promise<number> { return 1; }\n" +
			"async function main() { const [a] = await Promise.all([n()]); console.log(a); }\nmain();",
		"no positions": "async function main() { const e = await Promise.all([]); console.log(JSON.stringify(e)); }\nmain();",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			out := renderProgram(t, src)
			if !strings.Contains(out, "value.AllTuple(") {
				t.Fatalf("Promise.all over a tuple did not route to AllTuple:\n%s", out)
			}
		})
	}
}

// TestPromiseAllOverAnArrayStillLowers pins that the array path is untouched. A source
// that widens the literal, by annotating either the argument or the result, still produces
// a Promise of an array and still routes to value.All, which needs no closure because
// every input shares one element type.
func TestPromiseAllOverAnArrayStillLowers(t *testing.T) {
	for name, src := range map[string]string{
		"annotated argument": "async function n(x: number): Promise<number> { return x; }\n" +
			"async function main() { const ps: Promise<number>[] = [n(1), n(2)]; console.log((await Promise.all(ps)).join(\",\")); }\nmain();",
		"annotated result": "async function n(x: number): Promise<number> { return x; }\n" +
			"async function main() { const r: number[] = await Promise.all([n(1), n(2)]); console.log(r.join(\",\")); }\nmain();",
	} {
		t.Run(name, func(t *testing.T) {
			out := renderProgram(t, src)
			if !strings.Contains(out, "value.All[") {
				t.Fatalf("Promise.all over an array did not route to All:\n%s", out)
			}
			if strings.Contains(out, "value.AllTuple(") {
				t.Fatalf("Promise.all over an array took the tuple path:\n%s", out)
			}
		})
	}
}

// TestPromiseAllOverANonPromiseTupleHandsBack pins the soundness gate. Every position of
// the argument has to be a runtime promise of the matching fulfilled position, since the
// emit reads each one's Fulfilled to build the result tuple. A plain value among the
// promises, which the engine accepts and resolves, has no Fulfilled to read.
func TestPromiseAllOverANonPromiseTupleHandsBack(t *testing.T) {
	const src = `async function n(): Promise<number> { return 1; }
async function main() { const [a, b] = await Promise.all([n(), 2]); console.log(a, b); }
main();`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "Promise.all") {
		t.Fatalf("Promise.all over a mixed tuple did not hand back for that reason: %q", reason)
	}
}

// TestTupleBoxesAsAnArray pins the box a tuple crosses into a dynamic slot with. A tuple
// is an array in JavaScript, but its Go shape is a positional struct, so the fixed-object
// path used to claim it and box it into an object under its field names: JSON.stringify
// of a [number, string] read {"E0":1,"E1":"a"} where the engine reads [1,"a"]. That was a
// wrong answer rather than a hand-back, and the tuple box is what closes it.
func TestTupleBoxesAsAnArray(t *testing.T) {
	for name, src := range map[string]string{
		"console.log":    "const t: [number, string] = [1, \"a\"];\nconsole.log(t);",
		"JSON.stringify": "const t: [number, string] = [1, \"a\"];\nconsole.log(JSON.stringify(t));",
		"String":         "const t: [number, string] = [1, \"a\"];\nconsole.log(String(t));",
		"uniform":        "const t: [number, number] = [1, 2];\nconsole.log(JSON.stringify(t));",
	} {
		t.Run(name, func(t *testing.T) {
			out := renderProgram(t, src)
			if !strings.Contains(out, "value.NewArrayValue(") {
				t.Fatalf("a tuple did not box into an array value:\n%s", out)
			}
			if strings.Contains(out, "value.ObjectFromStruct(") {
				t.Fatalf("a tuple boxed into an object:\n%s", out)
			}
		})
	}
}

// TestTupleJoinLowers pins the array method a tuple most often reaches for. It is what a
// program does with the result of a Promise.all, and until now an unhosted array method on
// a tuple fell past the tuple path into a dispatch that emitted a call to a Go method the
// positional struct does not have, so the assembled program failed to compile rather than
// hand back at the boundary.
func TestTupleJoinLowers(t *testing.T) {
	for name, src := range map[string]string{
		"default separator":  "const t: [number, number] = [1, 2];\nconsole.log(t.join());",
		"explicit separator": "const t: [number, number] = [1, 2];\nconsole.log(t.join(\"|\"));",
	} {
		t.Run(name, func(t *testing.T) {
			out := renderProgram(t, src)
			if !strings.Contains(out, ".Join(") {
				t.Fatalf("a tuple join did not lower to the array Join:\n%s", out)
			}
		})
	}
}

// TestTupleLengthFolds pins the one property a tuple carries. The arity is fixed by the
// type, so the read answers a constant with no receiver read at all. Until now it fell
// into the interned-shape path, which read it as a struct field named Length that the
// positional struct does not have, so the assembled program failed to compile.
//
// The second case is why the fold has to be counted as a read of its receiver: nothing
// else in that program reads t, and a Go local declared and never used is an error.
func TestTupleLengthFolds(t *testing.T) {
	for name, src := range map[string]string{
		"beside another read": "const t: [number, string] = [1, \"a\"];\nconsole.log(t[0], t.length);",
		"the only read":       "const t: [number, string] = [1, \"a\"];\nconsole.log(t.length);",
		"three positions":     "const t: [number, string, boolean] = [1, \"a\", true];\nconsole.log(t.length);",
	} {
		t.Run(name, func(t *testing.T) {
			out := renderProgram(t, src)
			if strings.Contains(out, ".Length") || strings.Contains(out, ".Len()") {
				t.Fatalf("a tuple length did not fold to its arity:\n%s", out)
			}
		})
	}
}

// TestUnhostedTuplePropertyHandsBack pins the boundary beside the length fold. Every name
// on a tuple other than length belongs to Array.prototype or Object.prototype, and the
// positional struct carries neither, so the read reports the absence at the boundary
// rather than emit a selector the Go compiler rejects.
func TestUnhostedTuplePropertyHandsBack(t *testing.T) {
	const src = `const t: [number, number] = [1, 2];
const f = t.join;
console.log(String(f));`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "read off a tuple receiver") {
		t.Fatalf("an unhosted tuple property did not hand back for that reason: %q", reason)
	}
}

// TestUnhostedTupleMethodHandsBack pins the boundary the join fix put back where it
// belongs. A tuple's whole method surface is Array.prototype and Object.prototype, and its
// Go representation carries neither, so a method the tuple path does not host reports the
// absence here rather than let the Go compiler report it.
func TestUnhostedTupleMethodHandsBack(t *testing.T) {
	for _, call := range []string{"slice()", "concat()", "indexOf(1)", "reverse()"} {
		t.Run(call, func(t *testing.T) {
			src := "const t: [number, number] = [1, 2];\nconsole.log(String(t." + call + "));"
			reason := renderProgramHandBack(t, src)
			if !strings.Contains(reason, "borrowed on a tuple receiver") {
				t.Fatalf("%s on a tuple did not hand back for that reason: %q", call, reason)
			}
		})
	}
}
