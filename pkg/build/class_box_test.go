package build

import "testing"

// A class instance that crosses into a dynamic slot is a run-time surface rather than an
// emission, so what matters is what the built binary prints when a program logs one,
// walks its keys, serializes it, or compares two. These build real binaries and hold
// their whole output against what Node v24.18.0 prints for the same program.

// TestABoxedClassInstancePrintsLikeNode covers the rendering. The class's name in front
// of the braces is the whole point: a boxed instance that lost it would print as an
// object literal, which is what every one of these did before the box existed.
func TestABoxedClassInstancePrintsLikeNode(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"class P { x = 1; y = 's'; }\n"+
			"class Empty {}\n"+
			"class Nested { inner = new P(); }\n"+
			"class Base { a = 1; }\n"+
			"class Derived extends Base { b = 2; }\n"+
			"class Priv { #secret = 1; shown = 2; }\n"+
			"class Meth { v = 3; m() { return this.v; } get g() { return 4; } }\n"+
			"console.log(new P());\n"+
			"console.log(new Empty());\n"+
			"console.log(new Nested());\n"+
			"console.log(new Derived());\n"+
			"console.log(new Priv());\n"+
			"console.log(new Meth());\n")
	want := "P { x: 1, y: 's' }\n" +
		"Empty {}\n" +
		"Nested { inner: P { x: 1, y: 's' } }\n" +
		"Derived { a: 1, b: 2 }\n" +
		"Priv { shown: 2 }\n" +
		"Meth { v: 3 }\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestABoxedClassInstanceIsNamedInsideAContainer covers the positions with no boxing site
// of their own. An instance held in a field of another instance or in an array is reached
// by the reflection walk, which sees a Go type and nothing else, so the name has to come
// from the registry rather than from the place the box was asked for.
func TestABoxedClassInstanceIsNamedInsideAContainer(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"class P { x = 1; }\n"+
			"const people = [new P(), new P()];\n"+
			"console.log(people);\n"+
			"class Outer { inner = new P(); }\n"+
			"const o = new Outer();\n"+
			"console.log([o]);\n")
	want := "[ P { x: 1 }, P { x: 1 } ]\n" +
		"[ Outer { inner: P { x: 1 } } ]\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestABoxedClassInstanceWalksItsFields covers the key walk and the two JSON serializers.
// None of them see the constructor, which lives on the prototype and is not enumerable,
// so an instance serializes to exactly its fields.
func TestABoxedClassInstanceWalksItsFields(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"class P { x = 1; y = 's'; }\n"+
			"class Nested { inner = new P(); }\n"+
			"const p = new P();\n"+
			"console.log(Object.keys(p));\n"+
			"console.log(JSON.stringify(p));\n"+
			"console.log(JSON.stringify(new Nested()));\n"+
			"console.log(JSON.stringify(p, null, 2));\n")
	want := "[ 'x', 'y' ]\n" +
		"{\"x\":1,\"y\":\"s\"}\n" +
		"{\"inner\":{\"x\":1,\"y\":\"s\"}}\n" +
		"{\n  \"x\": 1,\n  \"y\": \"s\"\n}\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestABoxedClassInstanceComparesByItsClass covers the second thing the prototype buys.
// Node's strict comparison requires the same constructor and decides it by prototype
// identity, so two classes with identical fields are not equal and neither is an instance
// and a plain object; the loose comparison does not read the constructor and equates all
// three.
func TestABoxedClassInstanceComparesByItsClass(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const assert = require('node:assert');\n"+
			"class P { x = 1; y = 's'; }\n"+
			"class Q { x = 1; y = 's'; }\n"+
			"const plain = { x: 1, y: 's' };\n"+
			"assert.deepStrictEqual(new P(), new P());\n"+
			"console.log('same class equal');\n"+
			"try { assert.deepStrictEqual(new P(), new Q()); console.log('bad'); }\n"+
			"catch { console.log('cross class not equal'); }\n"+
			"try { assert.deepStrictEqual(new P(), plain); console.log('bad'); }\n"+
			"catch { console.log('class vs plain not equal'); }\n"+
			"assert.deepEqual(new P(), new Q());\n"+
			"assert.deepEqual(new P(), plain);\n"+
			"console.log('loose equal across both');\n")
	want := "same class equal\n" +
		"cross class not equal\n" +
		"class vs plain not equal\n" +
		"loose equal across both\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestABoxedClassInstanceAbbreviatesUnderItsName covers the depth cutoff and showHidden.
// Past the depth limit Node writes the class name in brackets rather than the [Object] a
// plain object gets, and showHidden adds nothing, since an instance has no internal slot
// to reveal the way a typed array or a map does.
func TestABoxedClassInstanceAbbreviatesUnderItsName(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const util = require('node:util');\n"+
			"class P { x = 1; y = 's'; }\n"+
			"class Nested { inner = new P(); }\n"+
			"console.log(util.inspect(new Nested(), { depth: 0 }));\n"+
			"console.log(util.inspect(new P(), { showHidden: true }));\n")
	want := "Nested { inner: [P] }\n" +
		"P { x: 1, y: 's' }\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestABoxedInstanceKeepsItsBuiltInFields covers the fields whose Go representation is a
// runtime struct rather than a scalar. Each takes its own box on the way into the
// instance's, so a date prints as a date rather than as the quoted ISO string its ToJSON
// answers and a map prints as a map rather than as an empty object. The plain object
// below it goes through the same walk, so it holds the same fix for an object literal
// bound to a name.
func TestABoxedInstanceKeepsItsBuiltInFields(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"class M { d = new Date(0); m = new Map(); s = new Set(); }\n"+
			"console.log(new M());\n"+
			"const o = { d: new Date(0), m: new Map(), s: new Set(), r: /ab/g };\n"+
			"console.log(o);\n"+
			"console.log(JSON.stringify(new M()));\n"+
			"console.log(JSON.stringify(o));\n")
	want := "M { d: 1970-01-01T00:00:00.000Z, m: Map(0) {}, s: Set(0) {} }\n" +
		"{ d: 1970-01-01T00:00:00.000Z, m: Map(0) {}, s: Set(0) {}, r: /ab/g }\n" +
		"{\"d\":\"1970-01-01T00:00:00.000Z\",\"m\":{},\"s\":{}}\n" +
		"{\"d\":\"1970-01-01T00:00:00.000Z\",\"m\":{},\"s\":{},\"r\":{}}\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
