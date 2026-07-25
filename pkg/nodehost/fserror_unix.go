//go:build !windows

package nodehost

import "syscall"

// posixErrno is the POSIX errno behind each libuv code. Off Windows the two
// vocabularies are the same one, so this table is a translation only in the sense
// that it turns a number back into the name Node prints.
//
// libuv's codes that have no POSIX errno are absent, EOF and ECHARSET among them:
// nothing here can produce them, since every key exists to answer a number a
// syscall actually returned. So are the codes only some Unixes define, EFTYPE
// being the one a filesystem call could plausibly raise, because naming it here
// would break the build on Linux.
var posixErrno = map[string]syscall.Errno{
	"EACCES":          syscall.EACCES,
	"EADDRINUSE":      syscall.EADDRINUSE,
	"EADDRNOTAVAIL":   syscall.EADDRNOTAVAIL,
	"EAFNOSUPPORT":    syscall.EAFNOSUPPORT,
	"EAGAIN":          syscall.EAGAIN,
	"EALREADY":        syscall.EALREADY,
	"EBADF":           syscall.EBADF,
	"EBUSY":           syscall.EBUSY,
	"ECANCELED":       syscall.ECANCELED,
	"ECONNABORTED":    syscall.ECONNABORTED,
	"ECONNREFUSED":    syscall.ECONNREFUSED,
	"ECONNRESET":      syscall.ECONNRESET,
	"EDESTADDRREQ":    syscall.EDESTADDRREQ,
	"EEXIST":          syscall.EEXIST,
	"EFAULT":          syscall.EFAULT,
	"EFBIG":           syscall.EFBIG,
	"EHOSTUNREACH":    syscall.EHOSTUNREACH,
	"EILSEQ":          syscall.EILSEQ,
	"EINVAL":          syscall.EINVAL,
	"EIO":             syscall.EIO,
	"EISCONN":         syscall.EISCONN,
	"EISDIR":          syscall.EISDIR,
	"ELOOP":           syscall.ELOOP,
	"EMFILE":          syscall.EMFILE,
	"EMLINK":          syscall.EMLINK,
	"EMSGSIZE":        syscall.EMSGSIZE,
	"ENAMETOOLONG":    syscall.ENAMETOOLONG,
	"ENETDOWN":        syscall.ENETDOWN,
	"ENETUNREACH":     syscall.ENETUNREACH,
	"ENFILE":          syscall.ENFILE,
	"ENOBUFS":         syscall.ENOBUFS,
	"ENODEV":          syscall.ENODEV,
	"ENOENT":          syscall.ENOENT,
	"ENOMEM":          syscall.ENOMEM,
	"ENOSPC":          syscall.ENOSPC,
	"ENOSYS":          syscall.ENOSYS,
	"ENOTCONN":        syscall.ENOTCONN,
	"ENOTDIR":         syscall.ENOTDIR,
	"ENOTEMPTY":       syscall.ENOTEMPTY,
	"ENOTSOCK":        syscall.ENOTSOCK,
	"ENOTSUP":         syscall.ENOTSUP,
	"ENOTTY":          syscall.ENOTTY,
	"ENXIO":           syscall.ENXIO,
	"EOVERFLOW":       syscall.EOVERFLOW,
	"EPERM":           syscall.EPERM,
	"EPIPE":           syscall.EPIPE,
	"EPROTO":          syscall.EPROTO,
	"EPROTONOSUPPORT": syscall.EPROTONOSUPPORT,
	"EPROTOTYPE":      syscall.EPROTOTYPE,
	"ERANGE":          syscall.ERANGE,
	"EROFS":           syscall.EROFS,
	"ESOCKTNOSUPPORT": syscall.ESOCKTNOSUPPORT,
	"ESPIPE":          syscall.ESPIPE,
	"ESRCH":           syscall.ESRCH,
	"ETIMEDOUT":       syscall.ETIMEDOUT,
	"ETXTBSY":         syscall.ETXTBSY,
	"EXDEV":           syscall.EXDEV,
}

// uvCode names the libuv code a POSIX errno stands for.
func uvCode(en syscall.Errno) string {
	for code, want := range posixErrno {
		if want == en {
			return code
		}
	}
	return "UNKNOWN"
}

// uvErrno is the number Node reports as err.errno for a code. Off Windows libuv
// negates the platform's own errno, which is what UV__ERR(x) expands to in
// include/uv/errno.h, so ENOENT reads -2 here and -4058 on Windows.
func uvErrno(code string) int {
	if en, ok := posixErrno[code]; ok {
		return -int(en)
	}
	if n, ok := uvOnlyErrno[code]; ok {
		return n
	}
	return uvUnknownErrno
}

// uvOnlyErrno is libuv's number for the codes that stand for no POSIX errno.
// libuv takes those out of its own block on every platform, since there is
// nothing on the platform to derive them from.
//
// EFTYPE is the one entry that is not quite that. The BSDs do define it, so libuv
// there reads their number rather than this one, but nothing off Windows produces
// the code through uvCode above, so the value is never reported and the portable
// number is the honest thing to name.
// The name resolution codes belong to that group too, on every platform: they
// stand for a getaddrinfo failure rather than an errno, so libuv takes them out
// of its own block everywhere. ENOTFOUND is Node's relabelling of EAI_NONAME and
// carries EAI_NONAME's number, which is what a Node program reads as err.errno
// for a host that does not resolve.
var uvOnlyErrno = map[string]int{
	"ECHARSET":   -4080,
	"EFTYPE":     -4028,
	"EOF":        -4095,
	"EAI_AGAIN":  -3001,
	"EAI_FAIL":   -3004,
	"EAI_NONAME": -3008,
	"ENOTFOUND":  -3008,
}

// uvUnknownErrno is libuv's UV_UNKNOWN, the same number on every platform since
// it stands for no platform error at all.
const uvUnknownErrno = -4094
