package lower

import (
	"strings"
	"testing"
)

// TestArrayBigIntJoinLowers pins that joining a bigint array reads each element to a
// string through value.BigIntToString, the shared elemToBStr bigint case.
func TestArrayBigIntJoinLowers(t *testing.T) {
	const src = "const a = [3n, 1n, 2n];\nconsole.log(a.join(\",\"));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.BigIntToString") {
		t.Fatalf("bigint array join did not stringify through value.BigIntToString:\n%s", source)
	}
}

// TestArrayBigIntStringifyRuns builds and runs bigint join and the comparator-less
// default sort and toSorted, which all coerce each element to a string. The default
// order is lexicographic ([10n, 9n, 100n, 1n] -> 1, 10, 100, 9), matching Node.
func TestArrayBigIntStringifyRuns(t *testing.T) {
	skipIfShort(t)
	const src = "const a = [3n, 1n, 2n];\n" +
		"console.log(a.join(\",\"));\n" +
		"console.log(a.sort().join(\",\"));\n" +
		"const b = [10n, 9n, 100n, 1n];\n" +
		"console.log(b.sort().join(\",\"));\n" +
		"console.log([2n, 30n, 1n].toSorted().join(\",\"));\n"
	got := runProgramGo(t, src)
	want := "3,1,2\n1,2,3\n1,10,100,9\n1,2,30\n"
	if got != want {
		t.Fatalf("bigint array stringify program printed %q, want %q", got, want)
	}
}
