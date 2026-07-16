package build

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// compileModule writes a set of named .ts files to a temp dir and runs the front
// half through Compile over the given entry, so a test can assert how a
// multi-file module goal lowers without paying for a real build. The map keys are
// file names relative to the temp dir and its values are the sources; the entry
// is the file name to compile.
func compileModule(t *testing.T, entry string, files map[string]string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return Compile(filepath.Join(dir, entry))
}

// TestNamedExportFunctionComposesAsPackageFunc pins the first slice of static
// import lowering: a function a sibling module exports by name lowers to a
// package-level Go func, and an entry that imports it by name calls that same Go
// name with no runtime indirection. The two files compose into one Go program,
// so the emitted source declares `func Area` once beside `func main` and the call
// site spells `Area(3)`, the exported name the declaration took.
func TestNamedExportFunctionComposesAsPackageFunc(t *testing.T) {
	out, err := compileModule(t, "main.ts", map[string]string{
		"geo.ts":  "export function area(r: number): number { return r * r; }\n",
		"main.ts": "import { area } from \"./geo\";\nconst a: number = area(3);\nconsole.log(a);\n",
	})
	if err != nil {
		t.Fatalf("a named function export should compose across modules, got: %v", err)
	}
	if strings.Count(out, "package main") != 1 {
		t.Fatalf("expected one composed package, got:\n%s", out)
	}
	if !strings.Contains(out, "func Area(r float64) float64") {
		t.Fatalf("expected the sibling export to lower to `func Area`, got:\n%s", out)
	}
	if !strings.Contains(out, "Area(3)") {
		t.Fatalf("expected the import to call `Area(3)`, got:\n%s", out)
	}
}

// TestNamedExportFunctionRunsComposed carries the same program through the full
// build and runs it, so the composed name agreement is proven at runtime, not
// just in the emitted text: area(3) is 9.
func TestNamedExportFunctionRunsComposed(t *testing.T) {
	dir := t.TempDir()
	for name, src := range map[string]string{
		"geo.ts":  "export function area(r: number): number { return r * r; }\n",
		"main.ts": "import { area } from \"./geo\";\nconst a: number = area(3);\nconsole.log(a);\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	bin := filepath.Join(dir, "prog")
	if err := Build(Options{Entry: filepath.Join(dir, "main.ts"), Output: bin}); err != nil {
		t.Fatalf("build composed program: %v", err)
	}
	got, err := exec.Command(bin).CombinedOutput()
	if err != nil {
		t.Fatalf("run composed program: %v (%s)", err, got)
	}
	if string(got) != "9\n" {
		t.Fatalf("want 9, got %q", got)
	}
}

// TestNamedExportCallsAcrossSiblings pins that a sibling can call another
// sibling's export, not only the entry: the entry imports one function whose body
// calls a second function the same module imports, and all three compose into one
// package. The chain resolves to package Go names throughout.
func TestNamedExportCallsAcrossSiblings(t *testing.T) {
	out, err := compileModule(t, "main.ts", map[string]string{
		"square.ts": "export function square(n: number): number { return n * n; }\n",
		"geo.ts":    "import { square } from \"./square\";\nexport function area(r: number): number { return square(r); }\n",
		"main.ts":   "import { area } from \"./geo\";\nconsole.log(area(4));\n",
	})
	if err != nil {
		t.Fatalf("a sibling-to-sibling call should compose, got: %v", err)
	}
	if !strings.Contains(out, "func Square(n float64) float64") || !strings.Contains(out, "func Area(r float64) float64") {
		t.Fatalf("expected both exports to lower as package funcs, got:\n%s", out)
	}
	if !strings.Contains(out, "return Square(r)") {
		t.Fatalf("expected area to call `Square(r)`, got:\n%s", out)
	}
}

// TestConstExportHandsBack pins that an export this slice does not yet compose
// hands back rather than miscompiling. A `const` export is a top-level binding
// whose initializer runs at the module's evaluation position, an order the
// composed unit would have to preserve, so the whole unit routes to the engine
// with a NotYetLowerable reason.
func TestConstExportHandsBack(t *testing.T) {
	_, err := compileModule(t, "main.ts", map[string]string{
		"k.ts":    "export const K: number = 7;\n",
		"main.ts": "import { K } from \"./k\";\nconsole.log(K);\n",
	})
	if err == nil {
		t.Fatal("a const export should hand back for a later slice, but it lowered")
	}
}

// TestAliasedImportHandsBack pins that renaming an import on the way in hands
// back, since spelling the local alias in Go is a later slice; the reference must
// keep taking the exported name until then.
func TestAliasedImportHandsBack(t *testing.T) {
	_, err := compileModule(t, "main.ts", map[string]string{
		"geo.ts":  "export function area(r: number): number { return r * r; }\n",
		"main.ts": "import { area as f } from \"./geo\";\nconsole.log(f(2));\n",
	})
	if err == nil {
		t.Fatal("an aliased import should hand back for a later slice, but it lowered")
	}
}
