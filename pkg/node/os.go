package node

import (
	"net"
	"os"
	"runtime"
	"strconv"
	"unsafe"
)

// osInfo is the snapshot the os module reads. It is marshaled to JSON and handed
// across the bridge on each __bento_os_info call so the JavaScript side stays
// free of platform detail.
type osInfo struct {
	Platform          string                `json:"platform"`
	Arch              string                `json:"arch"`
	Type              string                `json:"type"`
	Release           string                `json:"release"`
	Version           string                `json:"version"`
	Hostname          string                `json:"hostname"`
	Homedir           string                `json:"homedir"`
	Tmpdir            string                `json:"tmpdir"`
	Endianness        string                `json:"endianness"`
	Totalmem          uint64                `json:"totalmem"`
	Freemem           uint64                `json:"freemem"`
	Uptime            float64               `json:"uptime"`
	Loadavg           [3]float64            `json:"loadavg"`
	CPUs              []cpuInfo             `json:"cpus"`
	NetworkInterfaces map[string][]netIface `json:"networkInterfaces"`
	UserInfo          userInfo              `json:"userInfo"`
}

type cpuInfo struct {
	Model string   `json:"model"`
	Speed int      `json:"speed"`
	Times cpuTimes `json:"times"`
}

type cpuTimes struct {
	User int `json:"user"`
	Nice int `json:"nice"`
	Sys  int `json:"sys"`
	Idle int `json:"idle"`
	IRQ  int `json:"irq"`
}

type netIface struct {
	Address  string `json:"address"`
	Netmask  string `json:"netmask"`
	Family   string `json:"family"`
	MAC      string `json:"mac"`
	Internal bool   `json:"internal"`
	CIDR     string `json:"cidr"`
}

type userInfo struct {
	Username string `json:"username"`
	UID      int    `json:"uid"`
	GID      int    `json:"gid"`
	Shell    string `json:"shell"`
	Homedir  string `json:"homedir"`
}

// osHostFuncs exposes the single os snapshot call.
func osHostFuncs() map[string]HostFunc {
	return map[string]HostFunc{
		"__bento_os_info": func(_ []any) (any, error) { return jsonString(collectOSInfo()), nil },
	}
}

func collectOSInfo() osInfo {
	hostname, _ := os.Hostname()
	home, _ := os.UserHomeDir()
	return osInfo{
		Platform:          nodePlatform(),
		Arch:              nodeArch(),
		Type:              osType(),
		Release:           "",
		Version:           "",
		Hostname:          hostname,
		Homedir:           home,
		Tmpdir:            os.TempDir(),
		Endianness:        endianness(),
		Totalmem:          0,
		Freemem:           0,
		Uptime:            0,
		Loadavg:           [3]float64{0, 0, 0},
		CPUs:              cpuList(),
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

func cpuList() []cpuInfo {
	n := runtime.NumCPU()
	out := make([]cpuInfo, n)
	for i := range out {
		out[i] = cpuInfo{Model: "unknown", Speed: 0}
	}
	return out
}

func networkInterfaces() map[string][]netIface {
	out := map[string][]netIface{}
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
			out[iface.Name] = append(out[iface.Name], netIface{
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

func currentUser(home string) userInfo {
	name := os.Getenv("USER")
	if name == "" {
		name = os.Getenv("USERNAME")
	}
	uid, gid := os.Getuid(), os.Getgid()
	return userInfo{
		Username: name,
		UID:      uid,
		GID:      gid,
		Shell:    os.Getenv("SHELL"),
		Homedir:  home,
	}
}
