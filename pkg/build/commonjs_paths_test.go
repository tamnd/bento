package build

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestDirnameFilenameResolveModulePath pins slice G0.2a of the CommonJS module
// shape: a JavaScript module reading __filename and __dirname resolves them to its
// own absolute file path and that path's directory, the values Node fills its
// module wrapper with. Before this slice the names lowered to bare Go identifiers
// and the build failed to compile; now each lowers to the module file's path,
// known at compile time. The program is built and run so the resolution is proven
// at runtime, and the expected paths are computed through the same canonical form
// the build resolves the entry with, so a symlinked temp dir compares equal.
func TestDirnameFilenameResolveModulePath(t *testing.T) {
	dir := t.TempDir()
	entry := filepath.Join(dir, "mod.js")
	if err := os.WriteFile(entry, []byte("console.log(__filename);\nconsole.log(__dirname);\n"), 0o644); err != nil {
		t.Fatalf("write entry: %v", err)
	}
	bin := filepath.Join(dir, "prog")
	if err := Build(Options{Entry: entry, Output: bin}); err != nil {
		t.Fatalf("build module reading __filename/__dirname: %v", err)
	}
	got, err := exec.Command(bin).CombinedOutput()
	if err != nil {
		t.Fatalf("run program: %v (%s)", err, got)
	}
	wantFile := canonicalPath(entry)
	want := wantFile + "\n" + filepath.Dir(wantFile) + "\n"
	if string(got) != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
