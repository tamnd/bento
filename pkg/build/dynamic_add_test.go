package build

import "testing"

// These build real binaries and run them, so what is checked is the arithmetic's
// answer rather than the shape of the emitted Go. A boxed + coerced to the wrong
// thing would still compile and would still print a number; only running it says
// which number.

// TestAddingWhatABuiltinAnsweredKeepsTheNumber pins the assignment form. Node prints
// the total twice, once accumulated and once read straight, and they must match.
func TestAddingWhatABuiltinAnsweredKeepsTheNumber(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const os = require('os');\n"+
			"let n = 0;\n"+
			"n = n + os.totalmem();\n"+
			"console.log(n === os.totalmem());\n"+
			"console.log(typeof n);\n")
	if want := "true\nnumber\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestSummingBoxedReadsAcrossAChain pins the chain and the compound form together in
// the program a real one is written as: add up each core's busy time. The two totals
// are computed by different routes over the same numbers, so they must agree, and
// both must be numbers rather than a string the boxed + concatenated.
func TestSummingBoxedReadsAcrossAChain(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const os = require('os');\n"+
			"const cpus = os.cpus();\n"+
			"let a = 0;\n"+
			"let b = 0;\n"+
			"for (let i = 0; i < cpus.length; i++) {\n"+
			"  const t = cpus[i].times;\n"+
			"  a += t.user + t.sys;\n"+
			"  b = b + t.user;\n"+
			"  b = b + t.sys;\n"+
			"}\n"+
			"console.log(typeof a, typeof b);\n"+
			"console.log(a === b);\n")
	if want := "number number\ntrue\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestAddingABoxToAStringStillConcatenates pins the other side: a + with a string
// operand concatenates whatever the other side is, so the number a built-in answered
// joins the string rather than being added to it. Node prints the joined line.
func TestAddingABoxToAStringStillConcatenates(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const os = require('os');\n"+
			"const s = 'cores=' + os.availableParallelism();\n"+
			"console.log(typeof s);\n"+
			"console.log(s.indexOf('cores=') === 0 && s.length > 6);\n")
	if want := "string\ntrue\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
