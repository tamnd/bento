package node

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/tamnd/bento/pkg/nodehost"
)

// fsResult is the JSON envelope every filesystem host function returns. OK
// distinguishes success from a mapped error; the remaining fields are populated
// per operation and omitted when empty so the JS layer sees a tidy object.
type fsResult struct {
	OK bool `json:"ok"`
	// Code, Errno and Desc are the three parts of a Node filesystem error: the
	// string a program branches on, the number err.errno carries, and libuv's
	// description, which the JavaScript side puts between the code and the syscall
	// to build the message Node builds.
	Code  string `json:"code,omitempty"`
	Errno int    `json:"errno,omitempty"`
	Desc  string `json:"desc,omitempty"`
	// Syscall is set only when the call failed at a later step than the one the
	// JavaScript side would report on its own, and it overrides that name; see
	// failAt.
	Syscall string     `json:"syscall,omitempty"`
	B64     string     `json:"b64,omitempty"`
	Path    string     `json:"path,omitempty"`
	Stat    *statInfo  `json:"stat,omitempty"`
	Entries []dirEntry `json:"entries,omitempty"`
}

// statInfo is the platform-neutral stat snapshot the fs module turns into a
// Node Stats object. Times are milliseconds since the Unix epoch.
type statInfo struct {
	Size    int64   `json:"size"`
	Mode    uint32  `json:"mode"`
	Kind    string  `json:"kind"`
	MtimeMs float64 `json:"mtimeMs"`
	AtimeMs float64 `json:"atimeMs"`
	CtimeMs float64 `json:"ctimeMs"`
	Dev     int     `json:"dev"`
	Ino     int     `json:"ino"`
	Nlink   int     `json:"nlink"`
	UID     int     `json:"uid"`
	GID     int     `json:"gid"`
}

// dirEntry names one child of a directory and its kind (file, dir, symlink).
type dirEntry struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// jsonString marshals any envelope to a string for return across the bridge.
func jsonString[T any](v T) string {
	b, err := json.Marshal(v)
	if err != nil {
		return `{"ok":false,"code":"UNKNOWN","desc":"marshal failed"}`
	}
	return string(b)
}

// ok builds a success envelope; callers set the payload fields they need.
func ok(r fsResult) string {
	r.OK = true
	return jsonString(r)
}

// fail maps a Go error to a Node-style error envelope: the code a program
// branches on, the number err.errno carries, and libuv's description of it, which
// the JavaScript side assembles into Node's message. The classification is
// nodehost's so the interpreter and the AOT path answer the same thing, and so
// the Windows translation lives in one place; see pkg/nodehost/fserror.go for why
// it cannot be a comparison against syscall.ENOENT and friends.
func fail(err error) string {
	e := nodehost.ClassifyFSError(err)
	return jsonString(fsResult{OK: false, Code: e.Code, Errno: e.Errno, Desc: e.Desc})
}

// failAt is fail for a call that got further than the JavaScript side assumes.
// readFileSync reports itself as the open, since that is the step that fails for
// a path that is missing or unreadable, but a directory opens perfectly well and
// fails on the read. Node names the step that actually failed and gives no path
// at all in that case, printing "EISDIR: illegal operation on a directory, read",
// so the host says which syscall it was and the JS side takes its word for it.
func failAt(err error, syscall string) string {
	e := nodehost.ClassifyFSError(err)
	return jsonString(fsResult{OK: false, Code: e.Code, Errno: e.Errno, Desc: e.Desc, Syscall: syscall})
}

// fsHostFuncs returns the synchronous filesystem primitives the fs module builds
// on. The fs.promises and callback forms in JavaScript are layered on these.
func fsHostFuncs() map[string]HostFunc {
	return map[string]HostFunc{
		"__bento_fs_read":     hostFSRead,
		"__bento_fs_write":    hostFSWrite,
		"__bento_fs_stat":     func(a []any) (any, error) { return statEnvelope(str(a, 0), false), nil },
		"__bento_fs_lstat":    func(a []any) (any, error) { return statEnvelope(str(a, 0), true), nil },
		"__bento_fs_mkdir":    hostFSMkdir,
		"__bento_fs_rm":       hostFSRm,
		"__bento_fs_readdir":  hostFSReaddir,
		"__bento_fs_rename":   hostFSRename,
		"__bento_fs_copy":     hostFSCopy,
		"__bento_fs_realpath": hostFSRealpath,
		"__bento_fs_readlink": hostFSReadlink,
		"__bento_fs_symlink":  hostFSSymlink,
		"__bento_fs_chmod":    hostFSChmod,
		"__bento_fs_mkdtemp":  hostFSMkdtemp,
	}
}

func hostFSRead(a []any) (any, error) {
	data, err := os.ReadFile(str(a, 0))
	if err != nil {
		// os.ReadFile opens and then reads, and it says which of the two failed in
		// the PathError's Op, so a directory comes back as a failed read and is
		// reported as one.
		var perr *fs.PathError
		if errors.As(err, &perr) && perr.Op == "read" {
			return failAt(err, "read"), nil
		}
		return fail(err), nil
	}
	return ok(fsResult{B64: base64.StdEncoding.EncodeToString(data)}), nil
}

