package lower

import (
	"errors"
	"strings"
	"testing"
)

// TestEnumEmits pins the shape of a plain numeric enum: a float64-backed constant
// per member, explicit initializers kept and the gaps auto-incremented, and a
// member read resolving to the member's Go constant.
func TestEnumEmits(t *testing.T) {
	const src = `enum Color { Red, Green = 5, Blue }
console.log(Color.Red, Color.Green, Color.Blue);
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"const (",
		"ColorRed",
		"ColorGreen float64 = 5",
		"float64 = 0",
		"float64 = 6",
		"value.NumberToString(ColorRed)",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("enum emit did not print %q:\n%s", want, source)
		}
	}
}

// TestEnumNegativeMembers pins the negative-sentinel form: a member initialized to
// a negated literal folds to the negative value, and the auto-increment resumes one
// past it, so A=-2 gives B=-1 and C=0.
func TestEnumNegativeMembers(t *testing.T) {
	const src = `enum Signed { A = -2, B, C }
console.log(Signed.A, Signed.B, Signed.C);
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"SignedA float64 = -2",
		"SignedB float64 = -1",
		"SignedC float64 = 0",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("negative enum emit did not print %q:\n%s", want, source)
		}
	}
}

// TestEnumTypeIsFloat64 pins that an enum-typed annotation lowers to float64: a
// parameter and a return declared with the enum name become float64, since a
// registered numeric enum rides the number path the checker already gives its
// members.
func TestEnumTypeIsFloat64(t *testing.T) {
	const src = `enum E { X, Y }
function id(e: E): E { return e; }
console.log(id(E.X));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "func Id(e float64) float64") {
		t.Errorf("enum-typed signature did not lower to float64:\n%s", source)
	}
}

// TestConstEnumInlines pins that a const enum emits no package-level constants and
// inlines each member's value at the use site, the erasure TypeScript itself
// performs, so an expression over its members lowers to the plain numeric literals.
func TestConstEnumInlines(t *testing.T) {
	const src = `const enum Dir { Up = 1, Down = 2 }
console.log(Dir.Up + Dir.Down);
`
	source := renderProgram(t, src)
	if strings.Contains(source, "DirUp") || strings.Contains(source, "DirDown") {
		t.Errorf("const enum leaked a member constant, want it inlined:\n%s", source)
	}
	if !strings.Contains(source, "3.0") {
		t.Errorf("const enum did not inline its member values:\n%s", source)
	}
}

// TestEnumHandsBack pins the boundaries of the lowered subset: a heterogeneous enum
// that mixes number and string members has no single Go type, and a member whose
// initializer references another member is a computed value, so each hands the unit
// back with a named reason rather than emit a shape this file does not build.
func TestEnumHandsBack(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"heterogeneousEnum",
			"enum M { A = 0, B = \"b\" }\nconsole.log(1);\n",
			"heterogeneous enum mixing number and string",
		},
		{
			"memberReferenceInitializer",
			"enum C { A = 1, B = A }\nconsole.log(1);\n",
			"not a numeric literal",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prog := compile(t, tc.src)
			r := NewRenderer(prog)
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

// TestStringEnumEmits pins the shape of a plain string enum: a value.BStr var per
// member, each initialized with the member's string content, since a bento string
// has no Go constant form.
func TestStringEnumEmits(t *testing.T) {
	const src = `enum Fruit { Apple = "APPLE", Pear = "PEAR" }
console.log(Fruit.Apple);
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"var (",
		`FruitApple = value.FromGoString("APPLE")`,
		`FruitPear  = value.FromGoString("PEAR")`,
	} {
		if !strings.Contains(source, want) {
			t.Errorf("string enum emit did not print %q:\n%s", want, source)
		}
	}
}

// TestStringEnumType pins that a string-enum-typed annotation lowers to value.BStr:
// a parameter and a return declared with the enum name become value.BStr, since a
// registered string enum rides the string path the checker gives its members.
func TestStringEnumType(t *testing.T) {
	const src = `enum Fruit { Apple = "APPLE", Pear = "PEAR" }
function fruitName(f: Fruit): Fruit { return f; }
console.log(fruitName(Fruit.Apple));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "func FruitName(f value.BStr) value.BStr") {
		t.Errorf("string-enum-typed signature did not lower to value.BStr:\n%s", source)
	}
}

// TestConstStringEnumInlines pins that a const string enum emits no package-level
// vars and inlines each member's string at the use site, the erasure TypeScript
// performs, so a member read lowers to a plain value.FromGoString.
func TestConstStringEnumInlines(t *testing.T) {
	const src = `const enum Suit { Hearts = "H", Spades = "S" }
console.log(Suit.Hearts);
`
	source := renderProgram(t, src)
	if strings.Contains(source, "SuitHearts") || strings.Contains(source, "SuitSpades") {
		t.Errorf("const string enum leaked a member var, want it inlined:\n%s", source)
	}
	if !strings.Contains(source, `value.FromGoString("H")`) {
		t.Errorf("const string enum did not inline its member value:\n%s", source)
	}
}

// TestStringEnumRuns builds and runs a string enum end to end and matches the Node
// oracle: a member read, an enum-typed round trip through a function, a member
// compared against its string value, and a const string enum's inlined member.
func TestStringEnumRuns(t *testing.T) {
	skipIfShort(t)
	const src = `enum Fruit { Apple = "APPLE", Pear = "PEAR" }
const enum Suit { Hearts = "H", Spades = "S" }
function fruitName(f: Fruit): Fruit {
  return f;
}
console.log(Fruit.Apple);
console.log(fruitName(Fruit.Pear));
console.log(Fruit.Apple === "APPLE");
console.log(Suit.Hearts);
`
	got := runProgramGo(t, src)
	want := "APPLE\n" +
		"PEAR\n" +
		"true\n" +
		"H\n"
	if got != want {
		t.Fatalf("string enum program printed %q, want %q", got, want)
	}
}

// TestEnumRuns builds and runs enums end to end and matches the Node oracle: a
// plain enum's explicit and auto-incremented members, a negative sentinel, an
// enum-typed round trip through a function, a const enum's inlined arithmetic, and
// a member compared against its numeric value.
func TestEnumRuns(t *testing.T) {
	skipIfShort(t)
	const src = `enum Color { Red, Green = 5, Blue }
enum Signed { A = -2, B, C }
const enum Dir { Up = 1, Down = 2 }
function label(c: Color): Color {
  return c;
}
console.log(Color.Red, Color.Green, Color.Blue);
console.log(Signed.A, Signed.B, Signed.C);
console.log(label(Color.Blue));
console.log(Dir.Up, Dir.Down);
console.log(Color.Green === 5);
`
	got := runProgramGo(t, src)
	want := "0 5 6\n" +
		"-2 -1 0\n" +
		"6\n" +
		"1 2\n" +
		"true\n"
	if got != want {
		t.Fatalf("enum program printed %q, want %q", got, want)
	}
}
