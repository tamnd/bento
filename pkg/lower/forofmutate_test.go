package lower

import (
	"strings"
	"testing"
)

// TestForOfArrayPopVisitsLiveLength pins that a for...of whose body pops the array
// follows the array iterator's live length: after each pop the shortened array ends the
// loop where the live length now stops, not where a frozen entry-length range would. A
// captured Elems slice would freeze the length and visit stale elements, so the array
// case emits an index loop over the live Len with an AtI read. This is the
// statements/for-of/array-contract shape.
func TestForOfArrayPopVisitsLiveLength(t *testing.T) {
	const src = `const a = [1, 2, 3, 4];
let sum = 0;
for (const x of a) { sum += x; a.pop(); }
console.log(String(sum));`
	out := renderProgram(t, src)
	if !strings.Contains(out, ".AtI(") || !strings.Contains(out, ".Len()") {
		t.Fatalf("popping for...of did not lower to a live-length index loop:\n%s", out)
	}
}

// TestForOfArrayPopRuns builds and runs the popping loop: the iterator visits index 0
// and 1 before the shrunk length stops it, so the sum is 1+2.
func TestForOfArrayPopRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const a = [1, 2, 3, 4];
let sum = 0;
for (const x of a) { sum += x; a.pop(); }
console.log(String(sum));`
	if got, want := runProgramGo(t, src), "3\n"; got != want {
		t.Fatalf("popping for...of printed %q, want %q", got, want)
	}
}

// TestForOfArrayPushRuns builds and runs a for...of whose body pushes: the extended
// array grows the live length so the loop visits the appended elements, matching Node.
func TestForOfArrayPushRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const a = [1];
let count = 0;
for (const x of a) { count++; if (count < 3) { a.push(count); } }
console.log(String(count));`
	if got, want := runProgramGo(t, src), "3\n"; got != want {
		t.Fatalf("pushing for...of printed %q, want %q", got, want)
	}
}

// TestForOfArrayNonMutatingKeepsRange pins the fast path stays: a for...of whose body
// never resizes the array keeps ranging Elems(), the readable form every non-mutating
// loop uses, rather than the index loop the mutation hazard needs.
func TestForOfArrayNonMutatingKeepsRange(t *testing.T) {
	const src = `const a = [1, 2, 3];
let sum = 0;
for (const x of a) { sum += x; }
console.log(String(sum));`
	out := renderProgram(t, src)
	if !strings.Contains(out, "range a.Elems()") {
		t.Fatalf("non-mutating for...of did not keep the fast range form:\n%s", out)
	}
	if strings.Contains(out, ".AtI(") {
		t.Fatalf("non-mutating for...of wrongly took the live-length index loop:\n%s", out)
	}
}

// TestForOfArrayIndexGrowTakesLiveLength pins that an element-access assignment in the
// body (arr[i] = v), which grows the array when i is past its end, takes the live-length
// index loop too: the scan cannot prove the write stays in bounds, so it fires the same
// index loop a pop or push does rather than freeze the length.
func TestForOfArrayIndexGrowTakesLiveLength(t *testing.T) {
	const src = `const a = [1];
for (const x of a) { a[a.length] = 1; }
console.log(String(a.length));`
	out := renderProgram(t, src)
	if !strings.Contains(out, ".Len()") || strings.Contains(out, "range a.Elems()") {
		t.Fatalf("index-assign for...of did not take the live-length index loop:\n%s", out)
	}
}
