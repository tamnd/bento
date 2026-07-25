// Package cpath is bento's one path model.
//
// # The rule
//
// Above the file system, every path bento holds is a checker path: absolute,
// slash-separated on every platform, with the Windows volume kept, which is what
// `C:/Users/x/main.ts` looks like. That is the only spelling that reaches the
// typescript-go adapter, the lowerer, a diagnostic, a fixture key or a map key.
//
// A path becomes an operating system path at exactly two kinds of place: where
// bento touches the disk, and where it hands a path to an external tool. Those
// call ToOS. A path becomes a checker path where one enters bento from outside:
// the entry named on the command line, the working directory, a name a directory
// walk produced. Those call FromOS.
//
// # Why this convention and not the other one
//
// Because it is not bento's to choose. typescript-go's virtual file system panics
// on a key that is not in this form, and it panics again if one file map mixes a
// POSIX-style key with a Windows-style one, so a path that reaches the checker has
// to be a checker path and every path in that map has to agree. Meanwhile module
// resolution is specified over slash paths, a package.json carries slash paths,
// and an import specifier in a source file is a slash path. Almost everything
// bento handles is already this shape. The operating system's convention is the
// exception, confined to the moment of the syscall, so that is where the
// conversion lives.
//
// On Unix both conversions are the identity and this package costs nothing.
//
// # Why it does not just call the compiler's normalizer
//
// It cannot. Only pkg/frontend/adapter may import typescript-go, so that a version
// bump has a one-package blast radius; this package is imported by pkg/build,
// pkg/frontend and pkg/resolve, and reaching for the shim here would put the
// TypeScript compiler in the dependency graph of very nearly everything.
//
// So the reduction is written out below, and the agreement is proven rather than
// assumed: TestNormalizeAgreesWithTheCompiler in the adapter package, which is
// allowed to import the shim, runs both over a table of paths and fails on any
// disagreement. Drift is a test failure, not a Windows bug six months later.
//
// # The failure it prevents
//
// A half-normalized path is worse than either convention, because it compares
// unequal to itself: `C:\a\b.ts` and `C:/a/b.ts` name one file and are two map
// keys. Every function here is idempotent, so a path that has already crossed the
// boundary is unharmed by crossing it again, and the seam can be widened one
// caller at a time without a flag day.
package cpath

import (
	"path"
	"path/filepath"
	"strings"
)

