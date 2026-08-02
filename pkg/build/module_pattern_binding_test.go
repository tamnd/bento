package build

import "testing"

// TestModuleDestructuringBindingAFunctionReads covers a module-level destructuring
// binding whose names a top-level function reads:
//
//	const arr: number[] = [7, 8]
//	const [p, q] = arr
//	function pq(): number { return p + q }
//
// `const [p, q] = arr` declares p and q at module scope exactly as `const p = arr[0]`
// would, so a function that names either one reads module state and the binding has to
// reach package scope. The hoist never saw it: it read a declaration's name off its first
// child, and a pattern's first child is the pattern, whose text is `[p, q]`. No name
// matched, the statement stayed a local of main, and the emitted Go said `undefined: p`.
// That is a program that does not build, and it reproduces with a fully static number[],
// so it never had anything to do with what the pattern binds.
//
// The pattern shapes are here: array and object, a rest tail on each, a rename, a
// default, and a nesting of both kinds. So are the sources: an array literal, a typed
// array binding, a spread of a Map's keys, and a call. So are the readers that make a
// binding hoist: a function declaration, a nested one, an arrow, a function expression, a
// method, a static method, a static field initializer, a class field, and a closure built
// in a loop. A let binding covers the write side, both from main and from inside a
// function.
//
// Held against what Node v24.18.0 prints, one program.
func TestModuleDestructuringBindingAFunctionReads(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"type Row = { id: number; tag: string };\n"+
			"const arr: number[] = [7, 8, 9];\n"+
			"const [p, q] = arr;\n"+
			"const [head, ...tail] = arr;\n"+
			"const obj: Row = { id: 1, tag: 'x' };\n"+
			"const { id, tag } = obj;\n"+
			"const { id: renamed, tag: t2 } = obj;\n"+
			"const bag: { a: number; b: number; c: number } = { a: 1, b: 2, c: 3 };\n"+
			"const { a, ...rest } = bag;\n"+
			"const short: number[] = [4];\n"+
			"const [only, dflt = 5] = short;\n"+
			"const nested = { in: { deep: 2 }, xs: [3, 4] as number[] };\n"+
			"const { in: { deep }, xs: [x0, x1] } = nested;\n"+
			"const m = new Map<string, number>([['k', 1], ['j', 2]]);\n"+
			"const [k0, k1] = [...m.keys()];\n"+
			"function pair(): [number, string] { return [6, 'z']; }\n"+
			"const [pn, ps] = pair();\n"+
			"let [mut0, mut1] = arr;\n"+
			"const out: string[] = [];\n"+
			"function pq(): number { return p + q; }\n"+
			"function outer(): string { function inner(): number { return head; } return `${inner()} ${tail.join('-')}`; }\n"+
			"const arrow = (): string => `${id}${tag}`;\n"+
			"const fe = function (): string { return `${renamed}-${t2}`; };\n"+
			"class C { static s = a; f = only + dflt; m(): string { return `${deep}${x0}${x1}`; } static go(): string { return `${k0}${k1}`; } }\n"+
			"function bump(): void { mut0 = mut0 + 10; }\n"+
			"const fns: Array<() => number> = [];\n"+
			"for (let i = 0; i < 2; i++) { fns.push(() => p + i); }\n"+
			"out.push(`${pq()} ${p} ${q}`);\n"+
			"out.push(`${outer()} ${head} ${tail.length}`);\n"+
			"out.push(`${arrow()} ${fe()} ${rest.b + rest.c}`);\n"+
			"out.push(`${C.s} ${new C().f} ${new C().m()} ${C.go()}`);\n"+
			"out.push(`${pn}${ps} ${fns[0]()} ${fns[1]()}`);\n"+
			"bump();\n"+
			"mut1 = mut1 + 100;\n"+
			"out.push(`${mut0} ${mut1}`);\n"+
			"console.log(out.join(' / '));\n")
	want := "15 7 8 / 7 8-9 7 2 / 1x 1-x 5 / 1 9 234 kj / 6z 7 8 / 17 108\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
