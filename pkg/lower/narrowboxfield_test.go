package lower

import (
	"strings"
	"testing"
)

// TestOptionalEmptyObjectFieldBoxesLiteral pins that an object literal filling an
// optional narrowable-box field ({}) boxes the value into a live value.Object rather
// than building the field's empty shape struct or wrapping it in a redundant Opt. The
// field collapses its optional into a bare value.Value, so the literal must reach it as
// a boxed value the box already carries undefined for.
func TestOptionalEmptyObjectFieldBoxesLiteral(t *testing.T) {
	src := `const fn1 = (options: { headers?: {} }) => { };
fn1({ headers: { foo: 1 } });
`
	out := renderProgramTolerant(t, src)
	if !strings.Contains(out, `Headers: value.NewObject().Set(value.FromGoString("foo"), value.Number(1))`) {
		t.Fatalf("optional {} field did not box the literal into a live object:\n%s", out)
	}
	if strings.Contains(out, "value.Some[") {
		t.Fatalf("a narrowable-box field must not wrap its value in an Opt:\n%s", out)
	}
}

// TestDestructuredParamEmptyObjectDefaultFillsBox pins that a destructured parameter
// with an empty-object default ({ headers = {} }) fills from a bare value.Value box: the
// present branch takes the read value directly, not the no-argument Get an Opt peel
// would emit, since the optional narrowable-box field is a value.Value, not a value.Opt.
func TestDestructuredParamEmptyObjectDefaultFillsBox(t *testing.T) {
	src := `const fn2 = ({ headers = {} }) => { };
fn2({ headers: { foo: 1 } });
`
	out := renderProgramTolerant(t, src)
	if !strings.Contains(out, "headers = value.NewObject()") {
		t.Fatalf("empty-object default did not fill the undefined branch with a live object:\n%s", out)
	}
	if strings.Contains(out, ".Get()") {
		t.Fatalf("a bare-box default fill must take the read directly, not peel it with Get:\n%s", out)
	}
}
