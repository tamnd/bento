package build

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// compileJS writes a single JavaScript source to a temp dir under the given file
// name and runs the front half through Compile, so a test can assert how a
// dynamic .js entry lowers without paying for a real build. The name carries the
// extension under test (.js, .mjs, .cjs), which is the whole point: the gate this
// slice removed keyed on that extension.
func compileJS(t *testing.T, name, src string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	entry := filepath.Join(dir, name)
	if err := os.WriteFile(entry, []byte(src), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return Compile(entry)
}

// TestJavaScriptEntryLowers pins the foundation slice G0.1: a self-contained
// JavaScript entry with no type annotations compiles through the AOT front half
// rather than being rejected at the door. Before this slice the build gate turned
// a .js entry away outright; now allowJs admits it as a source file and the
// lowerer takes it. The source is deliberately untyped and dynamic (a bare
// parameter widening to any, arithmetic on it) so the test proves the untyped
// path lowers, not merely that some .js file with hidden annotations does.
func TestJavaScriptEntryLowers(t *testing.T) {
	out, err := compileJS(t, "entry.js", "function twice(x) { return x + x; }\nconsole.log(twice(21));\n")
	if err != nil {
		t.Fatalf("a self-contained .js entry should lower through the AOT path, got: %v", err)
	}
	if strings.Count(out, "package main") != 1 {
		t.Fatalf("expected one lowered package, got:\n%s", out)
	}
	if !strings.Contains(out, "func main() {") {
		t.Fatalf("expected a main entry point in the lowered output, got:\n%s", out)
	}
}

// TestJavaScriptEntryRuns carries a dynamic .js program through the full build and
// runs it, so admission is proven at runtime, not just in the emitted text. The
// program exercises the untyped value path end to end: a parameter that widens to
// any, arithmetic and string coercion on it, an array method, and console.log.
// The output must match what node prints for the same source, byte for byte.
func TestJavaScriptEntryRuns(t *testing.T) {
	dir := t.TempDir()
	entry := filepath.Join(dir, "main.js")
	src := "" +
		"function label(n) { return \"n=\" + n; }\n" +
		"const xs = [1, 2];\n" +
		"xs.push(3);\n" +
		"console.log(label(xs.join(\",\")));\n" +
		"console.log(1 + 2);\n"
	if err := os.WriteFile(entry, []byte(src), 0o644); err != nil {
		t.Fatalf("write entry: %v", err)
	}
	bin := filepath.Join(dir, "prog")
	if err := Build(Options{Entry: entry, Output: bin}); err != nil {
		t.Fatalf("build .js program: %v", err)
	}
	got, err := exec.Command(bin).CombinedOutput()
	if err != nil {
		t.Fatalf("run .js program: %v (%s)", err, got)
	}
	if string(got) != "n=1,2,3\n3\n" {
		t.Fatalf("want \"n=1,2,3\\n3\\n\", got %q", got)
	}
}
