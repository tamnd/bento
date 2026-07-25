package lower

import (
	"strings"
	"testing"
)

// A frozen symbol-keyed property drops a sloppy write and keeps its value, the
// silent failure Object.freeze produces, matching the string-keyed path.
func TestFrozenSymbolWriteSloppyDrops(t *testing.T) {
	skipIfShort(t)
	src := "var sym = Symbol('66');\nvar obj: any = {};\nobj[sym] = 1;\nObject.freeze(obj);\nobj[sym] = 2;\nconsole.log(String(obj[sym]));\n"
	if got := runProgramGoTolerant(t, src); got != "1\n" {
		t.Fatalf("frozen symbol sloppy write: got %q, want %q", got, "1\n")
	}
}

// A frozen symbol-keyed property write throws a TypeError under a strict program,
// the escalation a strict assignment raises where the sloppy write drops.
func TestFrozenSymbolWriteStrictThrows(t *testing.T) {
	skipIfShort(t)
	src := "'use strict';\nvar sym = Symbol('66');\nvar obj: any = {};\nobj[sym] = 1;\nObject.freeze(obj);\ntry {\nobj[sym] = 2;\nconsole.log('nothrow');\n} catch (e) {\nconsole.log(e instanceof TypeError ? 'TypeError' : 'other');\n}\n"
	if got := runProgramGoTolerant(t, src); got != "TypeError\n" {
		t.Fatalf("frozen symbol strict write: got %q, want TypeError", got)
	}
}

// A frozen string-keyed element write throws under a strict program, the string
// sibling of the symbol case, routed through the strict element store.
func TestFrozenStringElementWriteStrictThrows(t *testing.T) {
	skipIfShort(t)
	src := "'use strict';\nvar obj: any = {};\nobj['k'] = 1;\nObject.freeze(obj);\ntry {\nobj['k'] = 2;\nconsole.log('nothrow');\n} catch (e) {\nconsole.log(e instanceof TypeError ? 'TypeError' : 'other');\n}\n"
	if got := runProgramGoTolerant(t, src); got != "TypeError\n" {
		t.Fatalf("frozen string element strict write: got %q, want TypeError", got)
	}
}

// A new key on a non-extensible object throws under a strict program, the
// preventExtensions failure a strict assignment escalates.
func TestNewKeyNonExtensibleStrictThrows(t *testing.T) {
	skipIfShort(t)
	src := "'use strict';\nvar obj: any = {};\nObject.preventExtensions(obj);\ntry {\nobj['x'] = 1;\nconsole.log('nothrow');\n} catch (e) {\nconsole.log(e instanceof TypeError ? 'TypeError' : 'other');\n}\n"
	if got := runProgramGoTolerant(t, src); got != "TypeError\n" {
		t.Fatalf("new key on non-extensible strict: got %q, want TypeError", got)
	}
}

// preventExtensions leaves an existing symbol property deletable, so delete on it
// still reports true; only the addition of a new key is blocked.
func TestPreventExtensionsSymbolStaysDeletable(t *testing.T) {
	skipIfShort(t)
	src := "var symA = Symbol('a');\nvar obj: any = {};\nobj[symA] = 1;\nObject.preventExtensions(obj);\nconsole.log(String(delete obj[symA]));\n"
	if got := runProgramGoTolerant(t, src); got != "true\n" {
		t.Fatalf("delete symbol on non-extensible: got %q, want true", got)
	}
}

// A sealed object keeps its symbol property writable, so a sloppy overwrite lands
// while a new symbol key is dropped, matching Object.seal's data-writable state.
func TestSealedSymbolWriteStaysWritableNewKeyDropped(t *testing.T) {
	skipIfShort(t)
	src := "var symA = Symbol('A');\nvar symB = Symbol('B');\nvar obj: any = {};\nobj[symA] = 1;\nObject.seal(obj);\nobj[symA] = 2;\nobj[symB] = 1;\nconsole.log(String(obj[symA]));\nconsole.log(String(obj[symB]));\n"
	if got := runProgramGoTolerant(t, src); got != "2\nundefined\n" {
		t.Fatalf("sealed symbol write/new-key: got %q, want 2/undefined", got)
	}
}

// Under a sloppy program the strict element stores are not emitted, so a frozen
// write drops without throwing and the store methods stay the non-throwing forms.
func TestSloppyElementStoreStaysNonThrowing(t *testing.T) {
	skipIfShort(t)
	src := "var obj: any = {};\nobj['k'] = 1;\nObject.freeze(obj);\nobj['k'] = 2;\nconsole.log(String(obj['k']));\n"
	if got := runProgramGoTolerant(t, src); got != "1\n" {
		t.Fatalf("frozen string sloppy write: got %q, want 1", got)
	}
	if go1 := renderProgramTolerant(t, src); strings.Contains(go1, "SetKeyStrict") || strings.Contains(go1, "SetElemStrict") {
		t.Fatalf("sloppy program emitted a strict element store:\n%s", go1)
	}
}
