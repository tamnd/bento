package value

// This file completes the port of Node's path module with its two structural
// members, parse and format, which take a path apart and put one back together.
// They sit here rather than in nodepath.go because that file is the algorithms a
// syscall workload calls and these are the ones a program calls when it is
// rewriting a path rather than following one: swapping an extension, moving a file
// into a sibling directory, splitting a name from what holds it.
//
// The scanning loop is the same one dirname, basename and extname use, and it is
// the same one in Node, which is worth saying because parse is not built out of
// those three there and is not here either. Composing it from them would give the
// same answers on ordinary input and different ones at the edges, since each of the
// three resolves the "is this a dot file or an extension" question for its own
// result, and parse has to resolve it once for all five fields at the same time.

// pathParts is what parse answers: the five pieces a path splits into. They are
// held as Go strings and rendered to the object at the module boundary, so the
// algorithm is testable without building a value.
type pathParts struct {
	root string
	dir  string
	base string
	ext  string
	name string
}

// pathParsePosix is path.posix.parse. The loop walks the path from the right
// tracking three things at once: where the last segment starts, where the last dot
// in it is, and whether the characters before that dot make it an extension
// separator or part of the name.
//
// preDotState carries that last question. It is 0 while nothing but the trailing
// segment's end has been seen, 1 once a run of dots with nothing else has been
// seen, and -1 once an ordinary character has been seen before a dot. Only the -1
// case, or a leading dot that is not the whole name, produces an extension, which
// is why ".bashrc" has no extension and "a..b" has one.
func pathParsePosix(path string) pathParts {
	var ret pathParts
	if len(path) == 0 {
		return ret
	}
	isAbsolute := path[0] == pathSepPosix
	start := 0
	if isAbsolute {
		ret.root = "/"
		start = 1
	}
	startDot, startPart, end := -1, 0, -1
	matchedSlash := true
	preDotState := 0
	for i := len(path) - 1; i >= start; i-- {
		c := path[i]
		if c == pathSepPosix {
			// A separator that trails the path is skipped; the first one with a
			// character after it is where the last segment begins.
			if !matchedSlash {
				startPart = i + 1
				break
			}
			continue
		}
		if end == -1 {
			matchedSlash = false
			end = i + 1
		}
		if c == '.' {
			if startDot == -1 {
				startDot = i
			} else if preDotState != 1 {
				preDotState = 1
			}
		} else if startDot != -1 {
			preDotState = -1
		}
	}
	if end != -1 {
		// A leading slash belongs to the root rather than to the name, so an
		// absolute path with a single segment starts the name after it.
		s := startPart
		if startPart == 0 && isAbsolute {
			s = 1
		}
		fillParsedName(&ret, path, s, startPart, startDot, end, preDotState)
	}
	if startPart > 0 {
		ret.dir = path[:startPart-1]
	} else if isAbsolute {
		ret.dir = "/"
	}
	return ret
}

