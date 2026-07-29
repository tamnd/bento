package value

// This file builds the path built-in module a compiled program gets from
// require('path'). It is the CommonJS half of node:path, the counterpart of the os
// module in nodeos_module.go, and like that one it reads the same algorithms the
// import half lowers to, so the two ways of asking cannot answer differently.
//
// The import half stops short in two places. It exports only the functions the
// lowerer can call directly, so parse and format are missing there because they
// take and return objects, and it exports only the host's variant, so a program
// that wants path.win32 while running on Linux has nowhere to get it. Neither limit
// applies here: this is built at run time out of the value model, which has objects,
// and both variants were already ported in nodepath.go for exactly this reason.
//
// require('path') is the same object as require('path').posix on a posix host and
// the same object as require('path').win32 on Windows, which is Node's arrangement
// rather than a shortcut. Node's path module is one of the two variants with the
// other hung off it, so path.posix.posix === path.posix holds there and holds here.

// pathFlavor is one of the two path models, the set of operations that differ
// between posix and win32. Node ships both as complete modules and so does this: the
// module builder below runs once per flavor and knows nothing about which one it is
// building, so neither variant can quietly acquire a member the other lacks.
//
// The functions that need the working directory close over the read rather than
// take it as a parameter, so the directory is read when the program calls resolve
// and not when the module was built. A program that chdirs between the two must see
// the new one.
type pathFlavor struct {
	sep          string
	delimiter    string
	normalize    func(string) string
	join         func([]string) string
	resolve      func([]string) string
	isAbsolute   func(string) bool
	relative     func(from, to string) string
	dirname      func(string) string
	basename     func(p, suffix string) string
	extname      func(string) string
	toNamespaced func(string) string
	parse        func(string) pathParts
}

// posixPathFlavor is path.posix: one separator, no drive letters, and a
// toNamespacedPath that hands the path straight back because there is no namespace
// to name.
func posixPathFlavor() pathFlavor {
	return pathFlavor{
		sep:        "/",
		delimiter:  ":",
		normalize:  pathNormalizePosix,
		join:       pathJoinPosix,
		resolve:    func(parts []string) string { return pathResolvePosix(parts, pathCwd()) },
		isAbsolute: pathIsAbsolutePosix,
		relative:   func(from, to string) string { return pathRelativePosix(from, to, pathCwd()) },
		dirname:    pathDirnamePosix,
		basename: func(p, suffix string) string {
			return pathBasename(p, suffix, isPathSeparatorPosix, false)
		},
		extname:      func(p string) string { return pathExtname(p, isPathSeparatorPosix, false) },
		toNamespaced: pathToNamespacedPathPosix,
		parse:        pathParsePosix,
	}
}

