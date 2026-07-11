package lower

import (
	"strings"
	"testing"
)

// Item 1: a computed member assignment used as a value.

func TestElementAssignValueLowers(t *testing.T) {
	const src = `let o: any = {}; let r = (o["k"] = 5); console.log(r);`
	out := renderProgramTolerant(t, src)
	if !strings.Contains(out, ".SetKey(") {
		t.Fatalf("want a SetKey store, got:\n%s", out)
	}
}

func TestElementAssignValueRuns(t *testing.T) {
	const src = `let o: any = {}; let r = (o["k"] = 5); console.log(r); console.log(o["k"]);`
	if got, want := runProgramGoTolerant(t, src), "5\n5\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestElementAssignValueDynamicRuns(t *testing.T) {
	// The whole expression is dynamic, so the store's box is the result directly.
	const src = `let o: any = {}; let r: any = (o["k"] = "v"); console.log(r);`
	if got, want := runProgramGoTolerant(t, src), "v\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
