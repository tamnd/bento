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

// TestDefaultExportFunctionComposes pins the default export and import: a sibling
// that default-exports a function composes as the package-level Default func bento
// gives a default export, and a default import of it calls that Go name. The
// function's own name does not survive the default export, in Go as in JavaScript,
// so both the declaration and the call spell Default.
func TestDefaultExportFunctionComposes(t *testing.T) {
	out, err := compileModule(t, "main.ts", map[string]string{
		"dbl.ts":  "export default function double(n: number): number { return n * 2; }\n",
		"main.ts": "import double from \"./dbl\";\nconsole.log(double(4));\n",
	})
	if err != nil {
		t.Fatalf("a default function export should compose, got: %v", err)
	}
	if !strings.Contains(out, "func Default(n float64) float64") {
		t.Fatalf("expected the default export to lower to `func Default`, got:\n%s", out)
	}
	if !strings.Contains(out, "Default(4)") {
		t.Fatalf("expected the default import to call `Default(4)`, got:\n%s", out)
	}
}

// TestDefaultExportFunctionRunsComposed carries the default program through the
// build and runs it, so the Default name agreement holds at runtime: double(4) is
// 8.
func TestDefaultExportFunctionRunsComposed(t *testing.T) {
	dir := t.TempDir()
	for name, src := range map[string]string{
		"dbl.ts":  "export default function double(n: number): number { return n * 2; }\n",
		"main.ts": "import double from \"./dbl\";\nconsole.log(double(4));\n",
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
	if string(got) != "8\n" {
		t.Fatalf("want 8, got %q", got)
	}
}

// TestMixedDefaultAndNamedImportComposes pins that one import can carry both a
// default binding and a named list, `import def, { a } from`, composing each to its
// Go name in the same statement: the default calls Default and the named binding
// calls the exported name.
func TestMixedDefaultAndNamedImportComposes(t *testing.T) {
	out, err := compileModule(t, "main.ts", map[string]string{
		"m.ts":    "export function inc(n: number): number { return n + 1; }\nexport default function dbl(n: number): number { return n * 2; }\n",
		"main.ts": "import dbl, { inc } from \"./m\";\nconsole.log(dbl(inc(3)));\n",
	})
	if err != nil {
		t.Fatalf("a mixed default and named import should compose, got: %v", err)
	}
	if !strings.Contains(out, "Default(Inc(3))") {
		t.Fatalf("expected the mixed import to call `Default(Inc(3))`, got:\n%s", out)
	}
}

// TestNamespaceImportComposesMemberCall pins that a namespace import of a sibling
// (import * as m) composes: a member call on the binding, m.inc(1), resolves to the
// export's package-level Go func and lowers to a direct call, the same Inc the
// declaration took, with no runtime struct standing behind the namespace.
func TestNamespaceImportComposesMemberCall(t *testing.T) {
	out, err := compileModule(t, "main.ts", map[string]string{
		"m.ts":    "export function inc(n: number): number { return n + 1; }\n",
		"main.ts": "import * as m from \"./m\";\nconsole.log(m.inc(1));\n",
	})
	if err != nil {
		t.Fatalf("a namespace member call should compose, got: %v", err)
	}
	if !strings.Contains(out, "func Inc(n float64) float64") {
		t.Fatalf("expected the export to lower as a package func, got:\n%s", out)
	}
	if !strings.Contains(out, "Inc(1)") {
		t.Fatalf("expected the namespace member call to spell `Inc(1)`, got:\n%s", out)
	}
}

// TestNamespaceMemberValueReadHandsBack pins that reading a namespace member as a
// value rather than calling it hands back: the export has no Go value the namespace
// materializes, so a value read needs the struct this slice does not build.
func TestNamespaceMemberValueReadHandsBack(t *testing.T) {
	_, err := compileModule(t, "main.ts", map[string]string{
		"m.ts":    "export function inc(n: number): number { return n + 1; }\n",
		"main.ts": "import * as m from \"./m\";\nconst f = m.inc;\nconsole.log(f(1));\n",
	})
	if err == nil {
		t.Fatal("a namespace member read as a value should hand back, but it lowered")
	}
}

// TestNamespaceImportRunsComposed carries the namespace member call through the full
// build and runs it, proving the resolved Go name agrees at runtime: inc(1) is 2.
func TestNamespaceImportRunsComposed(t *testing.T) {
	dir := t.TempDir()
	for name, src := range map[string]string{
		"m.ts":    "export function inc(n: number): number { return n + 1; }\n",
		"main.ts": "import * as m from \"./m\";\nconsole.log(m.inc(1));\n",
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
	if string(got) != "2\n" {
		t.Fatalf("want 2, got %q", got)
	}
}

// TestReExportNamedComposes pins that a module re-exporting another's binding,
// `export { inc } from "./leaf"`, composes: the re-export forwards the binding and
// declares nothing of its own, so an entry importing the name through the middle
// module resolves the alias chain to the leaf's package-level Go func and calls Inc
// directly, with the re-export statement itself carrying no code.
func TestReExportNamedComposes(t *testing.T) {
	out, err := compileModule(t, "main.ts", map[string]string{
		"leaf.ts": "export function inc(n: number): number { return n + 1; }\n",
		"mid.ts":  "export { inc } from \"./leaf\";\n",
		"main.ts": "import { inc } from \"./mid\";\nconsole.log(inc(1));\n",
	})
	if err != nil {
		t.Fatalf("a named re-export should compose, got: %v", err)
	}
	if !strings.Contains(out, "func Inc(n float64) float64") {
		t.Fatalf("expected the leaf export to lower as a package func, got:\n%s", out)
	}
	if !strings.Contains(out, "Inc(1)") {
		t.Fatalf("expected the re-exported import to call `Inc(1)`, got:\n%s", out)
	}
}

// TestReExportAliasedComposes pins that renaming on the way out,
// `export { inc as bump } from "./leaf"`, composes the same way: the entry imports
// bump and the alias chain resolves through the rename to the leaf's Inc.
func TestReExportAliasedComposes(t *testing.T) {
	out, err := compileModule(t, "main.ts", map[string]string{
		"leaf.ts": "export function inc(n: number): number { return n + 1; }\n",
		"mid.ts":  "export { inc as bump } from \"./leaf\";\n",
		"main.ts": "import { bump } from \"./mid\";\nconsole.log(bump(1));\n",
	})
	if err != nil {
		t.Fatalf("an aliased re-export should compose, got: %v", err)
	}
	if !strings.Contains(out, "Inc(1)") {
		t.Fatalf("expected the aliased re-exported call to spell `Inc(1)`, got:\n%s", out)
	}
}

// TestReExportStarComposes pins that a star re-export, `export * from "./leaf"`,
// forwards the leaf's whole named surface: the entry imports one of those names
// through the middle module and it resolves to the leaf's Go func.
func TestReExportStarComposes(t *testing.T) {
	out, err := compileModule(t, "main.ts", map[string]string{
		"leaf.ts": "export function inc(n: number): number { return n + 1; }\n",
		"mid.ts":  "export * from \"./leaf\";\n",
		"main.ts": "import { inc } from \"./mid\";\nconsole.log(inc(1));\n",
	})
	if err != nil {
		t.Fatalf("a star re-export should compose, got: %v", err)
	}
	if !strings.Contains(out, "Inc(1)") {
		t.Fatalf("expected the star re-exported call to spell `Inc(1)`, got:\n%s", out)
	}
}

// TestReExportRunsComposed carries a named re-export through the full build and
// runs it, proving the forwarded name agrees at runtime: inc(1) through the middle
// module is 2.
func TestReExportRunsComposed(t *testing.T) {
	dir := t.TempDir()
	for name, src := range map[string]string{
		"leaf.ts": "export function inc(n: number): number { return n + 1; }\n",
		"mid.ts":  "export { inc } from \"./leaf\";\n",
		"main.ts": "import { inc } from \"./mid\";\nconsole.log(inc(1));\n",
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
	if string(got) != "2\n" {
		t.Fatalf("want 2, got %q", got)
	}
}

// TestLocalExportListHandsBack pins that a same-module `export { x }` with no `from`
// clause still hands back: it is not a re-export (it names no module), so the
// composed unit does not mistake it for a code-free forward. The binding x is a
// const whose top-level evaluation the composed unit would have to order.
func TestLocalExportListHandsBack(t *testing.T) {
	_, err := compileModule(t, "main.ts", map[string]string{
		"mid.ts":  "const x: number = 1;\nexport { x };\n",
		"main.ts": "import { x } from \"./mid\";\nconsole.log(x);\n",
	})
	if err == nil {
		t.Fatal("a local export list should hand back for a later slice, but it lowered")
	}
}
