package lower

import (
	"strings"
	"testing"
)

// A member delete under a "use strict" program routes through DeleteStrict, so a
// refused removal (a non-configurable property) throws rather than reporting false.
// A sloppy program keeps the false-returning Delete. The receiver is the
// empty-object bag, the dynamic path a member delete takes.
func TestStrictMemberDeleteUsesDeleteStrict(t *testing.T) {
	const strict = "\"use strict\";\nvar obj = {};\nObject.defineProperty(obj, \"p\", { value: 10, configurable: false });\ndelete obj.p;\n"
	out := renderTolerant(t, strict)
	if !strings.Contains(out, "DeleteStrict") {
		t.Fatalf("strict member delete did not route through DeleteStrict:\n%s", out)
	}
	const sloppy = "var obj = {};\nObject.defineProperty(obj, \"p\", { value: 10, configurable: false });\ndelete obj.p;\n"
	out = renderTolerant(t, sloppy)
	if strings.Contains(out, "DeleteStrict") {
		t.Fatalf("sloppy member delete wrongly routed through DeleteStrict:\n%s", out)
	}
	if !strings.Contains(out, ".Delete(") {
		t.Fatalf("sloppy member delete did not route through Delete:\n%s", out)
	}
}