// FromOS turns an operating system path into a checker path. It is the identity
// on Unix for a path already in this form. On Windows it turns the separators
// around and resolves away "." and ".." segments, so `C:\Users\x\..\y\main.ts`
// becomes `C:/Users/y/main.ts`.
//
// It does not make a relative path absolute: a caller that needs that calls Abs.
// Normalizing and resolving against the working directory are different questions
// and a function that silently did both would answer the wrong one for a path
// that was relative on purpose.
func FromOS(p string) string {
	if p == "" {
		return ""
	}
	p = strings.ReplaceAll(p, `\`, "/")
	root := p[:rootLength(p)]
	rest := p[len(root):]
	// The trailing separator survives normalization, because a path written as a
	// directory keeps saying so. The checker's file map wants it gone, but that is
	// the map's rule and RemoveTrailingSeparator is where it is applied.
	trailing := len(rest) > 0 && strings.HasSuffix(rest, "/")
	if rest != "" {
		rest = path.Clean(rest)
		if rest == "." {
			rest = ""
		}
		// path.Clean leaves a leading ".." in place, which is right: it cannot be
		// resolved without knowing what is above. Under a root there is nothing
		// above, so the compiler drops it and so does this.
		if root != "" {
			for strings.HasPrefix(rest, "../") {
				rest = rest[len("../"):]
			}
			if rest == ".." {
				rest = ""
			}
		}
		rest = strings.TrimPrefix(rest, "/")
	}
	out := root + rest
	if trailing && !strings.HasSuffix(out, "/") {
		out += "/"
	}
	if out == "" {
		return "."
	}
	return out
}

// rootLength is how many leading bytes of a slash-normalized path are its root:
// 1 for a POSIX "/", 2 for a bare DOS volume "c:", 3 for a rooted one "c:/", and
// the whole prefix through the share separator for a UNC "//server/". A relative
// path has no root and gets 0.
//
// bento never produces the two shapes the compiler also recognizes here, a URL
// and an "^/" untitled path, because every path it holds came from filepath.Abs
// on a real file or is one of its own two virtual prefixes. The differential test
// says so as well, so if that ever stops being true it fails rather than quietly
// diverging.
func rootLength(p string) int {
	if p == "" {
		return 0
	}
	if p[0] == '/' {
		if len(p) == 1 || p[1] != '/' {
			return 1
		}
		// UNC. The root runs through the separator after the server name, or is the
		// whole string when there is no such separator.
		if i := strings.IndexByte(p[2:], '/'); i >= 0 {
			return i + 3
		}
		return len(p)
	}
	if len(p) > 1 && p[1] == ':' && isDriveLetter(p[0]) {
		if len(p) == 2 {
			return 2
		}
		if p[2] == '/' {
			return 3
		}
	}
	return 0
}

func isDriveLetter(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// ToOS turns a checker path back into an operating system path, for a syscall or
// for an argument to an external tool. It is the identity on Unix; on Windows it
// turns the separators back around.
func ToOS(p string) string {
	if p == "" {
		return ""
	}
	return filepath.FromSlash(p)
}

// IsRoot reports whether a checker path is nothing but its root: "/", "C:/", or a
// UNC share prefix. Such a path names a directory and never a file, so it is not
// a key any file map carries, and it is the one path Dir maps to itself.
func IsRoot(p string) bool { return p != "" && rootLength(p) == len(p) }

// RemoveTrailingSeparator drops one trailing slash, which is what the checker's
// file map requires of its keys and what a path used as a map key should not
// carry, since "a/b" and "a/b/" would otherwise be two entries for one directory.
// A root keeps its slash: "/" and "C:/" are not "" and "C:".
func RemoveTrailingSeparator(p string) string {
	if len(p) > rootLength(p) && strings.HasSuffix(p, "/") {
		return p[:len(p)-1]
	}
	return p
}

// Abs resolves a path against the process's working directory and returns a
// checker path. It is what a path named on the command line goes through on its
// way in.
func Abs(p string) (string, error) {
	abs, err := filepath.Abs(ToOS(p))
	if err != nil {
		return "", err
	}
	return FromOS(abs), nil
}

// IsAbs reports whether a checker path is absolute. This is not path.IsAbs, which
// answers false for "C:/Users", and not filepath.IsAbs, which on Unix answers
// false for every Windows path there is.
func IsAbs(p string) bool {
	n := rootLength(p)
	// A bare "c:" is rooted at a volume but not at a directory: it means the
	// working directory on that drive, which is not something bento may hold.
	return n > 0 && !(n == 2 && p[1] == ':')
}

// Dir is path.Dir over a checker path, with the volume kept. path.Dir has no idea
// what a volume is, so for "C:/main.ts" it answers "C:", which names the working
// directory on that drive rather than its root, and joining against it would
// silently produce a different file.
func Dir(p string) string {
	if p == "" {
		return "."
	}
	root := p[:rootLength(p)]
	if root != "" && len(p) == len(root) {
		return p
	}
	d := path.Dir(p)
	if root != "" && len(d) < len(root) {
		return root
	}
	return d
}

// Base is path.Base over a checker path.
func Base(p string) string { return path.Base(p) }

// Ext is path.Ext over a checker path. It reads the same as filepath.Ext for any
// path that has been through FromOS, and unlike filepath.Ext it cannot be fooled
// by a backslash on Unix.
func Ext(p string) string { return path.Ext(p) }

// Join joins checker path elements and normalizes the result, so a ".." in a
// later element is resolved rather than carried and the answer is a checker path
// ready to be used as a key.
func Join(elem ...string) string {
	nonEmpty := elem[:0:0]
	for _, e := range elem {
		if e != "" {
			nonEmpty = append(nonEmpty, e)
		}
	}
	if len(nonEmpty) == 0 {
		return ""
	}
	return FromOS(strings.Join(nonEmpty, "/"))
}

// Volume is the volume a checker path is rooted at, with its trailing slash:
// "C:/" for "C:/Users/x", and the empty string for a POSIX path or a relative
// one. It is what a virtual path has to be given on Windows, because a file map
// may not mix a POSIX-style key with a Windows-style one.
func Volume(p string) string {
	if rootLength(p) == 3 && p[1] == ':' {
		return p[:3]
	}
	return ""
}

// Virtual roots a virtual path, one naming a file bento synthesizes rather than
// reads, in the same volume as the program it belongs to. The checker's file map
// may not mix a POSIX-style key with a Windows-style one, so on Windows a virtual
// path spelled "/__bento_ambient__.d.ts" alongside a real "C:/Users/x/main.ts" is
// not merely unusual, it panics the compiler. sibling is any real path from the
// program, and only its volume is read.
//
// On Unix, and on Windows for a program that is somehow rooted at "/", this is
// the identity, so a virtual path is stable wherever it can be.
func Virtual(p, sibling string) string {
	vol := Volume(sibling)
	if vol == "" || Volume(p) != "" {
		return p
	}
	return vol + strings.TrimPrefix(p, "/")
}
