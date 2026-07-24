package lower

import (
	"strings"
	"testing"
)

// A compound member store under a "use strict" program routes through SetStrict, so a
// failed write (a non-writable property here) throws rather than dropping. A sloppy
// program keeps the silent-drop Set. The receiver is the empty-object bag, the dynamic
// path these member stores take.
func TestStrictCompoundMemberStoreUsesSetStrict(t *testing.T) {
	const strict = "\"use strict\";\nvar obj = {};\nObject.defineProperty(obj, \"p\", { value: 10, writable: false });\nobj.p *= 20;\n"
	if out := renderTolerant(t, strict); !strings.Contains(out, "SetStrict") {
		t.Fatalf("strict compound store did not route through SetStrict:\n%s", out)
	}
	const sloppy = "var obj = {};\nObject.defineProperty(obj, \"p\", { value: 10, writable: false });\nobj.p *= 20;\n"
	if out := renderTolerant(t, sloppy); strings.Contains(out, "SetStrict") {
		t.Fatalf("sloppy compound store wrongly routed through SetStrict:\n%s", out)
	}
}

// A logical member store under a "use strict" program routes the guarded store through
// SetStrict, so a write that is attempted and fails throws. The guard short-circuits
// before the store when the operator does not assign, so only an attempted write throws.
func TestStrictLogicalMemberStoreUsesSetStrict(t *testing.T) {
	const strict = "\"use strict\";\nvar obj = {};\nObject.defineProperty(obj, \"p\", { value: 10, writable: false });\nobj.p &&= 20;\n"
	if out := renderTolerant(t, strict); !strings.Contains(out, "SetStrict") {
		t.Fatalf("strict logical store did not route through SetStrict:\n%s", out)
	}
	const sloppy = "var obj = {};\nObject.defineProperty(obj, \"p\", { value: 10, writable: false });\nobj.p &&= 20;\n"
	if out := renderTolerant(t, sloppy); strings.Contains(out, "SetStrict") {
		t.Fatalf("sloppy logical store wrongly routed through SetStrict:\n%s", out)
	}
}
