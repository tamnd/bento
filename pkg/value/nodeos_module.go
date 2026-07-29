package value

import (
	"sort"

	"github.com/tamnd/bento/pkg/nodehost"
)

// This file builds the os built-in module a compiled program gets from
// require('os'). It is the CommonJS half of node:os; the import half lowers to the
// helpers in nodefs.go, and both read the same measurements out of pkg/nodehost,
// so the two ways of asking cannot answer differently.
//
// The import half stops at the exports that answer with a string or a number,
// because the lowerer has no way to build an object at a call site yet. This half
// has no such limit: it is built at runtime out of the value model, which has
// objects and arrays, so os.cpus() and os.networkInterfaces() and os.userInfo()
// are all here. That makes require('os') the complete module and the import form
// the partial one, which is the opposite of the usual order and worth knowing.
//
// Every function is measured on call rather than captured once, except the ones
// that cannot change while a process runs. Node draws that line in the same place:
// a program that polls os.freemem() in a loop must see it move.

// osExports lists the module's members in the order Node's own os module defines
// them, which is the order Object.keys(require('os')) reports there. The order is
// part of what the module is: a program that enumerates the module, or prints it,
// sees what Node's prints. A Go map has no order, so the list is a slice.
//
// getPriority and setPriority are the two of Node's exports missing from it. They
// are the only two that change something rather than report it, they need a
// per-platform interface of their own, and a wrong nice value is the kind of
// plausible wrong number this whole surface is arranged to avoid, so they wait for
// a slice that can measure them.
var osExports = []struct {
	name string
	make func() Value
}{
	{"arch", func() Value { return osString("arch", nodehost.OSArch) }},
	{"availableParallelism", func() Value { return osNumber("availableParallelism", nodehost.OSAvailableParallelism) }},
	{"cpus", func() Value { return osCall("cpus", osCPUsValue) }},
	{"endianness", func() Value { return osString("endianness", nodehost.OSEndianness) }},
	{"freemem", func() Value { return osNumber("freemem", nodehost.OSFreemem) }},
	{"homedir", func() Value { return osString("homedir", nodehost.OSHomedir) }},
	{"hostname", func() Value { return osString("hostname", nodehost.OSHostname) }},
	{"loadavg", func() Value { return osCall("loadavg", osLoadavgValue) }},
	{"networkInterfaces", func() Value { return osCall("networkInterfaces", osNetworkInterfacesValue) }},
	{"platform", func() Value { return osString("platform", nodehost.OSPlatform) }},
	{"release", func() Value { return osString("release", nodehost.OSRelease) }},
	{"tmpdir", func() Value { return osCall("tmpdir", func() Value { return StringValue(Tmpdir()) }) }},
	{"totalmem", func() Value { return osNumber("totalmem", nodehost.OSTotalmem) }},
	{"type", func() Value { return osString("type", nodehost.OSType) }},
	{"userInfo", func() Value { return osCall("userInfo", osUserInfoValue) }},
	{"uptime", func() Value { return osNumber("uptime", nodehost.OSUptime) }},
	{"version", func() Value { return osString("version", nodehost.OSVersion) }},
	{"machine", func() Value { return osString("machine", nodehost.OSMachine) }},
	{"constants", osConstantsValue},
	// EOL and devNull are the module's two data exports, read rather than called,
	// and they answer from the same helpers the import form lowers to.
	{"EOL", func() Value { return StringValue(OSEOL()) }},
	{"devNull", func() Value { return StringValue(OSDevNull()) }},
}

// newOSModule builds the os core module, require('os') or require('node:os').
func newOSModule() Value {
	mod := NewObject()
	for _, e := range osExports {
		mod.Set(FromGoString(e.name), e.make())
	}
	return mod
}

// osString wraps a host fact that answers a string as the named module function.
// The fact is read on call rather than now, so a module built once still reports
// what is true when the program asks.
func osString(name string, answer func() string) Value {
	return WithName(NewFunc(func([]Value) Value {
		return StringValue(FromGoString(answer()))
	}), name)
}

// osNumber wraps a host fact that answers a number as the named module function.
func osNumber(name string, answer func() float64) Value {
	return WithName(NewFunc(func([]Value) Value {
		return Number(answer())
	}), name)
}

