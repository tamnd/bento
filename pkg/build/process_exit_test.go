package build

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// buildAndRunFile compiles a single self-contained entry and returns its combined
// output, failing on a build or run error. It writes the one source under a fresh
// temp directory so a test names only its own file.
func buildAndRunFile(t *testing.T, name, src string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	bin := filepath.Join(dir, "prog")
	if err := Build(Options{Entry: path, Output: bin}); err != nil {
		t.Fatalf("build %s: %v", name, err)
	}
	got, err := exec.Command(bin).CombinedOutput()
	if err != nil {
		t.Fatalf("run %s: %v (%s)", name, err, got)
	}
	return string(got)
}

// TestProcessExitRunsCallbacks pins slice G0.4: a process.on('exit', fn) listener
// runs after the top-level body completes, in registration order. The body logs
// first, then each exit callback logs, so a run that skipped the exit phase or ran
// the callbacks eagerly would print in the wrong order. Node prints the body line,
// then the two exit lines in the order they registered.
func TestProcessExitRunsCallbacks(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"process.on('exit', function () { console.log('exit-a'); });\n"+
			"process.on('exit', () => { console.log('exit-b'); });\n"+
			"console.log('body');\n")
	if want := "body\nexit-a\nexit-b\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestProcessExitCallbackSeesModuleState pins that an exit callback observes the
// module state as it stands at exit, not at registration: the listener reads a
// counter a later statement incremented, so it must run after the whole body. Node
// prints the final counter value the callback reads.
func TestProcessExitCallbackSeesModuleState(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"let n = 0;\n"+
			"process.on('exit', () => { console.log('final', n); });\n"+
			"n = 41;\n"+
			"n = n + 1;\n")
	if want := "final 42\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
