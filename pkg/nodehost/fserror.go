package nodehost

import (
	"errors"
	"io/fs"
	"syscall"
)

// UVError is one failure in the shape a Node program reads it: the code it
// branches on, the number err.errno carries, and the description that goes in the
// message. Filesystem and socket errors are the same shape because they are the
// same vocabulary, libuv's, so one type answers for both.
//
// # Why this is not a raw errno comparison
//
// Node's filesystem errors are libuv's, and libuv has one set of codes on every
// platform. Underneath, an error on Windows is a Win32 error number that has
// nothing to do with the POSIX one: a missing file is ERROR_FILE_NOT_FOUND, which
// is 2, and so is POSIX ENOENT, but a directory that is not empty is
// ERROR_DIR_NOT_EMPTY at 145 where POSIX ENOTEMPTY is 39, and access denied is
// ERROR_ACCESS_DENIED at 5 where POSIX EACCES is 13. Comparing the number an
// error carries against syscall.ENOENT and friends therefore answers by accident
// on Windows: right for the two numbers that happen to coincide, wrong or UNKNOWN
// for the rest. A program that branches on err.code === "ENOTEMPTY" got UNKNOWN.
//
// So the translation is per platform, the way libuv's own is: uvCode lives in a
// pair of build-tagged files, one reading POSIX errnos and one reading Win32
// error numbers, and both answer in libuv's vocabulary. The tables and the
// descriptions come from libuv itself, include/uv/errno.h and src/win/error.c,
// so bento reports what Node reports rather than an approximation of it.
//
// # Why errno is not just the code's POSIX number
//
// Because err.errno is libuv's number, not the platform's. On every platform but
// Windows that is the negated POSIX errno (UV__ERR(x) is -x), so ENOENT is -2. On
// Windows libuv does not trust the C runtime's errno values at all and assigns
// its own block, where ENOENT is -4058. Both are in the platform files next to
// the code translation they belong with.
type UVError struct {
	// Code is libuv's name for the failure, the string err.code carries: ENOENT,
	// EEXIST, ENOTEMPTY and so on, or UNKNOWN for a failure no table names.
	Code string
	// Errno is libuv's number for that code, the value err.errno carries. It is
	// always negative.
	Errno int
	// Desc is libuv's description of the code, the middle of Node's message:
	// "no such file or directory" in "ENOENT: no such file or directory, open
	// '/nope'". For an unclassified failure it is the Go error's own text, which
	// says more than "unknown error" does.
	Desc string
}

// ClassifyFSError maps a Go filesystem error to the Node error it stands for.
//
// The platform's errno is asked first and the standard library's sentinels are
// the fallback, which is the opposite of the obvious order and is deliberate. The
// errno is the precise answer: fs.ErrPermission matches both EACCES and EPERM,
// which are one sentinel in Go and two different codes in Node, and there is no
// sentinel at all for ENOTEMPTY, EISDIR or ENOTDIR. The sentinels still matter,
// because an error that never came from a syscall carries no errno: one an io/fs
// implementation returned, or a bare os.ErrNotExist a helper wrapped.
func ClassifyFSError(err error) UVError {
	code := "UNKNOWN"
	if en, ok := errors.AsType[syscall.Errno](err); ok {
		code = uvCode(en)
	}
	if code == "UNKNOWN" {
		switch {
		case errors.Is(err, fs.ErrNotExist):
			code = "ENOENT"
		case errors.Is(err, fs.ErrExist):
			code = "EEXIST"
		case errors.Is(err, fs.ErrPermission):
			code = "EACCES"
		case errors.Is(err, fs.ErrClosed):
			code = "EBADF"
		}
	}
	return UVError{Code: code, Errno: uvErrno(code), Desc: uvDesc(code, err)}
}

// uvDesc is libuv's description of a code, or the Go error's own text when
// nothing named the failure. Saying what actually happened is worth more than
// libuv's "unknown error", which throws away the only description there is.
func uvDesc(code string, err error) string {
	if desc, known := uvErrorDesc[code]; known && code != "UNKNOWN" {
		return desc
	}
	return err.Error()
}

