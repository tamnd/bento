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

// TestSwitchHandsBack pins the boundaries: a dynamic discriminant, whose case
// compare needs the runtime's strict equality, and a string case label that is
// not a literal each hand back with a named reason.
func TestSwitchHandsBack(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"dynamicDiscriminant",
			"function f(v: any): number {\n  switch (v) {\n    case 1:\n      return 1;\n    default:\n      return 0;\n  }\n}\nconsole.log(f(1));\n",
			"non-number discriminant",
		},
		{
			"stringLabelNotLiteral",
			"function f(s: string, a: string): number {\n  switch (s) {\n    case a:\n      return 1;\n    default:\n      return 0;\n  }\n}\nconsole.log(f(\"a\", \"a\"));\n",
			"not a string literal",
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

// TestSwitchOnStringEmits pins the string form: the discriminant compares by code
// unit, so the Go switch runs over its UTF-8 view through ToGoString and each
// label is a Go string literal.
func TestSwitchOnStringEmits(t *testing.T) {
	const src = `function f(s: string): number {
  switch (s) {
    case "a":
      return 1;
    case "b":
      return 2;
    default:
      return 0;
  }
}
console.log(f("b"));
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"switch s.ToGoString() {",
		"case \"a\":",
		"case \"b\":",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("string switch did not print %q:\n%s", want, source)
		}
	}
}

// TestSwitchOnTypeofEmits pins the assert._toString discriminant: a ternary whose
// branches are a string literal and a typeof counts as a string, so the whole
// selector lowers into the ToGoString switch.
func TestSwitchOnTypeofEmits(t *testing.T) {
	const src = `function tag(value: any): string {
  switch (value === null ? 'null' : typeof value) {
    case 'string':
      return 'S';
    case 'number':
      return 'N';
    default:
      return 'O';
  }
}
console.log(tag(1));
`
	source := renderProgram(t, src)
	for _, want := range []string{
		".ToGoString() {",
		"value_.TypeOf()",
		"case \"string\":",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("typeof switch did not print %q:\n%s", want, source)
		}
	}
}

// TestSwitchFallthroughEmits pins genuine fall-through: a case body that runs off
// its end takes an explicit Go fallthrough, and an empty case directly before
// default becomes its own case whose body is one fallthrough, since a Go default
// cannot carry its label.
func TestSwitchFallthroughEmits(t *testing.T) {
	const src = `function f(x: number): number {
  let r = 0;
  switch (x) {
    case 1:
      r = 1;
    case 2:
      r = r + 2;
      break;
  }
  return r;
}
function g(s: string): string {
  switch (s) {
    case "a":
    default:
      return "d";
  }
}
console.log(f(1));
console.log(g("a"));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "fallthrough") {
		t.Errorf("a case running off its end did not emit fallthrough:\n%s", source)
	}
	if !strings.Contains(source, "case \"a\":\n\t\tfallthrough\n\tdefault:") {
		t.Errorf("an empty case before default did not emit the fallthrough share:\n%s", source)
	}
}

// TestSwitchStringRuns builds and runs the string and fall-through forms end to
// end against the Node answers: the matched label returns its arm, a number case
// that falls through lands in default, and the ternary-typeof selector routes a
// dynamic value by its runtime tag.
func TestSwitchStringRuns(t *testing.T) {
	skipIfShort(t)
	const src = `function pick(s: string): string {
  switch (s) {
    case "a":
      return "ay";
    case "b":
      return "bee";
    default:
      return "other";
  }
}
function tag(value: any): string {
  switch (typeof value) {
    case 'string':
      return 'S:' + value;
    case 'number':
      if (value === 0) { return 'zero'; }
      // falls through
    default:
      return 'other';
  }
}
function accumulate(x: number): number {
  let r = 0;
  switch (x) {
    case 1:
      r = 1;
    case 2:
      r = r + 2;
      break;
    default:
      r = 9;
  }
  return r;
}
console.log(pick("a"));
console.log(pick("b"));
console.log(pick("z"));
console.log(tag("x"));
console.log(tag(0));
console.log(tag(3));
console.log(tag(true));
console.log(accumulate(1));
console.log(accumulate(2));
console.log(accumulate(5));
`
	got := runProgramGo(t, src)
	want := "ay\nbee\nother\nS:x\nzero\nother\nother\n3\n2\n9\n"
	if got != want {
		t.Fatalf("string switch program printed %q, want %q", got, want)
	}
}
