package build

import "testing"

// TestTheInOperatorReadsLikeNode is the capability this is for. The runtime existence
// check needs an object value to read, and a static receiver has no box, so until this
// every `key in obj` written against a class instance, a fixed-shape binding or an array
// handed the whole build back. A sealed shape names every key an object of that type
// carries, so the checker answers those on its own, and an array boxes instead because its
// keys are its live indices and no compile-time answer holds for them.
//
// This builds a real binary and holds its whole output against what Node v24.18.0 prints
// for the same program. The line worth reading twice is the array: 5 in a is false and
// 'length' in a is true off the same receiver, which is exactly the pair a fold cannot
// produce and a box gets right.
func TestTheInOperatorReadsLikeNode(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"class P { x = 1; m() { return 2; } }\n"+
			"const p = new P();\n"+
			"console.log('x' in p, 'z' in p, 'toString' in p, 'm' in p, 'constructor' in p);\n"+
			"const o: { a: number; b: string } = { a: 1, b: 'q' };\n"+
			"console.log('a' in o, 'b' in o, 'c' in o, 'hasOwnProperty' in o);\n"+
			"const a = [1, 2, 3];\n"+
			"console.log(0 in a, 2 in a, 5 in a, 'length' in a, 'map' in a, 'toString' in a);\n"+
			"const d: any = { a: 1 };\n"+
			"console.log('a' in d, 'zz' in d, 'valueOf' in d);\n")
	want := "true false true true true\n" +
		"true true false true\n" +
		"true true false true true true\n" +
		"true false true\n"
	if got != want {
		t.Errorf("the in operator read differently than Node:\ngot:\n%s\nwant:\n%s", got, want)
	}
}
