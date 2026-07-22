package lower

import (
	"strings"
	"testing"
)

// TestArrayAssertedToTupleReturn pins that an array value asserted to a tuple type and
// returned from a function whose declared return is that tuple, the arguments-object
// iterator shape <[any, any, any]>result, rebuilds the array into the tuple struct: an
// IIFE binds the array once and reads its leading positions through AtI into the
// tuple's fields, coercing each read to the field's number type. Without the bridge the
// array's *value.Array flowed straight into the tuple-struct return slot and did not
// build.
func TestArrayAssertedToTupleReturn(t *testing.T) {
	src := `function doubleAndReturnAsArray(x: number, y: number, z: number): [number, number, number] {
    let result = [];
    for (let arg of arguments) {
        result.push(arg + arg);
    }
    return <[any, any, any]>result;
}
`
	out := renderProgramTolerant(t, src)
	if !strings.Contains(out, "Tuple_num_num_num{E0:") {
		t.Fatalf("array assertion did not rebuild the tuple struct:\n%s", out)
	}
	if !strings.Contains(out, "_a.AtI(0)") || !strings.Contains(out, "_a.AtI(2)") {
		t.Fatalf("tuple fields were not read from the array positions:\n%s", out)
	}
	if strings.Contains(out, "return result\n") {
		t.Fatalf("array flowed into the tuple return slot unbridged:\n%s", out)
	}
}

// TestArrayAssertedToTupleBuilds builds the shape to prove the rebuilt tuple compiles;
// the function is never called, so the requirement is that it builds.
func TestArrayAssertedToTupleBuilds(t *testing.T) {
	skipIfShort(t)
	src := `function doubleAndReturnAsArray(x: number, y: number, z: number): [number, number, number] {
    let result = [];
    for (let arg of arguments) {
        result.push(arg + arg);
    }
    return <[any, any, any]>result;
}
doubleAndReturnAsArray(1, 2, 3);
`
	out := renderProgramTolerant(t, src)
	if got := goRunSource(t, out); got != "" {
		t.Fatalf("array-asserted-to-tuple run mismatch:\n got %q\nwant %q", got, "")
	}
}
