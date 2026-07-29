package lower

import (
	"strings"
	"testing"
)

// A property write onto a fixed-shape object that has been widened to a dynamic value
// lowers, through the coerce path, to value.ObjectFromStruct(o).Set(...). That box is a
// fresh copy of the struct made at the write site, so the Set mutates the throwaway and
// the write is lost: a later read reads undefined where the language reads the stored
// value. That is a wrong result, worse than a handback, so the write now hands back.

// TestCopyAliasWriteHandsBack pins the handback: writing a new property onto a
// fixed-shape object widened to any no longer emits a store that mutates a throwaway
// copy.
func TestCopyAliasWriteHandsBack(t *testing.T) {
	const src = `const o = { a: 1 }; (o as any).b = 2; console.log((o as any).b);`
	reason := renderProgramTolerantHandBack(t, src)
	if !strings.Contains(reason, "throwaway copy") {
		t.Fatalf("copy-aliasing write handback reason = %q, want the throwaway-copy guard", reason)
	}
}

// TestCopyAliasCompoundWriteHandsBack pins that a compound store onto the same fresh
// copy also hands back rather than emit a lost write. The parenthesized cast receiver
// is caught by the compound path's own side-effecting-receiver guard before the
// throwaway-copy guard; either way the unit hands back rather than run wrong.
func TestCopyAliasCompoundWriteHandsBack(t *testing.T) {
	const src = `const o = { a: 1 }; (o as any).a += 2; console.log((o as any).a);`
	// renderProgramTolerantHandBack fails the test unless the program hands back, so the
	// call returning is the assertion.
	_ = renderProgramTolerantHandBack(t, src)
}

// TestDynamicBagWriteStillLowers proves the guard does not touch a genuine dynamic bag:
// an object that lives as a value.Value from creation lowers its property write to a
// Set on the stored bag, which mutates the real object and reads back the value.
func TestDynamicBagWriteStillLowers(t *testing.T) {
	skipIfShort(t)
	const src = `const o: any = {}; o.b = 2; console.log(o.b);`
	got := runProgramGoTolerant(t, src)
	if want := "2\n"; got != want {
		t.Fatalf("dynamic bag write run = %q, want %q", got, want)
	}
}
