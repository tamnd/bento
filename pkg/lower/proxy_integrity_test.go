package lower

import "testing"

// Object.freeze on a proxy whose preventExtensions trap returns false throws a
// TypeError: the refused [[PreventExtensions]] is the failure SetIntegrityLevel
// surfaces before it reaches the key sweep.
func TestFreezeProxyRefusedThrows(t *testing.T) {
	skipIfShort(t)
	src := "var p: any = new Proxy({}, { preventExtensions: function(){ return false; } });\ntry {\nObject.freeze(p);\nconsole.log('nothrow');\n} catch (e) {\nconsole.log(e instanceof TypeError ? 'TypeError' : 'other');\n}\n"
	if got := runProgramGoTolerant(t, src); got != "TypeError\n" {
		t.Fatalf("freeze refused proxy: got %q, want TypeError", got)
	}
}

// Object.seal on a proxy whose preventExtensions trap returns false throws, the
// seal sibling of the refused-freeze case.
func TestSealProxyRefusedThrows(t *testing.T) {
	skipIfShort(t)
	src := "var p: any = new Proxy({}, { preventExtensions: function(){ return false; } });\ntry {\nObject.seal(p);\nconsole.log('nothrow');\n} catch (e) {\nconsole.log(e instanceof TypeError ? 'TypeError' : 'other');\n}\n"
	if got := runProgramGoTolerant(t, src); got != "TypeError\n" {
		t.Fatalf("seal refused proxy: got %q, want TypeError", got)
	}
}

// Object.freeze on a plain-backed proxy succeeds and Object.isFrozen reports the
// proxy frozen, the read routed through the proxy's own-key and descriptor traps.
func TestFreezeProxyMarksFrozen(t *testing.T) {
	skipIfShort(t)
	src := "var t: any = { a: 1 };\nvar p: any = new Proxy(t, {});\nObject.freeze(p);\nconsole.log(String(Object.isFrozen(p)));\nconsole.log(String(Object.isExtensible(p)));\n"
	if got := runProgramGoTolerant(t, src); got != "true\nfalse\n" {
		t.Fatalf("freeze proxy state: got %q, want true/false", got)
	}
}

// Object.seal on a plain-backed proxy succeeds and Object.isSealed reports the
// proxy sealed, while isFrozen stays false since the data property is still
// writable, the two levels the TestIntegrityLevel read tells apart.
func TestSealProxyMarksSealedNotFrozen(t *testing.T) {
	skipIfShort(t)
	src := "var t: any = { a: 1 };\nvar p: any = new Proxy(t, {});\nObject.seal(p);\nconsole.log(String(Object.isSealed(p)));\nconsole.log(String(Object.isFrozen(p)));\n"
	if got := runProgramGoTolerant(t, src); got != "true\nfalse\n" {
		t.Fatalf("seal proxy state: got %q, want true/false", got)
	}
}
