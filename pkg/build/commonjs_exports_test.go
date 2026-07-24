package build

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestModuleExportsLowerDynamically pins slice G0.2b of the CommonJS module shape:
// a JavaScript module reading and writing the exports and module globals routes
// every member access through the value model rather than a Go struct field. The
// checker's CommonJS export inference synthesizes a concrete shape for exports and
// module.exports from the module's own assignments, which would drive a static
// field read the ambient any declaration cannot back; the lowerer recognizes the
// synthesized symbol and emits a package-level value.Object for each name, so
// exports.a = v and module.exports.b = v become Set calls and the reads become
// Get calls. The program is built and run so the read-back is proven against the
// value Node produces.
func TestModuleExportsLowerDynamically(t *testing.T) {
	dir := t.TempDir()
	entry := filepath.Join(dir, "mod.js")
	src := "exports.a = 10;\n" +
		"module.exports.b = 20;\n" +
		"console.log(exports.a);\n" +
		"console.log(module.exports.a);\n" +
		"console.log(module.exports.b);\n"
	if err := os.WriteFile(entry, []byte(src), 0o644); err != nil {
		t.Fatalf("write entry: %v", err)
	}
	bin := filepath.Join(dir, "prog")
	if err := Build(Options{Entry: entry, Output: bin}); err != nil {
		t.Fatalf("build module using exports/module: %v", err)
	}
	got, err := exec.Command(bin).CombinedOutput()
	if err != nil {
		t.Fatalf("run program: %v (%s)", err, got)
	}
	// exports and module.exports name one object until it is reassigned, so a
	// property set through exports is visible through module.exports and back.
	if want := "10\n10\n20\n"; string(got) != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestModuleExportsReassignmentDivergesFromAlias pins the one place exports and
// module.exports part ways: assigning module.exports moves module's property to a
// new object without moving the exports alias, so a later read through exports
// still sees the original object. Node's wrapper has this divergence because
// exports is a parameter bound to module.exports at entry, not a live view of it;
// the lowerer reproduces it by keeping the alias var pointed at the first object
// while the module.exports set replaces module's property.
func TestModuleExportsReassignmentDivergesFromAlias(t *testing.T) {
	dir := t.TempDir()
	entry := filepath.Join(dir, "mod.js")
	src := "exports.a = 1;\n" +
		"module.exports = { b: 2 };\n" +
		"console.log(module.exports.b);\n" +
		"console.log(exports.a);\n" +
		"console.log(module.exports.a);\n"
	if err := os.WriteFile(entry, []byte(src), 0o644); err != nil {
		t.Fatalf("write entry: %v", err)
	}
	bin := filepath.Join(dir, "prog")
	if err := Build(Options{Entry: entry, Output: bin}); err != nil {
		t.Fatalf("build module reassigning module.exports: %v", err)
	}
	got, err := exec.Command(bin).CombinedOutput()
	if err != nil {
		t.Fatalf("run program: %v (%s)", err, got)
	}
	// module.exports is the new object (b=2, no a); exports still names the first
	// object (a=1); so module.exports.a reads undefined.
	if want := "2\n1\nundefined\n"; string(got) != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
