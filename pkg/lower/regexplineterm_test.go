package lower

import (
	"strings"
	"testing"
)

// TestRegexpRawLineSeparatorHandsBack pins that a regexp literal whose body carries a
// raw line separator (U+2028) hands back rather than lower. The grammar forbids an
// unescaped LineTerminator in a regexp body, so the source is a SyntaxError at parse,
// but bento's front end tokenizes the separator into the body instead of rejecting it.
// Lowering it would run a program the language never parses, so the literal declines.
// This is the language/literals/regexp/regexp-*-no-line-separator and
// language/line-terminators/invalid-regexp-ls shape.
func TestRegexpRawLineSeparatorHandsBack(t *testing.T) {
	const src = "const r = / /;\nconsole.log(typeof r);"
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "line terminator") {
		t.Fatalf("regexp with a raw line separator did not hand back for that reason: %q", reason)
	}
}

// TestRegexpRawParagraphSeparatorHandsBack is the paragraph-separator (U+2029) twin,
// the language/line-terminators/invalid-regexp-ps and regexp-*-no-paragraph-separator
// shape.
func TestRegexpRawParagraphSeparatorHandsBack(t *testing.T) {
	const src = "const r = / /;\nconsole.log(typeof r);"
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "line terminator") {
		t.Fatalf("regexp with a raw paragraph separator did not hand back for that reason: %q", reason)
	}
}

// TestRegexpPlainBodyStillLowers pins the fix is narrow: a regexp literal with an
// ordinary body still lowers to value.NewRegExpLiteral, so the common case is untouched.
func TestRegexpPlainBodyStillLowers(t *testing.T) {
	const src = "const r = /abc/g;\nconsole.log(typeof r);"
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.NewRegExpLiteral(") {
		t.Fatalf("plain regexp literal did not lower to NewRegExpLiteral:\n%s", out)
	}
}
