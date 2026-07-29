package nodehost

import "os"

// This file is the direct, one fact per call side of node:os. The interpreter
// reads the whole snapshot as JSON and picks fields out of it in JavaScript,
// which is the right shape there because the module hands back objects. A
// compiled program calls the one fact it asked for, so it pays for that fact and
// not for enumerating the network interfaces on the way to the hostname.
//
// Both sides answer from the same measurements underneath, so os.freemem() cannot
// mean one thing to a program that was run and another to the same program built.

// OSPlatform is os.platform(), Node's name for the operating system.
func OSPlatform() string { return nodePlatform() }

// OSArch is os.arch(), Node's name for the processor architecture the binary was
// built for, which is not the name uname gives it. See OSMachine.
func OSArch() string { return nodeArch() }

// OSType is os.type(), the operating system name uname reports: Linux, Darwin,
// Windows_NT.
func OSType() string { return osType() }

// OSEndianness is os.endianness(), the byte order of the processor this binary
// runs on, measured rather than assumed from the architecture name.
func OSEndianness() string { return endianness() }

// OSRelease is os.release(), the kernel release string.
func OSRelease() string { return readHostFacts().Release }

// OSVersion is os.version(), the kernel version string, which on Windows is the
// product name instead since there is no kernel version string there.
func OSVersion() string { return readHostFacts().Version }

// OSMachine is os.machine(), the hardware name uname reports. It is a different
// question from OSArch and has a different answer on a 64-bit Intel machine,
// where uname says x86_64 and Node's arch says x64.
func OSMachine() string { return readHostFacts().Machine }

// OSHostname is os.hostname(). A machine that will not answer its own name yields
// the empty string, which is what Node reports when the lookup fails.
func OSHostname() string {
	name, err := os.Hostname()
	if err != nil {
		return ""
	}
	return name
}

// OSHomedir is os.homedir(), the current user's home directory.
func OSHomedir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

// OSTotalmem is os.totalmem(), the machine's physical memory in bytes. It is a
// float64 because that is the only number JavaScript has, and it is the type the
// compiled program does arithmetic on; a byte count of physical memory is far
// inside the range a float64 counts exactly.
func OSTotalmem() float64 { return float64(readHostFacts().TotalMem) }

// OSFreemem is os.freemem(), the memory not in use, in bytes. It is measured on
// each call, since it is the one number here that moves while a program runs.
func OSFreemem() float64 { return float64(readHostFacts().FreeMem) }

// OSUptime is os.uptime(), the seconds since the machine booted.
func OSUptime() float64 { return readHostFacts().Uptime }

// OSAvailableParallelism is os.availableParallelism(), the count of cores this
// process may run on.
func OSAvailableParallelism() float64 { return float64(availableParallelism()) }

// OSLoadavg is os.loadavg(), the one, five and fifteen minute run queue averages.
// Windows keeps no such thing and Node reports three zeros there.
func OSLoadavg() [3]float64 { return readHostFacts().Loadavg }

// OSCPUs is os.cpus(), one entry per core the machine has. It is measured on each
// call, since the times in it climb while a program runs and a program that wants
// a utilization figure reads it twice and subtracts.
func OSCPUs() []CPUInfo { return cpuList() }

// OSNetworkInterfaces is os.networkInterfaces(), the addresses of each interface
// keyed by its name, measured on each call because an interface can come up or go
// away while a program runs.
func OSNetworkInterfaces() map[string][]NetInterface { return networkInterfaces() }

// OSUserInfo is os.userInfo(), the user this process runs as.
func OSUserInfo() UserInfo { return currentUser(OSHomedir()) }
