package build

import "testing"

// These build real binaries and run them, which is the only place util.isDeepStrictEqual
// is checked the way a program experiences it. The comparison itself is unit tested
// against node v24.18.0 in pkg/value over ninety pairs and three modes, and the routing
// is unit tested in pkg/lower; what neither proves is that the module survives
// compilation and that the require form and the import form answer the same. Every
// expected line here was taken from node v24.18.0 running the same program.

// deepEqualProgram is the body both tests run, minus the lines that reach the module
// and build its inputs, so the two forms cannot drift apart in what they ask.
//
// The cyclic and nested values come through JSON.parse rather than from a literal
// because a literal is a fixed-shape object in a compiled program: `o.self = o` adds a
// property the shape never declared, and `[1, [2]]` is an array literal of arrays.
// Both are their own slices, and JSON.parse hands back a dynamic value, which is what
// the comparison takes anyway.
const deepEqualProgram = "console.log(eq({ a: 1 }, { a: 1 }));\n" +
	"console.log(eq({ a: 1 }, { a: 2 }));\n" +
	"console.log(eq([1, 2], [1, 2]));\n" +
	"console.log(eq(nested1, nested2));\n" +
	"console.log(eq(0, -0));\n" +
	"console.log(eq(NaN, NaN));\n" +
	"console.log(eq(1, '1'));\n" +
	"console.log(eq(o, p));\n" +
	"console.log(eq(/a/g, /a/g));\n" +
	"console.log(eq('a', 'a'), eq(1n, 1n));\n"

// deepEqualWant is what node v24.18.0 prints for that body.
const deepEqualWant = "true\n" +
	"false\n" +
	"true\n" +
	"true\n" +
	"false\n" +
	"true\n" +
	"false\n" +
	"true\n" +
	"true\n" +
	"true true\n"

// deepEqualInputs builds the four dynamic values the body compares: two objects that
// point at themselves, and two of the same nested shape.
const deepEqualInputs = "o.self = o;\n" +
	"p.self = p;\n"

// TestRequireUtilIsDeepStrictEqual is the comparison as a CommonJS program reaches it.
// The two cyclic objects are the case that matters most in a compiled binary: a cycle
// is the one input that can turn a comparison into a hang or a stack overflow rather
// than a wrong answer.
func TestRequireUtilIsDeepStrictEqual(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const util = require('util');\n"+
			"const eq = util.isDeepStrictEqual;\n"+
			"const o = JSON.parse('{\"a\":1}');\n"+
			"const p = JSON.parse('{\"a\":1}');\n"+
			"const nested1 = JSON.parse('{\"x\":[1,{\"y\":2}]}');\n"+
			"const nested2 = JSON.parse('{\"x\":[1,{\"y\":2}]}');\n"+
			deepEqualInputs+
			deepEqualProgram+
			"console.log(typeof util.isDeepStrictEqual);\n")
	want := deepEqualWant + "function\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestImportNodeUtilIsDeepStrictEqual asks the same questions through the import path,
// which lowers to direct calls into the comparison rather than reading the registry
// module. Two paths through the compiler must not be able to answer differently.
func TestImportNodeUtilIsDeepStrictEqual(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"import { isDeepStrictEqual } from 'node:util';\n"+
			"import util from 'node:util';\n"+
			"const eq = (a: any, b: any): boolean => isDeepStrictEqual(a, b);\n"+
			"const o: any = JSON.parse('{\"a\":1}');\n"+
			"const p: any = JSON.parse('{\"a\":1}');\n"+
			"const nested1: any = JSON.parse('{\"x\":[1,{\"y\":2}]}');\n"+
			"const nested2: any = JSON.parse('{\"x\":[1,{\"y\":2}]}');\n"+
			deepEqualInputs+
			deepEqualProgram+
			"console.log(util.isDeepStrictEqual({ a: 1 }, { a: 1, b: 2 }));\n")
	want := deepEqualWant + "false\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
