package nodehost

import "syscall"

// The Win32 error numbers a filesystem call raises, written out rather than
// imported because Go's syscall package names only a handful of them and
// golang.org/x/sys is not a dependency bento carries. The values are winerror.h's
// and were taken from golang.org/x/sys/windows, which lists them all.
//
// They are ordinary small positive numbers, which is why the synthetic constants
// below can share the same switch: Go's own E* values on Windows start at
// 1 << 29, so the two sets cannot collide.
const (
	errorInvalidFunction      syscall.Errno = 1
	errorFileNotFound         syscall.Errno = 2
	errorPathNotFound         syscall.Errno = 3
	errorTooManyOpenFiles     syscall.Errno = 4
	errorAccessDenied         syscall.Errno = 5
	errorInvalidHandle        syscall.Errno = 6
	errorNotEnoughMemory      syscall.Errno = 8
	errorInvalidData          syscall.Errno = 13
	errorOutOfMemory          syscall.Errno = 14
	errorInvalidDrive         syscall.Errno = 15
	errorNotSameDevice        syscall.Errno = 17
	errorWriteProtect         syscall.Errno = 19
	errorCRC                  syscall.Errno = 23
	errorGenFailure           syscall.Errno = 31
	errorSharingViolation     syscall.Errno = 32
	errorLockViolation        syscall.Errno = 33
	errorHandleDiskFull       syscall.Errno = 39
	errorNotSupported         syscall.Errno = 50
	errorFileExists           syscall.Errno = 80
	errorCannotMake           syscall.Errno = 82
	errorInvalidParameter     syscall.Errno = 87
	errorBrokenPipe           syscall.Errno = 109
	errorOpenFailed           syscall.Errno = 110
	errorBufferOverflow       syscall.Errno = 111
	errorDiskFull             syscall.Errno = 112
	errorSemTimeout           syscall.Errno = 121
	errorInsufficientBuffer   syscall.Errno = 122
	errorInvalidName          syscall.Errno = 123
	errorModNotFound          syscall.Errno = 126
	errorDirNotEmpty          syscall.Errno = 145
	errorBadPathname          syscall.Errno = 161
	errorAlreadyExists        syscall.Errno = 183
	errorBadExeFormat         syscall.Errno = 193
	errorEnvvarNotFound       syscall.Errno = 203
	errorFilenameExcedRange   syscall.Errno = 206
	errorDirectory            syscall.Errno = 267
	errorEATableFull          syscall.Errno = 277
	errorElevationRequired    syscall.Errno = 740
	errorOperationAborted     syscall.Errno = 995
	errorNoAccess             syscall.Errno = 998
	errorInvalidFlags         syscall.Errno = 1004
	errorNoUnicodeTranslation syscall.Errno = 1113
	errorIODevice             syscall.Errno = 1117
	errorPrivilegeNotHeld     syscall.Errno = 1314
	errorDiskCorrupt          syscall.Errno = 1393
	errorSymlinkNotSupported  syscall.Errno = 1464
	errorCantAccessFile       syscall.Errno = 1920
	errorCantResolveFilename  syscall.Errno = 1921
	errorInvalidReparseData   syscall.Errno = 4392
	errorBadPipe              syscall.Errno = 230
	errorPipeBusy             syscall.Errno = 231
	errorNoData               syscall.Errno = 232
	errorPipeNotConnected     syscall.Errno = 233
)