// pathParseWin32 is path.win32.parse. It differs from the posix form in the root:
// windows has drive roots and UNC roots, both of which have to be measured off the
// front before the segment scan begins, and a path that is nothing but a root
// answers immediately rather than scanning a name out of it.
func pathParseWin32(path string) pathParts {
	var ret pathParts
	if len(path) == 0 {
		return ret
	}
	length := len(path)
	rootEnd := 0
	code := path[0]
	if length == 1 {
		if isPathSeparatorWin32(code) {
			ret.root = path
			ret.dir = path
			return ret
		}
		ret.base = path
		ret.name = path
		return ret
	}
	if isPathSeparatorWin32(code) {
		// Possibly a UNC root, \\server\share. It counts only when both the server
		// and the share are non-empty, so the scan walks non-separators, then
		// separators, then non-separators, and each run has to advance.
		rootEnd = 1
		if isPathSeparatorWin32(path[1]) {
			j := 2
			last := j
			for j < length && !isPathSeparatorWin32(path[j]) {
				j++
			}
			if j < length && j != last {
				last = j
				for j < length && isPathSeparatorWin32(path[j]) {
					j++
				}
				if j < length && j != last {
					last = j
					for j < length && !isPathSeparatorWin32(path[j]) {
						j++
					}
					if j == length {
						rootEnd = j
					} else if j != last {
						rootEnd = j + 1
					}
				}
			}
		}
	} else if isWindowsDeviceRoot(code) && path[1] == ':' {
		if length <= 2 {
			ret.root = path
			ret.dir = path
			return ret
		}
		rootEnd = 2
		if isPathSeparatorWin32(path[2]) {
			if length == 3 {
				ret.root = path
				ret.dir = path
				return ret
			}
			rootEnd = 3
		}
	}
	if rootEnd > 0 {
		ret.root = path[:rootEnd]
	}
	startDot, startPart, end := -1, rootEnd, -1
	matchedSlash := true
	preDotState := 0
	for i := length - 1; i >= rootEnd; i-- {
		c := path[i]
		if isPathSeparatorWin32(c) {
			if !matchedSlash {
				startPart = i + 1
				break
			}
			continue
		}
		if end == -1 {
			matchedSlash = false
			end = i + 1
		}
		if c == '.' {
			if startDot == -1 {
				startDot = i
			} else if preDotState != 1 {
				preDotState = 1
			}
		} else if startDot != -1 {
			preDotState = -1
		}
	}
	if end != -1 {
		fillParsedName(&ret, path, startPart, startPart, startDot, end, preDotState)
	}
	// When the last segment starts where the root ends there is no directory above
	// it inside the path, so the root is the directory, trailing separator and all:
	// C:\abc has dir C:\ while C:\abc\def has dir C:\abc.
	if startPart > 0 && startPart != rootEnd {
		ret.dir = path[:startPart-1]
	} else {
		ret.dir = ret.root
	}
	return ret
}

// fillParsedName sets base, name and ext from the segment the scan measured, the
// step both variants share. The dot only separates an extension when something
// other than dots came before it in the segment (preDotState is -1), or when the
// segment is exactly "..", which is spelled out by the three-way check rather than
// special-cased by name because that is how the condition is stated in Node.
//
// nameStart and segStart differ only for an absolute posix path with one segment,
// where the name starts after the root slash but the segment is measured from zero.
func fillParsedName(ret *pathParts, path string, nameStart, segStart, startDot, end, preDotState int) {
	if startDot == -1 || preDotState == 0 ||
		(preDotState == 1 && startDot == end-1 && startDot == segStart+1) {
		ret.base = path[nameStart:end]
		ret.name = ret.base
		return
	}
	ret.name = path[nameStart:startDot]
	ret.base = path[nameStart:end]
	ret.ext = path[startDot:end]
}

// pathFormat is path.format, the inverse of parse, shared by both variants since
// the only thing that differs is the separator it puts between the directory and
// the name.
//
// It is not quite an inverse. base wins over name and ext, so an object carrying
// all three ignores the pair, and dir wins over root, so the two never both appear.
// That is what lets a program do the ordinary thing, parse a path, change one
// field, format it back, without having to clear the field it did not want: change
// ext and clear base, or change base and leave the rest.
func pathFormat(sep string, dir, root, base, name, ext string) string {
	if dir == "" {
		dir = root
	}
	if base == "" {
		base = name + formatExt(ext)
	}
	if dir == "" {
		return base
	}
	// The root already ends in a separator, so joining it to the name with another
	// would double it: {root: "/", base: "f"} is "/f" and not "//f".
	if dir == root {
		return dir + base
	}
	return dir + sep + base
}

// formatExt renders the extension for format, adding the leading dot when the
// caller left it off. Node accepts both spellings because parse hands the extension
// back with its dot while a program writing one out by hand usually does not.
func formatExt(ext string) string {
	if ext == "" {
		return ""
	}
	if ext[0] == '.' {
		return ext
	}
	return "." + ext
}
