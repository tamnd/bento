package conformance

import (
	"bytes"
	"errors"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/bento/pkg/build"
)

// stamp is the fixed identifier written into every golden's generated header, in
// place of the version and commit the CLI records, so a golden names the corpus
// rather than the exact bento build that produced it and does not churn when that
// build's version moves. Regenerating with -update writes this stamp and the check
// compares against it, so the header is deterministic across machines.
const stamp = "conformance"

// update, set by `go test -update`, rewrites each emit.golden from the current
// lowering instead of checking against the committed one. It never touches
// oracle.txt: the expected output is the ground truth of correct JavaScript
// semantics, authored by hand, so it is never derived from the compiler it holds
// honest.
var update = flag.Bool("update", false, "rewrite emit.golden files from the current lowering")

// feature, set by `go test -feature math.hypot`, narrows the run to fixtures whose
// fixture.toml tags them with that feature. It is the incremental seam: working on
// one lowering path reruns only that path's fixtures. Empty runs the whole corpus.
var feature = flag.String("feature", "", "run only fixtures tagged with this feature")

// fixtures discovers the corpus once and applies the -feature filter. A discovery
// error or an empty corpus is fatal, since a run that silently checks nothing is
// worse than a red one.
func fixtures(t *testing.T) []Fixture {
	t.Helper()
	all, err := Discover("fixtures")
	if err != nil {
		t.Fatalf("discover fixtures: %v", err)
	}
	if len(all) == 0 {
		t.Fatal("no fixtures found under fixtures/")
	}
	if *feature == "" {
		return all
	}
	var kept []Fixture
	for _, f := range all {
		if f.Meta.Feature == *feature {
			kept = append(kept, f)
		}
	}
	if len(kept) == 0 {
		t.Fatalf("no fixtures tagged feature %q", *feature)
	}
	return kept
}

// TestMain redirects the golden go run builds to a build cache kept apart from the
// developer's shared GOCACHE, and bounds it. Each golden is a one-shot program the
// oracle compiles once and never reuses: its linked binary lands in the cache and,
// on a machine that builds every day, never ages out of Go's five-day retention, so
// sweep after sweep leaves gigabytes of dead binaries that fill the disk. A
// dedicated cache keeps the stdlib and pkg/value warm across runs so the oracle
// stays fast, and the teardown drops the whole cache once it grows past a cap, so
// one-shot golden churn is bounded and the developer's own cache never sees it.
func TestMain(m *testing.M) {
	cache := filepath.Join(os.TempDir(), "bento-conformance-gocache")
	if err := os.Setenv("GOCACHE", cache); err != nil {
		panic(err)
	}
	code := m.Run()
	// A cache past this size is mostly dead one-shot golden binaries, so drop it
	// rather than let it grow; the next run rewarms the stdlib into a fresh one.
	const maxGoldenCache = 2 << 30 // 2 GiB
	if dirSize(cache) > maxGoldenCache {
		_ = os.RemoveAll(cache)
	}
	os.Exit(code)
}

// dirSize sums the bytes of every file under root, the check TestMain uses to decide
// whether the golden cache has grown past its cap. A missing root sums to zero.
func dirSize(root string) int64 {
	var total int64
	_ = filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total
}

// TestStructure proves the corpus is well formed before any check runs its content.
// Every fixture directory must carry an input.ts and make at least one claim, and a
// handback fixture must emit nothing, so a directory added without a golden, an
// oracle, or a handback reason fails here rather than sitting unchecked.
func TestStructure(t *testing.T) {
	for _, f := range fixtures(t) {
		t.Run(f.Slug, func(t *testing.T) {
			if !fileExists(f.Input) {
				t.Fatalf("%s: missing input.ts", f.Slug)
			}
			if f.Meta.Skip != "" {
				return
			}
			if f.Meta.Handback != "" {
				if f.HasGolden {
					t.Errorf("%s: a handback fixture must not ship an emit.golden, it emits no Go", f.Slug)
				}
				if f.HasOracle {
					t.Errorf("%s: a handback fixture must not ship an oracle.txt, it never runs", f.Slug)
				}
				return
			}
			if *update {
				// Under -update the goldens are being written now, so their
				// presence is checked on the next normal run, not this one.
				return
			}
			if !f.HasGolden && !f.HasOracle {
				t.Errorf("%s: fixture makes no claim, add an emit.golden, an oracle.txt, or a handback reason", f.Slug)
			}
			if f.HasOracle && !f.HasGolden {
				t.Errorf("%s: has an oracle.txt but no emit.golden to run against it", f.Slug)
			}
			if f.HasGolden && !f.HasOracle {
				t.Errorf("%s: has an emit.golden but no oracle.txt, so the emitted Go is never proven to run", f.Slug)
			}
		})
	}
}

