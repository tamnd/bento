package resolve

import (
	"sort"
	"strings"

	"github.com/tamnd/bento/pkg/cpath"
)

// memFS is an in-memory FS for tests. Keys are module paths: cleaned, absolute,
// and slash-separated on every platform, which is what the FS interface takes.
// A file maps to its bytes; a directory is implied by any file beneath it. It
// uses cpath and a literal "/" rather than filepath, so a Windows run reads the
// same tree a Unix run does and the assertions need no platform branch.
type memFS struct {
	files map[string]string
	links map[string]string
}

func newMemFS() *memFS {
	return &memFS{files: map[string]string{}, links: map[string]string{}}
}

// add registers a file at a cleaned path.
func (m *memFS) add(path, content string) *memFS {
	m.files[cpath.FromOS(path)] = content
	return m
}

// link registers a symlink from -> to for RealPath resolution. The link path
// also stats as an existing file, matching how a symlink behaves on disk.
func (m *memFS) link(from, to string) *memFS {
	from = cpath.FromOS(from)
	m.links[from] = cpath.FromOS(to)
	if _, ok := m.files[from]; !ok {
		m.files[from] = ""
	}
	return m
}

func (m *memFS) Stat(path string) (FileInfo, error) {
	path = cpath.FromOS(path)
	if _, ok := m.files[path]; ok {
		return FileInfo{IsDir: false}, nil
	}
	prefix := path + "/"
	for f := range m.files {
		if strings.HasPrefix(f, prefix) {
			return FileInfo{IsDir: true}, nil
		}
	}
	return FileInfo{}, errMissing
}

func (m *memFS) ReadFile(path string) ([]byte, error) {
	if content, ok := m.files[cpath.FromOS(path)]; ok {
		return []byte(content), nil
	}
	return nil, errMissing
}

func (m *memFS) ReadDir(path string) ([]string, error) {
	path = cpath.FromOS(path)
	prefix := path + "/"
	seen := map[string]bool{}
	for f := range m.files {
		if !strings.HasPrefix(f, prefix) {
			continue
		}
		rest := strings.TrimPrefix(f, prefix)
		name, _, _ := strings.Cut(rest, "/")
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
	path = cpath.FromOS(path)
	if target, ok := m.links[path]; ok {
		return target, nil
	}
	return path, nil
}

type memError struct{}

func (memError) Error() string { return "no such file" }

var errMissing = memError{}
