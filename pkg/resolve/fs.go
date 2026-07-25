package resolve

import (
	"os"
	"path/filepath"

	"github.com/tamnd/bento/pkg/cpath"
)

// The FS interface is the resolver's seam. Above it every path is a module path:
// slash-separated on every platform, which is what the resolution algorithm is
// specified over, what a package.json carries, and what an import specifier is.
// Below it a path is whatever the backing store wants, so OSFS converts on the
// way down and back on the way up and nothing else in the package ever holds an
// operating system path. See pkg/cpath, which is the same rule the frontend
// keeps; the resolver's paths and the checker's are the same paths.
//
// Before this, the resolver called filepath throughout and so answered
// \app\node_modules\dual\esm\index.js on Windows where the algorithm, and every
// caller, means /app/node_modules/dual/esm/index.js.

// FS is the narrow filesystem the resolver reads through. Keeping it small lets
// the resolver run over a real disk, an in-memory tree in tests, or a build
// overlay without changing the algorithm. Its paths are module paths; an
// implementation backed by a real disk converts.
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

// OSFS is an FS backed by the real operating system filesystem. It is where a
// module path becomes an OS path and back; every method converts, so the
// resolver above it never sees a backslash.
type OSFS struct{}

// Stat implements FS over os.Stat.
func (OSFS) Stat(path string) (FileInfo, error) {
	info, err := os.Stat(cpath.ToOS(path))
	if err != nil {
		return FileInfo{}, err
	}
	return FileInfo{IsDir: info.IsDir()}, nil
}

// ReadFile implements FS over os.ReadFile.
func (OSFS) ReadFile(path string) ([]byte, error) { return os.ReadFile(cpath.ToOS(path)) }

// ReadDir implements FS over os.ReadDir, returning entry names. The names are
// bare, with no separator in them, so they need no conversion.
func (OSFS) ReadDir(path string) ([]string, error) {
	entries, err := os.ReadDir(cpath.ToOS(path))
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
// The answer comes back from the disk in the platform's spelling and converts,
// which matters: the realpath is a resolved module's identity and its cache key,
// so an unconverted one would be a second key for a file already resolved.
func (OSFS) RealPath(path string) (string, error) {
	real, err := filepath.EvalSymlinks(cpath.ToOS(path))
	if err != nil {
		return path, err
	}
	return cpath.FromOS(real), nil
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