// uvErrorDesc is libuv's description for each code, copied from UV_ERRNO_MAP in
// libuv's include/uv.h. It is the string Node puts between the code and the
// syscall in a filesystem error message, so it has to be libuv's wording and not
// a paraphrase of it.
//
// The set covers what a filesystem call and a socket call can produce, which is
// most of libuv's map.
var uvErrorDesc = map[string]string{
	"EACCES":          "permission denied",
	"EADDRINUSE":      "address already in use",
	"EADDRNOTAVAIL":   "address not available",
	"EAFNOSUPPORT":    "address family not supported",
	"EAGAIN":          "resource temporarily unavailable",
	"EAI_AGAIN":       "temporary failure",
	"EAI_FAIL":        "permanent failure",
	"EAI_NONAME":      "unknown node or service",
	"EALREADY":        "connection already in progress",
	"EBADF":           "bad file descriptor",
	"EBUSY":           "resource busy or locked",
	"ECANCELED":       "operation canceled",
	"ECHARSET":        "invalid Unicode character",
	"ECONNABORTED":    "software caused connection abort",
	"ECONNREFUSED":    "connection refused",
	"ECONNRESET":      "connection reset by peer",
	"EDESTADDRREQ":    "destination address required",
	"EEXIST":          "file already exists",
	"EFAULT":          "bad address in system call argument",
	"EFBIG":           "file too large",
	"EFTYPE":          "inappropriate file type or format",
	"EHOSTUNREACH":    "host is unreachable",
	"EILSEQ":          "illegal byte sequence",
	"EINVAL":          "invalid argument",
	"EIO":             "i/o error",
	"EISCONN":         "socket is already connected",
	"EISDIR":          "illegal operation on a directory",
	"ELOOP":           "too many symbolic links encountered",
	"EMFILE":          "too many open files",
	"EMLINK":          "too many links",
	"EMSGSIZE":        "message too long",
	"ENAMETOOLONG":    "name too long",
	"ENETDOWN":        "network is down",
	"ENETUNREACH":     "network is unreachable",
	"ENFILE":          "file table overflow",
	"ENOBUFS":         "no buffer space available",
	"ENODEV":          "no such device",
	"ENOENT":          "no such file or directory",
	"ENOMEM":          "not enough memory",
	"ENOSPC":          "no space left on device",
	"ENOSYS":          "function not implemented",
	"ENOTCONN":        "socket is not connected",
	"ENOTDIR":         "not a directory",
	"ENOTEMPTY":       "directory not empty",
	"ENOTSOCK":        "socket operation on non-socket",
	"ENOTSUP":         "operation not supported on socket",
	"ENOTTY":          "inappropriate ioctl for device",
	"ENXIO":           "no such device or address",
	"EOF":             "end of file",
	"EOVERFLOW":       "value too large for defined data type",
	"EPERM":           "operation not permitted",
	"EPIPE":           "broken pipe",
	"EPROTO":          "protocol error",
	"EPROTONOSUPPORT": "protocol not supported",
	"EPROTOTYPE":      "protocol wrong type for socket",
	"ERANGE":          "result too large",
	"EROFS":           "read-only file system",
	"ESOCKTNOSUPPORT": "socket type not supported",
	"ESPIPE":          "invalid seek",
	"ESRCH":           "no such process",
	"ETIMEDOUT":       "connection timed out",
	"ETXTBSY":         "text file is busy",
	"EXDEV":           "cross-device link not permitted",
	"UNKNOWN":         "unknown error",
	// ENOTFOUND is Node's own, not libuv's: a name that does not resolve reports
	// EAI_NONAME through libuv and Node relabels it. The description is the one
	// libuv gives EAI_NONAME, since that is the failure being described.
	"ENOTFOUND": "unknown node or service",
}