// osCall wraps a host fact that answers an object or an array as the named module
// function. Each of these builds a fresh value on every call, which is what Node
// does: a program that reads os.cpus() twice and subtracts must not be handed the
// same array twice.
func osCall(name string, answer func() Value) Value {
	return WithName(NewFunc(func([]Value) Value {
		return answer()
	}), name)
}

// osLoadavgValue renders the three run queue averages as the array os.loadavg()
// answers with.
func osLoadavgValue() Value {
	avg := nodehost.OSLoadavg()
	return NewArrayValue([]Value{Number(avg[0]), Number(avg[1]), Number(avg[2])})
}

// osCPUsValue renders the processor list as the array of objects os.cpus() answers
// with, each with its own times object, which is the shape a program that sums a
// core's states indexes into.
func osCPUsValue() Value {
	cpus := nodehost.OSCPUs()
	out := make([]Value, len(cpus))
	for i, c := range cpus {
		times := NewObject()
		times.Set(FromGoString("user"), Number(float64(c.Times.User)))
		times.Set(FromGoString("nice"), Number(float64(c.Times.Nice)))
		times.Set(FromGoString("sys"), Number(float64(c.Times.Sys)))
		times.Set(FromGoString("idle"), Number(float64(c.Times.Idle)))
		times.Set(FromGoString("irq"), Number(float64(c.Times.IRQ)))
		entry := NewObject()
		entry.Set(FromGoString("model"), StringValue(FromGoString(c.Model)))
		entry.Set(FromGoString("speed"), Number(float64(c.Speed)))
		entry.Set(FromGoString("times"), times)
		out[i] = entry
	}
	return NewArrayValue(out)
}

// osNetworkInterfacesValue renders the interfaces as the object os
// .networkInterfaces() answers with: interface name to array of addresses. The
// names are sorted, because a Go map has no order and a program that prints this
// object should print the same thing twice in a row.
func osNetworkInterfacesValue() Value {
	ifaces := nodehost.OSNetworkInterfaces()
	names := make([]string, 0, len(ifaces))
	for name := range ifaces {
		names = append(names, name)
	}
	sort.Strings(names)
	out := NewObject()
	for _, name := range names {
		addrs := ifaces[name]
		elems := make([]Value, len(addrs))
		for i, a := range addrs {
			entry := NewObject()
			entry.Set(FromGoString("address"), StringValue(FromGoString(a.Address)))
			entry.Set(FromGoString("netmask"), StringValue(FromGoString(a.Netmask)))
			entry.Set(FromGoString("family"), StringValue(FromGoString(a.Family)))
			entry.Set(FromGoString("mac"), StringValue(FromGoString(a.MAC)))
			entry.Set(FromGoString("internal"), Bool(a.Internal))
			entry.Set(FromGoString("cidr"), StringValue(FromGoString(a.CIDR)))
			elems[i] = entry
		}
		out.Set(FromGoString(name), NewArrayValue(elems))
	}
	return out
}

// osUserInfoValue renders the current user as the object os.userInfo() answers
// with. Node fills the two identifier fields with minus one on Windows, which has
// no such numbers, and that is what the platform's own lookup answers there.
func osUserInfoValue() Value {
	u := nodehost.OSUserInfo()
	out := NewObject()
	out.Set(FromGoString("uid"), Number(float64(u.UID)))
	out.Set(FromGoString("gid"), Number(float64(u.GID)))
	out.Set(FromGoString("username"), StringValue(FromGoString(u.Username)))
	out.Set(FromGoString("homedir"), StringValue(FromGoString(u.Homedir)))
	out.Set(FromGoString("shell"), StringValue(FromGoString(u.Shell)))
	return out
}

// osConstantsValue builds os.constants. Node groups them under signals, errno,
// priority and dlopen, and a program reads one of those groups rather than the
// object itself, so the groups are here and empty until the tables behind them
// are. An empty group is what the interpreter's os module already reports, so this
// is the two halves agreeing rather than a new gap.
func osConstantsValue() Value {
	out := NewObject()
	for _, group := range []string{"signals", "errno", "priority", "dlopen"} {
		out.Set(FromGoString(group), NewObject())
	}
	return out
}
