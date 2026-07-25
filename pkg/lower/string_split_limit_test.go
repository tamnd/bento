package lower

import (
	"strings"
	"testing"
)

// TestStringSplitLimitLowers pins that split with a string separator and a
// number limit lowers to a two-argument value.BStr.Split call, the Go-variadic
// method that caps the result, rather than handing back on the argument count.
func TestStringSplitLimitLowers(t *testing.T) {
	const src = "console.log(\"a,b,c,d\".split(\",\", 2).join(\"|\"));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".Split(") || !strings.Contains(source, ", 2)") {
		t.Fatalf("split with a limit did not lower to a two-argument Split:\n%s", source)
	}
}

// TestStringSplitLimitRuns builds and runs split with a limit across the edges
// JavaScript's limit specifies, each matching Node.
func TestStringSplitLimitRuns(t *testing.T) {
	skipIfShort(t)
	const src = "console.log(\"a,b,c,d\".split(\",\", 2).join(\"|\"));\n" +
		"console.log(\"a,b,c,d\".split(\",\", 0).length);\n" +
		"console.log(\"a,b,c,d\".split(\",\", 10).join(\"|\"));\n" +
		"console.log(\"abcd\".split(\"\", 2).join(\"|\"));\n" +
		"console.log(\"a-b-c\".split(\"-\", 2.9).join(\"|\"));\n"
	got := runProgramGo(t, src)
	want := "a|b\n0\na|b|c|d\na|b\na|b\n"
	if got != want {
		t.Fatalf("split-limit program printed %q, want %q", got, want)
	}
}
