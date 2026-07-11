package build

import (
	"strings"
	"testing"
)

// TestDeleteNonReferenceLowers pins that a delete over a non-reference operand no
// longer gates the build. The checker reports 2703 ("The operand of a 'delete'
// operator must be a property reference"), but delete still yields true, so the
// front door tolerates the report and the renderer folds the expression.
func TestDeleteNonReferenceLowers(t *testing.T) {
	src := "const b: boolean = delete 0;\nconsole.log(b);\n"
	out, err := compileSource(t, src)
	if err != nil {
		t.Fatalf("delete of a non-reference operand should lower, got: %v", err)
	}
	if !strings.Contains(out, "b := true") {
		t.Fatalf("expected the non-reference delete to fold to true, got:\n%s", out)
	}
}

// TestDeleteIdentifierStillGates pins the deliberate exclusion: deleting a bare
// variable in strict mode is an early SyntaxError the checker reports as 1102
// ("'delete' cannot be called on an identifier in strict mode"), which is not
// tolerated, so the build still gates on it.
func TestDeleteIdentifierStillGates(t *testing.T) {
	src := "let x = 1;\nconst b: boolean = delete x;\nconsole.log(b);\n"
	_, err := compileSource(t, src)
	if err == nil {
		t.Fatal("delete of an identifier in strict mode should still gate the build")
	}
	if !strings.Contains(err.Error(), "identifier in strict mode") {
		t.Fatalf("expected the strict-mode identifier-delete error, got: %v", err)
	}
}
