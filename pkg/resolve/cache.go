package resolve

import "sync"

// cache memoizes resolution answers. Resolving the same specifier from the same
// directory with the same conditions is common (shared dependencies), and the
// filesystem walk it avoids is the expensive part.
type cache struct {
	mu      sync.RWMutex
	results map[resolutionKey]Resolved
}

// resolutionKey identifies a resolution question. The parent directory, the
// active conditions, and the importer's module system (CommonJS vs ESM) all
// change the answer, so all three are part of the key. Format matters because a
// require and an import of the same specifier from the same directory can
// resolve to different files (index.js vs index.mjs).
type resolutionKey struct {
	dir        string
	specifier  string
	conditions string
	esm        bool
}

func newCache() *cache {
	return &cache{results: make(map[resolutionKey]Resolved)}
}

func (c *cache) get(key resolutionKey) (Resolved, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	r, ok := c.results[key]
	return r, ok
}

func (c *cache) put(key resolutionKey, r Resolved) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.results[key] = r
}

// Invalidate drops every cached resolution. The dev server calls this when a
// package.json or tsconfig on disk changes, since those alter what imports mean.
func (r *Resolver) Invalidate() {
	r.cache.mu.Lock()
	defer r.cache.mu.Unlock()
	r.cache.results = make(map[resolutionKey]Resolved)
}