func hostFSWrite(a []any) (any, error) {
	data, decErr := base64.StdEncoding.DecodeString(str(a, 1))
	if decErr != nil {
		return fail(decErr), nil
	}
	flag := os.O_CREATE | os.O_WRONLY
	if str(a, 2) == "a" {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}
	f, err := os.OpenFile(str(a, 0), flag, 0o644)
	if err != nil {
		return fail(err), nil
	}
	_, werr := f.Write(data)
	cerr := f.Close()
	if werr != nil {
		return fail(werr), nil
	}
	if cerr != nil {
		return fail(cerr), nil
	}
	return ok(fsResult{}), nil
}

func statEnvelope(path string, link bool) string {
	info, err := os.Stat(path)
	if link {
		info, err = os.Lstat(path)
	}
	if err != nil {
		return fail(err)
	}
	return ok(fsResult{Stat: statOf(info)})
}

func statOf(info os.FileInfo) *statInfo {
	kind := "file"
	switch {
	case info.IsDir():
		kind = "dir"
	case info.Mode()&fs.ModeSymlink != 0:
		kind = "symlink"
	}
	mtimeMs := float64(info.ModTime().UnixNano()) / 1e6
	return &statInfo{
		Size:    info.Size(),
		Mode:    uint32(info.Mode().Perm()),
		Kind:    kind,
		MtimeMs: mtimeMs,
		AtimeMs: mtimeMs,
		CtimeMs: mtimeMs,
		Nlink:   1,
	}
}

func hostFSMkdir(a []any) (any, error) {
	path := str(a, 0)
	var err error
	if boolArg(a, 1) {
		err = os.MkdirAll(path, 0o755)
	} else {
		err = os.Mkdir(path, 0o755)
	}
	if err != nil {
		return fail(err), nil
	}
	return ok(fsResult{Path: path}), nil
}

func hostFSRm(a []any) (any, error) {
	path := str(a, 0)
	var err error
	if boolArg(a, 1) {
		err = os.RemoveAll(path)
	} else {
		err = os.Remove(path)
	}
	if err != nil {
		return fail(err), nil
	}
	return ok(fsResult{}), nil
}

func hostFSReaddir(a []any) (any, error) {
	entries, err := os.ReadDir(str(a, 0))
	if err != nil {
		return fail(err), nil
	}
	list := make([]dirEntry, 0, len(entries))
	for _, e := range entries {
		kind := "file"
		switch {
		case e.IsDir():
			kind = "dir"
		case e.Type()&fs.ModeSymlink != 0:
			kind = "symlink"
		}
		list = append(list, dirEntry{Name: e.Name(), Kind: kind})
	}
	return ok(fsResult{Entries: list}), nil
}

func hostFSRename(a []any) (any, error) {
	if err := os.Rename(str(a, 0), str(a, 1)); err != nil {
		return fail(err), nil
	}
	return ok(fsResult{}), nil
}

func hostFSCopy(a []any) (any, error) {
	data, err := os.ReadFile(str(a, 0))
	if err != nil {
		return fail(err), nil
	}
	if err := os.WriteFile(str(a, 1), data, 0o644); err != nil {
		return fail(err), nil
	}
	return ok(fsResult{}), nil
}

func hostFSRealpath(a []any) (any, error) {
	p, err := filepath.Abs(str(a, 0))
	if err != nil {
		return fail(err), nil
	}
	// Node's realpathSync throws when the path does not exist rather than
	// answering with a path that names nothing, and every other reason
	// EvalSymlinks fails, a symlink loop or a permission denial, is a throw there
	// too. Swallowing the error handed the caller a path it had no evidence for.
	resolved, rerr := filepath.EvalSymlinks(p)
	if rerr != nil {
		return fail(rerr), nil
	}
	return ok(fsResult{Path: resolved}), nil
}

func hostFSReadlink(a []any) (any, error) {
	target, err := os.Readlink(str(a, 0))
	if err != nil {
		return fail(err), nil
	}
	return ok(fsResult{Path: target}), nil
}

func hostFSSymlink(a []any) (any, error) {
	if err := os.Symlink(str(a, 0), str(a, 1)); err != nil {
		return fail(err), nil
	}
	return ok(fsResult{}), nil
}

func hostFSChmod(a []any) (any, error) {
	if err := os.Chmod(str(a, 0), os.FileMode(intArg(a, 1)&0o777)); err != nil {
		return fail(err), nil
	}
	return ok(fsResult{}), nil
}

func hostFSMkdtemp(a []any) (any, error) {
	// Node appends six random characters directly to the prefix in its parent
	// directory. MkdirTemp substitutes the trailing "*" with the random run.
	//
	// The split is filepath.Split and not Dir plus Base, because those two do not
	// compose back to the prefix when it ends in a separator: Dir("/tmp/") is
	// "/tmp" and Base("/tmp/") is "tmp", so mkdtempSync("/tmp/") would have made
	// /tmp/tmpXXXXXX where Node makes /tmp/XXXXXX. Split leaves the stem empty,
	// which with the appended "*" says exactly what the caller meant.
	prefix := str(a, 0)
	parent, stem := filepath.Split(prefix)
	if parent == "" {
		parent = "."
	}
	dir, err := os.MkdirTemp(parent, stem+"*")
	if err != nil {
		return fail(err), nil
	}
	return ok(fsResult{Path: dir}), nil
}
