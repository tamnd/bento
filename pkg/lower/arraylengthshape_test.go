package lower

import (
	"strings"
	"testing"
)

// TestArrayIntoLengthShapeParam pins that an array argument passed where a
// { length: number } parameter is declared adapts into a fresh struct reading the
// array's length, since an array structurally satisfies that shape but its Go
// representation (*value.Array) is not the shape's (*ObjLength).
func TestArrayIntoLengthShapeParam(t *testing.T) {
	src := `function isEmpty(list: {length:number;}) { return list.length === 0; }
isEmpty([]);
`
	out := renderProgramTolerant(t, src)
	if !strings.Contains(out, "func IsEmpty(list *ObjLength)") {
		t.Fatalf("length shape parameter did not lower to the interned struct:\n%s", out)
	}
	if !strings.Contains(out, "Length: value.NewArray[value.Value]().Len()") {
		t.Fatalf("array argument did not adapt into the length shape struct:\n%s", out)
	}
}
