package lower

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/tamnd/bento/pkg/engine"
	_ "github.com/tamnd/bento/pkg/engine/quickjs" // registers the default backend
	"github.com/tamnd/bento/pkg/frontend"
)

// This file covers the program assembler two ways. The golden cases pin the
// exact Go source RenderProgram emits for a checked-in .ts module, so a reviewer
// reads the whole translation and a regression shows up as a diff. The
// equivalence cases go further and prove the assembled program means the same
// thing the TypeScript does: the module is run through bento's engine with a
// process shim that captures what it writes, the same module is lowered and
// compiled and run by the Go toolchain, and the two must print the identical
// bytes. Together they are the "bento build works end to end" guarantee at the
// program level, the shape the native build and the benchmarks compile.

// entryFile returns the single non-library source file of a compiled program,
// the module the assembler lowers. The ambient Node declarations bento injects
// are a FileDTS, so they are skipped; exactly one user module remains.
func entryFile(t *testing.T, prog *frontend.Program) frontend.Node {
	t.Helper()
	var entry frontend.Node
	found := false
	for _, sf := range prog.SourceFiles() {
		if sf.File().Kind == frontend.FileDTS {
			continue
		}
		if found {
			t.Fatal("more than one non-library source file")
		}
		entry, found = sf, true
	}
	if !found {
		t.Fatal("no entry source file")
	}
	return entry
}

// renderProgram compiles a checked-in module and assembles it to Go source. A
// hand-back here is a test failure: every program case is inside the lowerable
// subset by construction.
func renderProgram(t *testing.T, src string) string {
	t.Helper()
	prog := compile(t, src)
	r := NewRenderer(prog)
	p, err := r.RenderProgram(entryFile(t, prog))
	if err != nil {
		t.Fatalf("RenderProgram: %v", err)
	}
	return p.Source
}

// TestRenderProgramGoldens pins the assembled Go source for each checked-in
// module against its golden file, rewritten with -update.
func TestRenderProgramGoldens(t *testing.T) {
	cases := []struct {
		name   string
		file   string
		golden string
	}{
		{"sumLoop", "prog_sum_loop", "prog_sum_loop.golden"},
		{"funcArea", "prog_func_area", "prog_func_area.golden"},
		{"stdoutWrite", "prog_stdout_write", "prog_stdout_write.golden"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checkGolden(t, tc.golden, renderProgram(t, readTS(t, tc.file)))
		})
	}
}

// TestProgramTSAndGoAgree runs each module both ways and fails on any
// divergence in what it writes to standard output. It is skipped when the Go
// toolchain is not on PATH, because the subject side compiles and runs real Go.
func TestProgramTSAndGoAgree(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; program equivalence test needs it to run generated Go")
	}
	cases := []struct {
		name string
		file string
	}{
		{"sumLoop", "prog_sum_loop"},
		{"funcArea", "prog_func_area"},
		{"stdoutWrite", "prog_stdout_write"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := readTS(t, tc.file)
			want := runProgramTS(t, src)
			got := runProgramGo(t, src)
			if got != want {
				t.Errorf("stdout diverged:\n--- generated Go ---\n%q\n--- TypeScript ---\n%q", got, want)
			}
		})
	}
}

// processShim declares the minimal process and console globals that capture what
// the module writes to standard output into __bentoOut, so the engine side of the
// equivalence test observes the same bytes the compiled program prints. It is
// deliberately small: it mirrors only the surface the compute workloads use
// (console.log and process.stdout.write), which is exactly what the value-model
// lowering implements, so the two sides are compared on the same contract.
// console.log joins its arguments with a space and a trailing newline, and
// Array.prototype.join stringifies each with the same ToString value.ConsoleLog's
// stringified parts carry, so the captured text matches the compiled program's.
const processShim = `globalThis.__bentoOut = "";
var process = {
	stdout: { write: function (s) { globalThis.__bentoOut += s; return true; } },
	stderr: { write: function (s) { return true; } },
	env: {},
	argv: [],
};
function __bentoLog() { globalThis.__bentoOut += Array.prototype.join.call(arguments, " ") + "\n"; }
var console = {
	log: __bentoLog,
	info: __bentoLog,
	debug: __bentoLog,
	error: function () {},
	warn: function () {},
};
`

// runProgramTS is the reference: it transpiles the module to JavaScript, runs it
// in the engine under the process shim as a global script, and returns what the
// module wrote to standard output. The module has no imports or exports, so a
// global Eval reproduces its runtime meaning without a module scope.
func runProgramTS(t *testing.T, src string) string {
	t.Helper()
	js, err := frontend.Transpile(src, frontend.Options{Filename: "m.ts"})
	if err != nil {
		t.Fatalf("Transpile: %v", err)
	}
	eng, err := engine.New("")
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	defer func() { _ = eng.Close() }()
	if _, err := eng.Eval("shim.js", processShim); err != nil {
		t.Fatalf("Eval shim: %v", err)
	}
	if _, err := eng.Eval("m.js", js.Code); err != nil {
		t.Fatalf("Eval module: %v", err)
	}
	out, err := eng.Eval("read.js", "globalThis.__bentoOut")
	if err != nil {
		t.Fatalf("Eval read: %v", err)
	}
	s, ok := out.(string)
	if !ok {
		t.Fatalf("captured output is %T (%v), want a string", out, out)
	}
	return s
}

// runProgramGo is the subject: it assembles the module to a Go program, compiles
// and runs it with the Go toolchain, and returns what it printed to standard
// output. Like the function-level harness it builds inside the repository tree
// so the program links the real value package under bento's own go.mod with no
// separate module wiring, keeping the test offline. Standard error is captured
// apart from standard output so a build message never pollutes the comparison.
func runProgramGo(t *testing.T, src string) string {
	t.Helper()
	source := renderProgram(t, src)
	dir, err := os.MkdirTemp(repoRoot(t), "progrun-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go run failed: %v\n--- program ---\n%s\n--- stderr ---\n%s", err, source, stderr.String())
	}
	return stdout.String()
}
