package value

import (
	"os"
	"runtime"
	"strings"
)

// This file is a port of Node's path module, the part a compiled program calls.
// PathJoin already lived in nodefs.go and delegated to filepath; the rest could
// not, because filepath is Go's path model and not JavaScript's, and the two
// disagree on ordinary inputs: filepath.Clean drops the trailing separator that
// path.normalize keeps, filepath.Base answers "." for the empty string where
// path.basename answers "", filepath.Ext calls ".bashrc" an extension where
// path.extname does not, and filepath.Rel answers "." for two equal paths where
// path.relative answers "". A program that asked bento for a path and got Go's
// answer would be quietly wrong, so the algorithms are ported rather than
// delegated.
//
// Both variants are here, posix and win32, exactly as Node ships both. The
// exported helpers dispatch on the host the way Node's module export does: the
// win32 variant on Windows, the posix variant everywhere else. The variants are
// not folded together with a separator parameter, because they genuinely differ:
// win32 has drive-relative roots (C:a is not the same shape as C:\a), UNC roots,
// and two separator characters where posix has one.
//
// The port scans bytes rather than UTF-16 code units, which Node's does. That is
// sound rather than convenient: every character the algorithm decides on is
// ASCII (the separators, the dot, the colon, and the drive letters), and no byte
// of a multi-byte UTF-8 sequence is ASCII, so a byte scan takes the same branches
// on the same inputs and every slice it takes lands on a character boundary.

const (
	pathSepPosix = '/'
	pathSepWin32 = '\\'
)

// onWindows reports whether the host is the one whose path module Node would
// export as win32. It is a variable rather than a call so the tests can drive
// both variants on one machine, which is the only way to check the port against
// the answers Node gives for path.win32 while running on Linux.
var onWindows = runtime.GOOS == "windows"

// isPathSeparatorWin32 reports whether c separates path segments on Windows,
// where both slashes do.
func isPathSeparatorWin32(c byte) bool { return c == pathSepPosix || c == pathSepWin32 }

// isPathSeparatorPosix reports whether c separates path segments on posix, where
// only the forward slash does and a backslash is an ordinary filename character.
func isPathSeparatorPosix(c byte) bool { return c == pathSepPosix }

// isWindowsDeviceRoot reports whether c can be a drive letter.
func isWindowsDeviceRoot(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

// windowsReservedNames are the device names Windows resolves from any directory,
// so a path naming one is not a path into the filesystem at all. Node carries the
// list to keep normalize from handing back something the shell would open as a
// device, which is how CVE-2024-36139 was fixed. The superscript spellings are
// there because Windows accepts COM¹ for COM1.
var windowsReservedNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
	"COM¹": true, "COM²": true, "COM³": true,
	"LPT¹": true, "LPT²": true, "LPT³": true,
}

// isWindowsReservedName reports whether the part of path before colonIndex names
// a Windows device.
//
// Node passes the result of indexOf(':') straight in, so colonIndex is -1 when
// there is no colon, and the negative index makes its slice drop the last
// character instead of returning nothing. That is load-bearing rather than a
// slip: it is what makes normalize("conx") hand back ".\\conx". The negative case
// is reproduced here for that reason, not preserved out of caution.
func isWindowsReservedName(path string, colonIndex int) bool {
	if colonIndex < 0 {
		colonIndex = len(path) - 1
		if colonIndex < 0 {
			return false
		}
	}
	if colonIndex > len(path) {
		colonIndex = len(path)
	}
	return windowsReservedNames[strings.ToUpper(path[:colonIndex])]
}

