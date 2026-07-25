package conformance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"sync"
)

// This file makes a repeat conformance run cheap. The oracle check compiles and runs
// one golden per fixture, and there are hundreds of them; each is its own main package,
// so Go links a separate binary for each one and the link is what the run costs. Go's
// build cache does not help, because nothing about the second run is a cache hit at the
// link step it has to redo.
//
// So the result is cached instead of the build. A golden is generated Go: run it twice
// against the same runtime on the same toolchain and it prints the same thing, which
// makes its output a pure function of a few inputs that are all cheap to hash. The key
// is the golden's own bytes, the environment the fixture pins, a fingerprint of the
// runtime packages a golden can import, and the toolchain and platform. Change any of
// them and the entry misses and the golden runs for real.
//
// What is cached is the golden's stdout and exit code, not the pass or fail verdict.
// The comparison against oracle.txt still happens on every run, so editing an oracle
// re-checks the cached output immediately rather than needing the oracle in the key.
//
// The effect follows the work: a change under pkg/lower reruns only the goldens whose
// rendered output actually changed, which is the loop a lowering slice iterates in, and
// a change under pkg/value correctly invalidates every entry.

// Verdict is what a golden produced: the text it printed and the status it exited with,
// the two things the oracle check compares. It is the cached unit.
type Verdict struct {
	Stdout string `json:"stdout"`
	Exit   int    `json:"exit"`
}

// verdictCacheDir is where cached outputs live, one file per key. It sits in the
// temporary directory rather than the module tree so a run never leaves anything behind
// in the working copy, and beside the golden build cache it complements.
func verdictCacheDir() string {
	return filepath.Join(os.TempDir(), "bento-conformance-verdicts")
}

// cachingDisabled reports whether the run was told to ignore the cache. Setting
// BENTO_CONFORMANCE_NO_CACHE=1 forces every golden to compile and run, which is the
// escape hatch for confirming a suspicious result against the real toolchain.
func cachingDisabled() bool {
	return os.Getenv("BENTO_CONFORMANCE_NO_CACHE") == "1"
}

// VerdictKey returns the cache key for running a golden, or ok=false when the run must
// not be cached. The key covers everything that can change what a golden prints: its own
// bytes, the environment the fixture pins, the runtime packages it links against, and
// the toolchain and platform that build it. A fingerprint that cannot be computed
// returns ok=false, so a missing source tree turns caching off rather than keying every
// entry under a constant that would serve stale output forever.
func VerdictKey(golden []byte, env map[string]string) (string, bool) {
	if cachingDisabled() {
		return "", false
	}
	fp, ok := runtimeFingerprint()
	if !ok {
		return "", false
	}
	h := sha256.New()
	h.Write([]byte("bento-conformance-verdict-v1\x00"))
	h.Write([]byte(fp))
	h.Write([]byte{0})
	h.Write([]byte(runtime.Version()))
	h.Write([]byte{0})
	h.Write([]byte(runtime.GOOS + "/" + runtime.GOARCH))
	h.Write([]byte{0})
	// The pinned environment is sorted so two runs that built the same map in a
	// different order land on one key.
	names := make([]string, 0, len(env))
	for name := range env {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		h.Write([]byte(name))
		h.Write([]byte{'='})
		h.Write([]byte(env[name]))
		h.Write([]byte{0})
	}
	h.Write(golden)
	return hex.EncodeToString(h.Sum(nil)), true
}

// LookupVerdict returns the cached output for a key. A missing or unreadable entry is a
// miss rather than an error, since the caller can always fall back to running the
// golden, and a cache that fails loudly would be worse than one that is merely cold.
func LookupVerdict(key string) (Verdict, bool) {
	if key == "" {
		return Verdict{}, false
	}
	data, err := os.ReadFile(filepath.Join(verdictCacheDir(), key))
	if err != nil {
		return Verdict{}, false
	}
	var v Verdict
	if err := json.Unmarshal(data, &v); err != nil {
		return Verdict{}, false
	}
	return v, true
}

// StoreVerdict records a golden's output under its key. The write goes to a temporary
// file and is renamed into place, so a run killed mid-write leaves no half-written entry
// for the next run to read as a hit. A failure to store is ignored: the cache is an
// optimization, and a run that cannot write one must still pass or fail on its merits.
func StoreVerdict(key string, v Verdict) {
	if key == "" {
		return
	}
	dir := verdictCacheDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	tmp, err := os.CreateTemp(dir, "entry-")
	if err != nil {
		return
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return
	}
	if err := os.Rename(name, filepath.Join(dir, key)); err != nil {
		_ = os.Remove(name)
	}
}

// runtimeRoots are the packages a golden links against, relative to this package's
// directory, which is the working directory a test runs in. Every emit.golden imports
// exactly value and nodehost and neither imports another bento package, so hashing these
// two trees covers the whole of what a golden's behaviour depends on inside the module.
// A golden that grows a third import must add it here, which the drift test enforces.
var runtimeRoots = []string{
	filepath.Join("..", "..", "value"),
	filepath.Join("..", "..", "nodehost"),
}

var (
	fingerprintOnce sync.Once
	fingerprintVal  string
	fingerprintOK   bool
)

// runtimeFingerprint hashes the source of the runtime packages a golden links against,
// so a change to any of them invalidates every cached output. It is computed once per
// process, since a test run does not edit its own sources.
//
// Only non-test .go files count: a _test.go file is not linked into a golden, so
// including it would invalidate the whole cache every time a runtime test was edited.
// Test sources are the most-edited files in the tree, so that distinction is most of
// what makes the cache worth having.
func runtimeFingerprint() (string, bool) {
	fingerprintOnce.Do(func() {
		h := sha256.New()
		for _, root := range runtimeRoots {
			var paths []string
			err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() || !isLinkedSource(d.Name()) {
					return nil
				}
				paths = append(paths, path)
				return nil
			})
			if err != nil {
				return
			}
			// The walk order is already lexical per directory, but sorting makes the
			// fingerprint independent of the filesystem's ordering across platforms.
			sort.Strings(paths)
			for _, p := range paths {
				data, err := os.ReadFile(p)
				if err != nil {
					return
				}
				h.Write([]byte(p))
				h.Write([]byte{0})
				h.Write([]byte(strconv.Itoa(len(data))))
				h.Write([]byte{0})
				h.Write(data)
			}
		}
		fingerprintVal = hex.EncodeToString(h.Sum(nil))
		fingerprintOK = true
	})
	return fingerprintVal, fingerprintOK
}

// isLinkedSource reports whether a file is Go source that gets linked into a golden.
// Test files are excluded because a golden never links them, so editing one must not
// cost a full corpus rerun.
func isLinkedSource(name string) bool {
	if filepath.Ext(name) != ".go" {
		return false
	}
	const testSuffix = "_test.go"
	return len(name) < len(testSuffix) || name[len(name)-len(testSuffix):] != testSuffix
}
