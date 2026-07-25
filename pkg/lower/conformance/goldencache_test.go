package conformance

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestGoldenCacheDirIsOverridable pins the escape hatch that lets two corpora run on one
// machine without sharing a build cache. The default has to stay shared, since reusing
// the stdlib the last run compiled is most of why a warm run is quick.
func TestGoldenCacheDirIsOverridable(t *testing.T) {
	def := goldenCacheDir()
	if def != filepath.Join(os.TempDir(), "bento-conformance-gocache") {
		t.Errorf("default golden cache = %q, want the shared one under the temporary directory", def)
	}
	t.Setenv("BENTO_CONFORMANCE_GOCACHE", "/somewhere/private")
	if got := goldenCacheDir(); got != "/somewhere/private" {
		t.Errorf("golden cache = %q, want the override", got)
	}
}

// TestOnlyTheLastRunOutMayTrim is the guard against the failure that motivated the mark:
// a second run deleted the cache the first was still building against, and the first
// failed with an object file missing from deep inside the go tool. Nothing about that
// error says "two runs shared a machine", so it burned an afternoon reading as a
// compiler bug. A run that is not alone must decline to trim.
func TestOnlyTheLastRunOutMayTrim(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "gocache")

	first := markRunner(cache)
	second := markRunner(cache)
	if first() {
		t.Error("a run that leaves while another is still marked must not trim")
	}
	if !second() {
		t.Error("the last run out must be allowed to trim")
	}
}

// TestASingleRunMayTrim pins the ordinary case, one corpus on a machine, where the cap
// has to keep working; a mark that never let anyone trim would leave the cache growing
// without bound.
func TestASingleRunMayTrim(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "gocache")
	if !markRunner(cache)() {
		t.Error("a run with no other marks present must be allowed to trim")
	}
}

// TestAStaleMarkDoesNotBlockTrimmingForever pins the recovery path. A killed run leaves
// its mark behind, and treating that as a live run would disable the size cap for good
// on any machine where a corpus was ever interrupted.
func TestAStaleMarkDoesNotBlockTrimmingForever(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "gocache")
	release := markRunner(cache)

	dir := cache + ".runners"
	abandoned := filepath.Join(dir, "run-abandoned")
	if err := os.WriteFile(abandoned, nil, 0o644); err != nil {
		t.Fatalf("write stale mark: %v", err)
	}
	old := time.Now().Add(-24 * time.Hour)
	if err := os.Chtimes(abandoned, old, old); err != nil {
		t.Fatalf("age the stale mark: %v", err)
	}
	if !release() {
		t.Error("a mark left by a run that died must not block the cap forever")
	}
}