// normalizeStringPath resolves the "." and ".." segments of a path that has
// already had its root removed, joining what is left with sep. When
// allowAboveRoot is set, a ".." that would climb past the start is kept, which is
// what a relative path does and what an absolute path must not do.
//
// This is Node's normalizeString, the shared core of normalize, join, resolve and
// relative in both variants, and it is ported statement for statement rather than
// rewritten, because its output is the observable answer for every one of them.
func normalizeStringPath(path string, allowAboveRoot bool, sep byte, isSep func(byte) bool) string {
	var res strings.Builder
	lastSegmentLength := 0
	lastSlash := -1
	dots := 0
	var code byte
	for i := 0; i <= len(path); i++ {
		if i < len(path) {
			code = path[i]
		} else if isSep(code) {
			break
		} else {
			code = pathSepPosix
		}

		switch {
		case isSep(code):
			if lastSlash == i-1 || dots == 1 {
				// A repeated separator, or a "." segment: nothing to add.
			} else if dots == 2 {
				cur := res.String()
				if len(cur) < 2 || lastSegmentLength != 2 || cur[len(cur)-1] != '.' || cur[len(cur)-2] != '.' {
					if len(cur) > 2 {
						lastSlashIndex := strings.LastIndexByte(cur, sep)
						if lastSlashIndex == -1 {
							res.Reset()
							lastSegmentLength = 0
						} else {
							cur = cur[:lastSlashIndex]
							res.Reset()
							res.WriteString(cur)
							lastSegmentLength = len(cur) - 1 - strings.LastIndexByte(cur, sep)
						}
						lastSlash = i
						dots = 0
						continue
					} else if res.Len() != 0 {
						res.Reset()
						lastSegmentLength = 0
						lastSlash = i
						dots = 0
						continue
					}
				}
				if allowAboveRoot {
					if res.Len() > 0 {
						res.WriteByte(sep)
					}
					res.WriteString("..")
					lastSegmentLength = 2
				}
			} else {
				if res.Len() > 0 {
					res.WriteByte(sep)
				}
				res.WriteString(path[lastSlash+1 : i])
				lastSegmentLength = i - lastSlash - 1
			}
			lastSlash = i
			dots = 0
		case code == '.' && dots != -1:
			dots++
		default:
			dots = -1
		}
	}
	return res.String()
}

