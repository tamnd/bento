package build

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/bento/pkg/goimport"
)

// TestGateCgoBlocksWithoutAcknowledgment proves a reached cgo import stops the
// build with the section 9.5 diagnostic when neither --allow-cgo nor CGO_ENABLED=1
// is set, so the loss of the zero-cgo cross-compile is never silent. It pins
// CGO_ENABLED off for the test so a host that exports it does not turn the gate
// into an allow. runtime/cgo stands in for a cgo library, since detection needs no
// C toolchain.
func TestGateCgoBlocksWithoutAcknowledgment(t *testing.T) {
	t.Setenv("CGO_ENABLED", "0")
	needsCgo, err := gateCgo([]string{"runtime/cgo"}, false)
	if err == nil {
		t.Fatal("a cgo import built without an acknowledgment, want the section 9.5 diagnostic")
	}
	if needsCgo {
		t.Error("gate reported cgo needed while blocking the build, want false")
	}
	msg := err.Error()
	for _, want := range []string{"requires cgo", "zero-cgo", "runtime/cgo", "--allow-cgo"} {
		if !strings.Contains(msg, want) {
			t.Errorf("diagnostic missing %q:\n%s", want, msg)
		}
	}
}

// TestGateCgoAllowedByFlag proves --allow-cgo lets a cgo import through and tells
// the link to enable cgo, the explicit opt-in the spec requires. CGO_ENABLED is
// pinned off so the pass comes from the flag alone.
func TestGateCgoAllowedByFlag(t *testing.T) {
	t.Setenv("CGO_ENABLED", "0")
	needsCgo, err := gateCgo([]string{"runtime/cgo"}, true)
	if err != nil {
		t.Fatalf("--allow-cgo build blocked: %v", err)
	}
	if !needsCgo {
		t.Error("gate did not report cgo needed after acknowledgment, want true so the link enables cgo")
	}
}

// TestGateCgoAllowedByEnv proves CGO_ENABLED=1 in the environment is the other
// acknowledgment the spec names, so a build that already opted into cgo at the
// shell proceeds without the flag.
func TestGateCgoAllowedByEnv(t *testing.T) {
	t.Setenv("CGO_ENABLED", "1")
	needsCgo, err := gateCgo([]string{"runtime/cgo"}, false)
	if err != nil {
		t.Fatalf("CGO_ENABLED=1 build blocked: %v", err)
	}
	if !needsCgo {
		t.Error("gate did not report cgo needed under CGO_ENABLED=1, want true")
	}
}

// TestGateCgoPureGoPasses proves a program that reaches only pure-Go imports keeps
// the zero-cgo path: the gate returns cgo not needed and no error, so the link
// stays CGO_ENABLED=0.
func TestGateCgoPureGoPasses(t *testing.T) {
	t.Setenv("CGO_ENABLED", "0")
	needsCgo, err := gateCgo([]string{"strings", "math"}, false)
	if err != nil {
		t.Fatalf("pure-Go imports blocked: %v", err)
	}
	if needsCgo {
		t.Error("gate reported cgo needed for pure-Go imports, want false")
	}
}

// TestCgoDiagnosticPointsAtPureGoAlternative proves the diagnostic points an author
// at modernc.org/sqlite when the cgo library is go-sqlite3, the section 9.5 hint,
// and phrases the direct case (the named import is itself the cgo library) as the
// import using cgo rather than pulling it in through a dependency.
func TestCgoDiagnosticPointsAtPureGoAlternative(t *testing.T) {
	msg := cgoDiagnostic(goimport.CgoUse{
		Import: "github.com/mattn/go-sqlite3",
		Cgo:    "github.com/mattn/go-sqlite3",
	})
	if !strings.Contains(msg, "modernc.org/sqlite") {
		t.Errorf("diagnostic does not point at the pure-Go alternative:\n%s", msg)
	}
	if !strings.Contains(msg, "go:github.com/mattn/go-sqlite3 uses cgo") {
		t.Errorf("diagnostic does not phrase the direct cgo case:\n%s", msg)
	}
}

// TestCgoDiagnosticNamesTransitiveLibrary proves that when the named import is
// pure Go but pulls a cgo package in, the diagnostic names both the import and the
// cgo dependency, so the author can see which of their imports dragged cgo in.
func TestCgoDiagnosticNamesTransitiveLibrary(t *testing.T) {
	msg := cgoDiagnostic(goimport.CgoUse{
		Import: "example.com/lib",
		Cgo:    "example.com/lib/internal/cbits",
	})
	if !strings.Contains(msg, "go:example.com/lib pulls in cgo through example.com/lib/internal/cbits") {
		t.Errorf("diagnostic does not name the import and its cgo dependency:\n%s", msg)
	}
}

// TestBuildBlocksCgoImportEndToEnd proves the gate fires through the whole build,
// not just the unit under it: a real entry that reaches a cgo library through a
// go: import is stopped with the section 9.5 diagnostic naming both the import and
// the transitive cgo package. It also exercises the entry selection that lets the
// AOT build compile a go: program at all, since the import brings its generated
// .d.ts into the program as a second source file. CGO_ENABLED is pinned off so a
// host that exports it does not turn the gate into an allow.
func TestBuildBlocksCgoImportEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a real program through the frontend and go toolchain")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the AOT build needs it")
	}
	t.Setenv("CGO_ENABLED", "0")

	dir := t.TempDir()
	entry := filepath.Join(dir, "main.ts")
	const src = `import { Marker } from "go:github.com/tamnd/bento/pkg/goimport/cgodepfixture";
console.log(Marker());
`
	if err := os.WriteFile(entry, []byte(src), 0o644); err != nil {
		t.Fatalf("write entry: %v", err)
	}

	err := Build(Options{Entry: entry, Output: filepath.Join(dir, "out"), AllowCgo: false})
	if err == nil {
		t.Fatal("a cgo go: import built without an acknowledgment, want the section 9.5 diagnostic")
	}
	msg := err.Error()
	for _, want := range []string{"requires cgo", "cgodepfixture", "runtime/cgo", "--allow-cgo"} {
		if !strings.Contains(msg, want) {
			t.Errorf("diagnostic missing %q:\n%s", want, msg)
		}
	}
}
