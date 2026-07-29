package nodehost

import (
	"encoding/json"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// TestCPUListHasAShapeAProgramCanUse checks the array against what every entry of
// it means, on any platform. A negative time or a negative clock is a number read
// off the wrong offset or converted with the wrong sign, and neither is something
// a program that graphs these would survive.
func TestCPUListHasAShapeAProgramCanUse(t *testing.T) {
	cpus := cpuList()
	if len(cpus) == 0 {
		t.Fatal("cpus() is empty; a machine running this has at least one core")
	}
	for i, c := range cpus {
		if c.Model == "" {
			t.Errorf("cpu %d has an empty model; an unknown one is spelled %q", i, "unknown")
		}
		if c.Speed < 0 {
			t.Errorf("cpu %d has speed %d", i, c.Speed)
		}
		for _, tc := range []struct {
			what string
			ms   int
		}{
			{"user", c.Times.User},
			{"nice", c.Times.Nice},
			{"sys", c.Times.Sys},
			{"idle", c.Times.Idle},
			{"irq", c.Times.IRQ},
		} {
			if tc.ms < 0 {
				t.Errorf("cpu %d has %s time %d", i, tc.what, tc.ms)
			}
		}
	}
}

// TestCPUsAreMeasured pins that the platforms with a file of their own report a
// real processor rather than the unknown-at-zero row every platform used to
// report. The model and the clock are what a program prints; the times are checked
// where a platform keeps them.
func TestCPUsAreMeasured(t *testing.T) {
	if !measured() {
		t.Skipf("no processor list is read on %s yet", runtime.GOOS)
	}
	for i, c := range cpuList() {
		if c.Model == "unknown" {
			t.Errorf("cpu %d has an unknown model on %s", i, runtime.GOOS)
		}
		if c.Speed == 0 {
			// A machine with no cpufreq driver, which most virtual Linux ones are, has no
			// clock to report and Node reports zero there too. It is worth saying so
			// rather than failing, since the model above is the part that would be wrong.
			t.Logf("cpu %d reports no clock; this machine publishes none", i)
		}
	}
}

// TestCPUCountMatchesTheMachine checks the length against the count the platform
// publishes, which is the machine's core count and not the count of cores this
// process is scheduled on. The two differ under a taskset or a cpuset, and Node
// answers the machine's count from os.cpus() and the process's from
// os.availableParallelism().
func TestCPUCountMatchesTheMachine(t *testing.T) {
	var want int
	switch runtime.GOOS {
	case "linux":
		if _, err := exec.LookPath("nproc"); err != nil {
			t.Skip("nproc not found on PATH")
		}
		// --all counts the cores installed rather than the ones this process may use,
		// which is the question os.cpus() answers.
		want = intFromCommand(t, "nproc", "--all")
	case "darwin":
		if _, err := exec.LookPath("sysctl"); err != nil {
			t.Skip("sysctl not found on PATH")
		}
		want = intFromCommand(t, "sysctl", "-n", "hw.logicalcpu")
	default:
		t.Skipf("no core-count oracle on %s", runtime.GOOS)
	}
	if got := len(cpuList()); got != want {
		t.Errorf("cpus() has %d entries, the machine has %d cores", got, want)
	}
}

// TestCPUModelsMatchTheKernel checks the model string against the one the kernel
// publishes. It is a string copied out of a file or a sysctl, so what it catches
// is a copy that took the wrong field or trimmed the wrong end of it, which leaves
// something that still looks like a processor name.
func TestCPUModelsMatchTheKernel(t *testing.T) {
	cpus := cpuList()
	switch runtime.GOOS {
	case "linux":
		b, err := os.ReadFile("/proc/cpuinfo")
		if err != nil {
			t.Skipf("read /proc/cpuinfo: %v", err)
		}
		var want []string
		for _, line := range strings.Split(string(b), "\n") {
			key, val, ok := strings.Cut(line, ":")
			if ok && strings.TrimSpace(key) == "model name" {
				want = append(want, strings.TrimSpace(val))
			}
		}
		if len(want) == 0 {
			t.Skip("this kernel writes no model name to /proc/cpuinfo")
		}
		if len(want) != len(cpus) {
			t.Fatalf("/proc/cpuinfo names %d processors, cpus() has %d", len(want), len(cpus))
		}
		for i, c := range cpus {
			if c.Model != want[i] {
				t.Errorf("cpu %d model is %q, /proc/cpuinfo says %q", i, c.Model, want[i])
			}
		}
	case "darwin":
		if _, err := exec.LookPath("sysctl"); err != nil {
			t.Skip("sysctl not found on PATH")
		}
		out, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output()
		if err != nil {
			t.Fatalf("sysctl machdep.cpu.brand_string: %v", err)
		}
		want := strings.TrimSpace(string(out))
		for i, c := range cpus {
			if c.Model != want {
				t.Errorf("cpu %d model is %q, the kernel says %q", i, c.Model, want)
			}
		}
	default:
		t.Skipf("no model oracle on %s", runtime.GOOS)
	}
}

// TestCPUTimesMatchTheKernel checks the times against the counters the kernel
// publishes, in the unit Node reports rather than the unit the kernel keeps. The
// conversion is where these go wrong: a count of ticks reported as milliseconds is
// off by a factor of ten and still looks like a plausible number of milliseconds.
//
// The counters climb while the test runs, so the check is that each one is at
// least what it was when the kernel was read and no more than a second's worth of
// climbing past it.
func TestCPUTimesMatchTheKernel(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("no per-core time oracle on %s", runtime.GOOS)
	}
	want := procStatTimes(t)
	got := cpuList()
	if len(got) != len(want) {
		t.Fatalf("/proc/stat has %d cores, cpus() has %d", len(want), len(got))
	}
	// A core cannot have spent more than the wall time of this test in any one
	// state, and the two reads are milliseconds apart, so a generous second of slack
	// covers a slow machine and still catches a factor.
	const slack = 1000
	for i := range got {
		for _, tc := range []struct {
			what     string
			got, was int
		}{
			{"user", got[i].Times.User, want[i].User},
			{"nice", got[i].Times.Nice, want[i].Nice},
			{"sys", got[i].Times.Sys, want[i].Sys},
			{"idle", got[i].Times.Idle, want[i].Idle},
			{"irq", got[i].Times.IRQ, want[i].IRQ},
		} {
			if tc.got < tc.was || tc.got > tc.was+slack {
				t.Errorf("cpu %d %s time is %d ms, the kernel had counted %d ms just before", i, tc.what, tc.got, tc.was)
			}
		}
	}
}

