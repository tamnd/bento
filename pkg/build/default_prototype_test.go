package build

import "testing"

// TestTheDefaultPrototypeReadsLikeNode is the capability this is for. Every ordinary
// object inherits Object.prototype, and the value model carried only half of it: the
// reflection methods answered a read, the two coercion methods did not, and the
// existence probe the in operator makes answered none of them, which is why `'toString'
// in o` handed the build back rather than print a false the engine contradicts. This
// builds a real binary and holds its whole output against what Node v24.18.0 prints for
// the same program.
//
// The lines worth reading twice are the ones where a nearer prototype wins. An array is
// asked Array.prototype first, so its toString joins its elements where the default
// would report a tag, and it still reaches Object.prototype past it for hasOwnProperty.
// A boxed regexp and a Map each spell themselves, one from a brand and one from a
// Symbol.toStringTag, both read through the same one method.
func TestTheDefaultPrototypeReadsLikeNode(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"const o: any = { a: 1 };\n"+
			"console.log(o.toString(), String(o), o.valueOf() === o);\n"+
			"console.log('toString' in o, 'valueOf' in o, 'hasOwnProperty' in o, 'toLocaleString' in o, 'isPrototypeOf' in o, 'propertyIsEnumerable' in o);\n"+
			"console.log('__lookupGetter__' in o, '__defineGetter__' in o, 'nope' in o);\n"+
			"console.log(o.toLocaleString(), o.hasOwnProperty('a'), o.propertyIsEnumerable('a'));\n"+
			"const a: any = [1, 2];\n"+
			"console.log(a.toString(), 'map' in a, 'toString' in a, 'hasOwnProperty' in a, 'valueOf' in a);\n"+
			"const g: any = {};\n"+
			"g.__defineGetter__('x', function () { return 41 + 1; });\n"+
			"console.log(g.x, JSON.stringify(g), typeof g.__lookupGetter__('x'));\n"+
			"class P { x = 1; }\n"+
			"const p: any = new P();\n"+
			"console.log(p.toString(), 'toString' in p, p.valueOf() === p, 'hasOwnProperty' in p);\n"+
			"const m: any = new Map<string, number>();\n"+
			"console.log(m.toString(), 'toString' in m, 'valueOf' in m);\n"+
			"const re: any = /ab+/g;\n"+
			"console.log(re.toString(), String(re), 'toString' in re);\n"+
			"const d: any = new Date(0);\n"+
			"console.log(typeof d.toString(), 'toString' in d, 'valueOf' in d);\n")
	want := "[object Object] [object Object] true\n" +
		"true true true true true true\n" +
		"true true false\n" +
		"[object Object] true true\n" +
		"1,2 true true true true\n" +
		"42 {\"x\":42} function\n" +
		"[object Object] true true true\n" +
		"[object Map] true true\n" +
		"/ab+/g /ab+/g true\n" +
		"string true true\n"
	if got != want {
		t.Errorf("the default prototype read differently than Node:\ngot:\n%s\nwant:\n%s", got, want)
	}
}
