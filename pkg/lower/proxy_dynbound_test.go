package lower

import (
	"strings"
	"testing"
)

// A binding initialized by new Proxy holds the boxed proxy value, so a named member
// read p.attr off it dispatches through the runtime get trap rather than an interned
// Go struct selector the box does not carry. An undefined get trap forwards the read
// to the target, so p.attr answers the target's own value even when the target is a
// plain fixed-shape local, which the pre-scan boxes to a shared object.
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
// a later read of the same key off the original target binding sees the written
// value. The pre-scan boxes the fixed-shape target to a shared object, so the write
// through the proxy and the read off target reach the same value.
func TestProxyNamedWriteUndefinedSetTrapForwardsToTarget(t *testing.T) {
	skipIfShort(t)
	src := "var target = { attr: 1 };\nvar p = new Proxy(target, { set: undefined });\np.attr = 2;\nconsole.log(String(target.attr));\n"
	if got := runProgramGoTolerant(t, src); got != "2\n" {
		t.Fatalf("proxy named write forwards to target: got %q, want %q", got, "2\n")
	}
}

// Object.preventExtensions on the target is observed through the proxy, since the
// two share identity after the pre-scan boxes the target: a write of a new key
// through the proxy is refused once the shared target is non-extensible, so the
// original property stays and a read of it off the target still answers the old
// value rather than the dropped write.
func TestProxyWriteRefusedOnSharedNonExtensibleTarget(t *testing.T) {
	skipIfShort(t)
	src := "var target = { attr: 1 };\nvar p = new Proxy(target, { set: undefined });\nObject.preventExtensions(target);\np.other = 9;\nconsole.log(String(target.other));\n"
	if got := runProgramGoTolerant(t, src); got != "undefined\n" {
		t.Fatalf("new key through proxy over non-extensible shared target: got %q, want undefined", got)
	}
}

// A proxy over a fixed-shape target boxes that target to a value with no runtime
// property bag, so forwarding a trap to it would answer every property absent. The
// construction hands back rather than lowering to that wrong answer, the same
// boundary a bare Reflect.has on such a value keeps.
func TestProxyOverFixedShapeTargetHandsBack(t *testing.T) {
	for _, src := range []string{
		"var re = /(?:)/m;\nvar p = new Proxy(re, {});\nconsole.log(String(p));\n",
		"var f = function() {};\nvar p = new Proxy(f, {});\nconsole.log(String(p));\n",
	} {
		reason := renderProgramHandBack(t, src)
		if !strings.Contains(reason, "fixed-shape target") {
			t.Fatalf("reason = %q, want it to name the fixed-shape target", reason)
		}
	}
}

// A proxy over a plain-object target keeps lowering: the object is dynamic and
// carries a property bag the trap forwards to, so the guard leaves it untouched.
func TestProxyOverPlainObjectStillLowers(t *testing.T) {
	src := "var target = { attr: 1 };\nvar p = new Proxy(target, {});\nconsole.log(String(p.attr));\n"
	out := renderProgram(t, src)
	if !strings.Contains(out, "NewProxy") {
		t.Fatalf("want a NewProxy call in:\n%s", out)
	}
}
