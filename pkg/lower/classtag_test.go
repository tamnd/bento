package lower

import (
	"strings"
	"testing"
)

// TestObjectProtoToStringEmits pins the class-tag idiom: Object.prototype
// .toString.call(x) lowers to value.ClassTag on the boxed argument, not to a
// method on a string receiver and not to a hand-back. The argument is dynamic
// so it boxes with no wrapper; ClassTag reads its kind at runtime.
func TestObjectProtoToStringEmits(t *testing.T) {
	const src = `function tag(x: any): string {
  return Object.prototype.toString.call(x);
}
console.log(tag("s"));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.ClassTag(x)") {
		t.Errorf("Object.prototype.toString.call did not print value.ClassTag:\n%s", source)
	}
}

// TestObjectProtoToStringBoxesStatic pins the boxing of a static argument: a
// number reaching the class-tag idiom lifts through value.Number before the
// helper reads its kind, so ClassTag always takes a value.Value.
func TestObjectProtoToStringBoxesStatic(t *testing.T) {
	const src = `console.log(Object.prototype.toString.call(42));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.ClassTag(value.Number(42") {
		t.Errorf("class-tag on a static number did not box the argument:\n%s", source)
	}
}

// TestObjectProtoToStringApplyHandsBack pins the narrow scope: only .call(x) is
// the idiom, so borrowing Object.prototype.toString with .apply hands back
// rather than mislowering to the tag helper.
func TestObjectProtoToStringApplyHandsBack(t *testing.T) {
	const src = `function tag(x: any): string {
  return Object.prototype.toString.apply(x, []);
}
console.log(tag("s"));
`
	renderProgramHandBack(t, src)
}

// TestObjectProtoToStringRuns builds and runs the class-tag idiom against the
// Node oracle over the primitive kinds a static argument boxes into today; the
// full kind matrix, including the object and array tags, is pinned directly in
// the value package where a Value of any kind is built without going through
// static-to-dynamic boxing, which is its own later slice.
func TestObjectProtoToStringRuns(t *testing.T) {
	skipIfShort(t)
	const src = `function tag(x: any): string {
  return Object.prototype.toString.call(x);
}
console.log(tag(3));
console.log(tag("s"));
console.log(tag(true));
`
	got := runProgramGo(t, src)
	want := "[object Number]\n[object String]\n[object Boolean]\n"
	if got != want {
		t.Fatalf("class-tag program printed %q, want %q", got, want)
	}
}
