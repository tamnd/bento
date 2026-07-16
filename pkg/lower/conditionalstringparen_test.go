package lower

import (
	"strings"
	"testing"
)

// TestConditionalStringParenCoerceLowers pins that a parenthesized string ternary
// coerces to a string the same way the bare one does: isString sees through the
// grouping parentheses to the conditional within, so console.log takes the string
// identity rather than handing back on the paren node's literal-union type.
func TestConditionalStringParenCoerceLowers(t *testing.T) {
	src := `function f(x: number): void {
  console.log((x > 0 ? "hi" : "yo"));
}`
	out := renderProgram(t, src)
	if !strings.Contains(out, `value.FromGoString("hi")`) || !strings.Contains(out, `value.FromGoString("yo")`) {
		t.Fatalf("parenthesized string ternary did not lower:\n%s", out)
	}
}

// TestConditionalStringParenConcatLowers proves a parenthesized string ternary is a
// concat operand: with isString true on the paren node, the + path stringifies it
// rather than boxing it into a dynamic Add, so it joins through value.Concat.
func TestConditionalStringParenConcatLowers(t *testing.T) {
	src := `function f(x: number): void {
  console.log((x > 0 ? "hi" : "yo") + "!");
}`
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.Concat(") {
		t.Fatalf("string ternary concat did not lower to value.Concat:\n%s", out)
	}
}

// TestConditionalStringParenLengthLowers proves .length off a string ternary
// lowers: a member access on a ternary receiver is always parenthesized in the
// source, so the paren see-through routes the receiver to value.BStr.Length.
func TestConditionalStringParenLengthLowers(t *testing.T) {
	src := `function f(x: number): void {
  console.log((x > 0 ? "hi" : "yo").length);
}`
	out := renderProgram(t, src)
	if !strings.Contains(out, ".Length()") {
		t.Fatalf("string ternary .length did not lower to a BStr Length:\n%s", out)
	}
}

// TestConditionalStringParenRuns builds and runs the parenthesized shapes so the
// coercion, the concatenation, and the length read are proven to render the way
// JavaScript does.
func TestConditionalStringParenRuns(t *testing.T) {
	skipIfShort(t)
	src := `
function run(x: number): void {
  console.log((x > 0 ? "hi" : "yo"));
  console.log((x > 0 ? "hi" : "yo") + "!");
  console.log((x > 0 ? "hi" : "yo").length);
}
run(1);
run(-1);
`
	got := runProgramGo(t, src)
	want := "hi\nhi!\n2\nyo\nyo!\n2\n"
	if got != want {
		t.Fatalf("parenthesized string ternary run mismatch:\n got %q\nwant %q", got, want)
	}
}
