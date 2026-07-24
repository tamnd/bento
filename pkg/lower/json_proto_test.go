package lower

import "testing"

// JSON.parse installs an own "__proto__" data property, so a later obj.__proto__ read
// returns that own value, not the prototype, and a duplicate key keeps the last write.
// This runs JSON/parse/duplicate-proto end to end through the AOT path: the dynamic
// __proto__ member read lowers to ProtoRead, which prefers the own data property.
func TestJSONParseDuplicateProtoReadsOwn(t *testing.T) {
	skipIfShort(t)
	src := "const r = JSON.parse('{ \"__proto__\": 1, \"__proto__\": 2 }');\nconsole.log(r.__proto__);\n"
	if got := runProgramGoTolerant(t, src); got != "2\n" {
		t.Fatalf("parse duplicate __proto__ read = %q, want 2", got)
	}
}
