package lower

import (
	"strings"
	"testing"
)

// TestTypeofUndefinedAnnotationNoInitDeclares pins that a binding annotated with an
// unspelled type query and no initializer, var x: typeof undefined, lowers to an
// uninitialized value.Value declaration rather than folding its trailing annotation
// as if it were an initializer. The annotation and a delete or typeof initializer both
// surface as an unclassified node, so the readability folds tell them apart by the
// source separator: an absent '=' means the trailing node is an annotation with no
// value to fold. A miscompile here reads typeof undefined as the initializer and emits
// value.FromGoString("undefined").
func TestTypeofUndefinedAnnotationNoInitDeclares(t *testing.T) {
	const src = `var x: typeof undefined;
x = undefined;
`
	out := renderProgramTolerant(t, src)
	if strings.Contains(out, `value.FromGoString("undefined")`) {
		t.Fatalf("typeof undefined annotation was folded as a string initializer:\n%s", out)
	}
	if !strings.Contains(out, "var x value.Value") {
		t.Fatalf("uninitialized typeof undefined binding did not declare value.Value:\n%s", out)
	}
}

// TestDeleteInitializerStillFolds guards the other side of the same distinction: a
// binding whose initializer is a delete expression, const b = delete 0, still folds to
// b := true. The delete node is unclassified the same way the annotation is, so the fold
// must keep reading it as the initializer the '=' separator marks it to be.
func TestDeleteInitializerStillFolds(t *testing.T) {
	const src = `const b = delete 0;
console.log(b);
`
	out := renderProgramTolerant(t, src)
	if !strings.Contains(out, "b := true") {
		t.Fatalf("delete initializer did not fold to b := true:\n%s", out)
	}
}
