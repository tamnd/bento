package lower

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// renderProgramHandBack compiles a module and asserts the assembler hands it
// back as NotYetLowerable, the boundary a class construct outside the covered
// subset must keep, and returns the reason so a case can pin which rule fired.
func renderProgramHandBack(t *testing.T, src string) string {
	t.Helper()
	prog := compile(t, src)
	r := NewRenderer(prog)
	_, err := r.RenderProgram(entryFile(t, prog))
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
	}
	return nyl.Reason
}

// TestClassEmitsStruct pins the type half of the class lowering: the class
// becomes a named struct with one exported field per declared property, each
// tagged with the source property name, and a local holding an instance is
// typed as a pointer to it.
func TestClassEmitsStruct(t *testing.T) {
	const src = `class Point {
  x: number;
  y: number;
  constructor(x: number, y: number) {
    this.x = x;
    this.y = y;
  }
}
const p: Point = new Point(3, 4);
console.log(p.x);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "type Point struct {") {
		t.Errorf("class did not lower to a named struct:\n%s", source)
	}
	if !strings.Contains(source, "X float64 `json:\"x\"`") || !strings.Contains(source, "Y float64 `json:\"y\"`") {
		t.Errorf("class fields did not lower to tagged exported fields:\n%s", source)
	}
	if !strings.Contains(source, "p := NewPoint(3, 4)") {
		t.Errorf("instance local did not initialize through the constructor:\n%s", source)
	}
}

// TestClassCtorFoldsComposite pins the constructor fold: a constructor whose
// whole effect is storing pure values into fields prints as the one-line
// composite literal a person writes, not an allocate-assign-return sequence.
func TestClassCtorFoldsComposite(t *testing.T) {
	const src = `class Point {
  x: number;
  y: number;
  constructor(x: number, y: number) {
    this.x = x;
    this.y = y;
  }
}
console.log(new Point(3, 4).x);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "return &Point{X: x, Y: y}") {
		t.Errorf("pure constructor did not fold to a composite literal:\n%s", source)
	}
}

// TestClassDefaultCtorFoldsInits pins the declared-initializer path: a class
// with field initializers and no constructor gets a zero-argument NewX that
// folds the initializers into the composite literal.
func TestClassDefaultCtorFoldsInits(t *testing.T) {
	const src = `class Counter {
  count: number = 0;
  label: string = "hits";
}
const c: Counter = new Counter();
console.log(c.count);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "func NewCounter() *Counter {") {
		t.Errorf("class with no constructor did not get a zero-argument NewCounter:\n%s", source)
	}
	if !strings.Contains(source, "return &Counter{Count: 0, Label:") {
		t.Errorf("field initializers did not fold into the composite literal:\n%s", source)
	}
}

// TestClassCtorGeneralForm pins the unfolded constructor: a body statement that
// is not a pure field store keeps the allocate-assign-return sequence, with the
// lowered body running between the field initializers and the return.
func TestClassCtorGeneralForm(t *testing.T) {
	const src = `class Logger {
  count: number = 0;
  constructor() {
    console.log("born");
  }
}
const l: Logger = new Logger();
console.log(l.count);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "l := &Logger{}") {
		t.Errorf("impure constructor did not keep the general receiver form:\n%s", source)
	}
	if !strings.Contains(source, "l.Count = 0") {
		t.Errorf("field initializer did not assign in the general form:\n%s", source)
	}
	if !strings.Contains(source, "return l") {
		t.Errorf("general constructor did not return the receiver:\n%s", source)
	}
}

// TestClassMethodEmitsReceiver pins the method half: a method becomes a
// pointer-receiver Go method, this reads as the receiver, and a call on an
// instance dispatches to it directly.
func TestClassMethodEmitsReceiver(t *testing.T) {
	const src = `class Point {
  x: number;
  y: number;
  constructor(x: number, y: number) {
    this.x = x;
    this.y = y;
  }
  norm2(): number {
    return this.x * this.x + this.y * this.y;
  }
}
const p: Point = new Point(3, 4);
console.log(p.norm2());
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "func (p *Point) Norm2() float64 {") {
		t.Errorf("method did not lower to a pointer-receiver Go method:\n%s", source)
	}
	if !strings.Contains(source, "return p.X*p.X + p.Y*p.Y") {
		t.Errorf("this did not lower to the receiver in the method body:\n%s", source)
	}
	if !strings.Contains(source, "p.Norm2()") {
		t.Errorf("instance method call did not dispatch to the Go method:\n%s", source)
	}
}

