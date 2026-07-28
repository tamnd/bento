package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeSloppy writes one source file into dir and returns its path, so a test that
// needs a second file to make the entry a module can spell that in one line.
func writeSloppy(t *testing.T, dir, name, src string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

// The tests here pin the sloppy-mode gate: which files the Annex B forms are
// admitted in, and which they still gate. What those forms lower to is pinned next
// to the decoders, in pkg/lower/strlit_test.go and pkg/lower/numlit_test.go.
//
// The forms matter because a benchmark suite is made of the vintage of JavaScript
// that uses them. JetStream 3 could not parse two of its benchmarks before this
// gate existed: earley-boyer on an octal escape, typescript-compiler on an
// assignment to arguments.

// TestSloppyScriptAdmitsAnnexBForms pins that a JavaScript file with no "use
// strict" and no imports compiles through the forms that are a SyntaxError only
// under strict mode. Each is a separate checker diagnostic and each has to be in
// the tolerated set for the file to build.
func TestSloppyScriptAdmitsAnnexBForms(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"octal escape", `var s = "\251";` + "\nconsole.log(s);\n"},                       // 1487
		{"non-octal decimal escape", `var s = "\8";` + "\nconsole.log(s);\n"},             // 1488
		{"octal literal", "console.log(010);\n"},                                          // 1121
		{"non-octal decimal literal", "console.log(018);\n"},                              // 1489
		{"reserved word as a name", "var yield = 3;\nconsole.log(yield);\n"},              // 1212
		{"assignment to arguments", "function f() { arguments = 1; }\nconsole.log(1);\n"}, // 1100
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := compileJS(t, "entry.js", tc.src); err != nil {
				t.Fatalf("a sloppy script should admit %s, got: %v", tc.name, err)
			}
		})
	}
}

// TestUseStrictPrologueStillGates pins the first half of the gate: a JavaScript
// file that opted into strict mode is strict-mode source, so the report is correct
// and stays an error. The directive is preceded by a comment and a blank line, so
// the test also covers the prologue scan skipping what precedes it rather than
// matching only a first byte.
func TestUseStrictPrologueStillGates(t *testing.T) {
	src := "// a header comment\n\n\"use strict\";\nvar s = \"\\251\";\nconsole.log(s);\n"
	_, err := compileJS(t, "entry.js", src)
	if err == nil {
		t.Fatal("a file with a use strict prologue should still gate on an octal escape")
	}
	if !strings.Contains(err.Error(), "Octal escape") {
		t.Fatalf("expected the octal escape error, got: %v", err)
	}
}

// TestModuleStillGates pins the second half: a file with an import is a module, and
// a module is strict whatever its extension.
func TestModuleStillGates(t *testing.T) {
	dir := t.TempDir()
	writeSloppy(t, dir, "dep.js", "export default 1;\n")
	entry := writeSloppy(t, dir, "entry.js", "import \"./dep.js\";\nvar s = \"\\251\";\nconsole.log(s);\n")
	if _, err := Compile(entry); err == nil {
		t.Fatal("a module should still gate on an octal escape")
	}
}

// TestTypeScriptStillGates pins that none of this reaches TypeScript. A .ts file is
// strict-mode source by definition, so its reports are correct and the toleration
// never applies to it.
func TestTypeScriptStillGates(t *testing.T) {
	_, err := compileSource(t, "const s = \"\\251\";\nconsole.log(s);\n")
	if err == nil {
		t.Fatal("a TypeScript file should still gate on an octal escape")
	}
	if !strings.Contains(err.Error(), "Octal escape") {
		t.Fatalf("expected the octal escape error, got: %v", err)
	}
}

// TestCommonJSScriptIsStillSloppy pins the direction the gate got wrong when it
// asked about import edges instead of module syntax. A require call is an import
// edge, so a CommonJS file has edges, but it is a script and its code runs sloppy.
// Octane's typescript-compiler.js is exactly this shape and stayed gated until the
// test became IsModule.
func TestCommonJSScriptIsStillSloppy(t *testing.T) {
	dir := t.TempDir()
	writeSloppy(t, dir, "dep.js", "module.exports = 1;\n")
	entry := writeSloppy(t, dir, "entry.js", "var d = require(\"./dep.js\");\nvar s = \"\\251\";\nconsole.log(s, d);\n")
	if _, err := Compile(entry); err != nil {
		t.Fatalf("a CommonJS script should admit an octal escape, got: %v", err)
	}
}

// TestStrictModeModuleTwinsStillGate pins the codes deliberately left out of the
// tolerated set. The checker spells the strict-mode reports differently inside a
// module, and those spellings are correct wherever they appear, so a module that
// names a reserved word still gates rather than riding the sloppy toleration.
func TestStrictModeModuleTwinsStillGate(t *testing.T) {
	dir := t.TempDir()
	writeSloppy(t, dir, "dep.js", "export default 1;\n")
	entry := writeSloppy(t, dir, "entry.js", "import \"./dep.js\";\nvar yield = 3;\nconsole.log(yield);\n")
	if _, err := Compile(entry); err == nil {
		t.Fatal("a reserved word in a module should still gate the build")
	}
}

// TestHasUseStrictPrologue pins the prologue scan directly, including the shapes
// that decide it wrong if the scan is a prefix match: a directive that is not the
// first one, a "use strict" that is not in the prologue at all, and a comment or a
// second string standing in front of it.
func TestHasUseStrictPrologue(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		want bool
	}{
		{"bare directive", `"use strict";` + "\nvar x = 1;\n", true},
		{"single quoted", "'use strict';\n", true},
		{"after a line comment", "// c\n\"use strict\";\n", true},
		{"after a block comment", "/* c\n c */\n\"use strict\";\n", true},
		{"after another directive", "\"use asm\";\n\"use strict\";\n", true},
		{"no directive", "var x = 1;\n", false},
		{"not in the prologue", "var x = 1;\n\"use strict\";\n", false},
		{"only a substring", "\"do not use strict\";\n", false},
		{"unterminated comment", "/* c\n", false},
		{"empty", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasUseStrictPrologue(tc.text); got != tc.want {
				t.Fatalf("hasUseStrictPrologue(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}
