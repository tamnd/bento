package value

import (
	"os"
	"path"
)

// This file is the value-model side of the small node:fs, node:os, and node:path
// surface the AOT compiler lowers. A workload that writes a temp tree, reads it
// back, and removes it (the readwrite benchmark) lowers each of its node: calls
// to one of these helpers, so the compiled program does the syscalls directly
// through the Go standard library with no engine, event loop, or libuv in
// between. The surface is deliberately the synchronous, string-in string-out
// slice of Node: mkdtempSync, writeFileSync, readFileSync in its utf8 form,
// rmSync with the recursive and force flags, os.tmpdir, and path.join, each
// declared in aot_ambient.go exactly as it is lowered here.
//
// A JavaScript fs call throws on an I/O error, and the compiled program has no
// exception machinery yet (a later slice lowers throw and try/catch), so a real
// failure here panics rather than returning a silent wrong value. That keeps the
// honest boundary: a program that hits a genuine filesystem error stops loudly
// instead of continuing as if the write had happened. The one error a caller
// routinely suppresses, a missing path under rmSync with force, is handled in
// band so the common cleanup path does not panic.

// Mkdtemp creates a new temporary directory whose name starts with prefix and
// returns its path, the lowering of fs.mkdtempSync. Node treats the prefix as a
// literal path fragment and appends six random characters to it, which is exactly
// os.MkdirTemp's contract when the prefix's directory is split from its base, so
// the two agree on the created path shape. A creation failure (a missing parent,
// a permission denial) is a thrown error in Node, surfaced here as a panic.
func Mkdtemp(prefix BStr) BStr {
	dir, base := splitPrefix(prefix.ToGoString())
	out, err := os.MkdirTemp(dir, base+"*")
	if err != nil {
		panic("fs.mkdtempSync: " + err.Error())
	}
	return FromGoString(out)
}

// splitPrefix separates a mkdtemp prefix into the parent directory the temp
// directory is created in and the base name it starts with, matching Node, which
// appends the random suffix directly to the prefix. os.MkdirTemp instead places
// the suffix where a "*" appears in the pattern and creates the entry in its dir
// argument, so the prefix's own directory part is peeled off here and its base
// becomes the pattern stem. A prefix ending in a separator (its base is empty)
// means the caller wants a random name inside that directory, which an empty
// stem plus the "*" the caller appends expresses.
func splitPrefix(prefix string) (dir, base string) {
	dir, base = path.Split(prefix)
	if dir == "" {
		dir = "."
	}
	return dir, base
}

// WriteFileSync writes data to the file at the given path, creating it or
// truncating an existing file, the lowering of fs.writeFileSync(path, data) in
// its string-data form. The string is transcoded to its UTF-8 view, the byte
// sink Node uses when the data argument is a string with the default encoding.
// The 0o666 mode before umask matches Node's default for a newly created file. A
// write failure is a thrown error in Node, surfaced here as a panic.
func WriteFileSync(pathArg, data BStr) {
	if err := os.WriteFile(pathArg.ToGoString(), []byte(data.ToGoString()), 0o666); err != nil {
		panic("fs.writeFileSync: " + err.Error())
	}
}

// ReadFileSyncUTF8 reads the whole file at path and returns its contents as a
// string, the lowering of fs.readFileSync(path, "utf8"). The bytes are decoded
// as UTF-8 into a BStr, which keeps the UTF-8 fast path, matching the encoding
// argument the ambient declaration fixes. A read failure (a missing file, a
// permission denial) is a thrown error in Node, surfaced here as a panic.
func ReadFileSyncUTF8(pathArg BStr) BStr {
	b, err := os.ReadFile(pathArg.ToGoString())
	if err != nil {
		panic("fs.readFileSync: " + err.Error())
	}
	return FromGoString(string(b))
}

// RmSync removes the file or directory at path, the lowering of fs.rmSync with
// its recursive and force options. recursive removes a directory and everything
// under it, the RemoveAll semantics; without it only a single file or empty
// directory is removed, the Remove semantics, matching Node, which errors on a
// non-empty directory unless recursive is set. force suppresses the error a
// missing path would raise, exactly Node's force flag, so a cleanup that runs
// twice does not throw the second time; without force a missing path panics as a
// thrown error would. The compiler reads both flags from the options object at
// lower time, so they arrive here as plain booleans.
func RmSync(pathArg BStr, recursive, force bool) {
	p := pathArg.ToGoString()
	var err error
	if recursive {
		err = os.RemoveAll(p)
	} else {
		err = os.Remove(p)
	}
	if err != nil {
		if force && os.IsNotExist(err) {
			return
		}
		panic("fs.rmSync: " + err.Error())
	}
}

// Tmpdir returns the operating system's default directory for temporary files,
// the lowering of os.tmpdir. It reads the same environment the platform uses
// (TMPDIR and its kin), so a compiled program lands its temp tree where a Node
// program would.
func Tmpdir() BStr {
	return FromGoString(os.TempDir())
}

// PathJoin joins the parts with the platform separator and normalizes the
// result, the lowering of path.join. It collapses redundant separators and
// resolves the "." and ".." segments the same way Node's path.join does on a
// POSIX platform, so a compiled program builds the same path string. With no
// parts it returns ".", path.join's empty-input result.
func PathJoin(parts ...BStr) BStr {
	segs := make([]string, len(parts))
	for i, p := range parts {
		segs[i] = p.ToGoString()
	}
	return FromGoString(path.Join(segs...))
}
