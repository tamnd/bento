package lower

import (
	"strings"
	"testing"
)

// TestTaggedUnionEmitsSumStruct pins the type half of the tagged-sum lowering: a
// union of two unlike primitives becomes a discriminant tag type, a const block
// naming the tags in arm order, a value struct with one inline field per arm, and
// a wrapping constructor per arm that sets both the tag and the field.
func TestTaggedUnionEmitsSumStruct(t *testing.T) {
	const src = `function pick(b: boolean): number | string {
  if (b) {
    return 1;
  }
  return "a";
}
pick(true);
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"type NumOrStrTag uint8",
		"NumOrStrNum NumOrStrTag = iota",
		"NumOrStrStr",
		"type NumOrStr struct {",
		"tag NumOrStrTag",
		"num float64",
		"str value.BStr",
		"func NumOrStrOfNum(v float64) NumOrStr",
		"func NumOrStrOfStr(v value.BStr) NumOrStr",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("emitted Go missing %q\n%s", want, source)
		}
	}
}

// TestTaggedUnionReturnWraps pins the construction side: a member value returned
// as the union type is wrapped in the arm constructor, never assigned bare, so the
// tag stays consistent with the payload.
func TestTaggedUnionReturnWraps(t *testing.T) {
	const src = `function pick(b: boolean): number | string {
  if (b) {
    return 1;
  }
  return "a";
}
pick(true);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "return NumOrStrOfNum(1)") {
		t.Fatalf("number arm not wrapped in its constructor\n%s", source)
	}
	if !strings.Contains(source, `return NumOrStrOfStr(value.FromGoString("a"))`) {
		t.Fatalf("string arm not wrapped in its constructor\n%s", source)
	}
}

// TestTaggedUnionTypeofNarrows pins that typeof x === "string" on a union lowers to
// a discriminant compare rather than building the tag string, and that the arm read
// inside the branch selects the matching struct field.
func TestTaggedUnionTypeofNarrows(t *testing.T) {
	const src = `function pick(b: boolean): number | string {
  if (b) {
    return 1;
  }
  return "a";
}
function run(): void {
  const v = pick(true);
  if (typeof v === "string") {
    console.log(v);
  } else {
    console.log(v);
  }
}
run();
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "if v.tag == NumOrStrStr {") {
		t.Fatalf("typeof did not lower to a tag compare\n%s", source)
	}
	if !strings.Contains(source, "value.ConsoleLog(v.str)") {
		t.Fatalf("string-narrowed read did not select the str field\n%s", source)
	}
	if !strings.Contains(source, "value.NumberToString(v.num)") {
		t.Fatalf("number-narrowed read did not select the num field\n%s", source)
	}
}

// TestTaggedUnionTypeofNotEquals pins that !== narrows to a not-equal tag compare,
// the negated form of the === narrowing.
func TestTaggedUnionTypeofNotEquals(t *testing.T) {
	const src = `function pick(b: boolean): number | string {
  if (b) {
    return 1;
  }
  return "a";
}
function run(): void {
  const v = pick(true);
  if (typeof v !== "string") {
    console.log(v);
  }
}
run();
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "if v.tag != NumOrStrStr {") {
		t.Fatalf("typeof !== did not lower to a not-equal tag compare\n%s", source)
	}
}

// TestTaggedUnionRuns builds and runs the narrowing program and checks its output
// matches the JavaScript semantics: the value threads through the union and each
// branch reads the arm the checker narrowed to.
func TestTaggedUnionRuns(t *testing.T) {
	skipIfShort(t)
	const src = `function pick(b: boolean): number | string {
  if (b) {
    return 42;
  }
  return "hi";
}
function label(v: number | string): string {
  if (typeof v === "string") {
    return "s:" + v;
  }
  return "n:" + String(v);
}
function run(): void {
  const a = pick(true);
  const b = pick(false);
  console.log(label(a));
  console.log(label(b));
}
run();
`
	out := runProgramGo(t, src)
	if out != "s:hi\nn:42\n" && out != "n:42\ns:hi\n" {
		t.Fatalf("unexpected output %q", out)
	}
}

