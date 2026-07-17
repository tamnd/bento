package lower

import (
	"strings"
	"testing"
)

// TestIndexSignatureReadHandsBack pins that a read of an undeclared key on an object
// whose shape carries a string index signature hands back. The shape interns to a fixed
// struct that drops the signature, so the key is not provably absent and there is no Go
// field to select. Folding the read to value.MissingProperty would land a value.Value
// where the signature's concrete type, a number here, is wanted and fail go build, so
// the read hands back rather than miscompile.
func TestIndexSignatureReadHandsBack(t *testing.T) {
	const src = `type Dict = { [k: string]: number };
const o: Dict = { a: 1 };
console.log(String(o["b"]));
`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "reading a key typed by an object's index signature is a later slice") {
		t.Fatalf("hand-back reason = %q, want the index-signature reason", reason)
	}
}
