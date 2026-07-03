package goimport

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// This file is the .d.ts cache of document 16 section 4.5. Generating declarations
// from a Go package is not free, so the generated text is cached on disk keyed by
// the module version and the Go toolchain version. Because the key includes the
// exact version from go.mod and the toolchain that produced the types, the cache
// is safe: a version bump invalidates it and a toolchain bump invalidates it, so
// the declarations can never drift from the source they describe. The first import
// of a package pays the generation cost once; every import after is a file read.

// Cache stores generated declaration files under a directory, partitioned by the
// toolchain version so two toolchains never share an entry. It holds no state
// beyond its directory and toolchain string, so it is safe to share.
type Cache struct {
	dir       string
	toolchain string
}

// NewCache builds a cache rooted at dir. An empty toolchain defaults to the
// running Go toolchain version, which is the value that actually produced the
// types, so the default is the correct key in the common case.
func NewCache(dir, toolchain string) *Cache {
	if toolchain == "" {
		toolchain = runtime.Version()
	}
	return &Cache{dir: dir, toolchain: toolchain}
}

// LoadOrGenerate returns the cached declarations for a package at a version, or
// generates them with gen, stores the result, and returns it. An empty version is
// treated as uncacheable and always regenerates, because without a concrete
// version the key cannot guarantee freshness (section 4.5), and a stale cache hit
// would be worse than the regeneration cost.
func (c *Cache) LoadOrGenerate(importPath, version string, gen func() (string, error)) (string, error) {
	if version == "" {
		return gen()
	}
	if dts, ok, err := c.get(importPath, version); err != nil {
		return "", err
	} else if ok {
		return dts, nil
	}
	dts, err := gen()
	if err != nil {
		return "", err
	}
	if err := c.put(importPath, version, dts); err != nil {
		return "", err
	}
	return dts, nil
}

// Load is LoadOrGenerate wired to the package loader, so a caller gets a cached
// read when it can and a fresh generation when it must, without repeating the
// generate closure at every call site.
func (c *Cache) Load(importPath, version string) (string, error) {
	return c.LoadOrGenerate(importPath, version, func() (string, error) {
		return Load(importPath, version)
	})
}

// get reads a cached entry, reporting whether one was present. A missing file is
// a clean miss, not an error; any other read failure is surfaced.
func (c *Cache) get(importPath, version string) (string, bool, error) {
	path := c.entryPath(importPath, version)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("read .d.ts cache %s: %w", path, err)
	}
	return string(data), true, nil
}

// put writes an entry, creating the toolchain-partitioned directory as needed. It
// writes to a temporary file in the same directory and renames it into place, so a
// concurrent reader never sees a half-written declaration file.
func (c *Cache) put(importPath, version, dts string) error {
	path := c.entryPath(importPath, version)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create .d.ts cache dir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*.d.ts")
	if err != nil {
		return fmt.Errorf("create .d.ts cache temp: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.WriteString(dts); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write .d.ts cache temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close .d.ts cache temp: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("commit .d.ts cache entry: %w", err)
	}
	return nil
}

// entryPath is the on-disk path for one cache entry: the toolchain partitions the
// tree, and the file name carries the import path and version so the entry is
// legible when someone lists the cache while debugging the generator (section 5.5).
func (c *Cache) entryPath(importPath, version string) string {
	name := sanitizeSegment(importPath) + "@" + sanitizeSegment(version) + ".d.ts"
	return filepath.Join(c.dir, sanitizeSegment(c.toolchain), name)
}

// sanitizeSegment turns a value that may contain path separators or other awkward
// characters into one safe file-name segment, so an import path like
// github.com/klauspost/compress/zstd becomes a single flat name.
func sanitizeSegment(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '.', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "unpinned"
	}
	return b.String()
}