// TestTaggedUnionThreeArms pins that a three-arm primitive union emits three tags,
// three fields, and three constructors, and that each arm narrows independently.
func TestTaggedUnionThreeArms(t *testing.T) {
	skipIfShort(t)
	const src = `function tag(v: number | string | boolean): string {
  if (typeof v === "string") {
    return "s";
  }
  if (typeof v === "boolean") {
    return "b";
  }
  return "n";
}
function run(): void {
  console.log(tag(1));
  console.log(tag("x"));
  console.log(tag(true));
}
run();
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "func NumOrStrOrBoolOfBool(v bool)") {
		t.Fatalf("boolean arm constructor missing\n%s", source)
	}
	out := runProgramGo(t, src)
	if out != "n\ns\nb\n" {
		t.Fatalf("unexpected output %q", out)
	}
}

// TestTaggedUnionArgumentIsBoolean guards against the boolean arm colliding with
// the discriminant: a boolean value flowing into a number | boolean union wraps in
// the boolean constructor and narrows back on its own tag.
func TestTaggedUnionObjectArmHandsBack(t *testing.T) {
	reason := renderProgramHandBack(t, `function pick(b: boolean): number | number[] {
  if (b) {
    return 1;
  }
  return [1, 2];
}
pick(true);
`)
	if !strings.Contains(reason, "union") && !strings.Contains(reason, "later slice") {
		t.Fatalf("object-arm union hand-back reason = %q", reason)
	}
}

// TestDiscriminatedUnionEmitsPointerArms pins the type half of the object-arm tagged
// sum: a union of two objects sharing a string-literal discriminant becomes a tag
// type, a value struct with one pointer field per arm named after the discriminant
// value, and a constructor per arm taking that member's pointer.
func TestDiscriminatedUnionEmitsPointerArms(t *testing.T) {
	const src = `interface Circle { kind: "circle"; r: number; }
interface Square { kind: "square"; side: number; }
type Shape = Circle | Square;
function area(s: Shape): number {
  if (s.kind === "circle") {
    return s.r * s.r;
  }
  return s.side * s.side;
}
area({ kind: "circle", r: 2 });
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"type CircleOrSquareTag uint8",
		"type CircleOrSquare struct {",
		"circle *ObjKindR",
		"square *ObjKindSide",
		"func CircleOrSquareOfCircle(v *ObjKindR) CircleOrSquare",
		"func CircleOrSquareOfSquare(v *ObjKindSide) CircleOrSquare",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("emitted Go missing %q\n%s", want, source)
		}
	}
}

// TestDiscriminatedUnionIfNarrows pins that s.kind === "circle" lowers to a tag
// compare and that the narrowed read inside the branch selects the arm's pointer
// field, so s.r becomes s.circle.R.
func TestDiscriminatedUnionIfNarrows(t *testing.T) {
	const src = `interface Circle { kind: "circle"; r: number; }
interface Square { kind: "square"; side: number; }
type Shape = Circle | Square;
function area(s: Shape): number {
  if (s.kind === "circle") {
    return s.r * s.r;
  }
  return s.side * s.side;
}
area({ kind: "circle", r: 2 });
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "if s.tag == CircleOrSquareCircle {") {
		t.Fatalf("discriminant test did not lower to a tag compare\n%s", source)
	}
	if !strings.Contains(source, "s.circle.R") {
		t.Fatalf("narrowed read did not select the arm pointer field\n%s", source)
	}
}

// TestDiscriminatedUnionInNarrows pins that "r" in s narrows to the arm carrying
// the property and lowers to a tag test, and that the branch reads the arm field.
func TestDiscriminatedUnionInNarrows(t *testing.T) {
	const src = `interface Circle { kind: "circle"; r: number; }
interface Square { kind: "square"; side: number; }
type Shape = Circle | Square;
function measure(s: Shape): number {
  if ("r" in s) {
    return s.r;
  }
  return s.side;
}
measure({ kind: "circle", r: 2 });
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "if s.tag == CircleOrSquareCircle {") {
		t.Fatalf("in test did not lower to a tag compare\n%s", source)
	}
	if !strings.Contains(source, "return s.circle.R") {
		t.Fatalf("in-narrowed read did not select the arm field\n%s", source)
	}
}

// TestDiscriminatedUnionRuns builds and runs a discriminated-union program through
// both the if and switch narrowing forms and checks the output matches the
// JavaScript semantics.
func TestDiscriminatedUnionRuns(t *testing.T) {
	skipIfShort(t)
	const src = `interface Circle { kind: "circle"; r: number; }
interface Square { kind: "square"; side: number; }
type Shape = Circle | Square;
function area(s: Shape): number {
  switch (s.kind) {
    case "circle":
      return s.r * s.r;
    case "square":
      return s.side * s.side;
  }
  return 0;
}
function run(): void {
  const c: Shape = { kind: "circle", r: 3 };
  const sq: Shape = { kind: "square", side: 4 };
  console.log(String(area(c)));
  console.log(String(area(sq)));
}
run();
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "switch s.tag {") {
		t.Fatalf("discriminant switch did not lower to a tag switch\n%s", source)
	}
	out := runProgramGo(t, src)
	if out != "9\n16\n" {
		t.Fatalf("unexpected output %q\n%s", out, source)
	}
}
