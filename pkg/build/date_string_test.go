package build

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The Date string formats are a run-time surface, so what settles them is what a built
// binary prints, in a named zone, held against what Node prints for the same program in
// the same zone. Two zones are run rather than one: UTC alone would never show whether
// the local reading shifts the calendar day, and a zone five hours behind moves the date
// as well as the clock.

// buildAndRunFileInZone builds a program and runs it with TZ set, so a test that depends
// on the local zone reads the zone it names rather than the one the machine running the
// suite happens to be in. The binary is otherwise run exactly as buildAndRunFile runs it.
func buildAndRunFileInZone(t *testing.T, zone, name, src string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	bin := filepath.Join(dir, "prog")
	prog, err := Build(Options{Entry: path, Output: bin})
	if err != nil {
		t.Fatalf("build %s: %v", name, err)
	}
	cmd := exec.Command(prog)
	cmd.Env = append(os.Environ(), "TZ="+zone)
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run %s: %v (%s)", name, err, got)
	}
	return string(got)
}

// dateFormatProgram is the one program both zone tests run. It covers every format, the
// coercions that reach toString or valueOf without naming either, and the Invalid Date,
// which each format has its own answer for. The last two lines are the split that makes
// a date unlike every other object: an operator that reads it as a number gets the time
// value, while + gets the local reading, because a date is the one built-in whose
// default ToPrimitive hint is string.
const dateFormatProgram = "const d = new Date(1673751845006);\n" +
	"console.log(d.toString());\n" +
	"console.log(d.toDateString(), '|', d.toTimeString());\n" +
	"console.log(d.toUTCString());\n" +
	"console.log(String(d));\n" +
	"console.log(`${d}`);\n" +
	"console.log('' + d, d - 0);\n" +
	"const bad = new Date(NaN);\n" +
	"console.log(bad.toString(), '|', bad.toUTCString());\n" +
	"console.log(d.toJSON(), bad.toJSON());\n" +
	"const a = new Date(1000), b = new Date(2000);\n" +
	"console.log(b - a, a < b, b <= a, +a, -a, a * 2);\n" +
	"console.log(d + 1);\n"

// TestDateFormatsInUTCMatchNode is the zone with no offset and no daylight saving, where
// the local reading and the UTC one name the same wall clock. It is the case that says
// the formats themselves are right, with the zone arithmetic contributing nothing.
func TestDateFormatsInUTCMatchNode(t *testing.T) {
	got := buildAndRunFileInZone(t, "UTC", "main.js", dateFormatProgram)
	want := "Sun Jan 15 2023 03:04:05 GMT+0000 (Coordinated Universal Time)\n" +
		"Sun Jan 15 2023 | 03:04:05 GMT+0000 (Coordinated Universal Time)\n" +
		"Sun, 15 Jan 2023 03:04:05 GMT\n" +
		"Sun Jan 15 2023 03:04:05 GMT+0000 (Coordinated Universal Time)\n" +
		"Sun Jan 15 2023 03:04:05 GMT+0000 (Coordinated Universal Time)\n" +
		"Sun Jan 15 2023 03:04:05 GMT+0000 (Coordinated Universal Time) 1673751845006\n" +
		"Invalid Date | Invalid Date\n" +
		"2023-01-15T03:04:05.006Z null\n" +
		"1000 true false 1000 -1000 2000\n" +
		"Sun Jan 15 2023 03:04:05 GMT+0000 (Coordinated Universal Time)1\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestDateFormatsInAWesternZoneMatchNode is the same program five hours behind, where
// the local reading falls on the previous day. The UTC format does not move and the
// local ones do, which is the whole difference between the two families.
func TestDateFormatsInAWesternZoneMatchNode(t *testing.T) {
	got := buildAndRunFileInZone(t, "America/New_York", "main.js", dateFormatProgram)
	want := "Sat Jan 14 2023 22:04:05 GMT-0500 (Eastern Standard Time)\n" +
		"Sat Jan 14 2023 | 22:04:05 GMT-0500 (Eastern Standard Time)\n" +
		"Sun, 15 Jan 2023 03:04:05 GMT\n" +
		"Sat Jan 14 2023 22:04:05 GMT-0500 (Eastern Standard Time)\n" +
		"Sat Jan 14 2023 22:04:05 GMT-0500 (Eastern Standard Time)\n" +
		"Sat Jan 14 2023 22:04:05 GMT-0500 (Eastern Standard Time) 1673751845006\n" +
		"Invalid Date | Invalid Date\n" +
		"2023-01-15T03:04:05.006Z null\n" +
		"1000 true false 1000 -1000 2000\n" +
		"Sat Jan 14 2023 22:04:05 GMT-0500 (Eastern Standard Time)1\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestDateFormatsAcrossTheDaylightSwitchMatchNode runs the two instants an hour either
// side of a spring forward. The clock jumps two hours across a one-millisecond gap and
// the zone name changes with it, which is the reading a fixed offset would get wrong.
func TestDateFormatsAcrossTheDaylightSwitchMatchNode(t *testing.T) {
	got := buildAndRunFileInZone(t, "America/New_York", "main.js",
		"console.log(new Date(1678604399999).toString());\n"+
			"console.log(new Date(1678604400000).toString());\n"+
			"console.log(new Date(1678604400000).getTimezoneOffset());\n")
	want := "Sun Mar 12 2023 01:59:59 GMT-0500 (Eastern Standard Time)\n" +
		"Sun Mar 12 2023 03:00:00 GMT-0400 (Eastern Daylight Time)\n" +
		"240\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
