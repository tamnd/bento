package lower

import (
	"strings"
	"testing"
)

// Item 1: an array destructuring declaration element with a default.

func TestArrayDefaultLowers(t *testing.T) {
	const src = `let arr: number[] = [10]; let [a = 1, b = 2] = arr; console.log(a, b);`
	out := renderProgram(t, src)
	if !strings.Contains(out, ".AtOpt(") || !strings.Contains(out, ".IsUndefined()") {
		t.Fatalf("want a bounds-aware AtOpt read and an undefined test, got:\n%s", out)
	}
}

func TestArrayDefaultRuns(t *testing.T) {
	cases := map[string]string{
		// The present slot keeps its value; the missing slot takes the default.
		`let arr: number[] = [10]; let [a = 1, b = 2] = arr; console.log(a, b);`: "10 2\n",
		// A present zero is not defaulted; only a missing slot is.
		`let arr: number[] = [0, 5]; let [a = 9, b = 9] = arr; console.log(a, b);`: "0 5\n",
		// All slots missing take every default.
		`let arr: number[] = []; let [a = 1, b = 2] = arr; console.log(a, b);`: "1 2\n",
		// A default mixes with a plain trailing element.
		`let arr: number[] = [7]; let [a = 3, b] = arr; console.log(a, b);`: "7 0\n",
	}
	for src, want := range cases {
		if got := runProgramGo(t, src); got != want {
			t.Errorf("%s:\n got %q\nwant %q", src, got, want)
		}
	}
}

// Item 2: an object destructuring declaration property with a default.

func TestObjectDefaultLowers(t *testing.T) {
	const src = `let o: {x?: number, y: number} = {y: 3}; let {x = 5, y} = o; console.log(x, y);`
	out := renderProgram(t, src)
	if !strings.Contains(out, ".IsUndefined()") {
		t.Fatalf("want an undefined test on the optional field, got:\n%s", out)
	}
}

func TestObjectDefaultRuns(t *testing.T) {
	cases := map[string]string{
		// The missing optional property takes the default; the present one keeps it.
		`let o: {x?: number, y: number} = {y: 3}; let {x = 5, y} = o; console.log(x, y);`:       "5 3\n",
		`let o: {x?: number, y: number} = {x: 8, y: 3}; let {x = 5, y} = o; console.log(x, y);`: "8 3\n",
		// A present optional zero is not defaulted.
		`let o: {x?: number} = {x: 0}; let {x = 5} = o; console.log(x);`: "0\n",
		// A default on a required field never fires.
		`let o: {x: number} = {x: 1}; let {x = 5} = o; console.log(x);`: "1\n",
	}
	for src, want := range cases {
		if got := runProgramGo(t, src); got != want {
			t.Errorf("%s:\n got %q\nwant %q", src, got, want)
		}
	}
}
