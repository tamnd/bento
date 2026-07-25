package lower

import "testing"

// A binding initialized by new Proxy holds the boxed proxy value, so a named member
// read p.attr off it dispatches through the runtime get trap rather than an interned
// Go struct selector the box does not carry. A handler with an undefined get trap
// forwards the read to the target, so p.attr answers the target's own value.
func TestProxyNamedReadUndefinedGetTrapForwards(t *testing.T) {
	skipIfShort(t)
	src := "var target = { attr: 1 };\nvar p = new Proxy(target, { get: undefined });\nconsole.log(String(p.attr));\n"
	if got := runProgramGoTolerant(t, src); got != "1\n" {
		t.Fatalf("proxy named read with undefined get trap: got %q, want %q", got, "1\n")
	}
}

// A get trap overrides the target for a named read, so p.attr answers the trap
// result rather than the target property. The trap is typed to a number so the
// handler literal lowers without a union-typing handback.
func TestProxyNamedReadGetTrapOverrides(t *testing.T) {
	skipIfShort(t)
	src := "var target = { attr: 1 };\nvar p = new Proxy(target, { get: function(t: any, k: any): number { return 99; } });\nconsole.log(String(p.attr));\n"
	if got := runProgramGoTolerant(t, src); got != "99\n" {
		t.Fatalf("proxy named read with get trap: got %q, want %q", got, "99\n")
	}
}

// A named write through a proxy with an undefined set trap forwards to the target, so
// a later read of the same key off the target sees the written value.
func TestProxyNamedWriteUndefinedSetTrapForwards(t *testing.T) {
	skipIfShort(t)
	src := "var target: any = { attr: 1 };\nvar p = new Proxy(target, { set: undefined });\np.attr = 5;\nconsole.log(String(target.attr));\n"
	if got := runProgramGoTolerant(t, src); got != "5\n" {
		t.Fatalf("proxy named write with undefined set trap: got %q, want %q", got, "5\n")
	}
}