// uvCode names the libuv code a Windows error number stands for. The mapping is
// libuv's own uv_translate_sys_error from src/win/error.c, restricted to the
// errors a filesystem call can raise, so bento answers what Node answers rather
// than what a POSIX-shaped guess would.
//
// A few of them are worth reading twice, because they are not what a POSIX habit
// expects. ERROR_ACCESS_DENIED is EPERM and not EACCES. ERROR_DIRECTORY is ENOENT
// and not ENOTDIR. ERROR_INVALID_FUNCTION is EISDIR. ERROR_BROKEN_PIPE is EOF,
// which is a libuv code with no POSIX errno behind it.
//
// The second group is Go's own doing. Go's syscall package on Windows defines the
// POSIX names as synthetic values above 1 << 29 and returns them from the parts
// of the standard library that have no Win32 error to hand back, so those have to
// translate too. They are distinct numbers from the Win32 ones above, so one
// switch covers both.
func uvCode(en syscall.Errno) string {
	switch en {
	case errorElevationRequired, errorCantAccessFile, syscall.EACCES:
		return "EACCES"
	case errorNoData, syscall.EAGAIN:
		return "EAGAIN"
	case errorInvalidFlags, errorInvalidHandle, syscall.EBADF:
		return "EBADF"
	case errorLockViolation, errorPipeBusy, errorSharingViolation, syscall.EBUSY:
		return "EBUSY"
	case errorOperationAborted:
		return "ECANCELED"
	case errorNoUnicodeTranslation:
		return "ECHARSET"
	case errorAlreadyExists, errorFileExists, syscall.EEXIST:
		return "EEXIST"
	case errorNoAccess, syscall.EFAULT:
		return "EFAULT"
	case errorBadExeFormat:
		return "EFTYPE"
	case errorInsufficientBuffer, errorInvalidData, errorInvalidParameter,
		errorSymlinkNotSupported, syscall.EINVAL:
		return "EINVAL"
	case errorCRC, errorDiskCorrupt, errorGenFailure, errorIODevice, errorOpenFailed, syscall.EIO:
		return "EIO"
	case errorInvalidFunction, syscall.EISDIR:
		return "EISDIR"
	case errorCantResolveFilename, syscall.ELOOP:
		return "ELOOP"
	case errorTooManyOpenFiles, syscall.EMFILE:
		return "EMFILE"
	case errorBufferOverflow, errorFilenameExcedRange, syscall.ENAMETOOLONG:
		return "ENAMETOOLONG"
	// syscall.ENOENT and syscall.ENOTDIR are absent because on Windows they are
	// not separate values: Go defines them as aliases for ERROR_FILE_NOT_FOUND and
	// ERROR_PATH_NOT_FOUND, both of which are already here. That second alias is
	// the trap this file exists to close. A comparison against syscall.ENOTDIR on
	// Windows matches ERROR_PATH_NOT_FOUND, which is a missing directory somewhere
	// in the path, and libuv calls that ENOENT. The old mapping reported ENOTDIR
	// for it, so a Windows program checking err.code against Node's answer got a
	// different string than Node gives.
	case errorBadPathname, errorDirectory, errorEnvvarNotFound, errorFileNotFound,
		errorInvalidDrive, errorInvalidName, errorInvalidReparseData, errorModNotFound,
		errorPathNotFound:
		return "ENOENT"
	case errorNotEnoughMemory, errorOutOfMemory, syscall.ENOMEM:
		return "ENOMEM"
	case errorCannotMake, errorDiskFull, errorEATableFull, errorHandleDiskFull, syscall.ENOSPC:
		return "ENOSPC"
	case syscall.ENOSYS:
		return "ENOSYS"
	case errorDirNotEmpty, syscall.ENOTEMPTY:
		return "ENOTEMPTY"
	case errorNotSupported:
		return "ENOTSUP"
	case errorBrokenPipe:
		return "EOF"
	case errorAccessDenied, errorPrivilegeNotHeld, syscall.EPERM:
		return "EPERM"
	case errorBadPipe, errorPipeNotConnected, syscall.EPIPE:
		return "EPIPE"
	case errorWriteProtect, syscall.EROFS:
		return "EROFS"
	case errorSemTimeout:
		return "ETIMEDOUT"
	case errorNotSameDevice, syscall.EXDEV:
		return "EXDEV"
	}
	return "UNKNOWN"
}

// windowsErrno is libuv's number for each code on Windows, from the _WIN32 half
// of include/uv/errno.h. libuv assigns its own block there rather than reading
// the C runtime's errno values, because a Windows program is free to redefine
// those, so these numbers are not derivable from anything on the platform and
// have to be written down. This is why a Node program on Windows reports errno
// -4058 for a missing file where the same program on Linux reports -2.
var windowsErrno = map[string]int{
	"EACCES":       -4092,
	"EAGAIN":       -4088,
	"EBADF":        -4083,
	"EBUSY":        -4082,
	"ECANCELED":    -4081,
	"ECHARSET":     -4080,
	"EEXIST":       -4075,
	"EFAULT":       -4074,
	"EFBIG":        -4036,
	"EFTYPE":       -4028,
	"EILSEQ":       -4027,
	"EINVAL":       -4071,
	"EIO":          -4070,
	"EISDIR":       -4068,
	"ELOOP":        -4067,
	"EMFILE":       -4066,
	"EMLINK":       -4032,
	"ENAMETOOLONG": -4064,
	"ENFILE":       -4061,
	"ENODEV":       -4059,
	"ENOENT":       -4058,
	"ENOMEM":       -4057,
	"ENOSPC":       -4055,
	"ENOSYS":       -4054,
	"ENOTDIR":      -4052,
	"ENOTEMPTY":    -4051,
	"ENOTSUP":      -4049,
	"ENOTTY":       -4029,
	"ENXIO":        -4033,
	"EOF":          -4095,
	"EOVERFLOW":    -4026,
	"EPERM":        -4048,
	"EPIPE":        -4047,
	"ERANGE":       -4034,
	"EROFS":        -4043,
	"ESPIPE":       -4041,
	"ESRCH":        -4040,
	"ETIMEDOUT":    -4039,
	"ETXTBSY":      -4038,
	"EXDEV":        -4037,
}

// uvErrno is the number Node reports as err.errno for a code.
func uvErrno(code string) int {
	if n, ok := windowsErrno[code]; ok {
		return n
	}
	return uvUnknownErrno
}

// uvUnknownErrno is libuv's UV_UNKNOWN, the same number on every platform since
// it stands for no platform error at all.
const uvUnknownErrno = -4094
