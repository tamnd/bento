package nodehost

import (
	"strconv"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// memoryStatusEx is the MEMORYSTATUSEX the kernel fills in with the machine's
// memory counters. x/sys/windows does not declare it, so it is declared here, in
// the layout the header gives it; the Length field must hold the struct's own size
// before the call, which is how the kernel knows which version of the struct it
// was handed.
type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

var (
	kernel32                = windows.NewLazySystemDLL("kernel32.dll")
	procGlobalMemoryStatus  = kernel32.NewProc("GlobalMemoryStatusEx")
	procGetTickCount64      = kernel32.NewProc("GetTickCount64")
	procGetNativeSystemInfo = kernel32.NewProc("GetNativeSystemInfo")
)

// systemInfo is the SYSTEM_INFO the kernel fills in with the processor
// architecture, the only field read here.
type systemInfo struct {
	ProcessorArchitecture     uint16
	Reserved                  uint16
	PageSize                  uint32
	MinimumApplicationAddress uintptr
	MaximumApplicationAddress uintptr
	ActiveProcessorMask       uintptr
	NumberOfProcessors        uint32
	ProcessorType             uint32
	AllocationGranularity     uint32
	ProcessorLevel            uint16
	ProcessorRevision         uint16
}

// The processor architectures GetNativeSystemInfo reports, in the numbering the
// header gives them.
const (
	archIntel = 0
	archARM   = 5
	archIA64  = 6
	archAMD64 = 9
	archARM64 = 12
)

// readHostFacts answers the Windows facts. Windows has no uname, so libuv builds
// the same three strings out of three different interfaces, and this follows it:
// the release is the version triple, the version is the product name the installer
// wrote to the registry, and the machine is the processor architecture spelled the
// way uname would spell it on a Unix.
func readHostFacts() hostFacts {
	var f hostFacts
	v := windows.RtlGetVersion()
	f.Release = strconv.FormatUint(uint64(v.MajorVersion), 10) + "." +
		strconv.FormatUint(uint64(v.MinorVersion), 10) + "." +
		strconv.FormatUint(uint64(v.BuildNumber), 10)
	f.Version = windowsProductName()
	f.Machine = windowsMachine()
	var m memoryStatusEx
	m.Length = uint32(unsafe.Sizeof(m))
	if r, _, _ := procGlobalMemoryStatus.Call(uintptr(unsafe.Pointer(&m))); r != 0 {
		f.TotalMem = m.TotalPhys
		f.FreeMem = m.AvailPhys
	}
	ms, _, _ := procGetTickCount64.Call()
	f.Uptime = float64(uint64(ms)) / 1000
	// Windows keeps no load average, and Node reports three zeros there rather than
	// inventing one, so the zero value of the field is already the right answer.
	return f
}

// windowsProductName reads the product name the installer recorded, the string
// os.version() answers on Windows ("Windows 11 Pro" and its kin). A machine that
// does not carry the value answers the empty string rather than a guess.
func windowsProductName() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	name, _, err := k.GetStringValue("ProductName")
	if err != nil {
		return ""
	}
	return name
}

// windowsMachine spells the processor architecture the way uname does on a Unix,
// which is what os.machine() answers on every platform. The native architecture is
// read rather than the process one, so a 32-bit binary on a 64-bit machine reports
// the machine and not itself.
func windowsMachine() string {
	var si systemInfo
	// GetNativeSystemInfo returns nothing and cannot fail, so there is no result to
	// read; the struct it filled in is the whole answer.
	_, _, _ = procGetNativeSystemInfo.Call(uintptr(unsafe.Pointer(&si)))
	switch si.ProcessorArchitecture {
	case archAMD64:
		return "x86_64"
	case archIntel:
		return "i686"
	case archARM64:
		return "arm64"
	case archARM:
		return "arm"
	case archIA64:
		return "ia64"
	default:
		return ""
	}
}