// pathNormalizeWin32 is path.win32.normalize.
func pathNormalizeWin32(path string) string {
	if len(path) == 0 {
		return "."
	}
	rootEnd := 0
	var device string
	isAbsolute := false
	code := path[0]

	if len(path) == 1 {
		// A one-character path is either a separator, which normalizes to one
		// backslash, or an ordinary name that is already normal.
		if isPathSeparatorWin32(code) {
			return `\`
		}
		return path
	}

	if isPathSeparatorWin32(code) {
		// A path beginning with a separator is absolute; two of them start a UNC
		// root, \\server\share, which is part of the root rather than of the path.
		isAbsolute = true
		if isPathSeparatorWin32(path[1]) {
			j := 2
			last := j
			for j < len(path) && !isPathSeparatorWin32(path[j]) {
				j++
			}
			if j < len(path) && j != last {
				firstPart := path[last:j]
				last = j
				for j < len(path) && isPathSeparatorWin32(path[j]) {
					j++
				}
				if j < len(path) && j != last {
					last = j
					for j < len(path) && !isPathSeparatorWin32(path[j]) {
						j++
					}
					if j == len(path) || j != last {
						switch {
						case firstPart == "." || firstPart == "?":
							// \\.\ and \\?\ open a device rather than a directory, so the
							// four characters are the whole root.
							device = `\\` + firstPart
							rootEnd = 4
							colonIndex := strings.IndexByte(path, ':')
							possibleDevice := ""
							if colonIndex >= 0 && colonIndex+1 >= 4 {
								possibleDevice = path[4 : colonIndex+1]
							}
							if isWindowsReservedName(possibleDevice, len(possibleDevice)-1) {
								// \\?\COM1: names a reserved device, and the name belongs to
								// the root the same way a drive letter does.
								device = `\\?\` + possibleDevice
								rootEnd = 4 + len(possibleDevice)
							}
						case j == len(path):
							return `\\` + firstPart + `\` + path[last:] + `\`
						default:
							device = `\\` + firstPart + `\` + path[last:j]
							rootEnd = j
						}
					}
				}
			}
		} else {
			rootEnd = 1
		}
	} else if colonIndex := strings.IndexByte(path, ':'); colonIndex > 0 {
		if isWindowsDeviceRoot(code) && colonIndex == 1 {
			device = path[:2]
			rootEnd = 2
			if len(path) > 2 && isPathSeparatorWin32(path[2]) {
				// A drive with a separator, C:\, is absolute. A drive without one, C:a, is
				// relative to that drive's own working directory, which is why the two
				// cannot share a code path.
				isAbsolute = true
				rootEnd = 3
			}
		} else if isWindowsReservedName(path, colonIndex) {
			device = path[:colonIndex+1]
			rootEnd = colonIndex + 1
		}
	}

	var tail string
	if rootEnd < len(path) {
		tail = normalizeStringPath(path[rootEnd:], !isAbsolute, pathSepWin32, isPathSeparatorWin32)
	}
	if len(tail) == 0 && !isAbsolute {
		tail = "."
	}
	if len(tail) > 0 && isPathSeparatorWin32(path[len(path)-1]) {
		tail += `\`
	}
	if !isAbsolute && device == "" && strings.ContainsRune(path, ':') {
		// The path was relative and named no device, so the answer must not come
		// out looking like one: "a\\C:\\b" normalizes to a tail whose first segment
		// Windows would read as a drive. Node prefixes ".\\" to keep the answer
		// relative, which is CVE-2024-36139.
		if len(tail) >= 2 && isWindowsDeviceRoot(tail[0]) && tail[1] == ':' {
			return `.\` + tail
		}
		for index := strings.IndexByte(path, ':'); index >= 0; {
			if index == len(path)-1 || isPathSeparatorWin32(path[index+1]) {
				return `.\` + tail
			}
			next := strings.IndexByte(path[index+1:], ':')
			if next < 0 {
				break
			}
			index += 1 + next
		}
	}
	if isWindowsReservedName(path, strings.IndexByte(path, ':')) {
		return `.\` + device + tail
	}
	if device == "" {
		if isAbsolute {
			return `\` + tail
		}
		return tail
	}
	if isAbsolute {
		return device + `\` + tail
	}
	return device + tail
}

// pathNormalizePosix is path.posix.normalize.
func pathNormalizePosix(path string) string {
	if len(path) == 0 {
		return "."
	}
	isAbsolute := path[0] == pathSepPosix
	trailingSeparator := path[len(path)-1] == pathSepPosix

	path = normalizeStringPath(path, !isAbsolute, pathSepPosix, isPathSeparatorPosix)

	if len(path) == 0 {
		if isAbsolute {
			return "/"
		}
		if trailingSeparator {
			return "./"
		}
		return "."
	}
	if trailingSeparator {
		path += "/"
	}
	if isAbsolute {
		return "/" + path
	}
	return path
}

// pathResolveWin32 is path.win32.resolve. cwd supplies the working directory the
// walk falls back to, and cwdForDrive the working directory of a named drive,
// which Windows tracks per drive; on a host that does not, the caller passes the
// process directory and the answer degrades to that drive's root.
func pathResolveWin32(args []string, cwd string, cwdForDrive func(string) string) string {
	var resolvedDevice, resolvedTail string
	resolvedAbsolute := false

	for i := len(args) - 1; i >= -1; i-- {
		var path string
		switch {
		case i >= 0:
			path = args[i]
			if len(path) == 0 {
				continue
			}
		case resolvedDevice == "":
			path = cwd
		default:
			// A drive was named without a root, so the rest of the path is relative to
			// that drive's own working directory rather than to the process one.
			path = cwdForDrive(resolvedDevice)
			if path == "" {
				path = cwd
			}
			// The directory found has to actually be on that drive. If it names a
			// different one, the drive's root is the only thing left to resolve
			// against.
			if !strings.EqualFold(path[:min(len(path), 2)], resolvedDevice) && len(path) > 2 && path[2] == pathSepWin32 {
				path = resolvedDevice + `\`
			}
		}

		length := len(path)
		rootEnd := 0
		device := ""
		isAbsolute := false
		var code byte
		if length > 0 {
			code = path[0]
		}

		if length == 1 {
			if isPathSeparatorWin32(code) {
				rootEnd = 1
				isAbsolute = true
			}
		} else if length > 1 && isPathSeparatorWin32(code) {
			isAbsolute = true
			if isPathSeparatorWin32(path[1]) {
				j := 2
				last := j
				for j < length && !isPathSeparatorWin32(path[j]) {
					j++
				}
				if j < length && j != last {
					firstPart := path[last:j]
					last = j
					for j < length && isPathSeparatorWin32(path[j]) {
						j++
					}
					if j < length && j != last {
						last = j
						for j < length && !isPathSeparatorWin32(path[j]) {
							j++
						}
						if j == length || j != last {
							if firstPart != "." && firstPart != "?" {
								device = `\\` + firstPart + `\` + path[last:j]
								rootEnd = j
							} else {
								// \\.\ and \\?\ open a device, so the four characters are the
								// whole root and nothing after them is a directory name.
								device = `\\` + firstPart
								rootEnd = 4
							}
						}
					}
				}
			} else {
				rootEnd = 1
			}
		} else if isWindowsDeviceRoot(code) && path[1] == ':' {
			device = path[:2]
			rootEnd = 2
			if length > 2 && isPathSeparatorWin32(path[2]) {
				isAbsolute = true
				rootEnd = 3
			}
		}

		if device != "" {
			if resolvedDevice != "" {
				if !strings.EqualFold(device, resolvedDevice) {
					// A path on another drive says nothing about this one, so it is skipped
					// rather than joined.
					continue
				}
			} else {
				resolvedDevice = device
			}
		}

		if resolvedAbsolute {
			if resolvedDevice != "" {
				break
			}
		} else {
			resolvedTail = path[rootEnd:] + `\` + resolvedTail
			resolvedAbsolute = isAbsolute
			if isAbsolute && resolvedDevice != "" {
				break
			}
		}
	}

	resolvedTail = normalizeStringPath(resolvedTail, !resolvedAbsolute, pathSepWin32, isPathSeparatorWin32)
	if resolvedAbsolute {
		return resolvedDevice + `\` + resolvedTail
	}
	if out := resolvedDevice + resolvedTail; out != "" {
		return out
	}
	return "."
}

// pathResolvePosix is path.posix.resolve.
func pathResolvePosix(args []string, cwd string) string {
	resolved := ""
	resolvedAbsolute := false

	for i := len(args) - 1; i >= -1 && !resolvedAbsolute; i-- {
		var path string
		if i >= 0 {
			path = args[i]
		} else {
			path = cwd
		}
		if len(path) == 0 {
			continue
		}
		resolved = path + "/" + resolved
		resolvedAbsolute = path[0] == pathSepPosix
	}

	resolved = normalizeStringPath(resolved, !resolvedAbsolute, pathSepPosix, isPathSeparatorPosix)

	if resolvedAbsolute {
		return "/" + resolved
	}
	if resolved != "" {
		return resolved
	}
	return "."
}

// pathIsAbsoluteWin32 is path.win32.isAbsolute.
func pathIsAbsoluteWin32(path string) bool {
	if len(path) == 0 {
		return false
	}
	if isPathSeparatorWin32(path[0]) {
		return true
	}
	// A drive letter alone is not absolute: C:a is relative to that drive's own
	// working directory, so the separator after the colon is what decides.
	return len(path) > 2 && isWindowsDeviceRoot(path[0]) && path[1] == ':' && isPathSeparatorWin32(path[2])
}

// pathIsAbsolutePosix is path.posix.isAbsolute.
func pathIsAbsolutePosix(path string) bool {
	return len(path) > 0 && path[0] == pathSepPosix
}

// pathJoinWin32 is path.win32.join. The joined string is normalized, and a
// leading pair of separators is preserved only when the first argument really
// started a UNC root, which is the wrinkle that keeps this from being a normalize
// of the concatenation.
func pathJoinWin32(args []string) string {
	if len(args) == 0 {
		return "."
	}
	var joined, firstPart string
	for _, arg := range args {
		if len(arg) == 0 {
			continue
		}
		if joined == "" {
			joined = arg
			firstPart = arg
		} else {
			joined += `\` + arg
		}
	}
	if joined == "" {
		return "."
	}

	// Node's comment for this is worth keeping: the joined path can only start a
	// UNC root if the first argument did, and a "\\" that came from joining two
	// arguments must collapse rather than become one.
	needsReplace := true
	slashCount := 0
	if isPathSeparatorWin32(firstPart[0]) {
		slashCount++
		if len(firstPart) > 1 && isPathSeparatorWin32(firstPart[1]) {
			slashCount++
			if len(firstPart) > 2 {
				if isPathSeparatorWin32(firstPart[2]) {
					slashCount++
				} else {
					// The first argument is a UNC root, so its leading pair stays.
					needsReplace = false
				}
			}
		}
	}
	if needsReplace {
		for slashCount < len(joined) && isPathSeparatorWin32(joined[slashCount]) {
			slashCount++
		}
		if slashCount >= 2 {
			joined = `\` + joined[slashCount:]
		}
	}
	return pathNormalizeWin32(joined)
}

// pathJoinPosix is path.posix.join.
func pathJoinPosix(args []string) string {
	if len(args) == 0 {
		return "."
	}
	joined := ""
	for _, arg := range args {
		if len(arg) == 0 {
			continue
		}
		if joined == "" {
			joined = arg
		} else {
			joined += "/" + arg
		}
	}
	if joined == "" {
		return "."
	}
	return pathNormalizePosix(joined)
}

// splitTrimmingTrailingEmpty splits a win32 path on its separator and drops the
// empty segment a trailing separator leaves behind, the way Node's relative does
// before it compares two paths segment by segment.
func splitTrimmingTrailingEmpty(path string) []string {
	parts := strings.Split(path, `\`)
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}

// lowerEach lowercases every string in a slice.
func lowerEach(parts []string) []string {
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = strings.ToLower(p)
	}
	return out
}

// pathRelativeWin32 is path.win32.relative.
func pathRelativeWin32(from, to string, cwd string, cwdForDrive func(string) string) string {
	if from == to {
		return ""
	}
	fromOrig := pathResolveWin32([]string{from}, cwd, cwdForDrive)
	toOrig := pathResolveWin32([]string{to}, cwd, cwdForDrive)
	if fromOrig == toOrig {
		return ""
	}
	fromLower := strings.ToLower(fromOrig)
	toLower := strings.ToLower(toOrig)
	if fromLower == toLower {
		return ""
	}

	if len(fromOrig) != len(fromLower) || len(toOrig) != len(toLower) {
		// Lowercasing changed the length of one of them, so the walk below, which
		// indexes the lowercased strings and slices the originals, would cut in the
		// wrong place. A few characters do this: Turkish dotted capital I lowercases
		// to two of them. Node compares segment by segment instead, and so does this.
		fromSplit := splitTrimmingTrailingEmpty(fromOrig)
		toSplit := splitTrimmingTrailingEmpty(toOrig)
		fromLen, toLen := len(fromSplit), len(toSplit)
		length := min(fromLen, toLen)
		// The segments are compared lowercased rather than with EqualFold, because
		// Node compares them lowercased and the two are not the same test: EqualFold
		// says dotted capital I and its two-character lowercase are different, and
		// lowercasing says they are the same. This branch exists for that character,
		// so answering it the other way would defeat the point of being here.
		fromFold := lowerEach(fromSplit)
		toFold := lowerEach(toSplit)
		i := 0
		for ; i < length; i++ {
			if fromFold[i] != toFold[i] {
				break
			}
		}
		if i == 0 {
			return toOrig
		}
		if i == length {
			if toLen > length {
				return strings.Join(toSplit[i:], `\`)
			}
			if fromLen > length {
				return strings.Repeat(`..\`, fromLen-1-i) + ".."
			}
			return ""
		}
		return strings.Repeat(`..\`, fromLen-i) + strings.Join(toSplit[i:], `\`)
	}

	fromStart := 0
	for fromStart < len(fromLower) && fromLower[fromStart] == pathSepWin32 {
		fromStart++
	}
	fromEnd := len(fromLower)
	for fromEnd-1 > fromStart && fromLower[fromEnd-1] == pathSepWin32 {
		fromEnd--
	}
	fromLen := fromEnd - fromStart

	toStart := 0
	for toStart < len(toLower) && toLower[toStart] == pathSepWin32 {
		toStart++
	}
	toEnd := len(toLower)
	for toEnd-1 > toStart && toLower[toEnd-1] == pathSepWin32 {
		toEnd--
	}
	toLen := toEnd - toStart

	length := fromLen
	if toLen < length {
		length = toLen
	}
	lastCommonSep := -1
	i := 0
	for ; i < length; i++ {
		fromCode := fromLower[fromStart+i]
		if fromCode != toLower[toStart+i] {
			break
		}
		if fromCode == pathSepWin32 {
			lastCommonSep = i
		}
	}

	// The two paths are on different drives or different UNC roots, so there is no
	// relative path between them and the answer is the destination itself.
	if i != length {
		if lastCommonSep == -1 {
			return toOrig
		}
	} else {
		if toLen > length {
			if toLower[toStart+i] == pathSepWin32 {
				return toOrig[toStart+i+1:]
			}
			if i == 2 {
				return toOrig[toStart+i:]
			}
		}
		if fromLen > length {
			if fromLower[fromStart+i] == pathSepWin32 {
				lastCommonSep = i
			} else if i == 2 {
				lastCommonSep = 3
			}
		}
		if lastCommonSep == -1 {
			lastCommonSep = 0
		}
	}

	var out strings.Builder
	for i = fromStart + lastCommonSep + 1; i <= fromEnd; i++ {
		if i == fromEnd || fromLower[i] == pathSepWin32 {
			if out.Len() == 0 {
				out.WriteString("..")
			} else {
				out.WriteString(`\..`)
			}
		}
	}

	toStart += lastCommonSep
	if out.Len() > 0 {
		return out.String() + toOrig[toStart:toEnd]
	}
	if toStart < len(toOrig) && toOrig[toStart] == pathSepWin32 {
		toStart++
	}
	return toOrig[toStart:toEnd]
}

// pathRelativePosix is path.posix.relative.
func pathRelativePosix(from, to string, cwd string) string {
	if from == to {
		return ""
	}
	from = pathResolvePosix([]string{from}, cwd)
	to = pathResolvePosix([]string{to}, cwd)
	if from == to {
		return ""
	}

	fromStart := 1
	fromEnd := len(from)
	fromLen := fromEnd - fromStart
	toStart := 1
	toLen := len(to) - toStart

	length := fromLen
	if toLen < length {
		length = toLen
	}
	lastCommonSep := -1
	i := 0
	for ; i < length; i++ {
		fromCode := from[fromStart+i]
		if fromCode != to[toStart+i] {
			break
		}
		if fromCode == pathSepPosix {
			lastCommonSep = i
		}
	}
	if i == length {
		if toLen > length {
			if to[toStart+i] == pathSepPosix {
				return to[toStart+i+1:]
			}
			if i == 0 {
				return to[toStart+i:]
			}
		} else if fromLen > length {
			if from[fromStart+i] == pathSepPosix {
				lastCommonSep = i
			} else if i == 0 {
				lastCommonSep = 0
			}
		}
	}

	var out strings.Builder
	for i = fromStart + lastCommonSep + 1; i <= fromEnd; i++ {
		if i == fromEnd || from[i] == pathSepPosix {
			if out.Len() == 0 {
				out.WriteString("..")
			} else {
				out.WriteString("/..")
			}
		}
	}
	return out.String() + to[toStart+lastCommonSep:]
}

// pathDirnameWin32 is path.win32.dirname.
func pathDirnameWin32(path string) string {
	length := len(path)
	if length == 0 {
		return "."
	}
	rootEnd := -1
	offset := 0
	code := path[0]

	if length == 1 {
		if isPathSeparatorWin32(code) {
			return path
		}
		return "."
	}

	if isPathSeparatorWin32(code) {
		rootEnd = 1
		offset = 1
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
						return path
					}
					if j != last {
						rootEnd = j + 1
						offset = rootEnd
					}
				}
			}
		}
	} else if isWindowsDeviceRoot(code) && path[1] == ':' {
		if length > 2 && isPathSeparatorWin32(path[2]) {
			rootEnd = 3
		} else {
			rootEnd = 2
		}
		offset = rootEnd
	}

	end := -1
	matchedSlash := true
	for i := length - 1; i >= offset; i-- {
		if isPathSeparatorWin32(path[i]) {
			if !matchedSlash {
				end = i
				break
			}
		} else {
			matchedSlash = false
		}
	}

	if end == -1 {
		if rootEnd == -1 {
			return "."
		}
		return path[:rootEnd]
	}
	return path[:end]
}

// pathDirnamePosix is path.posix.dirname.
func pathDirnamePosix(path string) string {
	if len(path) == 0 {
		return "."
	}
	hasRoot := path[0] == pathSepPosix
	end := -1
	matchedSlash := true
	for i := len(path) - 1; i >= 1; i-- {
		if path[i] == pathSepPosix {
			if !matchedSlash {
				end = i
				break
			}
		} else {
			matchedSlash = false
		}
	}
	if end == -1 {
		if hasRoot {
			return "/"
		}
		return "."
	}
	if hasRoot && end == 1 {
		return "//"
	}
	return path[:end]
}

// pathBasename is path.basename for both variants: the two differ only in which
// characters separate segments and in the drive root win32 skips, so isSep and
// the offset carry the whole difference.
func pathBasename(path, suffix string, isSep func(byte) bool, win32 bool) string {
	start := 0
	end := -1
	matchedSlash := true

	if win32 && len(path) >= 2 && isWindowsDeviceRoot(path[0]) && path[1] == ':' {
		start = 2
	}

	if len(suffix) > 0 && len(suffix) <= len(path) {
		if suffix == path {
			return ""
		}
		extIdx := len(suffix) - 1
		firstNonSlashEnd := -1
		for i := len(path) - 1; i >= start; i-- {
			code := path[i]
			if isSep(code) {
				// A trailing separator is not part of the name, and once a non-separator
				// has been seen the scan has found the end of the segment.
				if !matchedSlash {
					start = i + 1
					break
				}
			} else {
				if firstNonSlashEnd == -1 {
					matchedSlash = false
					firstNonSlashEnd = i + 1
				}
				if extIdx >= 0 {
					if code == suffix[extIdx] {
						extIdx--
						if extIdx == -1 {
							end = i
						}
					} else {
						extIdx = -1
						end = firstNonSlashEnd
					}
				}
			}
		}

		if start == end {
			end = firstNonSlashEnd
		} else if end == -1 {
			end = len(path)
		}
		return path[start:end]
	}

	for i := len(path) - 1; i >= start; i-- {
		if isSep(path[i]) {
			if !matchedSlash {
				start = i + 1
				break
			}
		} else if end == -1 {
			matchedSlash = false
			end = i + 1
		}
	}
	if end == -1 {
		return ""
	}
	return path[start:end]
}

// pathExtname is path.extname for both variants, which differ only in isSep and
// in the drive root.
func pathExtname(path string, isSep func(byte) bool, win32 bool) string {
	start := 0
	if win32 && len(path) >= 2 && path[1] == ':' && isWindowsDeviceRoot(path[0]) {
		start = 2
	}
	startDot := -1
	startPart := start
	end := -1
	matchedSlash := true
	// preDotState counts the dots before the extension: 0 means the name so far is
	// all dots, which is what makes ".bashrc" a name rather than an extension.
	preDotState := 0

	for i := len(path) - 1; i >= start; i-- {
		code := path[i]
		if isSep(code) {
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
		if code == '.' {
			if startDot == -1 {
				startDot = i
			} else if preDotState != 1 {
				preDotState = 1
			}
		} else if startDot != -1 {
			preDotState = -1
		}
	}

	if startDot == -1 || end == -1 || preDotState == 0 || (preDotState == 1 && startDot == end-1 && startDot == startPart+1) {
		return ""
	}
	return path[startDot:end]
}

// pathToNamespacedPathWin32 is path.win32.toNamespacedPath, which spells an
// absolute path in the form that escapes the Windows MAX_PATH limit. Node leaves
// anything it cannot resolve alone, and so does this.
func pathToNamespacedPathWin32(path string, cwd string, cwdForDrive func(string) string) string {
	if path == "" {
		return path
	}
	resolvedPath := pathResolveWin32([]string{path}, cwd, cwdForDrive)
	if len(resolvedPath) <= 2 {
		return path
	}
	if resolvedPath[0] == pathSepWin32 {
		// A UNC root becomes \\?\UNC\server\share, unless it is already a long
		// path (\\?\) or a device path (\\.\), which are namespaced as they stand.
		if resolvedPath[1] == pathSepWin32 && resolvedPath[2] != '?' && resolvedPath[2] != '.' {
			return `\\?\UNC\` + resolvedPath[2:]
		}
	} else if isWindowsDeviceRoot(resolvedPath[0]) && resolvedPath[1] == ':' && resolvedPath[2] == pathSepWin32 {
		return `\\?\` + resolvedPath
	}
	return resolvedPath
}

// pathToNamespacedPathPosix is path.posix.toNamespacedPath, which hands the path
// back untouched. posix has no namespace to name, and Node still ships the
// function so that code written for both platforms can call it either way.
func pathToNamespacedPathPosix(path string) string { return path }

// pathCwd returns the process working directory, the base a resolve walks back
// to. A failure to read it is reported as the root, which is what a program with
// no working directory can still say something sensible about, and is the same
// fallback the loader takes.
func pathCwd() string {
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	if onWindows {
		return `C:\`
	}
	return "/"
}

// pathCwdForDrive returns the working directory Windows tracks for one drive.
// The shell publishes it as an environment variable named "=C:", which is where
// Node reads it from too, so a compiled program resolves a drive-relative path
// (C:file) against the same directory the shell would.
func pathCwdForDrive(device string) string {
	return os.Getenv("=" + device)
}

// PathJoin is path.join: the parts are joined with the platform separator and the
// result normalized. With no parts, or only empty ones, it is ".".
func PathJoin(parts ...BStr) BStr {
	args := pathArgStrings(parts)
	if onWindows {
		return FromGoString(pathJoinWin32(args))
	}
	return FromGoString(pathJoinPosix(args))
}

// PathNormalize is path.normalize.
func PathNormalize(p BStr) BStr {
	if onWindows {
		return FromGoString(pathNormalizeWin32(p.ToGoString()))
	}
	return FromGoString(pathNormalizePosix(p.ToGoString()))
}

// PathResolve is path.resolve, which walks its arguments from the right until it
// has an absolute path, falling back to the working directory.
func PathResolve(parts ...BStr) BStr {
	args := pathArgStrings(parts)
	if onWindows {
		return FromGoString(pathResolveWin32(args, pathCwd(), pathCwdForDrive))
	}
	return FromGoString(pathResolvePosix(args, pathCwd()))
}

// PathDirname is path.dirname.
func PathDirname(p BStr) BStr {
	if onWindows {
		return FromGoString(pathDirnameWin32(p.ToGoString()))
	}
	return FromGoString(pathDirnamePosix(p.ToGoString()))
}

// PathBasename is path.basename with one argument.
func PathBasename(p BStr) BStr {
	return PathBasenameSuffix(p, BStr{})
}

// PathBasenameSuffix is path.basename with the optional suffix, which is stripped
// only when it is not the whole name.
func PathBasenameSuffix(p, suffix BStr) BStr {
	if onWindows {
		return FromGoString(pathBasename(p.ToGoString(), suffix.ToGoString(), isPathSeparatorWin32, true))
	}
	return FromGoString(pathBasename(p.ToGoString(), suffix.ToGoString(), isPathSeparatorPosix, false))
}

// PathExtname is path.extname.
func PathExtname(p BStr) BStr {
	if onWindows {
		return FromGoString(pathExtname(p.ToGoString(), isPathSeparatorWin32, true))
	}
	return FromGoString(pathExtname(p.ToGoString(), isPathSeparatorPosix, false))
}

// PathIsAbsolute is path.isAbsolute.
func PathIsAbsolute(p BStr) bool {
	if onWindows {
		return pathIsAbsoluteWin32(p.ToGoString())
	}
	return pathIsAbsolutePosix(p.ToGoString())
}

// PathRelative is path.relative, the path that walks from one to the other.
func PathRelative(from, to BStr) BStr {
	if onWindows {
		return FromGoString(pathRelativeWin32(from.ToGoString(), to.ToGoString(), pathCwd(), pathCwdForDrive))
	}
	return FromGoString(pathRelativePosix(from.ToGoString(), to.ToGoString(), pathCwd()))
}

// PathToNamespacedPath is path.toNamespacedPath, which is the identity
// everywhere except Windows.
func PathToNamespacedPath(p BStr) BStr {
	if onWindows {
		return FromGoString(pathToNamespacedPathWin32(p.ToGoString(), pathCwd(), pathCwdForDrive))
	}
	return FromGoString(pathToNamespacedPathPosix(p.ToGoString()))
}

// PathSep is path.sep, the character that separates segments on this platform.
func PathSep() BStr {
	if onWindows {
		return FromGoString(`\`)
	}
	return FromGoString("/")
}

// PathDelimiter is path.delimiter, the character that separates entries in a
// PATH-style list on this platform.
func PathDelimiter() BStr {
	if onWindows {
		return FromGoString(";")
	}
	return FromGoString(":")
}

// pathArgStrings unwraps a slice of bento strings to Go strings, the shape every
// variadic path helper works on.
func pathArgStrings(parts []BStr) []string {
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = p.ToGoString()
	}
	return out
}
