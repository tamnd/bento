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

// The Winsock error numbers a socket call raises. They are a block of their own
// starting at 10000, so they collide with neither the Win32 numbers above nor
// Go's synthetic constants, and one switch can answer for all three.
//
// Windows keeps them apart from the Win32 errors deliberately: WSAECONNREFUSED is
// 10061 and has no relation to POSIX ECONNREFUSED, which is 111 on Linux and 61
// on macOS. A socket error on Windows therefore carries a number that matches
// nothing a POSIX-shaped comparison would look for, which is why bento reported
// no code at all for a refused connection there.
const (
	wsaEINTR           syscall.Errno = 10004
	wsaEACCES          syscall.Errno = 10013
	wsaEFAULT          syscall.Errno = 10014
	wsaEINVAL          syscall.Errno = 10022
	wsaEMFILE          syscall.Errno = 10024
	wsaEWOULDBLOCK     syscall.Errno = 10035
	wsaEALREADY        syscall.Errno = 10037
	wsaENOTSOCK        syscall.Errno = 10038
	wsaEDESTADDRREQ    syscall.Errno = 10039
	wsaEMSGSIZE        syscall.Errno = 10040
	wsaEPROTOTYPE      syscall.Errno = 10041
	wsaEPROTONOSUPPORT syscall.Errno = 10043
	wsaESOCKTNOSUPPORT syscall.Errno = 10044
	wsaEPFNOSUPPORT    syscall.Errno = 10046
	wsaEAFNOSUPPORT    syscall.Errno = 10047
	wsaEADDRINUSE      syscall.Errno = 10048
	wsaEADDRNOTAVAIL   syscall.Errno = 10049
	wsaENETDOWN        syscall.Errno = 10050
	wsaENETUNREACH     syscall.Errno = 10051
	wsaECONNABORTED    syscall.Errno = 10053
	wsaECONNRESET      syscall.Errno = 10054
	wsaENOBUFS         syscall.Errno = 10055
	wsaEISCONN         syscall.Errno = 10056
	wsaENOTCONN        syscall.Errno = 10057
	wsaESHUTDOWN       syscall.Errno = 10058
	wsaETIMEDOUT       syscall.Errno = 10060
	wsaECONNREFUSED    syscall.Errno = 10061
	wsaEHOSTUNREACH    syscall.Errno = 10065
	wsaHostNotFound    syscall.Errno = 11001
	wsaNoData          syscall.Errno = 11004
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
	case errorElevationRequired, errorCantAccessFile, wsaEACCES, syscall.EACCES:
		return "EACCES"
	case wsaEADDRINUSE:
		return "EADDRINUSE"
	case wsaEADDRNOTAVAIL:
		return "EADDRNOTAVAIL"
	case wsaEAFNOSUPPORT:
		return "EAFNOSUPPORT"
	case errorNoData, wsaEWOULDBLOCK, syscall.EAGAIN:
		return "EAGAIN"
	case wsaEALREADY:
		return "EALREADY"
	case errorInvalidFlags, errorInvalidHandle, syscall.EBADF:
		return "EBADF"
	case errorLockViolation, errorPipeBusy, errorSharingViolation, syscall.EBUSY:
		return "EBUSY"
	case errorOperationAborted, wsaEINTR:
		return "ECANCELED"
	case errorNoUnicodeTranslation:
		return "ECHARSET"
	case wsaECONNABORTED:
		return "ECONNABORTED"
	case wsaECONNREFUSED:
		return "ECONNREFUSED"
	case wsaECONNRESET:
		return "ECONNRESET"
	case wsaEDESTADDRREQ:
		return "EDESTADDRREQ"
	case errorAlreadyExists, errorFileExists, syscall.EEXIST:
		return "EEXIST"
	case errorNoAccess, wsaEFAULT, syscall.EFAULT:
		return "EFAULT"
	case errorBadExeFormat:
		return "EFTYPE"
	case wsaEHOSTUNREACH:
		return "EHOSTUNREACH"
	case errorInsufficientBuffer, errorInvalidData, errorInvalidParameter,
		errorSymlinkNotSupported, wsaEINVAL, wsaEPFNOSUPPORT, syscall.EINVAL:
		return "EINVAL"
	case errorCRC, errorDiskCorrupt, errorGenFailure, errorIODevice, errorOpenFailed, syscall.EIO:
		return "EIO"
	case wsaEISCONN:
		return "EISCONN"
	case errorInvalidFunction, syscall.EISDIR:
		return "EISDIR"
	case errorCantResolveFilename, syscall.ELOOP:
		return "ELOOP"
	case errorTooManyOpenFiles, wsaEMFILE, syscall.EMFILE:
		return "EMFILE"
	case wsaEMSGSIZE:
		return "EMSGSIZE"
	case errorBufferOverflow, errorFilenameExcedRange, syscall.ENAMETOOLONG:
		return "ENAMETOOLONG"
	case wsaENETDOWN:
		return "ENETDOWN"
	case wsaENETUNREACH:
		return "ENETUNREACH"
	case wsaENOBUFS:
		return "ENOBUFS"
	// syscall.ENOENT and syscall.ENOTDIR are absent because on Windows they are
	// not separate values: Go defines them as aliases for ERROR_FILE_NOT_FOUND and
	// ERROR_PATH_NOT_FOUND, both of which are already here. That second alias is
	// the trap this file exists to close. A comparison against syscall.ENOTDIR on
	// Windows matches ERROR_PATH_NOT_FOUND, which is a missing directory somewhere
	// in the path, and libuv calls that ENOENT. The old mapping reported ENOTDIR
	// for it, so a Windows program checking err.code against Node's answer got a
	// different string than Node gives.
	// The two Winsock entries are libuv's own doing: a name that does not resolve
	// reports ENOENT from uv_translate_sys_error, and Node relabels it ENOTFOUND on
	// its way out of getaddrinfo. Nothing here can tell the two apart, so the
	// relabelling belongs to the resolver path in ClassifySocketError, which sees a
	// *net.DNSError and answers ENOTFOUND before asking the errno at all.
	case errorBadPathname, errorDirectory, errorEnvvarNotFound, errorFileNotFound,
		errorInvalidDrive, errorInvalidName, errorInvalidReparseData, errorModNotFound,
		errorPathNotFound, wsaHostNotFound, wsaNoData:
		return "ENOENT"
	case errorNotEnoughMemory, errorOutOfMemory, syscall.ENOMEM:
		return "ENOMEM"
	case errorCannotMake, errorDiskFull, errorEATableFull, errorHandleDiskFull, syscall.ENOSPC:
		return "ENOSPC"
	case syscall.ENOSYS:
		return "ENOSYS"
	case wsaENOTCONN:
		return "ENOTCONN"
	case errorDirNotEmpty, syscall.ENOTEMPTY:
		return "ENOTEMPTY"
	case wsaENOTSOCK:
		return "ENOTSOCK"
	case errorNotSupported:
		return "ENOTSUP"
	case errorBrokenPipe:
		return "EOF"
	case errorAccessDenied, errorPrivilegeNotHeld, syscall.EPERM:
		return "EPERM"
	case errorBadPipe, errorPipeNotConnected, wsaESHUTDOWN, syscall.EPIPE:
		return "EPIPE"
	case wsaEPROTONOSUPPORT:
		return "EPROTONOSUPPORT"
	case wsaEPROTOTYPE:
		return "EPROTOTYPE"
	case errorWriteProtect, syscall.EROFS:
		return "EROFS"
	case wsaESOCKTNOSUPPORT:
		return "ESOCKTNOSUPPORT"
	case errorSemTimeout, wsaETIMEDOUT:
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
//
// The name resolution codes at the end are not from that block. They stand for a
// getaddrinfo failure rather than an error number, so libuv gives them the same
// values everywhere; ENOTFOUND is Node's relabelling of EAI_NONAME and carries
// EAI_NONAME's number.
var windowsErrno = map[string]int{
	"EACCES":          -4092,
	"EADDRINUSE":      -4091,
	"EADDRNOTAVAIL":   -4090,
	"EAFNOSUPPORT":    -4089,
	"EAGAIN":          -4088,
	"EALREADY":        -4084,
	"EBADF":           -4083,
	"EBUSY":           -4082,
	"ECANCELED":       -4081,
	"ECHARSET":        -4080,
	"ECONNABORTED":    -4079,
	"ECONNREFUSED":    -4078,
	"ECONNRESET":      -4077,
	"EDESTADDRREQ":    -4076,
	"EEXIST":          -4075,
	"EFAULT":          -4074,
	"EFBIG":           -4036,
	"EFTYPE":          -4028,
	"EHOSTUNREACH":    -4073,
	"EILSEQ":          -4027,
	"EINVAL":          -4071,
	"EIO":             -4070,
	"EISCONN":         -4069,
	"EISDIR":          -4068,
	"ELOOP":           -4067,
	"EMFILE":          -4066,
	"EMLINK":          -4032,
	"EMSGSIZE":        -4065,
	"ENAMETOOLONG":    -4064,
	"ENETDOWN":        -4063,
	"ENETUNREACH":     -4062,
	"ENFILE":          -4061,
	"ENOBUFS":         -4060,
	"ENODEV":          -4059,
	"ENOENT":          -4058,
	"ENOMEM":          -4057,
	"ENOSPC":          -4055,
	"ENOSYS":          -4054,
	"ENOTCONN":        -4053,
	"ENOTDIR":         -4052,
	"ENOTEMPTY":       -4051,
	"ENOTSOCK":        -4050,
	"ENOTSUP":         -4049,
	"ENOTTY":          -4029,
	"ENXIO":           -4033,
	"EOF":             -4095,
	"EOVERFLOW":       -4026,
	"EPERM":           -4048,
	"EPIPE":           -4047,
	"EPROTO":          -4046,
	"EPROTONOSUPPORT": -4045,
	"EPROTOTYPE":      -4044,
	"ERANGE":          -4034,
	"EROFS":           -4043,
	"ESOCKTNOSUPPORT": -4025,
	"ESPIPE":          -4041,
	"ESRCH":           -4040,
	"ETIMEDOUT":       -4039,
	"ETXTBSY":         -4038,
	"EXDEV":           -4037,
	"EAI_AGAIN":       -3001,
	"EAI_FAIL":        -3004,
	"EAI_NONAME":      -3008,
	"ENOTFOUND":       -3008,
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
