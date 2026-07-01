package resolve

import (
	"path/filepath"
	"sort"
	"strings"
)

// memFS is an in-memory FS for tests. Keys are cleaned absolute paths. A file
// maps to its bytes; a directory is implied by any file beneath it.
type memFS struct {
	files map[string]string
	links map[string]string
}

func newMemFS() *memFS {
	return &memFS{files: map[string]string{}, links: map[string]string{}}
}

// add registers a file at a cleaned path.
func (m *memFS) add(path, content string) *memFS {
	m.files[filepath.Clean(path)] = content
	return m
}

// link registers a symlink from -> to for RealPath resolution. The link path
// also stats as an existing file, matching how a symlink behaves on disk.
func (m *memFS) link(from, to string) *memFS {
	from = filepath.Clean(from)
	m.links[from] = filepath.Clean(to)
	if _, ok := m.files[from]; !ok {
		m.files[from] = ""
	}
	return m
}

func (m *memFS) Stat(path string) (FileInfo, error) {
	path = filepath.Clean(path)
	if _, ok := m.files[path]; ok {
		return FileInfo{IsDir: false}, nil
	}
	prefix := path + string(filepath.Separator)
	for f := range m.files {
		if strings.HasPrefix(f, prefix) {
			return FileInfo{IsDir: true}, nil
		}
	}
	return FileInfo{}, errMissing
}

func (m *memFS) ReadFile(path string) ([]byte, error) {
	if content, ok := m.files[filepath.Clean(path)]; ok {
		return []byte(content), nil
	}
	return nil, errMissing
}

func (m *memFS) ReadDir(path string) ([]string, error) {
	path = filepath.Clean(path)
	prefix := path + string(filepath.Separator)
	seen := map[string]bool{}
	for f := range m.files {
		if !strings.HasPrefix(f, prefix) {
			continue
		}
		rest := strings.TrimPrefix(f, prefix)
		name, _, _ := strings.Cut(rest, string(filepath.Separator))
		seen[name] = true
	}
	if len(seen) == 0 {
		return nil, errMissing
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	return names, nil
}

func (m *memFS) RealPath(path string) (string, error) {
	path = filepath.Clean(path)
	if target, ok := m.links[path]; ok {
		return target, nil
	}
	return path, nil
}

type memError struct{}

func (memError) Error() string { return "no such file" }

var errMissing = memError{}