// procStatTimes reads the per-core counters straight out of /proc/stat and
// converts them the way the kernel documents rather than the way the code under
// test does, so the two arrive at the same milliseconds by different routes.
func procStatTimes(t *testing.T) []cpuTimes {
	t.Helper()
	b, err := os.ReadFile("/proc/stat")
	if err != nil {
		t.Skipf("read /proc/stat: %v", err)
	}
	var out []cpuTimes
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 8 || !strings.HasPrefix(f[0], "cpu") || f[0] == "cpu" {
			continue
		}
		ms := func(field string) int {
			n, err := strconv.Atoi(field)
			if err != nil {
				t.Fatalf("parse %q from /proc/stat: %v", field, err)
			}
			// The kernel counts in ticks of USER_HZ, which is 100, so a tick is 10 ms.
			return n * 10
		}
		out = append(out, cpuTimes{
			User: ms(f[1]),
			Nice: ms(f[2]),
			Sys:  ms(f[3]),
			Idle: ms(f[4]),
			IRQ:  ms(f[6]),
		})
	}
	return out
}

// TestAvailableParallelismIsTheProcessCount pins that the two counts are answered
// from different places. They are equal on an unrestricted machine, which is what
// makes the mistake easy to miss, so what is checked is the source rather than the
// number: the parallelism is the runtime's count of cores this process may use.
func TestAvailableParallelismIsTheProcessCount(t *testing.T) {
	if got, want := availableParallelism(), runtime.NumCPU(); got != want {
		t.Errorf("availableParallelism() = %d, the runtime sees %d usable cores", got, want)
	}
	if availableParallelism() > len(cpuList()) {
		t.Errorf("availableParallelism() = %d exceeds the %d cores the machine has", availableParallelism(), len(cpuList()))
	}
}

// TestCPUSnapshotCarriesTheProcessors pins the JSON the os module parses. The
// module reads these by name, so a field spelled differently on one side would
// leave os.cpus() reporting undefined models with nothing else failing.
func TestCPUSnapshotCarriesTheProcessors(t *testing.T) {
	var got struct {
		CPUs []struct {
			Model string `json:"model"`
			Speed int    `json:"speed"`
			Times struct {
				User int `json:"user"`
				Nice int `json:"nice"`
				Sys  int `json:"sys"`
				Idle int `json:"idle"`
				IRQ  int `json:"irq"`
			} `json:"times"`
		} `json:"cpus"`
		Parallelism int `json:"availableParallelism"`
	}
	if err := json.Unmarshal([]byte(OSInfoJSON()), &got); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if len(got.CPUs) == 0 {
		t.Fatal("the snapshot carries no processors")
	}
	if got.CPUs[0].Model == "" {
		t.Error("the first processor in the snapshot has no model")
	}
	if got.Parallelism <= 0 {
		t.Errorf("the snapshot reports %d usable cores", got.Parallelism)
	}
}

// intFromCommand runs a command that prints one number and returns it.
func intFromCommand(t *testing.T, name string, args ...string) int {
	t.Helper()
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		t.Fatalf("%s %v: %v", name, args, err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatalf("parse %q from %s: %v", string(out), name, err)
	}
	return n
}