// TestClassReceiverAvoidsCollision pins the receiver naming rule: when the
// short receiver name is already an identifier the class body speaks (a
// parameter here), the receiver falls back rather than shadow it.
func TestClassReceiverAvoidsCollision(t *testing.T) {
	const src = `class Circle {
  r: number;
  constructor(c: number) {
    this.r = c;
  }
  area(): number {
    return this.r * this.r * Math.PI;
  }
}
console.log(new Circle(2).area());
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "func (circle *Circle) Area() float64 {") {
		t.Errorf("receiver did not fall back past the colliding short name:\n%s", source)
	}
}

// TestClassFieldStores pins the store shapes on a field: a compound store
// prints as the compound operator, a step of one prints as the increment, and
// both work through this inside a method and through an instance local outside.
func TestClassFieldStores(t *testing.T) {
	const src = `class Counter {
  count: number = 0;
  bump(): void {
    this.count += 1;
  }
  add(n: number): void {
    this.count += n;
  }
}
const c: Counter = new Counter();
c.bump();
c.count += 3;
c.count++;
c.count = c.count + 1;
console.log(c.count);
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"c.Count++",    // this.count += 1 inside bump, and the two step-of-one stores outside
		"c.Count += n", // the compound store keeps the compound operator
		"c.Count += 3", // a compound store through the instance local
	} {
		if !strings.Contains(source, want) {
			t.Errorf("field store did not print %q:\n%s", want, source)
		}
	}
}

// TestClassMutatingMethodRuns pins that mutation through a method sticks: the
// receiver is a pointer, so a scale method writes the caller's instance.
func TestClassMutatingMethodRuns(t *testing.T) {
	const src = `class Point {
  x: number;
  constructor(x: number) {
    this.x = x;
  }
  scale(f: number): void {
    this.x *= f;
  }
}
const p: Point = new Point(3);
p.scale(2);
console.log(p.x);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "p.X *= f") {
		t.Errorf("compound this-store did not lower to the compound operator:\n%s", source)
	}
}

// TestClassStructRoutesBeforeShapes pins the routing order: a class receiver
// is dispatched as the class it is, before the fingerprint paths that would
// otherwise claim its member names.
func TestClassStructRoutesBeforeShapes(t *testing.T) {
	// A class whose only field is named length must lower as the class, not
	// take the array-or-string .length path, and a class with a size field must
	// not take the Map .size path: the class routes are checked first.
	const src = `class Box {
  length: number = 2;
  size: number = 3;
}
const b: Box = new Box();
console.log(b.length + b.size);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "b.Length") || !strings.Contains(source, "b.Size") {
		t.Errorf("class fields named length and size did not read as struct fields:\n%s", source)
	}
}

// TestClassHandsBack pins the boundary: each class construct outside the core
// slice hands the unit back rather than lowering with the wrong semantics.
func TestClassHandsBack(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"extends",
			"class A { x: number = 1; }\nclass B extends A { }\nconsole.log(new B().x);\n",
			"heritage",
		},
		{
			"staticField",
			"class A { static n: number = 1; }\nconsole.log(A.n);\n",
			"static",
		},
		{
			"staticMethod",
			"class A { static f(): number { return 1; } }\nconsole.log(A.f());\n",
			"static",
		},
		{
			"getter",
			"class A { x: number = 1; get twice(): number { return this.x * 2; } }\nconsole.log(new A().twice);\n",
			"accessor",
		},
		{
			"definiteAssignment",
			"class A { x!: number; }\nconst a: A = new A();\nconsole.log(a.x);\n",
			"definite-assignment",
		},
		{
			"parameterProperty",
			"class A { constructor(private x: number) { } }\nconst a: A = new A(1);\nconsole.log(a);\n",
			"parameter property",
		},
		{
			"ctorReturn",
			"class A { x: number = 1; constructor() { return; } }\nconsole.log(new A().x);\n",
			"return inside a constructor",
		},
		{
			"thisInInitializer",
			"class A { x: number = 1; y: number = this.x + 1; }\nconsole.log(new A().y);\n",
			"initializer that reads this",
		},
		{
			"methodAsValue",
			"class A { f(): number { return 1; } }\nconst g: () => number = new A().f;\nconsole.log(g());\n",
			"read as a value",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reason := renderProgramHandBack(t, tc.src)
			if !strings.Contains(reason, tc.want) {
				t.Errorf("hand-back reason %q does not name %q", reason, tc.want)
			}
		})
	}
}

// TestClassCompletionRuns builds and runs a program that exercises the whole
// slice end to end: construction through the folded and general constructors,
// method dispatch, this-stores, instance-local stores, and pointer-receiver
// mutation, printing what node prints.
func TestClassCompletionRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the class test builds and runs generated Go")
	}
	const src = `class Point {
  x: number;
  y: number;
  constructor(x: number, y: number) {
    this.x = x;
    this.y = y;
  }
  norm2(): number {
    return this.x * this.x + this.y * this.y;
  }
  scale(f: number): void {
    this.x *= f;
    this.y *= f;
  }
}
class Counter {
  count: number = 0;
  label: string = "hits";
  bump(): void {
    this.count += 1;
  }
  report(): string {
    return this.label + ": " + this.count;
  }
}
const p: Point = new Point(3, 4);
console.log(p.norm2());
p.scale(2);
console.log(p.x);
console.log(p.y);
console.log(p.norm2());
const c: Counter = new Counter();
c.bump();
c.bump();
c.count += 3;
c.count++;
console.log(c.report());
const q: Point = new Point(p.x, 0);
q.x++;
console.log(q.x);
`
	got := runProgramGo(t, src)
	want := "25\n" +
		"6\n" +
		"8\n" +
		"100\n" +
		"hits: 6\n" +
		"7\n"
	if got != want {
		t.Fatalf("class completion program printed %q, want %q", got, want)
	}
}
