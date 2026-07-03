package goimport

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCacheGeneratesThenReads(t *testing.T) {
	c := NewCache(t.TempDir(), "go1.26.0")
	calls := 0
	gen := func() (string, error) {
		calls++
		return "declaration text", nil
	}

	first, err := c.LoadOrGenerate("example.com/mod", "v1.0.0", gen)
	if err != nil {
		t.Fatal(err)
	}
	if first != "declaration text" {
		t.Errorf("first result = %q", first)
	}
	if calls != 1 {
		t.Fatalf("gen ran %d times on the first call, want 1", calls)
	}

	second, err := c.LoadOrGenerate("example.com/mod", "v1.0.0", gen)
	if err != nil {
		t.Fatal(err)
	}
	if second != "declaration text" {
		t.Errorf("cached result = %q", second)
	}
	if calls != 1 {
		t.Fatalf("gen ran %d times, want the second call to be a cache hit", calls)
	}
}

func TestCacheKeyedByVersion(t *testing.T) {
	c := NewCache(t.TempDir(), "go1.26.0")
	if _, err := c.LoadOrGenerate("example.com/mod", "v1.0.0", func() (string, error) {
		return "v1 decls", nil
	}); err != nil {
		t.Fatal(err)
	}
	// A different version is a different key, so it regenerates rather than serving
	// the v1 entry.
	got, err := c.LoadOrGenerate("example.com/mod", "v2.0.0", func() (string, error) {
		return "v2 decls", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "v2 decls" {
		t.Errorf("a version bump served a stale entry: %q", got)
	}
}

func TestCacheKeyedByToolchain(t *testing.T) {
	dir := t.TempDir()
	old := NewCache(dir, "go1.25.0")
	if _, err := old.LoadOrGenerate("example.com/mod", "v1.0.0", func() (string, error) {
		return "old toolchain decls", nil
	}); err != nil {
		t.Fatal(err)
	}
	// A newer toolchain over the same directory does not see the old entry, because
	// the toolchain partitions the tree.
	fresh := NewCache(dir, "go1.26.0")
	calls := 0
	got, err := fresh.LoadOrGenerate("example.com/mod", "v1.0.0", func() (string, error) {
		calls++
		return "new toolchain decls", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "new toolchain decls" || calls != 1 {
		t.Errorf("a toolchain bump did not invalidate the cache: got %q, calls %d", got, calls)
	}
}

func TestCacheUnpinnedNeverCaches(t *testing.T) {
	c := NewCache(t.TempDir(), "go1.26.0")
	calls := 0
	gen := func() (string, error) {
		calls++
		return "decls", nil
	}
	for range 3 {
		if _, err := c.LoadOrGenerate("example.com/mod", "", gen); err != nil {
			t.Fatal(err)
		}
	}
	// Without a concrete version the key cannot guarantee freshness, so every call
	// regenerates rather than risk a stale hit.
	if calls != 3 {
		t.Errorf("an unpinned import was cached: gen ran %d times, want 3", calls)
	}
}

func TestCacheEntryPathIsLegibleAndFlat(t *testing.T) {
	c := NewCache("/cache", "go1.26.4")
	path := c.entryPath("github.com/klauspost/compress/zstd", "v1.17.9")
	// The toolchain partitions the tree and the file name is one flat, legible
	// segment carrying the import path and version.
	want := filepath.Join("/cache", "go1.26.4", "github.com_klauspost_compress_zstd@v1.17.9.d.ts")
	if path != want {
		t.Errorf("entry path = %q, want %q", path, want)
	}
	if strings.Count(path[len("/cache/go1.26.4/"):], "/") != 0 {
		t.Errorf("entry file name is not flat: %q", path)
	}
}
