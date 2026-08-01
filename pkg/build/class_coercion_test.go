package build

import "testing"

// TestABoxedInstanceCoercesLikeNode is the capability this is for. A boxed instance
// carries an instance's fields and its class name and, before this, none of its methods,
// so a class writing its own toString read as "[object Object]" the moment it crossed
// into a dynamic value. Anything holding such an instance, an array or a Map, therefore
// handed the build back rather than print a row of class tags. This builds a real binary
// and holds its whole output against what Node v24.18.0 prints for the same program.
//
// The lines worth reading twice are the ones where the same instance reads two ways. The
// string hint asks toString first and the default hint asks valueOf first, so a class
// writing only a valueOf reads String(v) as the class tag and 'x' + v as the number, and
// a class writing both answers each hint from its own method. The last pair is the other
// place the two part: x.toString() is the method call, so a toString answering a number
// answers a number, where String(x) is the coercion and answers the digits.
func TestABoxedInstanceCoercesLikeNode(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"class Q { y = 2; toString() { return 'Q!'; } }\n"+
			"class R extends Q { toString() { return 'R!'; } }\n"+
			"class S extends Q { z = 3; }\n"+
			"class V { v = 1; valueOf() { return 7; } }\n"+
			"class W { w = 1; toString() { return 'W!'; } valueOf() { return 9; } }\n"+
			"class N { n = 1; toString() { return 1; } }\n"+
			"class O { o = 1; valueOf() { return { a: 1 }; } }\n"+
			"class P { x = 1; }\n"+
			"const q = new Q(), r = new R(), sub = new S(), v = new V(), w = new W(), p = new P();\n"+
			"const qs: Q[] = [q, r, sub];\n"+
			"const vs: V[] = [v];\n"+
			"const ws: W[] = [w];\n"+
			"const ps: P[] = [p];\n"+
			"console.log(String(qs));\n"+
			"console.log(qs.join('|'));\n"+
			"console.log(String(vs), vs.join(','));\n"+
			"console.log(String(ws), ws.join(','));\n"+
			"console.log(String(ps));\n"+
			"console.log(`${qs}`);\n"+
			"console.log('x' + qs);\n"+
			"const m = new Map<string, Q>();\n"+
			"m.set('a', q);\n"+
			"console.log(String(m), String(m.get('a')));\n"+
			"const dq: any = q;\n"+
			"console.log(String(dq), dq.toString(), `${dq}`, 'x' + dq);\n"+
			"const dv: any = v;\n"+
			"console.log(String(dv), 'x' + dv, dv.valueOf());\n"+
			"const dw: any = w;\n"+
			"console.log(String(dw), 'x' + dw);\n"+
			"const dp: any = p;\n"+
			"console.log(String(dp), dp.toString());\n"+
			"console.log(String(new N()), 'x' + new N());\n"+
			"console.log(String(new O()), 'x' + new O());\n"+
			"const dn: any = new N();\n"+
			"console.log(dn.toString(), typeof dn.toString());\n"+
			"const dsub: any = sub;\n"+
			"console.log(dsub.toString(), String(dsub));\n")
	want := "Q!,R!,Q!\n" +
		"Q!|R!|Q!\n" +
		"[object Object] [object Object]\n" +
		"W! W!\n" +
		"[object Object]\n" +
		"Q!,R!,Q!\n" +
		"xQ!,R!,Q!\n" +
		"[object Map] Q!\n" +
		"Q! Q! Q! xQ!\n" +
		"[object Object] x7 7\n" +
		"W! x9\n" +
		"[object Object] [object Object]\n" +
		"1 x1\n" +
		"[object Object] x[object Object]\n" +
		"1 number\n" +
		"Q! Q!\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
