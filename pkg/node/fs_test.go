package node

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mkdtemp calls the host primitive the way the fs module does and returns the
// created path, failing the test if the envelope came back with an error.
func mkdtemp(t *testing.T, prefix string) string {
	t.Helper()
	raw, err := hostFSMkdtemp([]any{prefix})
	if err != nil {
		t.Fatalf("mkdtemp(%q): %v", prefix, err)
	}
	var res fsResult
	if err := json.Unmarshal([]byte(raw.(string)), &res); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if !res.OK {
		t.Fatalf("mkdtemp(%q) failed: %s %s", prefix, res.Code, res.Msg)
	}
	return res.Path
}

// TestMkdtempAppendsToThePrefix pins Node's contract: the six random characters
// go straight onto the prefix, in the prefix's own directory.
func TestMkdtempAppendsToThePrefix(t *testing.T) {
	parent := t.TempDir()
	dir := mkdtemp(t, filepath.Join(parent, "run-"))
	if got := filepath.Dir(dir); got != parent {
		t.Fatalf("created %q under %q, want it under %q", dir, got, parent)
	}
	if base := filepath.Base(dir); !strings.HasPrefix(base, "run-") {
		t.Fatalf("created dir base %q does not start with the prefix", base)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("stat %q: %v", dir, err)
	}
}

// TestMkdtempPrefixEndingInSeparator pins that a prefix ending in a separator
// asks for a randomly named directory inside it, the way Node's does. Splitting
// the prefix with Dir and Base instead of Split got this wrong on every platform,
// since Base("/tmp/") is "tmp": the directory came out as /tmp/tmpXXXXXX where
// Node makes /tmp/XXXXXX.
func TestMkdtempPrefixEndingInSeparator(t *testing.T) {
	parent := t.TempDir()
	dir := mkdtemp(t, parent+string(filepath.Separator))
	if got := filepath.Dir(dir); got != parent {
		t.Fatalf("created %q under %q, want it under %q", dir, got, parent)
	}
	if base := filepath.Base(dir); strings.HasPrefix(base, filepath.Base(parent)) {
		t.Fatalf("created dir base %q repeats the parent's name", base)
	}
}

// TestMkdtempBarePrefixLandsInCwd pins that a prefix with no directory part is
// created in the working directory. os.MkdirTemp reads an empty dir argument as
// the temp directory, which is not where Node puts it.
func TestMkdtempBarePrefixLandsInCwd(t *testing.T) {
	t.Chdir(t.TempDir())
	dir := mkdtemp(t, "run-")
	if filepath.Dir(dir) != "." {
		t.Fatalf("mkdtemp(%q) = %q, want it relative to the working directory", "run-", dir)
	}
}
