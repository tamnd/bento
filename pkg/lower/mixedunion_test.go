package lower

import (
	"strings"
	"testing"
)

// A union of one object member beside primitive members lowers to the tagged sum the
// all-primitive union already uses, with the object member holding its own Go type in a
// value field. The tests below pin the emitted shape; the conformance fixture covers
// what the program answers.

// TestMixedUnionInternsTheObjectArm pins the whole point of the slice: string | string[]
// gets a struct with both a string field and the array header, not a handback.
func TestMixedUnionInternsTheObjectArm(t *testing.T) {
	src := "const pairs: (string | string[])[] = [\"a\", [\"b\", \"c\"]];\nconsole.log(pairs.length);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "type StrArrOrStr struct") {
		t.Fatalf("the mixed union did not intern a tagged sum:\n%s", source)
	}
	fields := strings.Join(strings.Fields(source[strings.Index(source, "type StrArrOrStr struct"):]), " ")
	if !strings.Contains(fields, "strArr *value.Array[value.BStr]") {
		t.Errorf("the object arm does not hold the array header:\n%s", source)
	}
	if !strings.Contains(fields, "str value.BStr") {
		t.Errorf("the string arm did not survive beside it:\n%s", source)
	}
}

// TestMixedUnionConstructsFromEitherSide pins that a value of either member reaches its
// own arm's constructor, which is what an array literal of the union writes.
func TestMixedUnionConstructsFromEitherSide(t *testing.T) {
	src := "const pairs: (string | string[])[] = [\"a\", [\"b\", \"c\"]];\nconsole.log(pairs.length);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "StrArrOrStrOfStr(") {
		t.Errorf("the string member did not construct its arm:\n%s", source)
	}
	if !strings.Contains(source, "StrArrOrStrOfStrArr(") {
		t.Errorf("the array member did not construct its arm:\n%s", source)
	}
}

// TestMixedUnionTypeOfNamesTheObjectArmObject pins that the object arm reports the
// typeof JavaScript reports, so a typeof over the union answers "object" for the array
// and "string" for the string.
func TestMixedUnionTypeOfNamesTheObjectArmObject(t *testing.T) {
	src := "function pick(u: string | string[]): string { return typeof u; }\n" +
		"console.log(pick(\"a\"), pick([\"b\"]));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, `func (u StrArrOrStr) TypeOf()`) {
		t.Fatalf("no TypeOf method was emitted:\n%s", source)
	}
	obj := strings.Index(source, `FromGoString("object")`)
	str := strings.Index(source, `FromGoString("string")`)
	if obj < 0 || str < 0 {
		t.Errorf("TypeOf does not name both arms:\n%s", source)
	}
}

// TestMixedUnionNarrowsOnTypeof pins that a typeof guard folds to a tag compare rather
// than building the string and matching it, the narrowing the all-primitive union
// already gets.
func TestMixedUnionNarrowsOnTypeof(t *testing.T) {
	src := "function len(u: string | string[]): number {\n" +
		"  if (typeof u === \"string\") { return u.length; }\n  return u.length;\n}\n" +
		"console.log(len(\"abc\"), len([\"b\"]));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "u.tag == StrArrOrStrStr") {
		t.Errorf("the typeof guard did not fold to a tag compare:\n%s", source)
	}
	if !strings.Contains(source, "u.str") || !strings.Contains(source, "u.strArr") {
		t.Errorf("the narrowed branches did not select their arm fields:\n%s", source)
	}
}

// TestMixedUnionBoxesThroughItsArmBox pins that a mixed union crossing into a dynamic
// slot boxes each arm the way a lone value of that type would, the array through the
// header's own ToValue rather than a struct reflection that would drop it.
func TestMixedUnionBoxesThroughItsArmBox(t *testing.T) {
	src := "const pairs: (string | string[])[] = [\"a\", [\"b\"]];\nconst boxed: any = pairs[0];\nconsole.log(boxed);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "func (u StrArrOrStr) ToValue()") {
		t.Fatalf("no ToValue method was emitted:\n%s", source)
	}
	if !strings.Contains(source, "(*value.Array[value.BStr]).ToValue(u.strArr)") {
		t.Errorf("the object arm did not box through the array header:\n%s", source)
	}
}

// TestMixedUnionObjectArmIsAlwaysTruthy pins that the object arm needs no test in
// boolean position, since every object is truthy however empty it is.
func TestMixedUnionObjectArmIsAlwaysTruthy(t *testing.T) {
	src := "function truthy(u: string | string[]): string { return u ? \"yes\" : \"no\"; }\n" +
		"const empty: string[] = [];\nconsole.log(truthy(\"\"), truthy(empty));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "func (u StrArrOrStr) ToBoolean()") {
		t.Fatalf("no ToBoolean method was emitted:\n%s", source)
	}
	body := source[strings.Index(source, "func (u StrArrOrStr) ToBoolean()"):]
	body = body[:strings.Index(body, "\n}")]
	if !strings.Contains(body, "case StrArrOrStrStrArr:\n\t\treturn true") {
		t.Errorf("the object arm is not unconditionally truthy:\n%s", body)
	}
}

// TestMixedUnionStringifiesThroughToPrimitive pins that coercing the union to a string
// runs the language's own ToString on the object arm, so an array joins with commas
// rather than printing a Go header.
func TestMixedUnionStringifiesThroughToPrimitive(t *testing.T) {
	src := "function show(u: string | string[]): string { return `${u}`; }\n" +
		"console.log(show(\"a\"), show([\"b\", \"c\"]));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "func (u StrArrOrStr) ToString()") {
		t.Fatalf("no ToString method was emitted:\n%s", source)
	}
	if !strings.Contains(source, "value.ToString((*value.Array[value.BStr]).ToValue(u.strArr))") {
		t.Errorf("the object arm did not stringify through the runtime:\n%s", source)
	}
}

// TestMixedUnionWithTwoObjectMembersStillHandsBack pins the bar the slice sets: two
// object members have nothing to tell them apart without a discriminant, so the shape
// keeps its own refusal rather than take a tag it cannot select.
func TestMixedUnionWithTwoObjectMembersStillHandsBack(t *testing.T) {
	src := "function pick(u: string | string[] | number[]): number { return 1; }\nconsole.log(pick(\"a\"));\n"
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "union") {
		t.Errorf("a two-object mixed union did not hand back on the union: %s", reason)
	}
}

// TestMixedUnionNamesAClassArmAfterItsClass pins the naming rule: an arm whose Go type
// has a name of its own lends it, so the union reads the way the nullable-object form
// does rather than as a generic Obj.
func TestMixedUnionNamesAClassArmAfterItsClass(t *testing.T) {
	src := "class Point { x: number; constructor(x: number) { this.x = x; } }\n" +
		"function pick(u: string | Point): string { return typeof u; }\n" +
		"console.log(pick(\"a\"), pick(new Point(1)));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "type PointOrStr struct") {
		t.Errorf("the class arm did not lend its name to the union:\n%s", source)
	}
}
