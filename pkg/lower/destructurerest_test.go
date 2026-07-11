package lower

import (
	"strings"
	"testing"
)

// An array destructuring rest element, `[a, ...rest] = src`, binds the fixed slots by
// index and gathers the elements past them into a fresh array. It lowers the rest to
// Slice from the first unfixed index, the tail copy the array model's Slice makes, so
// the rest holds exactly what the named elements did not take.

// TestArrayRestDeclLowers proves an array rest declaration binds the head by index and
// the rest through Slice from the first unfixed slot.
func TestArrayRestDeclLowers(t *testing.T) {
	const src = "const arr = [1, 2, 3, 4];\nconst [a, ...rest] = arr;\nconsole.log(a);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "a := arr.AtI(0)") {
		t.Errorf("head element did not lower to an indexed read:\n%s", source)
	}
	if !strings.Contains(source, "rest := arr.Slice(1)") {
		t.Errorf("rest did not lower to Slice from the first unfixed slot:\n%s", source)
	}
}

// TestArrayRestDeclRuns builds and runs an array rest declaration so the head reads and
// the gathered tail are proven.
func TestArrayRestDeclRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
const arr = [10, 20, 30, 40];
const [a, b, ...rest] = arr;
console.log(a);
console.log(b);
console.log(rest.length);
console.log(rest[0]);
console.log(rest[1]);
`
	if got, want := runProgramGo(t, src), "10\n20\n2\n30\n40\n"; got != want {
		t.Fatalf("array rest declaration printed %q, want %q", got, want)
	}
}

// TestArrayRestDeclWholeCopy proves a lone rest, `[...rest]`, copies the whole source.
func TestArrayRestDeclWholeCopy(t *testing.T) {
	skipIfShort(t)
	const src = `
const arr = [1, 2, 3];
const [...rest] = arr;
console.log(rest.length);
console.log(rest[2]);
`
	if got, want := runProgramGo(t, src), "3\n3\n"; got != want {
		t.Fatalf("whole-copy rest printed %q, want %q", got, want)
	}
}

// TestArrayRestAfterDefaultRuns proves a rest starts after the last fixed slot even
// when a fixed slot carries a default: the default fills the missing slot and the rest
// gathers only what follows the fixed count, not the filled slot.
func TestArrayRestAfterDefaultRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
const arr = [1, 2, 3, 4];
const [a, b = 99, ...rest] = arr;
console.log(a);
console.log(b);
console.log(rest.length);
console.log(rest[0]);
`
	if got, want := runProgramGo(t, src), "1\n2\n2\n3\n"; got != want {
		t.Fatalf("rest after default printed %q, want %q", got, want)
	}
}

// TestArrayRestDefaultFiresThenEmptyRest proves the default fills a missing fixed slot
// and the rest is empty when the source runs out at the fixed count.
func TestArrayRestDefaultFiresThenEmptyRest(t *testing.T) {
	skipIfShort(t)
	const src = `
const arr = [1];
const [a, b = 99, ...rest] = arr;
console.log(a);
console.log(b);
console.log(rest.length);
`
	if got, want := runProgramGo(t, src), "1\n99\n0\n"; got != want {
		t.Fatalf("default-fires rest printed %q, want %q", got, want)
	}
}

// TestArrayRestAfterDefaultAssignRuns proves the same fixed-count start in an
// assignment where a fixed target carries a default.
func TestArrayRestAfterDefaultAssignRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
const arr = [5, 6, 7, 8];
let a = 0;
let b = 0;
let rest: number[] = [];
[a, b = 99, ...rest] = arr;
console.log(a);
console.log(b);
console.log(rest.length);
console.log(rest[0]);
`
	if got, want := runProgramGo(t, src), "5\n6\n2\n7\n"; got != want {
		t.Fatalf("rest after default assignment printed %q, want %q", got, want)
	}
}

// TestArrayRestAssignRuns builds and runs an array rest assignment so the tail is
// gathered into the already-declared rest target.
func TestArrayRestAssignRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
const arr = [5, 6, 7, 8];
let a = 0;
let rest: number[] = [];
[a, ...rest] = arr;
console.log(a);
console.log(rest.length);
console.log(rest[0]);
console.log(rest[2]);
`
	if got, want := runProgramGo(t, src), "5\n3\n6\n8\n"; got != want {
		t.Fatalf("array rest assignment printed %q, want %q", got, want)
	}
}
