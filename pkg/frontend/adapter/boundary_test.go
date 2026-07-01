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
func TestNoTypeScriptGoLeak(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "-json", "github.com/tamnd/bento/...").Output()
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

// TestFakeAdapterSatisfiesInterface is a compile-time guarantee that the fake is
// a full TSAdapter, so the partitioner and lowering tests can rely on it as a
// stand-in for the real checker.
func TestFakeAdapterSatisfiesInterface(t *testing.T) {
	var _ TSAdapter = (*FakeAdapter)(nil)
	if RealAdapterAvailable() {
		t.Error("RealAdapterAvailable is true, but no revision is pinned yet")
	}
}