// TestGoldenRender proves each emit.golden is exactly the Go bento lowers its
// input.ts to today. It re-runs the real front half through build.EmitGo and
// compares byte for byte, so a lowering change that would alter the generated code
// shows up as a diff a reviewer sees before it lands. With -update it writes the new
// lowering instead of failing, the one supported way to move a golden.
func TestGoldenRender(t *testing.T) {
	for _, f := range fixtures(t) {
		if f.Meta.Skip != "" || f.Meta.Handback != "" {
			continue
		}
		t.Run(f.Slug, func(t *testing.T) {
			t.Parallel()
			got, err := build.EmitGo(f.Input, stamp)
			if err != nil {
				t.Fatalf("EmitGo(%s): %v", f.Slug, err)
			}
			if *update {
				if err := os.WriteFile(f.Golden, []byte(got), 0o644); err != nil {
					t.Fatalf("write golden: %v", err)
				}
				return
			}
			want, err := os.ReadFile(f.Golden)
			if err != nil {
				t.Fatalf("read golden (run with -update to create it): %v", err)
			}
			if got != string(want) {
				t.Errorf("generated Go for %s does not match emit.golden\nrun `go test -update` after reviewing the change\n--- got ---\n%s", f.Slug, got)
			}
		})
	}
}

// TestHandback proves a fixture outside the lowerable subset hands its whole unit
// back cleanly: build.EmitGo must fail with a message containing the fixture's
// declared handback reason, and no Go is produced. This keeps the pending surface
// tested too, so a construct marked not-ready cannot start silently emitting wrong
// Go without a fixture catching it.
func TestHandback(t *testing.T) {
	for _, f := range fixtures(t) {
		if f.Meta.Skip != "" || f.Meta.Handback == "" {
			continue
		}
		t.Run(f.Slug, func(t *testing.T) {
			t.Parallel()
			_, err := build.EmitGo(f.Input, stamp)
			if err == nil {
				t.Fatalf("%s: expected a handback with reason %q, but lowering succeeded", f.Slug, f.Meta.Handback)
			}
			if !strings.Contains(err.Error(), f.Meta.Handback) {
				t.Errorf("%s: handback reason mismatch\nwant substring: %q\ngot: %v", f.Slug, f.Meta.Handback, err)
			}
		})
	}
}

// TestOracle proves the crossing works all the way through: it compiles the
// committed emit.golden against bento's runtime, runs it, and checks its stdout and
// exit code against oracle.txt. This is the end-to-end proof, the oracle is the
// behavior a developer expects and the golden is the code that must produce it, so a
// runtime regression that still compiles is caught by what it prints. It runs the
// checked-in golden, not a fresh lowering, so the artifact the corpus ships is the
// one exercised.
func TestOracle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping golden compile-and-run under -short")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; running a golden needs it")
	}
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for _, f := range fixtures(t) {
		if f.Meta.Skip != "" || f.Meta.Handback != "" || !f.HasOracle {
			continue
		}
		t.Run(f.Slug, func(t *testing.T) {
			t.Parallel()
			golden, err := os.ReadFile(f.Golden)
			if err != nil {
				t.Fatalf("read golden: %v", err)
			}
			oracleText, err := os.ReadFile(f.Oracle)
			if err != nil {
				t.Fatalf("read oracle: %v", err)
			}
			want, err := ParseOracle(string(oracleText))
			if err != nil {
				t.Fatalf("parse oracle: %v", err)
			}
			stdout, exit := runGolden(t, root, golden)
			if normalizeOut(stdout) != normalizeOut(want.Stdout) {
				t.Errorf("%s stdout mismatch\n--- got ---\n%s\n--- want ---\n%s", f.Slug, stdout, want.Stdout)
			}
			if exit != want.Exit {
				t.Errorf("%s exit code = %d, want %d", f.Slug, exit, want.Exit)
			}
		})
	}
}

// runGolden writes the golden into a scratch directory inside this module and runs
// it with `go run`, returning its stdout and exit code. The scratch directory sits
// under the module tree so the golden's import of bento's value package resolves
// from this module's requirements with no separate go.mod, the same way bento's own
// build compiles a program inside its module tree.
func runGolden(t *testing.T, root string, golden []byte) (string, int) {
	t.Helper()
	dir, err := os.MkdirTemp(root, "goldenrun-")
	if err != nil {
		t.Fatalf("scratch dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), golden, 0o644); err != nil {
		t.Fatalf("write golden main: %v", err)
	}
	// The build inherits GOCACHE from TestMain, which points it at the dedicated,
	// bounded golden cache rather than the developer's shared one, so a one-shot
	// golden binary never accumulates in the cache everyday builds rely on.
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return stdout.String(), exit.ExitCode()
		}
		t.Fatalf("go run failed to start: %v\n--- stderr ---\n%s", err, stderr.String())
	}
	return stdout.String(), 0
}

// normalizeOut trims trailing newlines so a one-line expected value in oracle.txt
// compares equal to the newline console.log leaves on real stdout.
func normalizeOut(s string) string {
	return strings.TrimRight(s, "\n")
}
