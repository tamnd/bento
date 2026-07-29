package build

import "testing"

// The wall this closes was a run-time one: a boxed array answered length, an index,
// Object.keys and JSON.stringify, so it passed for an array until the first method
// call, which invoked undefined and threw. A unit test on the value model cannot show
// that it is fixed for a compiled program, because the failure was in what the built
// binary did rather than in what the lowerer emitted. These build real binaries.

// TestMappingWhatABuiltinAnsweredWorks is the exact reproducer note 352 recorded, the
// line that threw "undefined is not a function". It reads a real machine's cpus, so
// the assertions are about shape rather than about particular numbers.
//
// The reduce sums model name lengths rather than idle times. Summing c.times.idle
// reads the same nested box and is the more natural thing to write, but Darwin reports
// no times, so it answers 0 on a mac and a positive number on Linux. A test that says
// the total is positive is then asserting which machine it ran on. The type of the
// idle read is still checked, which is the part that was broken.
func TestMappingWhatABuiltinAnsweredWorks(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const os = require('os');\n"+
			"const cpus = os.cpus();\n"+
			"const models = cpus.map(function (c) { return c.model; });\n"+
			"console.log(models.length === cpus.length);\n"+
			"console.log(typeof models[0]);\n"+
			"const chars = cpus.reduce(function (a, c) { return a + c.model.length; }, 0);\n"+
			"console.log(typeof chars, chars > 0);\n"+
			"console.log(typeof cpus[0].times.idle);\n")
	if want := "true\nstring\nnumber true\nnumber\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestEveryArrayMethodIsThereInABinary walks the whole prototype from inside a compiled
// program rather than a representative few. The point of the slice is that no method is
// missing, and a program finds a missing one at run time, so the check is that reading
// each name off a boxed array answers a function. The names are held in a boxed array
// too, built by JSON.parse, because a static array literal takes the lowerer's typed
// path and this is a test about the dynamic one.
func TestEveryArrayMethodIsThereInABinary(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const os = require('os');\n"+
			"const a = os.cpus();\n"+
			"const names = JSON.parse('[\"at\",\"concat\",\"copyWithin\",\"entries\",\"every\",\"fill\",\"filter\",\"find\",\"findIndex\",\"findLast\",\"findLastIndex\",\"flat\",\"flatMap\",\"forEach\",\"includes\",\"indexOf\",\"join\",\"keys\",\"lastIndexOf\",\"map\",\"pop\",\"push\",\"reduce\",\"reduceRight\",\"reverse\",\"shift\",\"slice\",\"some\",\"sort\",\"splice\",\"toLocaleString\",\"toReversed\",\"toSorted\",\"toSpliced\",\"toString\",\"unshift\",\"values\",\"with\"]');\n"+
			"let missing = '';\n"+
			"for (let i = 0; i < names.length; i++) {\n"+
			"  if (typeof a[names[i]] !== 'function') { missing = missing + names[i] + ' '; }\n"+
			"}\n"+
			"console.log(names.length);\n"+
			"console.log(missing === '' ? 'none' : missing);\n")
	if want := "38\nnone\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestTheArrayMethodsAnswerNodeInABinary runs each method over a boxed array of known
// contents inside a compiled program and prints what it answered, so the whole output is
// compared against what Node prints for the same program rather than one assertion at a
// time. The array comes from JSON.parse rather than a literal, since a literal takes the
// lowerer's typed path and would test the static methods instead of these.
func TestTheArrayMethodsAnswerNodeInABinary(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const a = JSON.parse('[3,1,2]');\n"+
			"console.log(typeof a.map);\n"+
			"console.log(a.map(function (x) { return x * 2; }).join(','));\n"+
			"console.log(a.filter(function (x) { return x > 1; }).join(','));\n"+
			"console.log(a.slice(1).join(','));\n"+
			"console.log(a.indexOf(2), a.includes(3), a.at(-1));\n"+
			"console.log(a.reduce(function (s, x) { return s + x; }, 0));\n"+
			"const b = JSON.parse('[1,2,3]');\n"+
			"b.push(4);\n"+
			"b.unshift(0);\n"+
			"console.log(b.join(','), b.pop(), b.shift(), b.join(','));\n"+
			"const c = JSON.parse('[1,2,3,4,5]');\n"+
			"console.log(c.splice(1, 2).join(','), c.join(','));\n"+
			"console.log(JSON.parse('[10,9,1]').sort().join(','));\n"+
			"console.log(JSON.parse('[[1,[2]],3]').flat(2).join(','));\n"+
			"console.log(JSON.parse('[1,2,3]').toReversed().join(','), JSON.parse('[1,2,3]').with(0, 9).join(','));\n")
	want := "function\n" +
		"6,2,4\n" +
		"3,2\n" +
		"1,2\n" +
		"2 true 2\n" +
		"6\n" +
		"0,1,2,3,4 4 0 1,2,3\n" +
		"2,3 1,4,5\n" +
		"1,10,9\n" +
		"1,2,3\n" +
		"3,2,1 9,2,3\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestForEachOverABoxedArrayRuns pins the callback form on its own, since forEach is how
// a program that only wants the side effect walks what a built-in answered, and it is
// the method a partial implementation is most likely to have skipped for returning
// nothing.
func TestForEachOverABoxedArrayRuns(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const os = require('os');\n"+
			"let n = 0;\n"+
			"os.cpus().forEach(function (c) { n = n + 1; });\n"+
			"console.log(n === os.cpus().length);\n"+
			"console.log(n === os.availableParallelism());\n")
	if want := "true\ntrue\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
