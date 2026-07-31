package lower

import (
	"strings"
	"testing"
)

// TestNestedCollectionBoxEmits pins how a container holding a container boxes. The inner
// element's boxer is the method expression for its own no-argument box, which every
// container has, so an array of Maps and a Map of Maps compose with no closure emitted
// and the same shape reaches an optional read of one.
func TestNestedCollectionBoxEmits(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"arrayOfArrays",
			"const xs: number[][] = [[1]];\nconsole.log(xs);\n",
			"value.ArrayValueOf(xs, (*value.Array[float64]).ToValue)",
		},
		{
			"arrayOfMaps",
			"const a: Map<string, number>[] = [];\nconsole.log(a);\n",
			"value.ArrayValueOf(a, (*value.Map[value.BStr, float64]).ToValue)",
		},
		{
			"mapOfMaps",
			"const m = new Map<string, Map<string, number>>();\nconsole.log(m);\n",
			"value.ConsoleValue(m.ToValue())",
		},
		{
			"optionalOfAMap",
			"const m = new Map<string, Map<string, number>>();\nconsole.log(m.get('k'));\n",
			"value.OptToValue(m.Get(value.FromGoString(\"k\")), (*value.Map[value.BStr, float64]).ToValue)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := renderProgram(t, tc.src)
			if !strings.Contains(source, tc.want) {
				t.Errorf("the nested box did not print %q:\n%s", tc.want, source)
			}
		})
	}
}

// TestNestedCollectionBoxHandsBack pins the boundary. An array's box is a copy with no
// pointer back to the typed array, so a value handed to has or delete cannot be
// recognized as the member that is there; a Map keyed by arrays and a Set of arrays
// would answer false about a collection that does hold one, so both hand back at the
// boxing site rather than answering wrong. An array as a Map's value is allowed, since
// there the copy costs a lost write through the box and not a wrong answer.
func TestNestedCollectionBoxHandsBack(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"mapKeyedByArrays",
			"const m = new Map<number[], number>();\nconsole.log(m);\n",
			"boxing a Map whose keys or values are not",
		},
		{
			"setOfArrays",
			"const s = new Set<number[]>();\nconsole.log(s);\n",
			"boxing a Set whose members are not",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := renderProgramHandBack(t, tc.src); !strings.Contains(got, tc.want) {
				t.Errorf("hand-back reason = %q, want it to contain %q", got, tc.want)
			}
		})
	}
}
