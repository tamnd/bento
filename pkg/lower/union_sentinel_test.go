package lower

import (
	"strings"
	"testing"
)

// TestUnionSentinelArmEmitsTagOnly pins the type half of the sentinel-arm lowering:
// a multi-member primitive union plus undefined interns a tag for the sentinel with
// no struct field and a no-argument constructor, while the value arms keep their
// field and value-taking constructor.
func TestUnionSentinelArmEmitsTagOnly(t *testing.T) {
	const src = `function pick(b: boolean): number | string | undefined {
  if (b) {
    return 1;
  }
  return undefined;
}
pick(true);
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"NumOrStrOrUndefUndef",
		"type NumOrStrOrUndef struct {",
		"num float64",
		"str value.BStr",
		"func NumOrStrOrUndefOfUndef() NumOrStrOrUndef",
		"return NumOrStrOrUndef{tag: NumOrStrOrUndefUndef}",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("emitted Go missing %q\n%s", want, source)
		}
	}
	// The sentinel arm carries no payload, so the struct grows no field for it.
	if strings.Contains(source, "undef ") || strings.Contains(source, "undef\t") {
		t.Fatalf("sentinel arm should emit no struct field\n%s", source)
	}
}

// TestUnionSentinelReturnConstructs pins the construction side: a bare undefined
// returned as the union wraps in the no-argument sentinel constructor, never a bare
// value, so the tag stays consistent.
func TestUnionSentinelReturnConstructs(t *testing.T) {
	const src = `function pick(b: boolean): number | string | undefined {
  if (b) {
    return 1;
  }
  return undefined;
}
pick(true);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "return NumOrStrOrUndefOfNum(1)") {
		t.Fatalf("number arm not wrapped in its constructor\n%s", source)
	}
	if !strings.Contains(source, "return NumOrStrOrUndefOfUndef()") {
		t.Fatalf("undefined arm not wrapped in its no-arg constructor\n%s", source)
	}
}

// TestUnionSentinelCompareNarrows pins that x === undefined on a union local lowers
// to a tag compare against the sentinel arm rather than building undefined and
// matching it, and x === null likewise.
func TestUnionSentinelCompareNarrows(t *testing.T) {
	const undefSrc = `function f(x: number | string | undefined): boolean {
  return x === undefined;
}
f(1);
`
	if got := renderProgram(t, undefSrc); !strings.Contains(got, "x.tag == NumOrStrOrUndefUndef") {
		t.Fatalf("=== undefined did not lower to a tag compare\n%s", got)
	}
	const nullSrc = `function f(x: number | string | null): boolean {
  return x !== null;
}
f(1);
`
	if got := renderProgram(t, nullSrc); !strings.Contains(got, "x.tag != NumOrStrOrNullNull") {
		t.Fatalf("!== null did not lower to a tag compare\n%s", got)
	}
}

// TestUnionSentinelTypeof pins that null's typeof arm answers "object" and
// undefined's answers "undefined" through the union's TypeOf method, the values
// JavaScript reports for each sentinel.
func TestUnionSentinelTypeof(t *testing.T) {
	const src = `function kind(x: number | string | null): string {
  return typeof x;
}
kind(1);
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"func (u NumOrStrOrNull) TypeOf() value.BStr",
		`case NumOrStrOrNullNull:`,
		`return value.FromGoString("object")`,
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("TypeOf method missing %q\n%s", want, source)
		}
	}
}

// TestUnionOptionalStaysOpt pins that a two-member T | undefined keeps the leaner
// value.Opt path rather than routing to the tagged sum: the sentinel arm rides only
// alongside two or more value arms, so an optional is untouched by this slice.
func TestUnionOptionalStaysOpt(t *testing.T) {
	const src = `function f(x: number | undefined): void {
  if (x !== undefined) {
    console.log(x + 1);
  }
}
f(1);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.Opt[float64]") {
		t.Fatalf("number | undefined should stay value.Opt\n%s", source)
	}
	if strings.Contains(source, "Tag uint8") {
		t.Fatalf("number | undefined should not intern a tagged union\n%s", source)
	}
}

// TestUnionSentinelRun builds and runs a program that narrows a number | string |
// undefined by === undefined and typeof, reassigns a number | string | null through
// its value and null arms, and prints each observable, so the tag-only arms compile
// and behave as the sentinels they stand for.
func TestUnionSentinelRun(t *testing.T) {
	skipIfShort(t)
	src := `
function f(x: number | string | undefined): string {
  if (x === undefined) return "none";
  if (typeof x === "number") return "n:" + x;
  return "s:" + x;
}
console.log(f(1));
console.log(f("hi"));
console.log(f(undefined));
let y: number | string | null = 5;
y = "world";
console.log(y);
y = null;
console.log(y === null);
console.log(typeof y);
`
	got := runProgramGo(t, src)
	want := "n:1\ns:hi\nnone\nworld\ntrue\nobject\n"
	if got != want {
		t.Fatalf("union sentinel run mismatch:\n got %q\nwant %q", got, want)
	}
}
