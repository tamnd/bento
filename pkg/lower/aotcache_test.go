package lower

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// The AOT equivalence and program tests each shell out to `go run`, which links
// a fresh binary per fixture. That link dominates the package's test time, and
// it is wasted work when nothing that would change the program's behavior has
// changed since the last run. cachedGoRun keys the recorded output on the exact
// program bytes plus a fingerprint of the value runtime the program links and
// the Go toolchain, so an unchanged fixture returns its output without building
// and the suite only pays the build cost on code that actually changed.
//
// Set BENTO_AOT_TESTCACHE=off to force every fixture to build, which is what a
// run that wants to exercise the toolchain end to end (a release check) does.

var (
	runtimeFPOnce sync.Once
	runtimeFP     string
)

// runtimeFingerprint hashes the Go sources the generated program links, the
// value runtime, together with the toolchain version. Any change to what the
// linked binary would actually do lands in this fingerprint and invalidates the
// cache, so a cache hit is only ever taken when the behavior cannot have moved.
func runtimeFingerprint() string {
	runtimeFPOnce.Do(func() {
		h := sha256.New()
		h.Write([]byte(runtime.Version()))
		h.Write([]byte{0})
		// pkg/value is the only bento package a generated program imports; the
		// rest of what it links is the standard library, which the toolchain
		// version already covers.
		root, err := filepath.Abs(filepath.Join("..", "value"))
		if err == nil {
			_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, walkErr error) error {
				if walkErr != nil || d.IsDir() || !strings.HasSuffix(p, ".go") {
					return nil
				}
				if b, readErr := os.ReadFile(p); readErr == nil {
					h.Write([]byte(p))
					h.Write([]byte{0})
					h.Write(b)
				}
				return nil
			})
		}
		runtimeFP = hex.EncodeToString(h.Sum(nil))
	})
	return runtimeFP
}

// cacheDisabled reports whether the content cache is turned off for this run.
func cacheDisabled() bool {
	return strings.EqualFold(os.Getenv("BENTO_AOT_TESTCACHE"), "off")
}

// aotCacheDir is where recorded outputs live. It sits under the user cache
// directory so it survives across runs and never lands in the repository tree;
// a machine without one falls back to the temp directory.
func aotCacheDir() string {
	base, err := os.UserCacheDir()
	if err != nil || base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "bento-aot-testcache")
}

// cachedGoRun returns the recorded output for a generated program, or calls run
// to build it and records what run returned. run does the actual compile and
// execute and returns the exact output to cache; it is only invoked on a miss,
// so a failing build (which fails the test inside run) is never recorded.
func cachedGoRun(t *testing.T, program string, run func() string) string {
	t.Helper()
	if cacheDisabled() {
		return run()
	}

	h := sha256.New()
	h.Write([]byte(runtimeFingerprint()))
	h.Write([]byte{0})
	h.Write([]byte(program))
	key := hex.EncodeToString(h.Sum(nil))

	file := filepath.Join(aotCacheDir(), key)
	if b, err := os.ReadFile(file); err == nil {
		return string(b)
	}

	out := run()
	if err := os.MkdirAll(aotCacheDir(), 0o755); err == nil {
		_ = os.WriteFile(file, []byte(out), 0o644)
	}
	return out
}
