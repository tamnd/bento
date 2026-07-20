package lower

import (
	"strings"
	"testing"
)

// TestObjectDefaultOverOptionalUnion pins that a destructuring default over an optional
// property whose type is a multi-member union fills through the union's discriminant
// rather than the value.Opt protocol the flat tagged sum does not carry. The property
// value?: string | number lowers its field to the tagged sum NumOrStrOrUndef, whose
// undefined arm is the absent member, so the fill switches that tag: the undefined arm
// takes the default and each value arm rebuilds the binding's undefined-stripped union.
func TestObjectDefaultOverOptionalUnion(t *testing.T) {
	const src = `type Params = { value?: string | number }
function pick(parameters: Params) {
    const { value = '123' } = parameters
    return ` + "`${value}`" + `
}
`
	out := renderProgram(t, src)
	if !strings.Contains(out, "var value_ NumOrStr") {
		t.Fatalf("binding was not declared with the undefined-stripped union type:\n%s", out)
	}
	if !strings.Contains(out, ".tag {") {
		t.Fatalf("fill did not switch the source union discriminant:\n%s", out)
	}
	if !strings.Contains(out, "case NumOrStrOrUndefUndef:") {
		t.Fatalf("fill did not cover the undefined arm:\n%s", out)
	}
	if !strings.Contains(out, "value_ = NumOrStrOfNum(_bt0.num)") {
		t.Fatalf("value arm did not rebuild the target union from its own field:\n%s", out)
	}
}

// TestObjectDefaultOverOptionalUnionRuns builds and runs the shape across all three
// paths: a present string arm reads the string, a present number arm reads the number,
// and an absent property takes the default, proving the discriminant fill routes each
// arm to the right value.
func TestObjectDefaultOverOptionalUnionRuns(t *testing.T) {
	skipIfShort(t)
	const src = `type Params = { value?: string | number }
function pick(parameters: Params): string {
    const { value = '123' } = parameters
    return ` + "`${value}`" + `
}
console.log(pick({ value: 'x' }));
console.log(pick({}));
console.log(pick({ value: 5 }));
`
	if got, want := runProgramGo(t, src), "x\n123\n5\n"; got != want {
		t.Fatalf("optional-union default run mismatch:\n got %q\nwant %q", got, want)
	}
}
