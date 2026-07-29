// Package nodehost holds the host functions the AOT-compiled Node builtins call
// into. A factory in pkg/node/js references a bare __bento_* callee for work that
// needs the Go runtime, os detail or an object inspector, and the lowerer emits a
// call to the matching function here. The package imports only the standard
// library, never pkg/engine, so an AOT binary that calls into it stays free of the
// interpreter the way the whole AOT path is; the interpreter's own pkg/node host
// layer delegates to the same functions so the two share one implementation and
// never drift. pkg/value depends on this package rather than the other way about,
// which is what lets a value helper answer os.freemem from the same measurement
// the interpreter reads, so nothing here may import it back. It also reaches for
// golang.org/x/sys, which is
// where the per-platform system calls the os module's numbers come from live; that
// is the standard library by another name and not a step toward the interpreter.
package nodehost

import (
	"encoding/json"
	"net"
	"os"
	"runtime"
	"strconv"
	"unsafe"
)

// osInfo is the snapshot the os module reads. It is marshaled to JSON and handed
// across the bridge on each OSInfoJSON call so the JavaScript side stays free of
// platform detail.
type osInfo struct {
	Platform          string                    `json:"platform"`
	Arch              string                    `json:"arch"`
	Type              string                    `json:"type"`
	Release           string                    `json:"release"`
	Version           string                    `json:"version"`
	Machine           string                    `json:"machine"`
	Hostname          string                    `json:"hostname"`
	Homedir           string                    `json:"homedir"`
	Tmpdir            string                    `json:"tmpdir"`
	Endianness        string                    `json:"endianness"`
	Totalmem          uint64                    `json:"totalmem"`
	Freemem           uint64                    `json:"freemem"`
	Uptime            float64                   `json:"uptime"`
	Loadavg           [3]float64                `json:"loadavg"`
	CPUs              []CPUInfo                 `json:"cpus"`
	Parallelism       int                       `json:"availableParallelism"`
	NetworkInterfaces map[string][]NetInterface `json:"networkInterfaces"`
	UserInfo          UserInfo                  `json:"userInfo"`
}

// NetInterface is one address of one interface, an entry of the arrays
// os.networkInterfaces() keys by interface name. An interface with four addresses
// is four of these under one key, which is the shape Node reports.
type NetInterface struct {
	Address  string `json:"address"`
	Netmask  string `json:"netmask"`
	Family   string `json:"family"`
	MAC      string `json:"mac"`
	Internal bool   `json:"internal"`
	CIDR     string `json:"cidr"`
}

// UserInfo is what os.userInfo() answers about the user this process runs as.
// The two Windows fields have no meaning there and Node reports minus one for
// them, which is what the platform's own lookup answers.
type UserInfo struct {
	Username string `json:"username"`
	UID      int    `json:"uid"`
	GID      int    `json:"gid"`
	Shell    string `json:"shell"`
	Homedir  string `json:"homedir"`
}

// OSInfoJSON returns the os module snapshot marshaled to a JSON string, the value
// the os.js factory parses back into the numbers and strings os.platform, os.cpus,
// and the rest report. A marshal error yields "{}", so the JavaScript JSON.parse
// always has a well-formed object to read.
func OSInfoJSON() string {
	b, err := json.Marshal(collectOSInfo())
	if err != nil {
		return "{}"
	}
	return string(b)
}

func collectOSInfo() osInfo {
	hostname, _ := os.Hostname()
	home, _ := os.UserHomeDir()
	facts := readHostFacts()
	return osInfo{
		Platform:          nodePlatform(),
		Arch:              nodeArch(),
		Type:              osType(),
		Release:           facts.Release,
		Version:           facts.Version,
		Machine:           facts.Machine,
		Hostname:          hostname,
		Homedir:           home,
		Tmpdir:            os.TempDir(),
		Endianness:        endianness(),
		Totalmem:          facts.TotalMem,
		Freemem:           facts.FreeMem,
		Uptime:            facts.Uptime,
		Loadavg:           facts.Loadavg,
		CPUs:              cpuList(),
		Parallelism:       availableParallelism(),
		NetworkInterfaces: networkInterfaces(),
		UserInfo:          currentUser(home),
	}
}

// nodePlatform maps GOOS to the strings Node uses for process.platform.
func nodePlatform() string {
	if runtime.GOOS == "windows" {
		return "win32"
	}
	return runtime.GOOS
}

// nodeArch maps GOARCH to the strings Node uses for process.arch.
func nodeArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x64"
	case "386":
		return "ia32"
	default:
		return runtime.GOARCH
	}
}

// osType returns the os.type() string Node reports for the platform.
func osType() string {
	switch runtime.GOOS {
	case "darwin":
		return "Darwin"
	case "windows":
		return "Windows_NT"
	case "linux":
		return "Linux"
	default:
		return runtime.GOOS
	}
}

func endianness() string {
	var x uint16 = 1
	if *(*byte)(unsafe.Pointer(&x)) == 1 {
		return "LE"
	}
	return "BE"
}

func networkInterfaces() map[string][]NetInterface {
	out := map[string][]NetInterface{}
	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		internal := iface.Flags&net.FlagLoopback != 0
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			family := "IPv4"
			if ipnet.IP.To4() == nil {
				family = "IPv6"
			}
			ones, _ := ipnet.Mask.Size()
			out[iface.Name] = append(out[iface.Name], NetInterface{
				Address:  ipnet.IP.String(),
				Netmask:  net.IP(ipnet.Mask).String(),
				Family:   family,
				MAC:      iface.HardwareAddr.String(),
				Internal: internal,
				CIDR:     ipnet.IP.String() + "/" + strconv.Itoa(ones),
			})
		}
	}
	return out
}

func currentUser(home string) UserInfo {
	name := os.Getenv("USER")
	if name == "" {
		name = os.Getenv("USERNAME")
	}
	uid, gid := os.Getuid(), os.Getgid()
	return UserInfo{
		Username: name,
		UID:      uid,
		GID:      gid,
		Shell:    os.Getenv("SHELL"),
		Homedir:  home,
	}
}
