package resolve

import (
	"os"
	"path/filepath"
)

// FS is the narrow filesystem the resolver reads through. Keeping it small lets
// the resolver run over a real disk, an in-memory tree in tests, or a build
// overlay without changing the algorithm.
type FS interface {
	// Stat reports whether a path exists and whether it is a directory.
	Stat(path string) (FileInfo, error)
	// ReadFile returns a file's bytes.
	ReadFile(path string) ([]byte, error)
	// ReadDir lists a directory's entries by name.
	ReadDir(path string) ([]string, error)
	// RealPath canonicalizes a path, resolving symlinks.
	RealPath(path string) (string, error)
}

// FileInfo is the subset of file metadata the resolver needs.
type FileInfo struct {
	IsDir bool
}

// OSFS is an FS backed by the real operating system filesystem.
type OSFS struct{}

// Stat implements FS over os.Stat.
func (OSFS) Stat(path string) (FileInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return FileInfo{}, err
	}
	return FileInfo{IsDir: info.IsDir()}, nil
}

// ReadFile implements FS over os.ReadFile.
func (OSFS) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

// ReadDir implements FS over os.ReadDir, returning entry names.
func (OSFS) ReadDir(path string) ([]string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names, nil
}

// RealPath implements FS over filepath.EvalSymlinks, falling back to the input
// when the path cannot be canonicalized so resolution errors stay meaningful.
func (OSFS) RealPath(path string) (string, error) {
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return path, err
	}
	return real, nil
}

// fileExists reports whether path is a regular file.
func (r *Resolver) fileExists(path string) bool {
	info, err := r.fs.Stat(path)
	return err == nil && !info.IsDir
}

// dirExists reports whether path is a directory.
func (r *Resolver) dirExists(path string) bool {
	info, err := r.fs.Stat(path)
	return err == nil && info.IsDir
}

// realPath canonicalizes a path for stable cache identity, honoring
// preserveSymlinks. A failure falls back to the input path.
func (r *Resolver) realPath(path string) string {
	if r.preserveSymlinks {
		return path
	}
	real, err := r.fs.RealPath(path)
	if err != nil {
		return path
	}
	return real
}
