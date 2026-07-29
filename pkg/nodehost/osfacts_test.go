package nodehost

import (
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// measured reports whether the platform has a file that measures the host facts.
// The others answer zeros on purpose, so the assertions below would be asking them
// for something they do not claim.
func measured() bool {
	switch runtime.GOOS {
	case "linux", "darwin", "windows":
		return true
	}
	return false
}

// TestHostFactsAreMeasured pins that the facts are real rather than the zeros the
// snapshot used to carry. Each one is checked against what it means rather than
// against a fixed value, since none of them is the same on two machines: the
// memory counts have to be positive and free cannot exceed total, and a machine
// that is running has been up for some time.
func TestHostFactsAreMeasured(t *testing.T) {
	if !measured() {
		t.Skipf("no host facts are measured on %s yet", runtime.GOOS)
	}
	f := readHostFacts()
	if f.Release == "" {
		t.Error("release is empty")
	}
	if f.Machine == "" {
		t.Error("machine is empty")
	}
	if f.TotalMem == 0 {
		t.Error("total memory is zero")
	}
	if f.FreeMem == 0 {
		t.Error("free memory is zero")
	}
	if f.FreeMem > f.TotalMem {
		t.Errorf("free memory %d exceeds total %d", f.FreeMem, f.TotalMem)
	}
	if f.Uptime <= 0 {
		t.Errorf("uptime is %v, want a positive number of seconds", f.Uptime)
	}
	for i, l := range f.Loadavg {
		if l < 0 || math.IsNaN(l) {
			t.Errorf("load average %d is %v", i, l)
		}
	}
	// Windows keeps no load average and Node reports three zeros there, so a nonzero
	// one would mean something was invented.
	if runtime.GOOS == "windows" {
		if f.Loadavg != [3]float64{} {
			t.Errorf("load average on windows is %v, want three zeros", f.Loadavg)
		}
	}
}

// TestUnameStringsMatchUname checks the three strings uname owns against uname
// itself, which is the same interface Node reads them through. It is the one
// oracle available on any Unix without installing anything, and it catches the
// mistake this code is most likely to make: reading a utsname field with the wrong
// length or off the wrong offset, which yields a plausible-looking prefix rather
// than an obvious error.
func TestUnameStringsMatchUname(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows has no uname; its three strings come from three other interfaces")
	}
	if !measured() {
		t.Skipf("no host facts are measured on %s yet", runtime.GOOS)
	}
	if _, err := exec.LookPath("uname"); err != nil {
		t.Skip("uname not found on PATH")
	}
	f := readHostFacts()
	for _, tc := range []struct {
		flag string
		got  string
		what string
	}{
		{"-r", f.Release, "release"},
		{"-v", f.Version, "version"},
		{"-m", f.Machine, "machine"},
	} {
		out, err := exec.Command("uname", tc.flag).Output()
		if err != nil {
			t.Fatalf("uname %s: %v", tc.flag, err)
		}
		if want := strings.TrimSpace(string(out)); tc.got != want {
			t.Errorf("%s = %q, uname %s says %q", tc.what, tc.got, tc.flag, want)
		}
	}
}

// TestLoadavgMatchesTheKernel checks the three averages against the file or the
// sysctl the kernel publishes them in. The Darwin path is the one worth checking:
// the sysctl hands back a struct of fixed-point numbers and a scale to divide them
// by, and a wrong offset for the scale gives numbers that are wrong by a factor
// rather than obviously broken.
//
// The averages move, so the comparison is loose. It is still tight enough to catch
// a scale read from the wrong place, which is off by orders of magnitude.
func TestLoadavgMatchesTheKernel(t *testing.T) {
	var want [3]float64
	switch runtime.GOOS {
	case "linux":
		b, err := os.ReadFile("/proc/loadavg")
		if err != nil {
			t.Skipf("read /proc/loadavg: %v", err)
		}
		fields := strings.Fields(string(b))
		if len(fields) < 3 {
			t.Fatalf("/proc/loadavg has %d fields: %q", len(fields), string(b))
		}
		for i := range want {
			v, err := strconv.ParseFloat(fields[i], 64)
			if err != nil {
				t.Fatalf("parse %q: %v", fields[i], err)
			}
			want[i] = v
		}
	case "darwin":
		if _, err := exec.LookPath("sysctl"); err != nil {
			t.Skip("sysctl not found on PATH")
		}
		out, err := exec.Command("sysctl", "-n", "vm.loadavg").Output()
		if err != nil {
			t.Fatalf("sysctl vm.loadavg: %v", err)
		}
		// The sysctl prints the three numbers wrapped in braces.
		fields := strings.Fields(strings.Trim(strings.TrimSpace(string(out)), "{} "))
		if len(fields) < 3 {
			t.Fatalf("vm.loadavg has %d fields: %q", len(fields), string(out))
		}
		for i := range want {
			v, err := strconv.ParseFloat(fields[i], 64)
			if err != nil {
				t.Fatalf("parse %q: %v", fields[i], err)
			}
			want[i] = v
		}
	default:
		t.Skipf("no load average oracle on %s", runtime.GOOS)
	}
	got := readHostFacts().Loadavg
	for i := range want {
		if diff := math.Abs(got[i] - want[i]); diff > 0.5+want[i]*0.5 {
			t.Errorf("load average %d is %v, the kernel says %v", i, got[i], want[i])
		}
	}
}

// TestOSInfoJSONCarriesTheFacts pins that the snapshot the os module parses is the
// one the facts were measured into. The module reads the fields by name, so a
// field renamed on one side and not the other would leave os.machine undefined
// with nothing else failing.
func TestOSInfoJSONCarriesTheFacts(t *testing.T) {
	var got map[string]any
	if err := json.Unmarshal([]byte(OSInfoJSON()), &got); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	for _, key := range []string{
		"platform", "arch", "type", "release", "version", "machine", "hostname",
		"homedir", "tmpdir", "endianness", "totalmem", "freemem", "uptime",
		"loadavg", "cpus", "networkInterfaces", "userInfo",
	} {
		if _, ok := got[key]; !ok {
			t.Errorf("snapshot has no %q", key)
		}
	}
	if !measured() {
		return
	}
	if n, _ := got["totalmem"].(float64); n <= 0 {
		t.Errorf("snapshot totalmem is %v", got["totalmem"])
	}
	if s, _ := got["machine"].(string); s == "" {
		t.Error("snapshot machine is empty")
	}
}
