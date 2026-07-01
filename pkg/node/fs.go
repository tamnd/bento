package node

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// fsResult is the JSON envelope every filesystem host function returns. OK
// distinguishes success from a mapped error; the remaining fields are populated
// per operation and omitted when empty so the JS layer sees a tidy object.
type fsResult struct {
	OK      bool       `json:"ok"`
	Code    string     `json:"code,omitempty"`
	Errno   int        `json:"errno,omitempty"`
	Msg     string     `json:"msg,omitempty"`
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
		return `{"ok":false,"code":"UNKNOWN","msg":"marshal failed"}`
	}
	return string(b)
}

// ok builds a success envelope; callers set the payload fields they need.
func ok(r fsResult) string {
	r.OK = true
	return jsonString(r)
}

// fail maps a Go error to a Node-style error envelope (ENOENT, EEXIST, ...).
func fail(err error) string {
	code, errno := "UNKNOWN", -1
	if perr, ok := errors.AsType[*fs.PathError](err); ok {
		err = perr.Err
	}
	switch {
	case errors.Is(err, fs.ErrNotExist):
		code, errno = "ENOENT", -2
	case errors.Is(err, fs.ErrExist):
		code, errno = "EEXIST", -17
	case errors.Is(err, fs.ErrPermission):
		code, errno = "EACCES", -13
	default:
		if en, ok := errors.AsType[syscall.Errno](err); ok {
			code = errnoName(en)
		}
	}
	return jsonString(fsResult{OK: false, Code: code, Errno: errno, Msg: err.Error()})
}

func errnoName(en syscall.Errno) string {
	switch en {
	case syscall.ENOENT:
		return "ENOENT"
	case syscall.EEXIST:
		return "EEXIST"
	case syscall.EACCES:
		return "EACCES"
	case syscall.ENOTDIR:
		return "ENOTDIR"
	case syscall.EISDIR:
		return "EISDIR"
	case syscall.ENOTEMPTY:
		return "ENOTEMPTY"
	case syscall.EINVAL:
		return "EINVAL"
	default:
		return "UNKNOWN"
	}
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
	if resolved, rerr := filepath.EvalSymlinks(p); rerr == nil {
		p = resolved
	}
	return ok(fsResult{Path: p}), nil
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
	prefix := str(a, 0)
	dir, err := os.MkdirTemp(filepath.Dir(prefix), filepath.Base(prefix)+"*")
	if err != nil {
		return fail(err), nil
	}
	return ok(fsResult{Path: dir}), nil
}