// win32PathFlavor is path.win32: two separators, drive and UNC roots, and a
// per-drive working directory that a drive-relative path (C:file) resolves against.
func win32PathFlavor() pathFlavor {
	return pathFlavor{
		sep:       `\`,
		delimiter: ";",
		normalize: pathNormalizeWin32,
		join:      pathJoinWin32,
		resolve: func(parts []string) string {
			return pathResolveWin32(parts, pathCwd(), pathCwdForDrive)
		},
		isAbsolute: pathIsAbsoluteWin32,
		relative: func(from, to string) string {
			return pathRelativeWin32(from, to, pathCwd(), pathCwdForDrive)
		},
		dirname: pathDirnameWin32,
		basename: func(p, suffix string) string {
			return pathBasename(p, suffix, isPathSeparatorWin32, true)
		},
		extname: func(p string) string { return pathExtname(p, isPathSeparatorWin32, true) },
		toNamespaced: func(p string) string {
			return pathToNamespacedPathWin32(p, pathCwd(), pathCwdForDrive)
		},
		parse: pathParseWin32,
	}
}

// pathModules holds the one posix module and the one win32 module for the process.
// They are built together because each carries a reference to the other, and they
// are held here rather than rebuilt because a program compares them:
// require('path/posix') === require('path').posix is true in Node and has to be
// true here. It is a package-level value without a lock, the same single-goroutine
// module-load assumption the rest of the built-in registry makes.
var pathModules struct {
	posix Value
	win32 Value
	built bool
}

// buildPathModules constructs both variants and cross-links them, once. Each
// variant carries both under the names Node gives them, so path.win32.posix reaches
// the posix module and path.posix.posix is the posix module itself. That last one is
// a cycle, and it is a cycle in Node too; a program that hands the module to
// JSON.stringify fails there for the same reason it fails here.
func buildPathModules() {
	if pathModules.built {
		return
	}
	pathModules.built = true
	posix := newPathModuleFor(posixPathFlavor())
	win32 := newPathModuleFor(win32PathFlavor())
	for _, m := range []Value{posix, win32} {
		m.Set(FromGoString("win32"), win32)
		m.Set(FromGoString("posix"), posix)
		// _makeLong is Node's older name for toNamespacedPath, kept there as the same
		// function object rather than a second one, and kept here because published
		// packages still call it. It is set last because that is where Node's own
		// module defines it, and the order is what Object.keys reports.
		m.Set(FromGoString("_makeLong"), m.Get(FromGoString("toNamespacedPath")))
	}
	pathModules.posix = posix
	pathModules.win32 = win32
}

// newPathModule builds the path core module, require('path') or
// require('node:path'), which is the variant matching the host: win32 on Windows and
// posix everywhere else, the same choice Node's own module makes.
func newPathModule() Value {
	buildPathModules()
	if onWindows {
		return pathModules.win32
	}
	return pathModules.posix
}

// newPathPosixModule is require('path/posix'), the posix variant named directly.
func newPathPosixModule() Value {
	buildPathModules()
	return pathModules.posix
}

// newPathWin32Module is require('path/win32'), the win32 variant named directly.
func newPathWin32Module() Value {
	buildPathModules()
	return pathModules.win32
}

// newPathModuleFor builds one variant's module object. The members are set in the
// order Node's own path module defines them, which is the order
// Object.keys(require('path')) reports there, so a program that enumerates or prints
// the module sees what Node's prints.
//
// Every function validates its arguments rather than coercing them, because that is
// what Node does and programs depend on it: path.dirname(5) throws
// ERR_INVALID_ARG_TYPE there rather than answering the dirname of "5", and a bento
// that answered would turn a caught mistake into a wrong path.
func newPathModuleFor(f pathFlavor) Value {
	mod := NewObject()
	set := func(name string, fn func([]Value) Value) {
		mod.Set(FromGoString(name), WithName(NewFunc(fn), name))
	}

	set("resolve", func(args []Value) Value {
		return StringValue(FromGoString(f.resolve(pathStringArgs(args))))
	})
	set("normalize", func(args []Value) Value {
		return StringValue(FromGoString(f.normalize(requireString(Arg(args, 0), "path"))))
	})
	set("isAbsolute", func(args []Value) Value {
		return Bool(f.isAbsolute(requireString(Arg(args, 0), "path")))
	})
	set("join", func(args []Value) Value {
		return StringValue(FromGoString(f.join(pathStringArgs(args))))
	})
	set("relative", func(args []Value) Value {
		from := requireString(Arg(args, 0), "from")
		to := requireString(Arg(args, 1), "to")
		return StringValue(FromGoString(f.relative(from, to)))
	})
	toNamespacedPath := WithName(NewFunc(func(args []Value) Value {
		// Node's toNamespacedPath is the one member that does not validate: it hands
		// back anything that is not a string untouched, because it is called on paths
		// that have already been through the module and a throw here would be a second
		// report of a failure already raised.
		p := Arg(args, 0)
		if p.Kind() != KindString {
			return p
		}
		return StringValue(FromGoString(f.toNamespaced(p.AsString().ToGoString())))
	}), "toNamespacedPath")
	mod.Set(FromGoString("toNamespacedPath"), toNamespacedPath)
	set("dirname", func(args []Value) Value {
		return StringValue(FromGoString(f.dirname(requireString(Arg(args, 0), "path"))))
	})
	set("basename", func(args []Value) Value {
		// The suffix is validated before the path, which is the order Node checks them
		// in, so basename(5, 5) reports the suffix rather than the path.
		suffix := ""
		if s := Arg(args, 1); s.Kind() != KindUndefined {
			suffix = requireString(s, "suffix")
		}
		return StringValue(FromGoString(f.basename(requireString(Arg(args, 0), "path"), suffix)))
	})
	set("extname", func(args []Value) Value {
		return StringValue(FromGoString(f.extname(requireString(Arg(args, 0), "path"))))
	})
	set("format", func(args []Value) Value {
		o := requireObject(Arg(args, 0), "pathObject")
		return StringValue(FromGoString(pathFormat(f.sep,
			pathField(o, "dir"), pathField(o, "root"),
			pathField(o, "base"), pathField(o, "name"), pathField(o, "ext"))))
	})
	set("parse", func(args []Value) Value {
		return pathPartsValue(f.parse(requireString(Arg(args, 0), "path")))
	})
	set("matchesGlob", func(args []Value) Value {
		// The two arguments are still validated before the refusal, so a program that
		// passed the wrong type learns that first and gets Node's error for it.
		requireString(Arg(args, 0), "path")
		requireString(Arg(args, 1), "pattern")
		Throw(NewNodeError("Error", "ERR_NOT_IMPLEMENTED",
			FromGoString("path.matchesGlob is not implemented in bento yet")))
		return Undefined
	})
	mod.Set(FromGoString("sep"), StringValue(FromGoString(f.sep)))
	mod.Set(FromGoString("delimiter"), StringValue(FromGoString(f.delimiter)))
	return mod
}

// pathStringArgs unwraps the arguments of a variadic path function, validating each
// as Node does: every one must be a string, and the first that is not names "path"
// in the error whatever its position, which is how Node reports it.
func pathStringArgs(args []Value) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = requireString(a, "path")
	}
	return out
}

// pathField reads one field of the object handed to format, treating anything that
// is not a string as absent. Node reads the fields with plain property access and
// tests them for truthiness, so a missing field and an empty one behave the same,
// and this collapses both to the empty string.
func pathField(o Value, name string) string {
	v := o.Get(FromGoString(name))
	if v.Kind() != KindString {
		return ""
	}
	return v.AsString().ToGoString()
}

// pathPartsValue renders a parsed path as the object parse answers with. The five
// keys are set in Node's order, which is the order they print in, and all five are
// always present even when empty: a program that reads .ext off a path with no
// extension gets "" rather than undefined.
func pathPartsValue(p pathParts) Value {
	out := NewObject()
	out.Set(FromGoString("root"), StringValue(FromGoString(p.root)))
	out.Set(FromGoString("dir"), StringValue(FromGoString(p.dir)))
	out.Set(FromGoString("base"), StringValue(FromGoString(p.base)))
	out.Set(FromGoString("ext"), StringValue(FromGoString(p.ext)))
	out.Set(FromGoString("name"), StringValue(FromGoString(p.name)))
	return out
}
