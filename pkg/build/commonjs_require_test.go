package build

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestRequireIsCallableAndThrows pins slice G0.2c of the CommonJS module shape:
// require is a callable global. A module reads typeof require as "function" and a
// require(specifier) call runs, so the name resolves to a value the program can
// both inspect and call. require is typed any, so the call has no signature to
// bind its argument against; the lowerer routes it through the dynamic call path,
// which boxes the specifier and passes it to the package-level require function
// value rather than dropping it the way the declared-parameter path would. The
// loader itself throws until the module system lands, so the call reports that the
// module cannot be found, and the message carries the specifier the call passed.
func TestRequireIsCallableAndThrows(t *testing.T) {
	dir := t.TempDir()
	entry := filepath.Join(dir, "mod.js")
	src := "console.log(typeof require);\n" +
		"try { require('./nope'); } catch (e) { console.log(e.message); }\n"
	if err := os.WriteFile(entry, []byte(src), 0o644); err != nil {
		t.Fatalf("write entry: %v", err)
	}
	bin := filepath.Join(dir, "prog")
	if err := Build(Options{Entry: entry, Output: bin}); err != nil {
		t.Fatalf("build module calling require: %v", err)
	}
	got, err := exec.Command(bin).CombinedOutput()
	if err != nil {
		t.Fatalf("run program: %v (%s)", err, got)
	}
	// typeof require is "function", and the loader throws with the specifier the
	// call passed, so the caught message names the module that was not found.
	if want := "function\nCannot find module './nope'\n"; string(got) != want {
		t.Fatalf("want %q, got %q", want, got)
	}
	// The specifier must reach the loader through the box: a dropped argument would
	// leave the message reading "undefined" rather than the passed path.
	if strings.Contains(string(got), "undefined") {
		t.Fatalf("require argument was dropped, message read undefined: %q", got)
	}
}
