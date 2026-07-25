package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestRuntimeRootsCoverEveryGoldenImport is the guard that keeps the verdict cache
// honest. The cache is only sound if runtimeRoots names every bento package a golden
// links against: a package left out could change behaviour without invalidating a single
// entry, and the corpus would then replay stale output and report green. So this scans
// the whole corpus for bento imports and fails if one is not covered.
//
// It is what lets a new import be added safely. The lowering that first emits a call
// into a third runtime package makes this test red, which is the reminder to add the
// package to runtimeRoots rather than a silent hole in the cache.
func TestRuntimeRootsCoverEveryGoldenImport(t *testing.T) {
	covered := map[string]bool{}
	for _, root := range runtimeRoots {
		covered["github.com/tamnd/bento/pkg/"+filepath.Base(root)] = true
	}

	importPat := regexp.MustCompile(`"(github\.com/tamnd/bento/[^"]+)"`)
	seen := map[string]string{}
	for _, f := range mustDiscover(t) {
		if !f.HasGolden {
			continue
		}
		data, err := os.ReadFile(f.Golden)
		if err != nil {
			t.Fatalf("read golden for %s: %v", f.Slug, err)
		}
		for _, m := range importPat.FindAllStringSubmatch(string(data), -1) {
			if _, ok := seen[m[1]]; !ok {
				seen[m[1]] = f.Slug
			}
		}
	}
	if len(seen) == 0 {
		t.Fatal("no bento imports found in any golden; the scan is not reading the corpus")
	}
	for path, slug := range seen {
		if !covered[path] {
			t.Errorf("golden %s imports %s, which runtimeRoots does not cover; "+
				"add it there or the verdict cache will serve stale output when it changes", slug, path)
		}
	}
}

// mustDiscover reads the corpus for a test that needs it outside the -feature filter,
// since a coverage check has to see every fixture rather than the narrowed set.
func mustDiscover(t *testing.T) []Fixture {
	t.Helper()
	all, err := Discover("fixtures")
	if err != nil {
		t.Fatalf("discover fixtures: %v", err)
	}
	if len(all) == 0 {
		t.Fatal("no fixtures found under fixtures/")
	}
	return all
}

// TestVerdictKeyRespondsToEveryInput pins what the cache key covers. Each of these
// inputs changes what a golden prints, so each must produce a different key; a key that
// ignored one would serve the wrong output after exactly the change the cache exists to
// notice.
func TestVerdictKeyRespondsToEveryInput(t *testing.T) {
	// Caching is turned back on for this test, since a run that disabled it would
	// otherwise get no key at all and fail on the harness's own setting rather than on
	// anything about the key.
	t.Setenv("BENTO_CONFORMANCE_NO_CACHE", "")
	golden := []byte("package main\n\nfunc main() {}\n")
	env := map[string]string{"TZ": "UTC"}

	base, ok := VerdictKey(golden, env)
	if !ok {
		t.Fatal("expected the key to be computable from the module tree")
	}

	changedGolden, _ := VerdictKey([]byte("package main\n\nfunc main() { println(1) }\n"), env)
	if changedGolden == base {
		t.Error("a different golden must key differently")
	}
	changedEnv, _ := VerdictKey(golden, map[string]string{"TZ": "Asia/Tokyo"})
	if changedEnv == base {
		t.Error("a different pinned environment must key differently")
	}
	extraEnv, _ := VerdictKey(golden, map[string]string{"TZ": "UTC", "LANG": "C"})
	if extraEnv == base {
		t.Error("an additional pinned variable must key differently")
	}
	noEnv, _ := VerdictKey(golden, nil)
	if noEnv == base {
		t.Error("dropping the pinned environment must key differently")
	}
}

// TestVerdictKeyIsStableAcrossEnvOrdering pins that the key does not depend on the order
// a fixture's environment map happened to be built in. Go randomizes map iteration, so a
// key that hashed the map as it ranged would differ run to run and never hit.
func TestVerdictKeyIsStableAcrossEnvOrdering(t *testing.T) {
	t.Setenv("BENTO_CONFORMANCE_NO_CACHE", "")
	golden := []byte("package main\n\nfunc main() {}\n")
	first, ok := VerdictKey(golden, map[string]string{"A": "1", "B": "2", "C": "3"})
	if !ok {
		t.Fatal("expected the key to be computable from the module tree")
	}
	for i := 0; i < 8; i++ {
		again, _ := VerdictKey(golden, map[string]string{"C": "3", "B": "2", "A": "1"})
		if again != first {
			t.Fatalf("key changed across map orderings: %s vs %s", first, again)
		}
	}
}

// TestVerdictRoundTrips pins the store and lookup pair: what a run records is what the
// next run reads back, including a non-zero exit and output that is not valid UTF-8 free
// text. A miss on an unknown key must be a miss rather than an error, since the caller
// treats it as "run the golden".
func TestVerdictRoundTrips(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())

	key := "test-key-" + t.Name()
	if _, hit := LookupVerdict(key); hit {
		t.Fatal("a key never stored must miss")
	}
	want := Verdict{Stdout: "line one\nline two\n", Exit: 3}
	StoreVerdict(key, want)
	got, hit := LookupVerdict(key)
	if !hit {
		t.Fatal("a stored key must hit")
	}
	if got != want {
		t.Fatalf("want %+v, got %+v", got, want)
	}
}

// TestCachingDisabledTurnsOffKeying pins the escape hatch: with
// BENTO_CONFORMANCE_NO_CACHE set, no key is computable, so every golden compiles and runs
// for real. That is how a suspicious cached result gets confirmed against the toolchain.
func TestCachingDisabledTurnsOffKeying(t *testing.T) {
	t.Setenv("BENTO_CONFORMANCE_NO_CACHE", "1")
	if _, ok := VerdictKey([]byte("package main"), nil); ok {
		t.Error("caching must be off when BENTO_CONFORMANCE_NO_CACHE is set")
	}
}

// TestFingerprintSkipsTestSources pins the distinction that makes the cache worth
// having: a _test.go file is never linked into a golden, so it must not count toward the
// fingerprint. Test sources are the most frequently edited files in the runtime packages,
// and counting them would invalidate the whole corpus on every runtime test edit.
func TestFingerprintSkipsTestSources(t *testing.T) {
	cases := []struct {
		name   string
		linked bool
	}{
		{"value.go", true},
		{"timers.go", true},
		{"value_test.go", false},
		{"timers_test.go", false},
		{"_test.go", false},
		{"README.md", false},
		{"testdata.go", true},
	}
	for _, c := range cases {
		if got := isLinkedSource(c.name); got != c.linked {
			t.Errorf("isLinkedSource(%q) = %v, want %v", c.name, got, c.linked)
		}
	}
}

// TestRuntimeFingerprintIsNonEmptyAndStable pins that the fingerprint actually reads the
// runtime tree and settles on one value for the process, which is what makes two fixtures
// in the same run agree on a key.
func TestRuntimeFingerprintIsNonEmptyAndStable(t *testing.T) {
	fp, ok := runtimeFingerprint()
	if !ok {
		t.Fatal("expected the runtime fingerprint to be computable from the module tree")
	}
	if strings.TrimSpace(fp) == "" {
		t.Fatal("the fingerprint must not be empty")
	}
	again, _ := runtimeFingerprint()
	if again != fp {
		t.Fatalf("the fingerprint must be stable within a process: %s vs %s", fp, again)
	}
}
