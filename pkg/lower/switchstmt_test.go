package lower

import (
	"errors"
	"strings"
	"testing"
)

// TestSwitchEmits pins the shape of the lowering: a break-terminated switch on a
// number becomes a Go switch on the same value, the trailing break is dropped since
// Go breaks for it, and shared empty cases merge into one Go case with several
// labels.
func TestSwitchEmits(t *testing.T) {
	const src = `function f(x: number): string {
  let r = "";
  switch (x) {
    case 1:
      r = "one";
      break;
    case 2:
    case 3:
      r = "few";
      break;
    default:
      r = "many";
  }
  return r;
}
console.log(f(1));
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"switch x {",
		"case 1:",
		"case 2, 3:",
		"default:",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("switch emit did not print %q:\n%s", want, source)
		}
	}
	// The JavaScript break is the Go case terminator and must not survive as a Go
	// break, which would be redundant.
	if strings.Contains(source, "break") {
		t.Errorf("switch emit leaked a Go break:\n%s", source)
	}
}

// TestSwitchOnEnum pins that a switch over a numeric enum lowers to a Go switch on
// the float64 the enum value is, with each case label the member's constant, the
// pairing the enum slice was built to compose with.
func TestSwitchOnEnum(t *testing.T) {
	const src = `enum Color { Red, Green, Blue }
function colorName(c: Color): string {
  switch (c) {
    case Color.Red:
      return "red";
    case Color.Green:
      return "green";
    default:
      return "blue";
  }
}
console.log(colorName(Color.Green));
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"switch c {",
		"case ColorRed:",
		"case ColorGreen:",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("enum switch did not print %q:\n%s", want, source)
		}
	}
}

// TestSwitchHandsBack pins the boundaries: a genuine fall-through (a non-empty case
// body that runs off its end into the next), a string discriminant, and a
// non-number case label each hand back with a named reason.
func TestSwitchHandsBack(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"fallThrough",
			"function f(x: number): number {\n  let r = 0;\n  switch (x) {\n    case 1:\n      r = 1;\n    case 2:\n      r = 2;\n      break;\n  }\n  return r;\n}\nconsole.log(f(1));\n",
			"falls through into the next",
		},
		{
			"stringDiscriminant",
			"function f(s: string): number {\n  switch (s) {\n    case \"a\":\n      return 1;\n    default:\n      return 0;\n  }\n}\nconsole.log(f(\"a\"));\n",
			"non-number discriminant",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prog := compile(t, tc.src)
			r := NewRenderer(prog)
			r.SetGoSignatures(testGoSignatures())
			_, err := r.RenderProgram(entryFile(t, prog))
			var nyl *NotYetLowerable
			if !errors.As(err, &nyl) {
				t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
			}
			if !strings.Contains(nyl.Reason, tc.want) {
				t.Errorf("hand-back reason = %q, want it to contain %q", nyl.Reason, tc.want)
			}
		})
	}
}

// TestSwitchRuns builds and runs a switch end to end and matches the Node oracle: a
// matched case, a shared empty-case body, an early return from a case, the default
// arm, and an enum switch all take the branch the source means.
func TestSwitchRuns(t *testing.T) {
	skipIfShort(t)
	const src = `function classify(x: number): string {
  switch (x) {
    case 0:
      return "zero";
    case 1:
    case 2:
      return "small";
    case 3: {
      let r = "three";
      return r;
    }
    default:
      return "big";
  }
}
enum Dir { North, East, South, West }
function turn(d: Dir): string {
  let out = "";
  switch (d) {
    case Dir.North:
      out = "up";
      break;
    case Dir.South:
      out = "down";
      break;
    default:
      out = "side";
  }
  return out;
}
console.log(classify(0));
console.log(classify(1));
console.log(classify(2));
console.log(classify(3));
console.log(classify(9));
console.log(turn(Dir.North));
console.log(turn(Dir.South));
console.log(turn(Dir.East));
`
	got := runProgramGo(t, src)
	want := "zero\n" +
		"small\n" +
		"small\n" +
		"three\n" +
		"big\n" +
		"up\n" +
		"down\n" +
		"side\n"
	if got != want {
		t.Fatalf("switch program printed %q, want %q", got, want)
	}
}
