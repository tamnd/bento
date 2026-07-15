package adapter

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

// TestNoTypeScriptGoLeak fails if any bento package outside this one imports
// microsoft/typescript-go directly. This is the machine enforcement of the
// 04_frontend_typescript_go.md section 3 rule that keeps the version-churn blast
// radius to a single package: only pkg/frontend/adapter may name typescript-go,
// and everything else speaks bento's own vocabulary.
//
// It also guards the current reality that bento imports no typescript-go package
// at all, because the upstream compiler is under internal/ and cannot be
// imported yet. The day a real adapter lands, this test keeps the import
// confined here.
//
// The list runs with -e so a sibling package's throwaway compile dirs cannot make
// it flake. The equivalence tests in pkg/lower write and delete eqrun-* module
// dirs at the repo root while they run, and the CI equivalence job runs those
// alongside this package in one go test invocation, so one of those dirs can vanish
// mid-walk and fail a plain list with "no such file or directory". -e turns that
// transient filesystem error into a per-package Error field instead of a nonzero
// exit, and the scan below is unaffected: a real bento package still loads and
// reports its imports, so a genuine typescript-go leak is still caught.
func TestNoTypeScriptGoLeak(t *testing.T) {
	out, err := exec.Command("go", "list", "-e", "-deps", "-json", "github.com/tamnd/bento/...").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}

	const tsgo = "github.com/microsoft/typescript-go"
	const allowed = "github.com/tamnd/bento/pkg/frontend/adapter"

	dec := json.NewDecoder(strings.NewReader(string(out)))
	for dec.More() {
		var pkg struct {
			ImportPath string
			Imports    []string
		}
		if err := dec.Decode(&pkg); err != nil {
			t.Fatalf("decode go list output: %v", err)
		}
		if !strings.HasPrefix(pkg.ImportPath, "github.com/tamnd/bento") {
			continue
		}
		if pkg.ImportPath == allowed {
			continue
		}
		for _, imp := range pkg.Imports {
			if strings.HasPrefix(imp, tsgo) {
				t.Errorf("%s imports typescript-go directly: %s", pkg.ImportPath, imp)
			}
		}
	}
}

// TestRealAdapterSatisfiesInterface is a compile-time guarantee that the real
// checker-backed adapter is a full TSAdapter. It is the only implementation:
// every test drives the real fork, so there is no second double to keep in sync.
func TestRealAdapterSatisfiesInterface(t *testing.T) {
	var _ TSAdapter = (*RealAdapter)(nil)
	if !RealAdapterAvailable() {
		t.Error("RealAdapterAvailable is false, but a fork revision is pinned")
	}
}
