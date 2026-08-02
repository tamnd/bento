package build

import "testing"

// TestVarStatementDeclaringAPatternBesideAName covers a variable statement that declares
// a destructuring pattern alongside another declaration:
//
//	const [p, q] = arr, extra = 9
//
// A statement declares one binding per comma and a pattern is one of them, but both
// entry points of the destructuring lowering take the statement whole and decline it
// unless it holds exactly one declaration. So the statement fell to the plain path, which
// spells a binding by asking for a Go identifier, and a pattern's text is `[p, q]`: that
// came back mangled rather than refused, and the Go said `declared and not used:
// U5B_pU2C_U20_qU5D_` and `undefined: p`. With a function reading one of the names it
// handed back instead, since the hoist gave every name a package var and only the plain
// declaration assigned into its own.
//
// The statement now lowers declaration by declaration in source order, each through the
// path that already lowers it: the per-declaration destructure cores a for loop's
// initializer shares, and varDeclStmt as a group of one, which is what makes a hoisted
// name a redeclaration of all of its group rather than a mix.
//
// The orders are here: a pattern first, a plain name first, a pattern between two names,
// two patterns in one statement, and a declaration reading a name an earlier one bound.
// So are the pattern shapes: an array, an object, a nesting, a rest, and a default. So
// are the kinds, const, let and var, with a reassignment of each. So are the scopes: the
// module, a function body, a class method, and a hoisted statement whose leaves and plain
// name a function reads. So is a boxed source, a leaf no one reads, and the left-to-right
// evaluation order of a statement whose initializers have side effects.
//
// Held against what Node v24.18.0 prints, one program.
func TestVarStatementDeclaringAPatternBesideAName(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"type Row = { id: number; tag: string };\n"+
			"const arr: number[] = [1, 2];\n"+
			"const obj = { x: 3, y: 4 };\n"+
			"const rows: number[][] = [[5, 6], [7, 8]];\n"+
			"const order: string[] = [];\n"+
			"function tag(s: string, v: number): number { order.push(s); return v; }\n"+
			"function pair(): number[] { order.push('pair'); return arr; }\n"+
			"const raw = JSON.parse('{\"a\":{\"id\":9,\"tag\":\"z\"}}') as Record<string, Row>;\n"+
			"\n"+
			"const [p, q] = arr, extra = 9;\n"+
			"const first = 4, [r0, r1] = arr, mid = 5, { x, y } = obj, last = 6;\n"+
			"const [n0, n1] = arr, sum = n0 + n1;\n"+
			"const deep = { o: { n: 1, s: 'a' } };\n"+
			"const { o: { n, s } } = deep, k = 3;\n"+
			"const three: number[] = [1, 2, 3];\n"+
			"const [head, ...rest] = three, tailz = 4;\n"+
			"const one: number[] = [1];\n"+
			"const [d0, d1 = 5] = one, dz = 2;\n"+
			"const { a } = raw, label = 'L';\n"+
			"const [unused0, unused1] = arr, onlyRead = 7;\n"+
			"let [m0, m1] = arr, mx = 9;\n"+
			"var [v0, v1] = arr, vz = 1;\n"+
			"\n"+
			"function f(): number { return p + q + extra; }\n"+
			"function g(): number { return first + r0 + r1 + mid + x + y + last; }\n"+
			"function h(): string { return `${sum} ${n}${s}${k} ${head}${rest.join('')}${tailz} ${d0}${d1}${dz}`; }\n"+
			"function bx(): string { return `${a.tag}${a.id}${label}`; }\n"+
			"function mm(): number { return m0 + m1 + mx; }\n"+
			"class C { m(): number { const [c0, c1] = rows[0], bump = 1; return c0 + c1 + bump; } }\n"+
			"function body(): number { const [b0, b1] = rows[1], bump = 2; return b0 + b1 + bump; }\n"+
			"const both: number[] = [tag('l', 1), tag('r', 2)];\n"+
			"const [ord0, ord1] = both, after = tag('after', 3);\n"+
			"const eager = tag('eager', 1), [lz0, lz1] = pair(), late = tag('late', 2);\n"+
			"\n"+
			"m0 = 20;\n"+
			"mx = 1;\n"+
			"console.log(f(), g(), h());\n"+
			"console.log(bx(), onlyRead, mm(), v0 + v1 + vz);\n"+
			"console.log(new C().m(), body(), ord0 + ord1 + after, eager + lz0 + lz1 + late);\n"+
			"console.log(order.join(','));\n")
	want := "12 25 3 1a3 1234 152\n" +
		"z9L 7 23 4\n" +
		"12 17 6 6\n" +
		"l,r,after,eager,pair,late\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
